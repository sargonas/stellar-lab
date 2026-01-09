package main

import (
	"crypto/sha256"
	"log"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// IDBits is the number of bits in a UUID (128 bits)
	IDBits = 128

	// BaseBucketK is the minimum entries per bucket
	BaseBucketK = 5

	// DefaultBucketK is used when star class isn't available
	DefaultBucketK = 8

	// SpatialReplacementThreshold - candidate must be this much closer (20%)
	SpatialReplacementThreshold = 0.8

	// MaxFailCount before a node is considered dead
	// With 5-min liveness checks, 12 failures = 60 min (double longevity reset threshold)
	MaxFailCount = 12
)

// BucketEntry represents a node in a k-bucket
type BucketEntry struct {
	System       *System   // Full system info
	LastSeen     time.Time // Last successful contact
	LastVerified time.Time // Last successful ping response
	FailCount    int       // Consecutive failures
}

// KBucket holds nodes at a specific XOR distance range
type KBucket struct {
	entries    []*BucketEntry
	lastAccess time.Time
	mu         sync.RWMutex
}

// RoutingTable manages k-buckets for DHT routing
type RoutingTable struct {
	localID     uuid.UUID
	localSystem *System
	buckets     [IDBits]*KBucket
	bucketK     int // Max entries per bucket (derived from star class)
	mu          sync.RWMutex

	// System cache - all known systems (not just routing table)
	systemCache map[uuid.UUID]*CachedSystem
	cacheMu     sync.RWMutex

	// Storage for persistence
	storage *Storage
}

// CachedSystem stores full system info with metadata
type CachedSystem struct {
	System       *System
	LearnedAt    time.Time  // When we first learned about this system (or got authoritative update)
	LearnedFrom  uuid.UUID
	Verified     bool       // Have we directly communicated with them?
	LastVerified time.Time  // When we last had direct contact (zero if never)
}

// NewRoutingTable creates a new routing table for the local node
func NewRoutingTable(localSystem *System, storage *Storage) *RoutingTable {
	rt := &RoutingTable{
		localID:     localSystem.ID,
		localSystem: localSystem,
		bucketK:     calculateBucketK(localSystem),
		systemCache: make(map[uuid.UUID]*CachedSystem),
		storage:     storage,
	}

	// Initialize all buckets
	for i := 0; i < IDBits; i++ {
		rt.buckets[i] = &KBucket{
			entries:    make([]*BucketEntry, 0, rt.bucketK),
			lastAccess: time.Now(),
		}
	}

	// Load cached systems from storage
	rt.loadFromStorage()

	return rt
}

// calculateBucketK determines bucket capacity based on star class
func calculateBucketK(sys *System) int {
	if sys == nil {
		return DefaultBucketK
	}
	maxPeers := sys.GetMaxPeers()
	// Half of MaxPeers gives good bucket capacity while leaving room for churn
	// Ensures at least BaseBucketK
	k := maxPeers / 2
	if k < BaseBucketK {
		k = BaseBucketK
	}
	return k
}

// XORDistance calculates the XOR distance between two UUIDs
func XORDistance(a, b uuid.UUID) [16]byte {
	var result [16]byte
	for i := 0; i < 16; i++ {
		result[i] = a[i] ^ b[i]
	}
	return result
}

// XORDistanceLeadingZeros returns the number of leading zero bits in XOR distance
// This determines which bucket a node belongs to
func XORDistanceLeadingZeros(a, b uuid.UUID) int {
	for i := 0; i < 16; i++ {
		xor := a[i] ^ b[i]
		if xor == 0 {
			continue
		}
		// Count leading zeros in this byte
		for j := 7; j >= 0; j-- {
			if xor&(1<<j) != 0 {
				return i*8 + (7 - j)
			}
		}
	}
	return 128 // Identical UUIDs
}

// BucketIndex returns the bucket index for a given node ID
// Bucket 0 = most distant (first bit differs), Bucket 127 = closest
func (rt *RoutingTable) BucketIndex(nodeID uuid.UUID) int {
	leadingZeros := XORDistanceLeadingZeros(rt.localID, nodeID)
	if leadingZeros >= IDBits {
		return IDBits - 1 // Same ID (shouldn't happen)
	}
	return leadingZeros
}

