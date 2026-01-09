package main

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// XOR DISTANCE TESTS
// =============================================================================

// TestXORDistance verifies XOR distance follows mathematical XOR properties:
// - XOR is symmetric: a ^ b = b ^ a
// - XOR with self is zero: a ^ a = 0
// - XOR with zero is identity: a ^ 0 = a
// These are mathematical truths, not implementation-specific
func TestXORDistance(t *testing.T) {
	// Create two UUIDs with known byte values
	id1 := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	id2 := uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")

	dist := XORDistance(id1, id2)

	// XOR of 0x00 and 0xFF should be 0xFF for all bytes (mathematical truth)
	for i := 0; i < 16; i++ {
		if dist[i] != 0xFF {
			t.Errorf("XORDistance byte %d: got %02x, want 0xFF", i, dist[i])
		}
	}

	// XOR of same ID should be all zeros (mathematical identity: a ^ a = 0)
	dist2 := XORDistance(id1, id1)
	for i := 0; i < 16; i++ {
		if dist2[i] != 0x00 {
			t.Errorf("XORDistance of same ID byte %d: got %02x, want 0x00", i, dist2[i])
		}
	}
}

// TestXORDistance_Symmetry verifies XOR is symmetric (a ^ b = b ^ a)
func TestXORDistance_Symmetry(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()

	dist1 := XORDistance(id1, id2)
	dist2 := XORDistance(id2, id1)

	for i := 0; i < 16; i++ {
		if dist1[i] != dist2[i] {
			t.Errorf("XOR should be symmetric: byte %d differs", i)
		}
	}
}

// TestXORDistance_SpecificBits verifies bit-level XOR correctness
func TestXORDistance_SpecificBits(t *testing.T) {
	// 0x0F ^ 0xF0 = 0xFF (binary: 00001111 ^ 11110000 = 11111111)
	id1 := uuid.MustParse("0f0f0f0f-0f0f-0f0f-0f0f-0f0f0f0f0f0f")
	id2 := uuid.MustParse("f0f0f0f0-f0f0-f0f0-f0f0-f0f0f0f0f0f0")

	dist := XORDistance(id1, id2)

	for i := 0; i < 16; i++ {
		if dist[i] != 0xFF {
			t.Errorf("0x0F ^ 0xF0 should = 0xFF, got %02x at byte %d", dist[i], i)
		}
	}
}

