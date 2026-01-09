package main

import (
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// MaxFailCount before a node is considered dead
	// With 5-min liveness checks, 6 failures = 30 min
	MaxFailCount = 6

	// MaxPeers is the maximum number of active peers to maintain
	// This replaces star-class-based bucket sizing with a simple limit
	MaxPeers = 50
)

// CachedSystem stores full system info with metadata
type CachedSystem struct {
	System          *System
	LearnedAt       time.Time // When we first learned about this system
	LearnedFrom     uuid.UUID
	Verified        bool      // Have we directly communicated with them?
	LastVerified    time.Time // When we last had direct contact (zero if never)
	LastGossipHeard time.Time // When we last heard about this system via gossip
	FailCount       int       // Consecutive ping failures
}

// RoutingTable manages known peers for the DHT
// Simplified from Kademlia k-buckets to a simple map since we want full visibility
type RoutingTable struct {
	localID     uuid.UUID
	localSystem *System
	mu          sync.RWMutex

	// All known systems - this IS the peer database
	systemCache map[uuid.UUID]*CachedSystem
	cacheMu     sync.RWMutex

	// Storage for persistence
	storage *Storage
}

// NewRoutingTable creates a new routing table for the local node
func NewRoutingTable(localSystem *System, storage *Storage) *RoutingTable {
	rt := &RoutingTable{
		localID:     localSystem.ID,
		localSystem: localSystem,
		systemCache: make(map[uuid.UUID]*CachedSystem),
		storage:     storage,
	}

	// Load cached systems from storage
	rt.loadFromStorage()

	return rt
}

// Update adds or updates a node - simplified from Kademlia to just cache it
func (rt *RoutingTable) Update(sys *System) {
	if sys == nil || sys.ID == rt.localID {
		return
	}
	// Just cache the system - no bucket logic needed
	rt.CacheSystem(sys, uuid.Nil, false)
}

// MarkFailed increments the fail count for a node
func (rt *RoutingTable) MarkFailed(nodeID uuid.UUID) {
	rt.cacheMu.Lock()
	defer rt.cacheMu.Unlock()

	if cached, ok := rt.systemCache[nodeID]; ok {
		cached.FailCount++
	}
}

// MarkVerified marks a node as successfully verified (ping response received)
func (rt *RoutingTable) MarkVerified(nodeID uuid.UUID) {
	now := time.Now()

	rt.cacheMu.Lock()
	if cached, ok := rt.systemCache[nodeID]; ok {
		cached.Verified = true
		cached.LastVerified = now
		cached.LastGossipHeard = now
		cached.FailCount = 0
	}
	rt.cacheMu.Unlock()

	// Update storage timestamp
	if rt.storage != nil {
		rt.storage.TouchPeerSystem(nodeID)
	}
}

// EvictDeadNodes removes nodes with too many failures
func (rt *RoutingTable) EvictDeadNodes() int {
	rt.cacheMu.Lock()
	defer rt.cacheMu.Unlock()

	evicted := 0
	for id, cached := range rt.systemCache {
		if cached.FailCount >= MaxFailCount {
			delete(rt.systemCache, id)
			evicted++
		}
	}
	return evicted
}

// GetAllPeers returns all verified peers (replaces GetClosest for FIND_NODE)
// Limited to maxCount to avoid overwhelming responses
func (rt *RoutingTable) GetAllPeers(maxCount int) []*System {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()

	result := make([]*System, 0, maxCount)
	verificationCutoff := time.Now().Add(-VerificationCutoff)

	for _, cached := range rt.systemCache {
		if len(result) >= maxCount {
			break
		}
		// Only return verified peers with recent verification
		if cached.Verified && !cached.LastVerified.IsZero() &&
			cached.LastVerified.After(verificationCutoff) &&
			cached.FailCount < MaxFailCount {
			result = append(result, cached.System)
		}
	}

	return result
}

// GetClosest is now just an alias for GetAllPeers (no XOR distance sorting)
// Kept for compatibility with existing code
func (rt *RoutingTable) GetClosest(targetID uuid.UUID, count int) []*System {
	// In a full-visibility network, we just return verified peers
	// XOR distance is not meaningful when everyone knows everyone
	return rt.GetAllPeers(count)
}

// GetAllRoutingTableNodes returns all active (verified, not dead) peers
func (rt *RoutingTable) GetAllRoutingTableNodes() []*System {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()

	verificationCutoff := time.Now().Add(-VerificationCutoff)
	result := make([]*System, 0)

	for _, cached := range rt.systemCache {
		// "Active peer" = verified recently and not failing
		if cached.Verified && !cached.LastVerified.IsZero() &&
			cached.LastVerified.After(verificationCutoff) &&
			cached.FailCount < MaxFailCount {
			result = append(result, cached.System)
		}
	}
	return result
}

// GetRoutingTableSize returns the count of active peers
func (rt *RoutingTable) GetRoutingTableSize() int {
	return len(rt.GetAllRoutingTableNodes())
}

