package main

import (
	"testing"

	"github.com/google/uuid"
)

// =============================================================================
// HARDWARE IDENTIFIER TESTS
// =============================================================================

func TestGetHardwareID(t *testing.T) {
	hwid, err := GetHardwareID()

	if err != nil {
		t.Fatalf("GetHardwareID() error = %v", err)
	}

	if hwid == nil {
		t.Fatal("GetHardwareID() returned nil")
	}

	// CombinedHash should always be set
	if hwid.CombinedHash == "" {
		t.Error("CombinedHash should not be empty")
	}

	// Hash should be 16 hex characters (8 bytes)
	if len(hwid.CombinedHash) != 16 {
		t.Errorf("CombinedHash length = %d, expected 16", len(hwid.CombinedHash))
	}
}

func TestGetHardwareID_Consistency(t *testing.T) {
	hwid1, _ := GetHardwareID()
	hwid2, _ := GetHardwareID()

	// Same machine should return same hash
	if hwid1.CombinedHash != hwid2.CombinedHash {
		t.Error("Same machine should return consistent CombinedHash")
	}

	if hwid1.Hostname != hwid2.Hostname {
		t.Error("Same machine should return consistent Hostname")
	}
}

// =============================================================================
// SEMI-DETERMINISTIC UUID TESTS
// =============================================================================

func TestGenerateSemiDeterministicUUID_Deterministic(t *testing.T) {
	seed := "test_seed_123"

	uuid1, err1 := GenerateSemiDeterministicUUID(seed)
	uuid2, err2 := GenerateSemiDeterministicUUID(seed)

	if err1 != nil || err2 != nil {
		t.Fatalf("GenerateSemiDeterministicUUID() errors: %v, %v", err1, err2)
	}

	// Same seed on same machine should produce same UUID
	if uuid1 != uuid2 {
		t.Errorf("Same seed should produce same UUID: %s != %s", uuid1, uuid2)
	}
}

func TestGenerateSemiDeterministicUUID_DifferentSeeds(t *testing.T) {
	uuid1, _ := GenerateSemiDeterministicUUID("seed_one")
	uuid2, _ := GenerateSemiDeterministicUUID("seed_two")

	if uuid1 == uuid2 {
		t.Error("Different seeds should produce different UUIDs")
	}
}

func TestGenerateSemiDeterministicUUID_ValidUUID(t *testing.T) {
	result, err := GenerateSemiDeterministicUUID("test")

	if err != nil {
		t.Fatalf("GenerateSemiDeterministicUUID() error = %v", err)
	}

	// Should be a valid UUID
	if result == uuid.Nil {
		t.Error("Generated UUID should not be Nil")
	}

	// Check version bits (should be version 5)
	version := result[6] >> 4
	if version != 5 {
		t.Errorf("UUID version = %d, expected 5", version)
	}

	// Check variant bits (should be RFC 4122)
	variant := result[8] >> 6
	if variant != 2 {
		t.Errorf("UUID variant = %d, expected 2 (RFC 4122)", variant)
	}
}

func TestGenerateSemiDeterministicUUID_EmptySeed(t *testing.T) {
	uuid1, _ := GenerateSemiDeterministicUUID("")
	uuid2, _ := GenerateSemiDeterministicUUID("")

	// Empty seed should still be deterministic
	if uuid1 != uuid2 {
		t.Error("Empty seed should still be deterministic")
	}
}

// =============================================================================
// RANDOM UUID TESTS
// =============================================================================

func TestGenerateRandomUUID(t *testing.T) {
	uuid1 := GenerateRandomUUID()
	uuid2 := GenerateRandomUUID()

	if uuid1 == uuid.Nil {
		t.Error("Generated UUID should not be Nil")
	}

	if uuid1 == uuid2 {
		t.Error("Random UUIDs should be different")
	}
}

func TestGenerateRandomUUID_ValidFormat(t *testing.T) {
	result := GenerateRandomUUID()

	// UUID should have correct format
	str := result.String()
	if len(str) != 36 {
		t.Errorf("UUID string length = %d, expected 36", len(str))
	}

	// Should have dashes in correct positions
	if str[8] != '-' || str[13] != '-' || str[18] != '-' || str[23] != '-' {
		t.Errorf("UUID format incorrect: %s", str)
	}
}

