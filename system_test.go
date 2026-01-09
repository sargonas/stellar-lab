package main

import (
	"math"
	"testing"

	"github.com/google/uuid"
)

// =============================================================================
// STAR GENERATION TESTS
// =============================================================================

// TestGenerateSingleStar_Distribution verifies star class distribution
// From system.go comments:
// - M (red dwarf): 40% (roll >= 60000)
// - K (orange dwarf): 25% (roll 35000-59999)
// - G (yellow dwarf): 17.5% (roll 17500-34999)
// - F (yellow-white): 10% (roll 7500-17499)
// - A (white): 5% (roll 2500-7499)
// - B (blue giant): 2% (roll 500-2499)
// - O (blue supergiant): 0.5% (roll 0-499)
func TestGenerateSingleStar_Distribution(t *testing.T) {
	tests := []struct {
		name      string
		seed      uint64
		wantClass string
		reason    string
	}{
		{"O type (seed 0)", 0, "O", "roll=0, 0 < 500 = O type"},
		{"O type (seed 499)", 499, "O", "roll=499, 499 < 500 = O type"},
		{"B type (seed 500)", 500, "B", "roll=500, 500 < 2500 = B type"},
		{"B type (seed 2499)", 2499, "B", "roll=2499, 2499 < 2500 = B type"},
		{"A type (seed 2500)", 2500, "A", "roll=2500, 2500 < 7500 = A type"},
		{"A type (seed 7499)", 7499, "A", "roll=7499, 7499 < 7500 = A type"},
		{"F type (seed 7500)", 7500, "F", "roll=7500, 7500 < 17500 = F type"},
		{"F type (seed 17499)", 17499, "F", "roll=17499, 17499 < 17500 = F type"},
		{"G type (seed 17500)", 17500, "G", "roll=17500, 17500 < 35000 = G type"},
		{"G type (seed 34999)", 34999, "G", "roll=34999, 34999 < 35000 = G type"},
		{"K type (seed 35000)", 35000, "K", "roll=35000, 35000 < 60000 = K type"},
		{"K type (seed 59999)", 59999, "K", "roll=59999, 59999 < 60000 = K type"},
		{"M type (seed 60000)", 60000, "M", "roll=60000, 60000 >= 60000 = M type"},
		{"M type (seed 99999)", 99999, "M", "roll=99999, default = M type"},
		{"M type wraps at 100000", 100000, "O", "roll=100000%100000=0 = O type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			star := generateSingleStar(tt.seed)
			if star.Class != tt.wantClass {
				t.Errorf("generateSingleStar(%d) class = %s, want %s\nReason: %s",
					tt.seed, star.Class, tt.wantClass, tt.reason)
			}
		})
	}
}

// TestGenerateSingleStar_Properties verifies star properties are set correctly
func TestGenerateSingleStar_Properties(t *testing.T) {
	// Test each star class has correct properties
	starClasses := []struct {
		class       string
		seed        uint64
		description string
		minTemp     int
		maxTemp     int
	}{
		{"O", 0, "Blue Supergiant", 30000, 50000},
		{"B", 500, "Blue Giant", 10000, 20000},
		{"A", 2500, "White Star", 7500, 10000},
		{"F", 7500, "Yellow-White Star", 6000, 7500},
		{"G", 17500, "Yellow Dwarf", 5200, 6000},
		{"K", 35000, "Orange Dwarf", 3700, 5200},
		{"M", 60000, "Red Dwarf", 2400, 3700},
	}

	for _, tc := range starClasses {
		t.Run(tc.class+" type properties", func(t *testing.T) {
			star := generateSingleStar(tc.seed)

			if star.Class != tc.class {
				t.Errorf("Expected class %s, got %s", tc.class, star.Class)
			}

			if star.Description != tc.description {
				t.Errorf("Expected description %s, got %s", tc.description, star.Description)
			}

			if star.Temperature < tc.minTemp || star.Temperature > tc.maxTemp {
				t.Errorf("Temperature %d outside expected range [%d, %d]",
					star.Temperature, tc.minTemp, tc.maxTemp)
			}

			if star.Color == "" {
				t.Error("Color should not be empty")
			}

			if star.Luminosity <= 0 {
				t.Error("Luminosity should be positive")
			}
		})
	}
}