// CompareXORDistance returns -1 if a is closer to target, 1 if b is closer, 0 if equal
func CompareXORDistance(target, a, b uuid.UUID) int {
	distA := XORDistance(target, a)
	distB := XORDistance(target, b)

	for i := 0; i < 16; i++ {
		if distA[i] < distB[i] {
			return -1
		}
		if distA[i] > distB[i] {
			return 1
		}
	}
	return 0
}

// Update adds or updates a node in the routing table
// Returns true if the node was added/updated, false if rejected
func (rt *RoutingTable) Update(sys *System) bool {
	if sys == nil || sys.ID == rt.localID {
		return false
	}

	// Always cache the system info (CacheSystem handles version checking)
	rt.CacheSystem(sys, uuid.Nil, false)

	bucketIdx := rt.BucketIndex(sys.ID)
	bucket := rt.buckets[bucketIdx]

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	bucket.lastAccess = time.Now()

	// Check if already in bucket
	for i, entry := range bucket.entries {
		if entry.System.ID == sys.ID {
			// Check InfoVersion before updating routing table entry
			if sys.InfoVersion > 0 && entry.System.InfoVersion > 0 {
				if sys.InfoVersion <= entry.System.InfoVersion {
					// Stale info - don't update routing table entry
					return true // Still return true because the node exists
				}
			} else if sys.InfoVersion == 0 && entry.System.InfoVersion > 0 {
				// Incoming is old protocol, we have versioned - keep ours
				return true
			}
			
			// Update existing entry's system info but preserve FailCount
			// Only direct communication (MarkVerified) should reset FailCount
			bucket.entries[i].System = sys
			bucket.entries[i].LastSeen = time.Now()
			// Note: FailCount is NOT reset here - only MarkVerified() resets it
			// This prevents "ghost" nodes from persisting due to peer gossip
			return true
		}
	}

	// Not in bucket - try to add
	newEntry := &BucketEntry{
		System:   sys,
		LastSeen: time.Now(),
		FailCount: 0,
	}

	if len(bucket.entries) < rt.bucketK {
		// Bucket has space
		bucket.entries = append(bucket.entries, newEntry)
		return true
	}

	// Bucket full - check for failed nodes to replace
	for i, entry := range bucket.entries {
		if entry.FailCount >= MaxFailCount {
			bucket.entries[i] = newEntry
			return true
		}
	}

	// Check spatial distance for replacement
	if rt.shouldReplaceSpatially(bucket, newEntry) {
		return true // Replacement happened in shouldReplaceSpatially
	}

	// Bucket full, no replacement criteria met
	return false
}

// shouldReplaceSpatially checks if we should replace a node based on spatial proximity
func (rt *RoutingTable) shouldReplaceSpatially(bucket *KBucket, candidate *BucketEntry) bool {
	if rt.localSystem == nil || candidate.System == nil {
		return false
	}

	localCoords := Point3D{rt.localSystem.X, rt.localSystem.Y, rt.localSystem.Z}
	candidateDist := localCoords.DistanceTo(Point3D{
		candidate.System.X, candidate.System.Y, candidate.System.Z,
	})

	// Find the spatially furthest node
	var furthestIdx int = -1
	var furthestDist float64 = 0

	for i, entry := range bucket.entries {
		if entry.System == nil {
			continue
		}
		dist := localCoords.DistanceTo(Point3D{
			entry.System.X, entry.System.Y, entry.System.Z,
		})
		if dist > furthestDist {
			furthestDist = dist
			furthestIdx = i
		}
	}

	// Replace if candidate is significantly closer
	if furthestIdx >= 0 && candidateDist < furthestDist*SpatialReplacementThreshold {
		bucket.entries[furthestIdx] = candidate
		return true
	}

	return false
}

// Point3D represents 3D coordinates
type Point3D struct {
	X, Y, Z float64
}