// =============================================================================
// HARDWARE FINGERPRINT TESTS
// =============================================================================

func TestGetHardwareFingerprint(t *testing.T) {
	fingerprint := GetHardwareFingerprint()

	if fingerprint == "" {
		t.Error("Fingerprint should not be empty")
	}

	// Should be consistent
	fingerprint2 := GetHardwareFingerprint()
	if fingerprint != fingerprint2 {
		t.Error("Fingerprint should be consistent")
	}
}

func TestGetHardwareFingerprint_MatchesHWID(t *testing.T) {
	fingerprint := GetHardwareFingerprint()
	hwid, _ := GetHardwareID()

	// Fingerprint should match CombinedHash from HWID
	if fingerprint != hwid.CombinedHash && fingerprint != "unknown" {
		t.Errorf("Fingerprint %s doesn't match CombinedHash %s", fingerprint, hwid.CombinedHash)
	}
}

// =============================================================================
// HARDWARE IDENTIFIER STRUCTURE TESTS
// =============================================================================

func TestHardwareIdentifier_Fields(t *testing.T) {
	hwid, _ := GetHardwareID()

	// Hostname should typically be set (unless running in unusual environment)
	// We don't enforce this as it may fail in some containers
	if hwid.Hostname == "" {
		t.Log("Warning: Hostname is empty (may be expected in some environments)")
	}

	// MachineID may or may not be set depending on OS
	// On Linux, it should typically be set
	// Just verify it doesn't cause issues if empty
	_ = hwid.MachineID

	// MACAddress may or may not be available
	// Just verify it doesn't cause issues if empty
	_ = hwid.MACAddress
}

// =============================================================================
// UUID VERSION AND VARIANT TESTS
// =============================================================================

func TestUUIDVersion5Format(t *testing.T) {
	// Generate multiple UUIDs and verify they all have correct version/variant
	for i := 0; i < 10; i++ {
		result, _ := GenerateSemiDeterministicUUID("test_" + string(rune('a'+i)))

		// Version 5 check: version bits should be 0101
		versionByte := result[6]
		version := versionByte >> 4
		if version != 5 {
			t.Errorf("UUID %d version = %d, expected 5", i, version)
		}

		// Variant check: variant bits should be 10xx
		variantByte := result[8]
		variant := variantByte >> 6
		if variant != 2 {
			t.Errorf("UUID %d variant = %d, expected 2", i, variant)
		}
	}
}

// =============================================================================
// EDGE CASE TESTS
// =============================================================================

func TestGenerateSemiDeterministicUUID_SpecialCharacters(t *testing.T) {
	seeds := []string{
		"normal_seed",
		"seed with spaces",
		"seed\nwith\nnewlines",
		"seed\twith\ttabs",
		"日本語シード", // Japanese characters
		"🚀🌟💫",    // Emoji
		string([]byte{0, 1, 2, 3}), // Binary data
	}

	for _, seed := range seeds {
		t.Run(seed, func(t *testing.T) {
			uuid1, err := GenerateSemiDeterministicUUID(seed)
			if err != nil {
				t.Errorf("Failed with seed %q: %v", seed, err)
				return
			}

			// Should be deterministic
			uuid2, _ := GenerateSemiDeterministicUUID(seed)
			if uuid1 != uuid2 {
				t.Errorf("Non-deterministic for seed %q", seed)
			}

			// Should be valid UUID
			if uuid1 == uuid.Nil {
				t.Errorf("Nil UUID for seed %q", seed)
			}
		})
	}
}

func TestGenerateSemiDeterministicUUID_LongSeed(t *testing.T) {
	// Create a very long seed
	longSeed := ""
	for i := 0; i < 10000; i++ {
		longSeed += "x"
	}

	uuid1, err := GenerateSemiDeterministicUUID(longSeed)
	if err != nil {
		t.Fatalf("Failed with long seed: %v", err)
	}

	uuid2, _ := GenerateSemiDeterministicUUID(longSeed)
	if uuid1 != uuid2 {
		t.Error("Long seed should still be deterministic")
	}
}
