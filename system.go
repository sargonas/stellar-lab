package main

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"time"

	"github.com/google/uuid"
)

// StarType represents the classification of a star
type StarType struct {
	Class       string  `json:"class"`        // O, B, A, F, G, K, M, X (black hole)
	Description string  `json:"description"`  // Human readable description
	Color       string  `json:"color"`        // Visual color (hex)
	Temperature int     `json:"temperature"`  // Kelvin
	Luminosity  float64 `json:"luminosity"`   // Relative to Sol
}

// MultiStarSystem represents the stellar composition of a system
type MultiStarSystem struct {
	Primary   StarType   `json:"primary"`             // Primary star
	Secondary *StarType  `json:"secondary,omitempty"` // Secondary star (if binary)
	Tertiary  *StarType  `json:"tertiary,omitempty"`  // Tertiary star (if trinary)
	IsBinary  bool       `json:"is_binary"`           // Is this a binary system?
	IsTrinary bool       `json:"is_trinary"`          // Is this a trinary system?
	Count     int        `json:"count"`               // Total number of stars (1, 2, or 3)
}

// System represents a star system node in the network
type System struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	X           float64         `json:"x"`
	Y           float64         `json:"y"`
	Z           float64         `json:"z"`
	Stars       MultiStarSystem `json:"stars"`
	CreatedAt   time.Time       `json:"created_at"`
	LastSeenAt  time.Time       `json:"last_seen_at"`
	Address     string          `json:"address"`      // network address (host:port)
	Keys        *KeyPair        `json:"-"`            // Cryptographic keys (never serialized)
	PeerAddress string          `json:"peer_address"` // Peer mesh address
	SponsorID   *uuid.UUID      `json:"sponsor_id,omitempty"` // Node that sponsored our network entry
}

// generateSingleStar creates a deterministic star from a seed
// Distribution roughly matches real galaxy: M (76%), K (12%), G (8%), F (3%), A (0.6%), B (0.13%), O (0.00003%)
func generateSingleStar(seed uint64) StarType {
	roll := seed % 100000

	var class string
	var desc string
	var color string
	var temp int
	var lum float64

	// Distribution adjusted for small network variety (few thousand nodes)
	// More rare stars visible while keeping red dwarfs most common
	switch {
	case roll < 500: // 0.5% - O type
		class = "O"
		desc = "Blue Supergiant"
		color = "#6b8cff"
		temp = 30000 + int(seed%20000)
		lum = 30000.0 + float64(seed%20000)
	case roll < 2500: // 2% - B type
		class = "B"
		desc = "Blue Giant"
		color = "#8eb4f0"
		temp = 10000 + int(seed%10000)
		lum = 25.0 + float64(seed%1000)
	case roll < 7500: // 5% - A type
		class = "A"
		desc = "White Star"
		color = "#e8e8ff"
		temp = 7500 + int(seed%2500)
		lum = 5.0 + float64(seed%20)
	case roll < 17500: // 10% - F type
		class = "F"
		desc = "Yellow-White Star"
		color = "#fffde8"
		temp = 6000 + int(seed%1500)
		lum = 1.5 + float64(seed%10)/10.0
	case roll < 35000: // 17.5% - G type (like our Sun)
		class = "G"
		desc = "Yellow Dwarf"
		color = "#ffeb3b"
		temp = 5200 + int(seed%800)
		lum = 0.6 + float64(seed%10)/10.0
	case roll < 60000: // 25% - K type
		class = "K"
		desc = "Orange Dwarf"
		color = "#ff9800"
		temp = 3700 + int(seed%1500)
		lum = 0.08 + float64(seed%50)/100.0
	default: // 40% - M type
		class = "M"
		desc = "Red Dwarf"
		color = "#e85d4c"
		temp = 2400 + int(seed%1300)
		lum = 0.001 + float64(seed%80)/1000.0
	}

	return StarType{
		Class:       class,
		Description: desc,
		Color:       color,
		Temperature: temp,
		Luminosity:  lum,
	}
}

