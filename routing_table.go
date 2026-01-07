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
	BaseBucketK = 3

	// DefaultBucketK is used when star class isn't available
	DefaultBucketK = 5

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
	System      *System
	LearnedAt   time.Time
	LearnedFrom uuid.UUID
	Verified    bool // Have we directly communicated with them?
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
	// Divide MaxPeers across expected active buckets (~10-15 for large networks)
	// But ensure at least BaseBucketK
	k := maxPeers / 4
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

	// Always cache the system info
	rt.CacheSystem(sys, uuid.Nil, false)

	bucketIdx := rt.BucketIndex(sys.ID)
	bucket := rt.buckets[bucketIdx]

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	bucket.lastAccess = time.Now()

	// Check if already in bucket
	for i, entry := range bucket.entries {
		if entry.System.ID == sys.ID {
			// Update existing entry
			bucket.entries[i].System = sys
			bucket.entries[i].LastSeen = time.Now()
			bucket.entries[i].FailCount = 0
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
	bucketIdx := rt.BucketIndex(nodeID)
	bucket := rt.buckets[bucketIdx]

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	for i, entry := range bucket.entries {
		if entry.System.ID == nodeID {
			bucket.entries[i].LastVerified = time.Now()
			bucket.entries[i].LastSeen = time.Now()
			bucket.entries[i].FailCount = 0
			return
		}
	}

	// Also mark in cache
	rt.cacheMu.Lock()
	if cached, ok := rt.systemCache[nodeID]; ok {
		cached.Verified = true
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

	// Collect all entries from all buckets
	var allEntries []*BucketEntry
	for _, bucket := range rt.buckets {
		bucket.mu.RLock()
		allEntries = append(allEntries, bucket.entries...)
		bucket.mu.RUnlock()
	}

	// Also include verified cached systems not in routing table
	rt.cacheMu.RLock()
	seenIDs := make(map[uuid.UUID]bool)
	for _, entry := range allEntries {
		seenIDs[entry.System.ID] = true
	}
	for id, cached := range rt.systemCache {
		if !seenIDs[id] && cached.Verified {
			allEntries = append(allEntries, &BucketEntry{
				System:   cached.System,
				LastSeen: cached.LearnedAt,
				Verified: true,
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
func (rt *RoutingTable) CacheSystem(sys *System, learnedFrom uuid.UUID, verified bool) {
	if sys == nil || sys.ID == rt.localID {
		return
	}

	rt.cacheMu.Lock()
	defer rt.cacheMu.Unlock()

	existing, exists := rt.systemCache[sys.ID]
	if exists {
		// Update existing entry
		existing.System = sys
		if verified {
			existing.Verified = true
		}
	} else {
		rt.systemCache[sys.ID] = &CachedSystem{
			System:      sys,
			LearnedAt:   time.Now(),
			LearnedFrom: learnedFrom,
			Verified:    verified,
		}
	}

	// Persist to storage
	if rt.storage != nil {
		rt.storage.SavePeerSystem(sys)
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

// GetVerifiedCachedSystems returns only verified systems from the cache
func (rt *RoutingTable) GetVerifiedCachedSystems() []*System {
	rt.cacheMu.RLock()
	defer rt.cacheMu.RUnlock()

	result := make([]*System, 0)
	for _, cached := range rt.systemCache {
		if cached.Verified {
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
func (rt *RoutingTable) PruneCache(maxAge time.Duration) int {
	rt.cacheMu.Lock()
	defer rt.cacheMu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	pruned := 0

	for id, cached := range rt.systemCache {
		// Don't prune verified systems or those in routing table
		if cached.Verified {
			continue
		}
		if rt.isInRoutingTable(id) {
			continue
		}
		if cached.LearnedAt.Before(cutoff) {
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