// === System Cache Methods ===

// CacheSystem adds a system to the cache
// InfoVersion is used to prevent stale gossip from overwriting fresh info
// LastGossipHeard is updated conditionally to prevent dead nodes from persisting
func (rt *RoutingTable) CacheSystem(sys *System, learnedFrom uuid.UUID, verified bool) {
	if sys == nil || sys.ID == rt.localID {
		return
	}

	rt.cacheMu.Lock()
	defer rt.cacheMu.Unlock()

	now := time.Now()
	existing, exists := rt.systemCache[sys.ID]

	if exists {
		// Update LastVerified on direct contact regardless of version
		if verified {
			existing.LastVerified = now
			existing.Verified = true
			existing.LastGossipHeard = now
			existing.FailCount = 0
			if rt.storage != nil {
				rt.storage.TouchPeerSystem(sys.ID)
			}
		} else {
			// For gossip-only updates: only extend the prune timer if:
			// 1. The system was verified within the cutoff, OR
			// 2. The system has a newer InfoVersion
			staleCutoff := now.Add(-VerificationCutoff)
			hasRecentVerification := existing.Verified && existing.LastVerified.After(staleCutoff)
			hasNewerVersion := sys.InfoVersion > 0 && sys.InfoVersion > existing.System.InfoVersion

			if hasRecentVerification || hasNewerVersion {
				existing.LastGossipHeard = now
			}
		}

		// Check InfoVersion to decide whether to update system info
		shouldUpdate := false
		if sys.InfoVersion > 0 && existing.System.InfoVersion > 0 {
			shouldUpdate = sys.InfoVersion > existing.System.InfoVersion
		} else if sys.InfoVersion == 0 && existing.System.InfoVersion > 0 {
			shouldUpdate = false
		} else if sys.InfoVersion > 0 && existing.System.InfoVersion == 0 {
			shouldUpdate = true
		} else {
			shouldUpdate = verified
		}

		if shouldUpdate {
			existing.System = sys
			existing.LearnedAt = now
			// Always persist updates with newer InfoVersion to storage
			// The storage layer has its own InfoVersion check to prevent stale overwrites
			if rt.storage != nil {
				rt.storage.SavePeerSystem(sys)
			}
		}
	} else {
		// New system - add to cache
		cached := &CachedSystem{
			System:          sys,
			LearnedAt:       now,
			LearnedFrom:     learnedFrom,
			Verified:        verified,
			LastGossipHeard: now,
			FailCount:       0,
		}
		if verified {
			cached.LastVerified = now
		}
		rt.systemCache[sys.ID] = cached

		// Persist new systems to storage
		if rt.storage != nil {
			rt.storage.SavePeerSystem(sys)
			if verified {
				rt.storage.TouchPeerSystem(sys.ID)
			}
		}
	}
}

// GetCachedSystem retrieves a system from the cache
func (rt *RoutingTable) GetCachedSystem(id uuid.UUID) *System {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()

	if cached, ok := rt.systemCache[id]; ok {
		return cached.System
	}
	return nil
}

// GetCachedSystemMeta returns the full cache metadata for a system
func (rt *RoutingTable) GetCachedSystemMeta(id uuid.UUID) *CachedSystem {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()

	if cached, ok := rt.systemCache[id]; ok {
		return cached
	}
	return nil
}

// GetSystemIDByAddress looks up a system's UUID by its peer address
func (rt *RoutingTable) GetSystemIDByAddress(address string) uuid.UUID {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()

	for _, cached := range rt.systemCache {
		if cached.System.PeerAddress == address {
			return cached.System.ID
		}
	}
	return uuid.Nil
}

// GetAllCachedSystems returns all systems in the cache
func (rt *RoutingTable) GetAllCachedSystems() []*System {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()

	result := make([]*System, 0, len(rt.systemCache))
	for _, cached := range rt.systemCache {
		result = append(result, cached.System)
	}
	return result
}

// GetVerifiedCachedSystems returns only systems verified within maxAge
func (rt *RoutingTable) GetVerifiedCachedSystems(maxAge time.Duration) []*System {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()

	cutoff := time.Now().Add(-maxAge)
	result := make([]*System, 0)
	for _, cached := range rt.systemCache {
		if cached.Verified && !cached.LastVerified.IsZero() && cached.LastVerified.After(cutoff) {
			result = append(result, cached.System)
		}
	}
	return result
}

// GetUnverifiedCachedSystems returns unverified systems sorted by LastGossipHeard (oldest first)
func (rt *RoutingTable) GetUnverifiedCachedSystems() []*System {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()

	type unverifiedEntry struct {
		system     *System
		lastGossip time.Time
	}
	entries := make([]unverifiedEntry, 0)

	for _, cached := range rt.systemCache {
		if !cached.Verified {
			entries = append(entries, unverifiedEntry{
				system:     cached.System,
				lastGossip: cached.LastGossipHeard,
			})
		}
	}

	// Sort by LastGossipHeard ascending (oldest first)
	for i := 1; i < len(entries); i++ {
		j := i
		for j > 0 && entries[j].lastGossip.Before(entries[j-1].lastGossip) {
			entries[j], entries[j-1] = entries[j-1], entries[j]
			j--
		}
	}

	result := make([]*System, len(entries))
	for i, entry := range entries {
		result[i] = entry.system
	}
	return result
}