// TestGenerateSingleStar_Deterministic verifies same seed = same star
func TestGenerateSingleStar_Deterministic(t *testing.T) {
	seed := uint64(12345)

	star1 := generateSingleStar(seed)
	star2 := generateSingleStar(seed)

	if star1.Class != star2.Class {
		t.Error("Same seed should produce same star class")
	}
	if star1.Temperature != star2.Temperature {
		t.Error("Same seed should produce same temperature")
	}
	if star1.Luminosity != star2.Luminosity {
		t.Error("Same seed should produce same luminosity")
	}
}

// =============================================================================
// MULTI-STAR SYSTEM TESTS
// =============================================================================

// TestGenerateMultiStarSystem_Distribution verifies system type distribution
// From system.go:
// - Single: 50% (roll 0-49)
// - Binary: 40% (roll 50-89)
// - Trinary: 10% (roll 90-99)
func TestGenerateMultiStarSystem_Deterministic(t *testing.T) {
	// Create a system with a known UUID
	sys := &System{ID: uuid.MustParse("12345678-1234-1234-1234-123456789012")}
	sys.GenerateMultiStarSystem()

	// Generate again with same UUID
	sys2 := &System{ID: uuid.MustParse("12345678-1234-1234-1234-123456789012")}
	sys2.GenerateMultiStarSystem()

	if sys.Stars.Primary.Class != sys2.Stars.Primary.Class {
		t.Error("Same UUID should produce same primary star class")
	}
	if sys.Stars.IsBinary != sys2.Stars.IsBinary {
		t.Error("Same UUID should produce same binary status")
	}
	if sys.Stars.IsTrinary != sys2.Stars.IsTrinary {
		t.Error("Same UUID should produce same trinary status")
	}
	if sys.Stars.Count != sys2.Stars.Count {
		t.Error("Same UUID should produce same star count")
	}
}

func TestGenerateMultiStarSystem_StarCount(t *testing.T) {
	// Test multiple UUIDs to verify star count is consistent with flags
	for i := 0; i < 100; i++ {
		sys := &System{ID: uuid.New()}
		sys.GenerateMultiStarSystem()

		// Verify count matches flags
		if sys.Stars.IsTrinary {
			if sys.Stars.Count != 3 {
				t.Errorf("Trinary system should have count=3, got %d", sys.Stars.Count)
			}
			if sys.Stars.Secondary == nil || sys.Stars.Tertiary == nil {
				t.Error("Trinary system should have secondary and tertiary stars")
			}
		} else if sys.Stars.IsBinary {
			if sys.Stars.Count != 2 {
				t.Errorf("Binary system should have count=2, got %d", sys.Stars.Count)
			}
			if sys.Stars.Secondary == nil {
				t.Error("Binary system should have secondary star")
			}
			if sys.Stars.Tertiary != nil {
				t.Error("Binary system should not have tertiary star")
			}
		} else {
			if sys.Stars.Count != 1 {
				t.Errorf("Single system should have count=1, got %d", sys.Stars.Count)
			}
			if sys.Stars.Secondary != nil || sys.Stars.Tertiary != nil {
				t.Error("Single system should not have companion stars")
			}
		}

		// Primary star should always exist
		if sys.Stars.Primary.Class == "" {
			t.Error("Primary star class should not be empty")
		}
	}
}

// =============================================================================
// COORDINATE GENERATION TESTS
// =============================================================================

// TestGenerateDeterministicCoordinates verifies coordinate determinism
func TestGenerateDeterministicCoordinates(t *testing.T) {
	id := uuid.MustParse("12345678-1234-1234-1234-123456789012")

	sys1 := &System{ID: id}
	sys1.GenerateDeterministicCoordinates()

	sys2 := &System{ID: id}
	sys2.GenerateDeterministicCoordinates()

	if sys1.X != sys2.X || sys1.Y != sys2.Y || sys1.Z != sys2.Z {
		t.Error("Same UUID should produce same coordinates")
	}
}