// GenerateMultiStarSystem creates a deterministic multi-star system from UUID
// Distribution:
// - Single stars: ~50%
// - Binary systems: ~40%
// - Trinary systems: ~10%
func (s *System) GenerateMultiStarSystem() {
	// Determine if single, binary, or trinary
	systemTypeSeed := s.DeterministicSeed("system_type")
	systemTypeRoll := systemTypeSeed % 100

	// Generate primary star
	primarySeed := s.DeterministicSeed("primary_star")
	primary := generateSingleStar(primarySeed)

	var secondary *StarType
	var tertiary *StarType
	isBinary := false
	isTrinary := false
	count := 1

	if systemTypeRoll < 50 { // 50% - Single star
		// Just the primary
		count = 1
	} else if systemTypeRoll < 90 { // 40% - Binary system
		isBinary = true
		count = 2

		// Secondary star is usually smaller than primary
		// Use a modified seed to ensure different star type
		secondarySeed := s.DeterministicSeed("secondary_star")
		secondaryStar := generateSingleStar(secondarySeed)

		// Secondary stars are typically lower mass - shift distribution
		// If we got a large star, re-roll with bias toward smaller classes
		if secondaryStar.Class == "O" || secondaryStar.Class == "B" || secondaryStar.Class == "A" {
			// Force to smaller star classes for realism
			secondarySeed = secondarySeed ^ 0xFFFFFFFF // XOR to change seed
			secondaryStar = generateSingleStar(secondarySeed + 50000) // Offset pushes toward M/K
		}

		secondary = &secondaryStar
	} else { // 10% - Trinary system
		isTrinary = true
		count = 3

		// Generate secondary
		secondarySeed := s.DeterministicSeed("secondary_star")
		secondaryStar := generateSingleStar(secondarySeed)

		// Generate tertiary (usually smallest)
		tertiarySeed := s.DeterministicSeed("tertiary_star")
		tertiaryStar := generateSingleStar(tertiarySeed + 70000) // Offset toward smaller stars

		// Bias both toward smaller classes
		if secondaryStar.Class == "O" || secondaryStar.Class == "B" {
			secondaryStar = generateSingleStar(secondarySeed + 50000)
		}
		if tertiaryStar.Class == "O" || tertiaryStar.Class == "B" || tertiaryStar.Class == "A" {
			tertiaryStar = generateSingleStar(tertiarySeed + 80000)
		}

		secondary = &secondaryStar
		tertiary = &tertiaryStar
	}

	s.Stars = MultiStarSystem{
		Primary:   primary,
		Secondary: secondary,
		Tertiary:  tertiary,
		IsBinary:  isBinary,
		IsTrinary: isTrinary,
		Count:     count,
	}
}

// ValidateStarSystem checks if a system's stars match what its UUID should deterministically produce
// Returns true if valid, false if the star configuration appears to be spoofed
func ValidateStarSystem(sys *System) bool {
	// Skip validation for class X (genesis black hole - special case)
	if sys.Stars.Primary.Class == "X" {
		// In isolated mode, allow any Class X (for dev/test networks)
		if isolatedMode != nil && *isolatedMode {
			return true
		}
		// In production, only the real genesis UUID is allowed to have class X
		return sys.ID.String() == "f467e75d-00b8-5ac7-9f0f-4e7cd1c8eb20"
	}

	// Generate expected star configuration from UUID
	expected := &System{ID: sys.ID}
	expected.GenerateMultiStarSystem()

	// Compare primary class and multi-star status
	return sys.Stars.Primary.Class == expected.Stars.Primary.Class &&
		sys.Stars.IsBinary == expected.Stars.IsBinary &&
		sys.Stars.IsTrinary == expected.Stars.IsTrinary
}