// GetUnverifiedCount returns the number of unverified systems in the cache
func (rt *RoutingTable) GetUnverifiedCount() int {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()

	count := 0
	for _, cached := range rt.systemCache {
		if !cached.Verified {
			count++
		}
	}
	return count
}

// PeerStateBreakdown provides a clear view of peer states
type PeerStateBreakdown struct {
	Total    int `json:"total"`    // All known systems (not including self)
	Active   int `json:"active"`   // Verified, recent, responding (fail=0)
	Degraded int `json:"degraded"` // Verified but failing (0 < fail < max)
	Pending  int `json:"pending"`  // Heard via gossip, not yet verified
	Stale    int `json:"stale"`    // Was verified but outside cutoff window
}

// GetPeerStateBreakdown returns counts of peers in each state
func (rt *RoutingTable) GetPeerStateBreakdown() PeerStateBreakdown {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-VerificationCutoff)

	breakdown := PeerStateBreakdown{
		Total: len(rt.systemCache),
	}

	for _, cached := range rt.systemCache {
		if !cached.Verified {
			// Never verified - pending
			breakdown.Pending++
		} else if cached.LastVerified.IsZero() || cached.LastVerified.Before(cutoff) {
			// Verified but too long ago - stale
			breakdown.Stale++
		} else if cached.FailCount > 0 {
			// Recently verified but now failing - degraded
			breakdown.Degraded++
		} else {
			// Verified, recent, responding - active
			breakdown.Active++
		}
	}

	return breakdown
}

// GetCacheSize returns the number of cached systems
func (rt *RoutingTable) GetCacheSize() int {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()
	return len(rt.systemCache)
}

// Remove completely removes a node from the cache
func (rt *RoutingTable) Remove(nodeID uuid.UUID) {
	rt.RemoveFromCache(nodeID)
}

// RemoveFromCache removes a system from the cache
func (rt *RoutingTable) RemoveFromCache(id uuid.UUID) {
	rt.cacheMu.Lock()
	defer rt.cacheMu.Unlock()
	delete(rt.systemCache, id)
}

// loadFromStorage loads cached systems from persistent storage
func (rt *RoutingTable) loadFromStorage() {
	if rt.storage == nil {
		return
	}

	systemsWithMeta, err := rt.storage.GetAllPeerSystemsWithMeta()
	if err != nil {
		log.Printf("Failed to load peer systems from storage: %v", err)
		return
	}

	log.Printf("Loading %d cached systems from storage", len(systemsWithMeta))

	now := time.Now()
	maxAge := 48 * time.Hour
	ageCutoff := now.Add(-maxAge)

	loaded := 0
	skipped := 0
	for _, meta := range systemsWithMeta {
		sys := meta.System
		if sys.ID == rt.localID {
			continue
		}

		var lastVerified time.Time
		var lastGossipHeard time.Time
		verified := false

		if meta.LastVerified > 0 {
			lastVerified = time.Unix(meta.LastVerified, 0)
			verified = true
		}

		if meta.UpdatedAt > 0 {
			lastGossipHeard = time.Unix(meta.UpdatedAt, 0)
		} else if verified {
			lastGossipHeard = lastVerified
		} else {
			lastGossipHeard = ageCutoff
		}

		// Skip systems that are too old
		if lastGossipHeard.Before(ageCutoff) && !verified {
			skipped++
			continue
		}

		rt.cacheMu.Lock()
		rt.systemCache[sys.ID] = &CachedSystem{
			System:          sys,
			LearnedAt:       lastGossipHeard,
			Verified:        verified,
			LastVerified:    lastVerified,
			LastGossipHeard: lastGossipHeard,
			FailCount:       0,
		}
		rt.cacheMu.Unlock()
		loaded++
	}

	if skipped > 0 {
		log.Printf("  Loaded %d systems, skipped %d stale unverified systems", loaded, skipped)
	} else {
		log.Printf("  Loaded %d systems", loaded)
	}
}

// PruneCache removes stale entries from the cache
func (rt *RoutingTable) PruneCache(maxAge time.Duration) int {
	rt.cacheMu.Lock()
	defer rt.cacheMu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	pruned := 0

	for id, cached := range rt.systemCache {
		shouldPrune := false
		if cached.Verified && !cached.LastVerified.IsZero() {
			shouldPrune = cached.LastVerified.Before(cutoff)
		} else {
			shouldPrune = cached.LastGossipHeard.Before(cutoff)
		}

		if shouldPrune {
			delete(rt.systemCache, id)
			pruned++
		}
	}

	return pruned
}