// TestGenerateDeterministicCoordinates_Range verifies coordinates are in expected range
func TestGenerateDeterministicCoordinates_Range(t *testing.T) {
	// Test multiple UUIDs
	for i := 0; i < 100; i++ {
		sys := &System{ID: uuid.New()}
		sys.GenerateDeterministicCoordinates()

		// Range should be -10000 to +10000
		if sys.X < -10000 || sys.X > 10000 {
			t.Errorf("X coordinate %f outside [-10000, 10000]", sys.X)
		}
		if sys.Y < -10000 || sys.Y > 10000 {
			t.Errorf("Y coordinate %f outside [-10000, 10000]", sys.Y)
		}
		if sys.Z < -10000 || sys.Z > 10000 {
			t.Errorf("Z coordinate %f outside [-10000, 10000]", sys.Z)
		}
	}
}

// TestGenerateClusteredCoordinates_Distance verifies clustered coords are near sponsor
func TestGenerateClusteredCoordinates_Distance(t *testing.T) {
	sponsor := &System{
		ID: uuid.New(),
		X:  1000,
		Y:  2000,
		Z:  3000,
	}

	// Test multiple systems
	for i := 0; i < 50; i++ {
		sys := &System{ID: uuid.New()}
		sys.GenerateClusteredCoordinates(sponsor)

		// Distance should be 100-500 units from sponsor
		dx := sys.X - sponsor.X
		dy := sys.Y - sponsor.Y
		dz := sys.Z - sponsor.Z
		distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

		if distance < 100 || distance > 500 {
			t.Errorf("Distance %f should be in range [100, 500]", distance)
		}

		// Sponsor ID should be set
		if sys.SponsorID == nil || *sys.SponsorID != sponsor.ID {
			t.Error("SponsorID should be set to sponsor's ID")
		}
	}
}

// TestGenerateClusteredCoordinates_Deterministic verifies same UUID+sponsor = same coords
func TestGenerateClusteredCoordinates_Deterministic(t *testing.T) {
	sponsor := &System{ID: uuid.New(), X: 100, Y: 200, Z: 300}
	sysID := uuid.New()

	sys1 := &System{ID: sysID}
	sys1.GenerateClusteredCoordinates(sponsor)

	sys2 := &System{ID: sysID}
	sys2.GenerateClusteredCoordinates(sponsor)

	if sys1.X != sys2.X || sys1.Y != sys2.Y || sys1.Z != sys2.Z {
		t.Error("Same UUID+sponsor should produce same coordinates")
	}
}

// =============================================================================
// COORDINATE VALIDATION TESTS
// =============================================================================

// TestCalculateExpectedCoordinates verifies expected coords match generated
func TestCalculateExpectedCoordinates(t *testing.T) {
	sponsor := &System{ID: uuid.New(), X: 500, Y: 600, Z: 700}
	sysID := uuid.New()

	// Generate coordinates
	sys := &System{ID: sysID}
	sys.GenerateClusteredCoordinates(sponsor)

	// Calculate expected
	expX, expY, expZ := CalculateExpectedCoordinates(sysID, sponsor.ID, sponsor.X, sponsor.Y, sponsor.Z)

	// Should match exactly
	if math.Abs(sys.X-expX) > 0.0001 || math.Abs(sys.Y-expY) > 0.0001 || math.Abs(sys.Z-expZ) > 0.0001 {
		t.Errorf("Generated coords (%f, %f, %f) don't match expected (%f, %f, %f)",
			sys.X, sys.Y, sys.Z, expX, expY, expZ)
	}
}

// TestCoordsApproxEqual verifies approximate equality check
func TestCoordsApproxEqual(t *testing.T) {
	tests := []struct {
		a, b float64
		want bool
	}{
		{0, 0, true},
		{1.0, 1.0, true},
		{1.0, 1.005, true},  // Within epsilon (0.01)
		{1.0, 1.009, true},  // Within epsilon
		{1.0, 1.02, false},  // Outside epsilon
		{-5.0, -5.005, true},
		{-5.0, -5.02, false},
	}

	for _, tt := range tests {
		result := coordsApproxEqual(tt.a, tt.b)
		if result != tt.want {
			t.Errorf("coordsApproxEqual(%f, %f) = %v, want %v", tt.a, tt.b, result, tt.want)
		}
	}
}