// GenerateCoordinates creates spatial coordinates
// If nearbySystem is provided, clusters near it. Otherwise uses deterministic placement.
func (s *System) GenerateCoordinates(nearbySystem *System) {
	if nearbySystem != nil {
		// Cluster near the provided system
		s.GenerateClusteredCoordinates(nearbySystem)
	} else {
		// Fall back to deterministic coordinates for bootstrap/first nodes
		s.GenerateDeterministicCoordinates()
	}
}

// GenerateDeterministicCoordinates creates deterministic spatial coordinates from UUID
// Used for bootstrap nodes or when no nearby system is available
// Range: -10000 to +10000 for each axis
func (s *System) GenerateDeterministicCoordinates() {
	hash := sha256.Sum256(s.ID[:])

	// Use different parts of the hash for each coordinate
	xSeed := binary.BigEndian.Uint64(hash[0:8])
	ySeed := binary.BigEndian.Uint64(hash[8:16])
	zSeed := binary.BigEndian.Uint64(hash[16:24])

	// Normalize to -10000 to +10000 range
	maxUint64 := float64(math.MaxUint64)
	s.X = (float64(xSeed) / maxUint64 * 20000) - 10000
	s.Y = (float64(ySeed) / maxUint64 * 20000) - 10000
	s.Z = (float64(zSeed) / maxUint64 * 20000) - 10000
}

// GenerateClusteredCoordinates places this system near its sponsor
// Uses Hash(UUID + SponsorID) to deterministically generate offset
// This makes coordinates verifiable by any node that knows both UUIDs
func (s *System) GenerateClusteredCoordinates(sponsor *System) {
	// Hash our UUID + sponsor's UUID together for deterministic positioning
	combined := append(s.ID[:], sponsor.ID[:]...)
	hash := sha256.Sum256(combined)

	// Use hash to generate deterministic spherical coordinates
	// This gives more natural distribution than axis-aligned offsets
	distSeed := binary.BigEndian.Uint64(hash[0:8])
	thetaSeed := binary.BigEndian.Uint64(hash[8:16])  // azimuth angle
	phiSeed := binary.BigEndian.Uint64(hash[16:24])   // polar angle

	maxUint64 := float64(math.MaxUint64)

	// Distance: 100-500 units from sponsor
	distance := (float64(distSeed)/maxUint64)*400.0 + 100.0

	// Spherical coordinates for even distribution
	theta := (float64(thetaSeed) / maxUint64) * 2 * math.Pi        // 0 to 2π
	phi := math.Acos(2*(float64(phiSeed)/maxUint64) - 1)           // 0 to π (uniform on sphere)

	// Convert to Cartesian offsets
	xOffset := distance * math.Sin(phi) * math.Cos(theta)
	yOffset := distance * math.Sin(phi) * math.Sin(theta)
	zOffset := distance * math.Cos(phi)

	// Apply offsets to sponsor's coordinates
	s.X = sponsor.X + xOffset
	s.Y = sponsor.Y + yOffset
	s.Z = sponsor.Z + zOffset

	// Store sponsor reference
	s.SponsorID = &sponsor.ID
}

// CalculateExpectedCoordinates computes where a system should be based on UUID + Sponsor
// Used for validation - any node can verify coordinates are legitimate
func CalculateExpectedCoordinates(systemID, sponsorID uuid.UUID, sponsorX, sponsorY, sponsorZ float64) (x, y, z float64) {
	combined := append(systemID[:], sponsorID[:]...)
	hash := sha256.Sum256(combined)

	distSeed := binary.BigEndian.Uint64(hash[0:8])
	thetaSeed := binary.BigEndian.Uint64(hash[8:16])
	phiSeed := binary.BigEndian.Uint64(hash[16:24])

	maxUint64 := float64(math.MaxUint64)

	distance := (float64(distSeed)/maxUint64)*400.0 + 100.0
	theta := (float64(thetaSeed) / maxUint64) * 2 * math.Pi
	phi := math.Acos(2*(float64(phiSeed)/maxUint64) - 1)

	xOffset := distance * math.Sin(phi) * math.Cos(theta)
	yOffset := distance * math.Sin(phi) * math.Sin(theta)
	zOffset := distance * math.Cos(phi)

	return sponsorX + xOffset, sponsorY + yOffset, sponsorZ + zOffset
}