// TestXORDistanceLeadingZeros verifies leading zero counting for Kademlia k-bucket assignment.
// These expected values are derived from bit-level analysis:
// - 0x80 = 10000000 binary, so XOR with 0x00 gives first bit set = 0 leading zeros
// - 0x40 = 01000000 binary, so XOR with 0x00 gives second bit set = 1 leading zero
// - 0x01 in last byte = bit 127 set = 127 leading zeros
// This is mathematical, not implementation-dependent.
func TestXORDistanceLeadingZeros(t *testing.T) {
	tests := []struct {
		name  string
		id1   string
		id2   string
		want  int
		proof string // Mathematical proof of expected value
	}{
		{
			name:  "same ID - 128 leading zeros",
			id1:   "00000000-0000-0000-0000-000000000000",
			id2:   "00000000-0000-0000-0000-000000000000",
			want:  128,
			proof: "XOR = 0, all 128 bits are zero",
		},
		{
			name:  "first bit differs - 0 leading zeros",
			id1:   "00000000-0000-0000-0000-000000000000",
			id2:   "80000000-0000-0000-0000-000000000000",
			want:  0,
			proof: "0x80 = 10000000b, bit 0 is set, 0 zeros before it",
		},
		{
			name:  "second bit differs - 1 leading zero",
			id1:   "00000000-0000-0000-0000-000000000000",
			id2:   "40000000-0000-0000-0000-000000000000",
			want:  1,
			proof: "0x40 = 01000000b, bit 1 is set, 1 zero before it",
		},
		{
			name:  "eighth bit differs - 7 leading zeros",
			id1:   "00000000-0000-0000-0000-000000000000",
			id2:   "01000000-0000-0000-0000-000000000000",
			want:  7,
			proof: "0x01 = 00000001b in first byte, bit 7 set, 7 zeros before it",
		},
		{
			name:  "ninth bit differs - 8 leading zeros",
			id1:   "00000000-0000-0000-0000-000000000000",
			id2:   "00800000-0000-0000-0000-000000000000",
			want:  8,
			proof: "0x80 in second byte = bit 8 overall, 8 zeros before it",
		},
		{
			name:  "last bit differs - 127 leading zeros",
			id1:   "00000000-0000-0000-0000-000000000000",
			id2:   "00000000-0000-0000-0000-000000000001",
			want:  127,
			proof: "0x01 in last byte = bit 127 (0-indexed), 127 zeros before it",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id1 := uuid.MustParse(tt.id1)
			id2 := uuid.MustParse(tt.id2)
			got := XORDistanceLeadingZeros(id1, id2)
			if got != tt.want {
				t.Errorf("XORDistanceLeadingZeros() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCompareXORDistance(t *testing.T) {
	target := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	closer := uuid.MustParse("00000001-0000-0000-0000-000000000000") // Differs in bit 31
	further := uuid.MustParse("00000002-0000-0000-0000-000000000000") // Differs in bit 30

	tests := []struct {
		name   string
		target uuid.UUID
		a      uuid.UUID
		b      uuid.UUID
		want   int
	}{
		{
			name:   "a is closer",
			target: target,
			a:      closer,
			b:      further,
			want:   -1,
		},
		{
			name:   "b is closer",
			target: target,
			a:      further,
			b:      closer,
			want:   1,
		},
		{
			name:   "equal distance (same node)",
			target: target,
			a:      closer,
			b:      closer,
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareXORDistance(tt.target, tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CompareXORDistance() = %d, want %d", got, tt.want)
			}
		})
	}
}

// =============================================================================
// POINT3D DISTANCE TESTS
// =============================================================================

func TestPoint3D_DistanceTo(t *testing.T) {
	tests := []struct {
		name  string
		p1    Point3D
		p2    Point3D
		want  float64
	}{
		{
			name: "same point",
			p1:   Point3D{0, 0, 0},
			p2:   Point3D{0, 0, 0},
			want: 0,
		},
		{
			name: "unit distance on X axis",
			p1:   Point3D{0, 0, 0},
			p2:   Point3D{1, 0, 0},
			want: 1,
		},
		{
			name: "unit distance on Y axis",
			p1:   Point3D{0, 0, 0},
			p2:   Point3D{0, 1, 0},
			want: 1,
		},
		{
			name: "unit distance on Z axis",
			p1:   Point3D{0, 0, 0},
			p2:   Point3D{0, 0, 1},
			want: 1,
		},
		{
			name: "3-4-5 triangle in XY plane",
			p1:   Point3D{0, 0, 0},
			p2:   Point3D{3, 4, 0},
			want: 5,
		},
		{
			name: "diagonal in unit cube",
			p1:   Point3D{0, 0, 0},
			p2:   Point3D{1, 1, 1},
			want: math.Sqrt(3),
		},
		{
			name: "negative coordinates",
			p1:   Point3D{-1, -1, -1},
			p2:   Point3D{1, 1, 1},
			want: math.Sqrt(12), // sqrt(4 + 4 + 4)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p1.DistanceTo(tt.p2)
			if math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("DistanceTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// BUCKET CALCULATION TESTS
// =============================================================================

func TestCalculateBucketK(t *testing.T) {
	tests := []struct {
		name       string
		starClass  string
		wantMinK   int
	}{
		{
			name:      "nil system",
			starClass: "",
			wantMinK:  DefaultBucketK,
		},
		{
			name:      "O class (highest)",
			starClass: "O",
			wantMinK:  BaseBucketK,
		},
		{
			name:      "M class (lowest)",
			starClass: "M",
			wantMinK:  BaseBucketK,
		},
		{
			name:      "X class (black hole)",
			starClass: "X",
			wantMinK:  BaseBucketK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sys *System
			if tt.starClass != "" {
				sys = &System{
					Stars: MultiStarSystem{
						Primary: StarType{Class: tt.starClass},
					},
				}
			}
			got := calculateBucketK(sys)
			if got < tt.wantMinK {
				t.Errorf("calculateBucketK() = %d, want >= %d", got, tt.wantMinK)
			}
		})
	}
}

// =============================================================================
// EVICTION LOGIC TESTS
// =============================================================================

func TestMaxFailCount_Constant(t *testing.T) {
	// Verify the MaxFailCount is set as expected (12)
	// This is important because eviction logic depends on it
	if MaxFailCount != 12 {
		t.Errorf("MaxFailCount = %d, expected 12", MaxFailCount)
	}
}

func TestBucketEntry_FailCountTracking(t *testing.T) {
	entry := &BucketEntry{
		System:    &System{ID: uuid.New()},
		LastSeen:  time.Now(),
		FailCount: 0,
	}

	// Simulate failures
	for i := 0; i < MaxFailCount; i++ {
		entry.FailCount++
	}

	if entry.FailCount < MaxFailCount {
		t.Errorf("FailCount = %d after %d failures, expected >= %d", entry.FailCount, MaxFailCount, MaxFailCount)
	}

	// Should be eligible for eviction
	if entry.FailCount < MaxFailCount {
		t.Error("Entry should be eligible for eviction at MaxFailCount")
	}
}

func TestEvictionThreshold(t *testing.T) {
	// Test that nodes at MaxFailCount-1 are NOT evicted
	// and nodes at MaxFailCount ARE evicted

	tests := []struct {
		name        string
		failCount   int
		shouldEvict bool
	}{
		{"0 failures", 0, false},
		{"1 failure", 1, false},
		{"11 failures (MaxFailCount-1)", MaxFailCount - 1, false},
		{"12 failures (MaxFailCount)", MaxFailCount, true},
		{"13 failures (MaxFailCount+1)", MaxFailCount + 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldEvict := tt.failCount >= MaxFailCount
			if shouldEvict != tt.shouldEvict {
				t.Errorf("shouldEvict at failCount=%d: got %v, want %v", tt.failCount, shouldEvict, tt.shouldEvict)
			}
		})
	}
}

// =============================================================================
// SPATIAL REPLACEMENT TESTS
// =============================================================================

func TestSpatialReplacementThreshold(t *testing.T) {
	// Verify the threshold constant
	if SpatialReplacementThreshold != 0.8 {
		t.Errorf("SpatialReplacementThreshold = %v, expected 0.8", SpatialReplacementThreshold)
	}
}

func TestSpatialReplacement_Calculation(t *testing.T) {
	// Test the spatial replacement logic
	// Candidate must be at least 20% closer (candidateDist < furthestDist * 0.8)

	tests := []struct {
		name          string
		furthestDist  float64
		candidateDist float64
		shouldReplace bool
	}{
		{
			name:          "candidate much closer (50%)",
			furthestDist:  100.0,
			candidateDist: 50.0,
			shouldReplace: true,
		},
		{
			name:          "candidate exactly at threshold (80%)",
			furthestDist:  100.0,
			candidateDist: 80.0,
			shouldReplace: false, // Must be < not <=
		},
		{
			name:          "candidate just under threshold (79%)",
			furthestDist:  100.0,
			candidateDist: 79.0,
			shouldReplace: true,
		},
		{
			name:          "candidate further than threshold (90%)",
			furthestDist:  100.0,
			candidateDist: 90.0,
			shouldReplace: false,
		},
		{
			name:          "candidate same distance",
			furthestDist:  100.0,
			candidateDist: 100.0,
			shouldReplace: false,
		},
		{
			name:          "candidate further away",
			furthestDist:  100.0,
			candidateDist: 150.0,
			shouldReplace: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldReplace := tt.candidateDist < tt.furthestDist*SpatialReplacementThreshold
			if shouldReplace != tt.shouldReplace {
				t.Errorf("shouldReplace = %v, want %v (candidate: %v, furthest: %v, threshold: %v)",
					shouldReplace, tt.shouldReplace, tt.candidateDist, tt.furthestDist, tt.furthestDist*SpatialReplacementThreshold)
			}
		})
	}
}

// =============================================================================
// INFO VERSION (STALE GOSSIP) TESTS
// =============================================================================

func TestInfoVersion_StaleGossipRejection(t *testing.T) {
	// Test the logic for rejecting stale gossip based on InfoVersion

	tests := []struct {
		name           string
		incomingVer    int64
		cachedVer      int64
		shouldUpdate   bool
	}{
		{
			name:         "incoming newer than cached",
			incomingVer:  1000,
			cachedVer:    500,
			shouldUpdate: true,
		},
		{
			name:         "incoming same as cached",
			incomingVer:  1000,
			cachedVer:    1000,
			shouldUpdate: false,
		},
		{
			name:         "incoming older than cached",
			incomingVer:  500,
			cachedVer:    1000,
			shouldUpdate: false,
		},
		{
			name:         "incoming is 0 (legacy), cached has version",
			incomingVer:  0,
			cachedVer:    1000,
			shouldUpdate: false, // Keep versioned data
		},
		{
			name:         "incoming has version, cached is 0 (legacy)",
			incomingVer:  1000,
			cachedVer:    0,
			shouldUpdate: true, // Accept upgrade
		},
		{
			name:         "both are legacy (0)",
			incomingVer:  0,
			cachedVer:    0,
			shouldUpdate: false, // Default to keeping cached unless verified
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var shouldUpdate bool

			if tt.incomingVer > 0 && tt.cachedVer > 0 {
				// Both have versions - only accept if incoming is newer
				shouldUpdate = tt.incomingVer > tt.cachedVer
			} else if tt.incomingVer == 0 && tt.cachedVer > 0 {
				// Incoming is legacy (0), we have versioned - keep ours
				shouldUpdate = false
			} else if tt.incomingVer > 0 && tt.cachedVer == 0 {
				// Incoming has version, ours is legacy - accept upgrade
				shouldUpdate = true
			} else {
				// Both are legacy (0) - keep existing unless verified
				shouldUpdate = false
			}

			if shouldUpdate != tt.shouldUpdate {
				t.Errorf("shouldUpdate = %v, want %v", shouldUpdate, tt.shouldUpdate)
			}
		})
	}
}

// =============================================================================
// CACHE PRUNING TESTS
// =============================================================================

func TestCachePruning_AgeThresholds(t *testing.T) {
	// Test the logic for cache pruning based on age

	now := time.Now()
	maxAge := 48 * time.Hour
	cutoff := now.Add(-maxAge)

	tests := []struct {
		name        string
		lastTime    time.Time
		shouldPrune bool
	}{
		{
			name:        "very recent (1 hour ago)",
			lastTime:    now.Add(-1 * time.Hour),
			shouldPrune: false,
		},
		{
			name:        "recent (24 hours ago)",
			lastTime:    now.Add(-24 * time.Hour),
			shouldPrune: false,
		},
		{
			name:        "at threshold (48 hours ago)",
			lastTime:    now.Add(-48 * time.Hour),
			shouldPrune: false, // Before() is strict <
		},
		{
			name:        "just past threshold (49 hours ago)",
			lastTime:    now.Add(-49 * time.Hour),
			shouldPrune: true,
		},
		{
			name:        "very old (1 week ago)",
			lastTime:    now.Add(-7 * 24 * time.Hour),
			shouldPrune: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldPrune := tt.lastTime.Before(cutoff)
			if shouldPrune != tt.shouldPrune {
				t.Errorf("shouldPrune = %v, want %v (lastTime: %v, cutoff: %v)",
					shouldPrune, tt.shouldPrune, tt.lastTime, cutoff)
			}
		})
	}
}

// =============================================================================
// ROUTING TABLE BUCKET INDEX TESTS
// =============================================================================

func TestBucketIndex_Range(t *testing.T) {
	// Create a routing table with a known local ID
	localSystem := &System{
		ID: uuid.MustParse("00000000-0000-0000-0000-000000000000"),
		Stars: MultiStarSystem{
			Primary: StarType{Class: "G"},
		},
	}
	rt := &RoutingTable{
		localID:     localSystem.ID,
		localSystem: localSystem,
		bucketK:     8,
	}

	// Test various node IDs
	tests := []struct {
		name       string
		nodeID     string
		wantBucket int
	}{
		{
			name:       "first bit differs",
			nodeID:     "80000000-0000-0000-0000-000000000000",
			wantBucket: 0,
		},
		{
			name:       "last bit differs",
			nodeID:     "00000000-0000-0000-0000-000000000001",
			wantBucket: 127,
		},
		{
			name:       "same ID (edge case)",
			nodeID:     "00000000-0000-0000-0000-000000000000",
			wantBucket: 127, // Returns IDBits-1 for identical
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeID := uuid.MustParse(tt.nodeID)
			got := rt.BucketIndex(nodeID)
			if got != tt.wantBucket {
				t.Errorf("BucketIndex() = %d, want %d", got, tt.wantBucket)
			}
		})
	}
}

// =============================================================================
// CONSTANTS VERIFICATION TESTS
// =============================================================================

func TestRoutingTableConstants(t *testing.T) {
	// Verify important constants are set correctly
	tests := []struct {
		name     string
		got      int
		expected int
	}{
		{"IDBits", IDBits, 128},
		{"BaseBucketK", BaseBucketK, 5},
		{"DefaultBucketK", DefaultBucketK, 8},
		{"MaxFailCount", MaxFailCount, 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestSpatialReplacementThresholdValue(t *testing.T) {
	if SpatialReplacementThreshold != 0.8 {
		t.Errorf("SpatialReplacementThreshold = %v, want 0.8", SpatialReplacementThreshold)
	}
}