// =============================================================================
// DISTANCE CALCULATION TESTS
// =============================================================================

// TestSystem_DistanceTo verifies distance calculation matches Euclidean formula
func TestSystem_DistanceTo(t *testing.T) {
	tests := []struct {
		name string
		s1   *System
		s2   *System
		want float64
	}{
		{
			name: "same point",
			s1:   &System{X: 0, Y: 0, Z: 0},
			s2:   &System{X: 0, Y: 0, Z: 0},
			want: 0,
		},
		{
			name: "unit X",
			s1:   &System{X: 0, Y: 0, Z: 0},
			s2:   &System{X: 1, Y: 0, Z: 0},
			want: 1,
		},
		{
			name: "3-4-5 triangle",
			s1:   &System{X: 0, Y: 0, Z: 0},
			s2:   &System{X: 3, Y: 4, Z: 0},
			want: 5,
		},
		{
			name: "unit cube diagonal",
			s1:   &System{X: 0, Y: 0, Z: 0},
			s2:   &System{X: 1, Y: 1, Z: 1},
			want: math.Sqrt(3),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.s1.DistanceTo(tt.s2)
			if math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("DistanceTo() = %f, want %f", got, tt.want)
			}
		})
	}
}

// =============================================================================
// MAX PEERS TESTS
// =============================================================================

// TestGetMaxPeers verifies peer limits by star class
// From system.go: X=20, O=18, B=16, A=15, F=14, G=12, K=11, M=10
// Binary: +3, Trinary: +5
func TestGetMaxPeers(t *testing.T) {
	tests := []struct {
		name     string
		class    string
		isBinary bool
		isTrinary bool
		want     int
	}{
		{"X (genesis)", "X", false, false, 20},
		{"O single", "O", false, false, 18},
		{"B single", "B", false, false, 16},
		{"A single", "A", false, false, 15},
		{"F single", "F", false, false, 14},
		{"G single", "G", false, false, 12},
		{"K single", "K", false, false, 11},
		{"M single", "M", false, false, 10},
		{"M binary", "M", true, false, 13},  // 10 + 3
		{"G binary", "G", true, false, 15},  // 12 + 3
		{"M trinary", "M", false, true, 15}, // 10 + 5
		{"G trinary", "G", false, true, 17}, // 12 + 5
		{"empty class", "", false, false, 10}, // Default fallback
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sys := &System{
				Stars: MultiStarSystem{
					Primary:   StarType{Class: tt.class},
					IsBinary:  tt.isBinary,
					IsTrinary: tt.isTrinary,
				},
			}
			got := sys.GetMaxPeers()
			if got != tt.want {
				t.Errorf("GetMaxPeers() = %d, want %d", got, tt.want)
			}
		})
	}
}

// =============================================================================
// DETERMINISTIC SEED TESTS
// =============================================================================

// TestDeterministicSeed verifies seed generation is deterministic
func TestDeterministicSeed(t *testing.T) {
	sys := &System{ID: uuid.MustParse("12345678-1234-1234-1234-123456789012")}

	seed1 := sys.DeterministicSeed("test_salt")
	seed2 := sys.DeterministicSeed("test_salt")

	if seed1 != seed2 {
		t.Error("Same UUID + salt should produce same seed")
	}

	// Different salts should produce different seeds
	seed3 := sys.DeterministicSeed("different_salt")
	if seed1 == seed3 {
		t.Error("Different salts should produce different seeds")
	}
}

// TestDeterministicSeed_DifferentUUIDs verifies different UUIDs produce different seeds
func TestDeterministicSeed_DifferentUUIDs(t *testing.T) {
	sys1 := &System{ID: uuid.New()}
	sys2 := &System{ID: uuid.New()}

	seed1 := sys1.DeterministicSeed("same_salt")
	seed2 := sys2.DeterministicSeed("same_salt")

	if seed1 == seed2 {
		t.Error("Different UUIDs should produce different seeds")
	}
}