// ValidateCoordinates checks if a system's coordinates match expected position
// Returns true if valid (or unverifiable), false if definitely spoofed
// lookupSponsor returns sponsor System or nil if unknown
func ValidateCoordinates(sys *System, lookupSponsor func(uuid.UUID) *System) bool {
	// No sponsor = must be genesis
	if sys.SponsorID == nil {
		// Class X (genesis) is allowed without a sponsor if at origin
		if sys.Stars.Primary.Class == "X" {
			// In isolated mode, any Class X at origin is valid
			if isolatedMode != nil && *isolatedMode {
				return sys.X == 0 && sys.Y == 0 && sys.Z == 0
			}
			// In production, only the real genesis UUID at origin is valid
			return sys.X == 0 && sys.Y == 0 && sys.Z == 0
		}
		// All other nodes must have a sponsor
		return false
	}

	// Has sponsor - look them up
	sponsor := lookupSponsor(*sys.SponsorID)
	if sponsor == nil {
		// Can't verify - sponsor unknown to us
		// Accept for now (lenient) - we'll learn sponsor eventually
		return true
	}

	// Calculate expected position
	expX, expY, expZ := CalculateExpectedCoordinates(sys.ID, *sys.SponsorID, sponsor.X, sponsor.Y, sponsor.Z)

	return coordsApproxEqual(sys.X, expX) &&
		coordsApproxEqual(sys.Y, expY) &&
		coordsApproxEqual(sys.Z, expZ)
}

// coordsApproxEqual checks if two coordinates are approximately equal
// Allows for small floating point differences
func coordsApproxEqual(a, b float64) bool {
	const epsilon = 0.01 // Allow tiny rounding differences
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

// DistanceTo calculates Euclidean distance to another system
func (s *System) DistanceTo(other *System) float64 {
	dx := s.X - other.X
	dy := s.Y - other.Y
	dz := s.Z - other.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// DeterministicSeed returns a seed value derived from UUID for future feature generation
// This allows any system property to be deterministically generated from the UUID
func (s *System) DeterministicSeed(salt string) uint64 {
	data := append(s.ID[:], []byte(salt)...)
	hash := sha256.Sum256(data)
	return binary.BigEndian.Uint64(hash[0:8])
}

// Peer represents a known neighboring system
type Peer struct {
	SystemID   uuid.UUID `json:"system_id"`
	Address    string    `json:"address"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

type DiscoverySystem struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	X                  float64 `json:"x"`
	Y                  float64 `json:"y"`
	Z                  float64 `json:"z"`
	PeerAddress        string  `json:"peer_address"`
	DistanceFromOrigin float64 `json:"distance_from_origin"`
	CurrentPeers       int     `json:"current_peers"`
	MaxPeers           int     `json:"max_peers"`
	HasCapacity        bool    `json:"has_capacity"`
}

// GetMaxPeers returns the maximum peer connections based on star configuration
// Larger/rarer star systems can maintain more connections, acting as network hubs
// Note: This affects topology only, not attestation rate (which is capped)
func (s *System) GetMaxPeers() int {
	if s.Stars.Primary.Class == "" {
		return 10 // Default fallback
	}

	// Base peers by primary star class (10-20 range)
	basePeers := 10
	switch s.Stars.Primary.Class {
	case "X": // Supermassive Black Hole - galactic core hub
		return 20
	case "O":
		basePeers = 18
	case "B":
		basePeers = 16
	case "A":
		basePeers = 15
	case "F":
		basePeers = 14
	case "G":
		basePeers = 12
	case "K":
		basePeers = 11
	case "M":
		basePeers = 10
	}

	// Bonus for multi-star systems
	if s.Stars.IsTrinary {
		basePeers += 5
	} else if s.Stars.IsBinary {
		basePeers += 3
	}

	return basePeers
}