// DistanceTo calculates Euclidean distance to another point
func (p Point3D) DistanceTo(other Point3D) float64 {
	dx := p.X - other.X
	dy := p.Y - other.Y
	dz := p.Z - other.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// MarkFailed increments the fail count for a node
func (rt *RoutingTable) MarkFailed(nodeID uuid.UUID) {
	bucketIdx := rt.BucketIndex(nodeID)
	bucket := rt.buckets[bucketIdx]

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	for i, entry := range bucket.entries {
		if entry.System.ID == nodeID {
			bucket.entries[i].FailCount++
			return
		}
	}
}

// MarkVerified marks a node as successfully verified (ping response received)
func (rt *RoutingTable) MarkVerified(nodeID uuid.UUID) {
	now := time.Now()
	bucketIdx := rt.BucketIndex(nodeID)
	bucket := rt.buckets[bucketIdx]

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	for i, entry := range bucket.entries {
		if entry.System.ID == nodeID {
			bucket.entries[i].LastVerified = now
			bucket.entries[i].LastSeen = now
			bucket.entries[i].FailCount = 0
			// Also update storage timestamp
			if rt.storage != nil {
				rt.storage.TouchPeerSystem(nodeID)
			}
			return
		}
	}

	// Also mark in cache and update LastVerified
	rt.cacheMu.Lock()
	if cached, ok := rt.systemCache[nodeID]; ok {
		cached.Verified = true
		cached.LastVerified = now
		cached.LearnedAt = now // Also refresh LearnedAt since we have fresh contact
		// Also update storage timestamp
		if rt.storage != nil {
			rt.storage.TouchPeerSystem(nodeID)
		}
	}
	rt.cacheMu.Unlock()
}

// EvictDeadNodes removes nodes with too many failures from routing table
func (rt *RoutingTable) EvictDeadNodes() int {
	evicted := 0
	for _, bucket := range rt.buckets {
		bucket.mu.Lock()
		newEntries := make([]*BucketEntry, 0, len(bucket.entries))
		for _, entry := range bucket.entries {
			if entry.FailCount < MaxFailCount {
				newEntries = append(newEntries, entry)
			} else {
				evicted++
			}
		}
		bucket.entries = newEntries
		bucket.mu.Unlock()
	}
	return evicted
}

// GetClosest returns the K closest nodes to a target ID
func (rt *RoutingTable) GetClosest(targetID uuid.UUID, count int) []*System {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Collect all entries from all buckets (routing table entries are always "live")
	var allEntries []*BucketEntry
	for _, bucket := range rt.buckets {
		bucket.mu.RLock()
		allEntries = append(allEntries, bucket.entries...)
		bucket.mu.RUnlock()
	}

	// Also include recently-verified cached systems not in routing table
	// Only include systems verified within the last 24 hours to prevent ghost propagation
	rt.cacheMu.RLock()
	seenIDs := make(map[uuid.UUID]bool)
	for _, entry := range allEntries {
		seenIDs[entry.System.ID] = true
	}
	verificationCutoff := time.Now().Add(-24 * time.Hour)
	for id, cached := range rt.systemCache {
		if !seenIDs[id] && cached.Verified && 
		   !cached.LastVerified.IsZero() && cached.LastVerified.After(verificationCutoff) {
			allEntries = append(allEntries, &BucketEntry{
				System:   cached.System,
				LastSeen: cached.LearnedAt,
			})
		}
	}
	rt.cacheMu.RUnlock()

	// Sort by XOR distance to target
	sortByXORDistance(allEntries, targetID)

	// Return top 'count' systems
	result := make([]*System, 0, count)
	for i := 0; i < len(allEntries) && i < count; i++ {
		if allEntries[i].System != nil {
			result = append(result, allEntries[i].System)
		}
	}

	return result
}

// sortByXORDistance sorts entries by XOR distance to target (in place)
func sortByXORDistance(entries []*BucketEntry, target uuid.UUID) {
	// Simple insertion sort (good for small lists)
	for i := 1; i < len(entries); i++ {
		j := i
		for j > 0 && CompareXORDistance(target, entries[j].System.ID, entries[j-1].System.ID) < 0 {
			entries[j], entries[j-1] = entries[j-1], entries[j]
			j--
		}
	}
}

// GetBucketNodes returns all nodes in a specific bucket
func (rt *RoutingTable) GetBucketNodes(bucketIdx int) []*System {
	if bucketIdx < 0 || bucketIdx >= IDBits {
		return nil
	}

	bucket := rt.buckets[bucketIdx]
	bucket.mu.RLock()
	defer bucket.mu.RUnlock()

	result := make([]*System, 0, len(bucket.entries))
	for _, entry := range bucket.entries {
		if entry.System != nil {
			result = append(result, entry.System)
		}
	}
	return result
}

// RandomIDInBucket generates a random ID that would fall in the specified bucket
func (rt *RoutingTable) RandomIDInBucket(bucketIdx int) uuid.UUID {
	// Create ID that shares first 'bucketIdx' bits with local ID, then differs
	result := rt.localID

	// Flip the bit at position bucketIdx
	byteIdx := bucketIdx / 8
	bitIdx := 7 - (bucketIdx % 8)

	result[byteIdx] ^= (1 << bitIdx)

	// Randomize remaining bits
	hash := sha256.Sum256(append(result[:], byte(time.Now().UnixNano())))
	for i := byteIdx + 1; i < 16; i++ {
		result[i] = hash[i]
	}

	return result
}

// GetAllRoutingTableNodes returns all nodes currently in the routing table
func (rt *RoutingTable) GetAllRoutingTableNodes() []*System {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var result []*System
	for _, bucket := range rt.buckets {
		bucket.mu.RLock()
		for _, entry := range bucket.entries {
			if entry.System != nil {
				result = append(result, entry.System)
			}
		}
		bucket.mu.RUnlock()
	}
	return result
}

// GetRoutingTableSize returns the total number of nodes in the routing table
func (rt *RoutingTable) GetRoutingTableSize() int {
	count := 0
	for _, bucket := range rt.buckets {
		bucket.mu.RLock()
		count += len(bucket.entries)
		bucket.mu.RUnlock()
	}
	return count
}

// BucketLastAccess returns when a bucket was last accessed
func (rt *RoutingTable) BucketLastAccess(bucketIdx int) time.Time {
	if bucketIdx < 0 || bucketIdx >= IDBits {
		return time.Time{}
	}
	rt.buckets[bucketIdx].mu.RLock()
	defer rt.buckets[bucketIdx].mu.RUnlock()
	return rt.buckets[bucketIdx].lastAccess
}

// === System Cache Methods ===

// CacheSystem adds a system to the cache
// InfoVersion is used to prevent stale gossip from overwriting fresh info:
// - If incoming version > cached version → update system info
// - If incoming version <= cached version → ignore system info (stale)
// - If incoming version == 0 (old protocol) → special handling
// LastVerified is ONLY updated on verified direct contact
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
			// Update DB's last_verified timestamp
			if rt.storage != nil {
				rt.storage.TouchPeerSystem(sys.ID)
			}
		}
		
		// Check InfoVersion to decide whether to update system info
		shouldUpdate := false
		
		if sys.InfoVersion > 0 && existing.System.InfoVersion > 0 {
			// Both have versions - only accept if incoming is newer
			shouldUpdate = sys.InfoVersion > existing.System.InfoVersion
		} else if sys.InfoVersion == 0 && existing.System.InfoVersion > 0 {
			// Incoming is legacy (0), we have versioned - keep ours
			shouldUpdate = false
		} else if sys.InfoVersion > 0 && existing.System.InfoVersion == 0 {
			// Incoming has version, ours is legacy - accept upgrade
			shouldUpdate = true
		} else {
			// Both are legacy (0) - accept if verified, otherwise keep existing
			shouldUpdate = verified
		}
		
		if shouldUpdate {
			existing.System = sys
			existing.LearnedAt = now
			// Save updated info to DB if verified
			if verified && rt.storage != nil {
				rt.storage.SavePeerSystem(sys)
			}
		}
	} else {
		// New system - add to cache
		cached := &CachedSystem{
			System:      sys,
			LearnedAt:   now,
			LearnedFrom: learnedFrom,
			Verified:    verified,
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

// GetSystemIDByAddress looks up a system's UUID by its peer address
// Returns uuid.Nil if the address is not known
func (rt *RoutingTable) GetSystemIDByAddress(address string) uuid.UUID {
	// First check routing table
	rt.mu.RLock()
	for _, bucket := range rt.buckets {
		bucket.mu.RLock()
		for _, entry := range bucket.entries {
			if entry.System.PeerAddress == address {
				id := entry.System.ID
				bucket.mu.RUnlock()
				rt.mu.RUnlock()
				return id
			}
		}
		bucket.mu.RUnlock()
	}
	rt.mu.RUnlock()

	// Then check cache
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
// Used for FIND_NODE responses - only share peers we've actually talked to recently
// This breaks the ghost propagation loop by not sharing stale/unverified peers
func (rt *RoutingTable) GetVerifiedCachedSystems(maxAge time.Duration) []*System {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()

	cutoff := time.Now().Add(-maxAge)
	result := make([]*System, 0)
	for _, cached := range rt.systemCache {
		// Only include if verified AND LastVerified is recent
		if cached.Verified && !cached.LastVerified.IsZero() && cached.LastVerified.After(cutoff) {
			result = append(result, cached.System)
		}
	}
	return result
}

// GetCacheSize returns the number of cached systems
func (rt *RoutingTable) GetCacheSize() int {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()
	return len(rt.systemCache)
}

// Remove completely removes a node from both routing table and cache
func (rt *RoutingTable) Remove(nodeID uuid.UUID) {
	// Remove from routing table bucket
	bucketIdx := rt.BucketIndex(nodeID)
	bucket := rt.buckets[bucketIdx]

	bucket.mu.Lock()
	newEntries := make([]*BucketEntry, 0, len(bucket.entries))
	for _, entry := range bucket.entries {
		if entry.System.ID != nodeID {
			newEntries = append(newEntries, entry)
		}
	}
	bucket.entries = newEntries
	bucket.mu.Unlock()

	// Remove from cache
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

	systems, err := rt.storage.GetAllPeerSystems()
	if err != nil {
		log.Printf("Failed to load peer systems from storage: %v", err)
		return
	}

	log.Printf("Loading %d cached systems from storage", len(systems))

	for _, sys := range systems {
		if sys.ID == rt.localID {
			continue
		}
		
		// Add to cache directly (avoiding CacheSystem to prevent redundant DB write)
		rt.cacheMu.Lock()
		rt.systemCache[sys.ID] = &CachedSystem{
			System:    sys,
			LearnedAt: time.Now(),
			Verified:  false,
		}
		rt.cacheMu.Unlock()

		// Try to add to routing table (Update will skip CacheSystem since already cached)
		bucketIdx := rt.BucketIndex(sys.ID)
		bucket := rt.buckets[bucketIdx]

		bucket.mu.Lock()
		// Check if already in bucket
		found := false
		for _, entry := range bucket.entries {
			if entry.System.ID == sys.ID {
				found = true
				break
			}
		}
		if !found && len(bucket.entries) < rt.bucketK {
			bucket.entries = append(bucket.entries, &BucketEntry{
				System:    sys,
				LastSeen:  time.Now(),
				FailCount: 0,
			})
		}
		bucket.mu.Unlock()
	}
}

// PruneCache removes stale entries from the cache
// - Verified systems: pruned if LastVerified > maxAge (no recent direct contact)
// - Unverified systems: pruned if LearnedAt > maxAge (never contacted, just gossip)
func (rt *RoutingTable) PruneCache(maxAge time.Duration) int {
	rt.cacheMu.Lock()
	defer rt.cacheMu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	pruned := 0

	for id, cached := range rt.systemCache {
		// Don't prune systems currently in the routing table (active peers)
		if rt.isInRoutingTable(id) {
			continue
		}
		
		shouldPrune := false
		if cached.Verified && !cached.LastVerified.IsZero() {
			// Verified system: prune based on LastVerified
			shouldPrune = cached.LastVerified.Before(cutoff)
		} else {
			// Unverified system: prune based on LearnedAt
			shouldPrune = cached.LearnedAt.Before(cutoff)
		}
		
		if shouldPrune {
			delete(rt.systemCache, id)
			pruned++
		}
	}

	return pruned
}

// isInRoutingTable checks if a node is in the routing table (must hold cacheMu)
func (rt *RoutingTable) isInRoutingTable(id uuid.UUID) bool {
	bucketIdx := rt.BucketIndex(id)
	bucket := rt.buckets[bucketIdx]

	bucket.mu.RLock()
	defer bucket.mu.RUnlock()

	for _, entry := range bucket.entries {
		if entry.System.ID == id {
			return true
		}
	}
	return false
}

// === Persistence ===

// SaveSnapshot saves the routing table state to storage
func (rt *RoutingTable) SaveSnapshot() error {
	if rt.storage == nil {
		return nil
	}

	nodes := rt.GetAllRoutingTableNodes()
	for _, sys := range nodes {
		if err := rt.storage.SavePeerSystem(sys); err != nil {
			return err
		}
	}
	return nil
}