// =============================================================================
// BUG DETECTION TESTS - Verify tests would catch common bugs
// =============================================================================

// TestGetMaxPeers_WouldCatchBugs verifies our tests would detect incorrect implementations
func TestGetMaxPeers_WouldCatchBugs(t *testing.T) {
	// Bug: If someone set all star classes to same peer count
	oStar := &System{Stars: MultiStarSystem{Primary: StarType{Class: "O"}}}
	mStar := &System{Stars: MultiStarSystem{Primary: StarType{Class: "M"}}}

	if oStar.GetMaxPeers() == mStar.GetMaxPeers() {
		t.Error("BUG: O-type stars should have more peers than M-type")
	}

	// Bug: If binary bonus wasn't applied
	single := &System{Stars: MultiStarSystem{Primary: StarType{Class: "G"}, IsBinary: false}}
	binary := &System{Stars: MultiStarSystem{Primary: StarType{Class: "G"}, IsBinary: true}}

	if binary.GetMaxPeers() <= single.GetMaxPeers() {
		t.Error("BUG: Binary systems should have more peers than single")
	}

	// Bug: If trinary bonus was same as binary
	trinary := &System{Stars: MultiStarSystem{Primary: StarType{Class: "G"}, IsTrinary: true}}

	if trinary.GetMaxPeers() <= binary.GetMaxPeers() {
		t.Error("BUG: Trinary systems should have more peers than binary")
	}
}

// TestStarDistribution_WouldCatchBugs verifies distribution tests catch errors
func TestStarDistribution_WouldCatchBugs(t *testing.T) {
	// Bug: If the modulo was wrong (e.g., % 1000 instead of % 100000)
	// Seed 500 should be B-type, not O-type
	star := generateSingleStar(500)
	if star.Class == "O" {
		t.Error("BUG: Seed 500 should be B-type (roll=500, 500 >= 500 threshold)")
	}

	// Bug: If thresholds were off by one
	// Seed 499 should be O-type (last valid O)
	star = generateSingleStar(499)
	if star.Class != "O" {
		t.Errorf("BUG: Seed 499 should be O-type, got %s", star.Class)
	}

	// Bug: If M-type threshold was wrong
	// Seed 60000 should be M-type (first valid M)
	star = generateSingleStar(60000)
	if star.Class != "M" {
		t.Errorf("BUG: Seed 60000 should be M-type, got %s", star.Class)
	}
}

// TestCoordinateRange_WouldCatchBugs verifies coordinate tests catch errors
func TestCoordinateRange_WouldCatchBugs(t *testing.T) {
	// Generate many systems and verify ALL are in range
	// Would catch bugs like wrong divisor or missing offset
	for i := 0; i < 1000; i++ {
		sys := &System{ID: uuid.New()}
		sys.GenerateDeterministicCoordinates()

		if sys.X < -10000 || sys.X > 10000 ||
			sys.Y < -10000 || sys.Y > 10000 ||
			sys.Z < -10000 || sys.Z > 10000 {
			t.Errorf("BUG: Coordinates out of range: (%f, %f, %f)", sys.X, sys.Y, sys.Z)
		}
	}
}

// TestClusteredDistance_WouldCatchBugs verifies distance tests catch errors
func TestClusteredDistance_WouldCatchBugs(t *testing.T) {
	sponsor := &System{ID: uuid.New(), X: 0, Y: 0, Z: 0}

	// Generate many systems and verify ALL are in distance range
	for i := 0; i < 100; i++ {
		sys := &System{ID: uuid.New()}
		sys.GenerateClusteredCoordinates(sponsor)

		dist := math.Sqrt(sys.X*sys.X + sys.Y*sys.Y + sys.Z*sys.Z)

		if dist < 100 || dist > 500 {
			t.Errorf("BUG: Distance %f outside [100, 500]", dist)
		}
	}
}
