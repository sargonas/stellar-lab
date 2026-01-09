package main

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// KEYPAIR TESTS
// =============================================================================

func TestGenerateKeyPair(t *testing.T) {
	keys, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	if keys == nil {
		t.Fatal("GenerateKeyPair() returned nil")
	}

	if len(keys.PublicKey) == 0 {
		t.Error("PublicKey is empty")
	}

	if len(keys.PrivateKey) == 0 {
		t.Error("PrivateKey is empty")
	}

	// Ed25519 public keys are 32 bytes
	if len(keys.PublicKey) != 32 {
		t.Errorf("PublicKey length = %d, want 32", len(keys.PublicKey))
	}

	// Ed25519 private keys are 64 bytes
	if len(keys.PrivateKey) != 64 {
		t.Errorf("PrivateKey length = %d, want 64", len(keys.PrivateKey))
	}
}

func TestGenerateKeyPair_Uniqueness(t *testing.T) {
	keys1, _ := GenerateKeyPair()
	keys2, _ := GenerateKeyPair()

	// Keys should be unique
	if string(keys1.PublicKey) == string(keys2.PublicKey) {
		t.Error("Generated keypairs should be unique")
	}
}

// =============================================================================
// ATTESTATION SIGNING AND VERIFICATION TESTS
// =============================================================================

func TestSignAttestation(t *testing.T) {
	keys, _ := GenerateKeyPair()
	fromID := uuid.New()
	toID := uuid.New()

	att := SignAttestation(fromID, toID, "dht_ping", keys.PrivateKey, keys.PublicKey)

	if att == nil {
		t.Fatal("SignAttestation() returned nil")
	}

	if att.FromSystemID != fromID {
		t.Error("FromSystemID mismatch")
	}

	if att.ToSystemID != toID {
		t.Error("ToSystemID mismatch")
	}

	if att.MessageType != "dht_ping" {
		t.Error("MessageType mismatch")
	}

	if att.Signature == "" {
		t.Error("Signature is empty")
	}

	if att.PublicKey == "" {
		t.Error("PublicKey is empty")
	}

	if att.Timestamp == 0 {
		t.Error("Timestamp should not be zero")
	}
}

func TestAttestation_Verify(t *testing.T) {
	keys, _ := GenerateKeyPair()
	fromID := uuid.New()
	toID := uuid.New()

	att := SignAttestation(fromID, toID, "dht_ping", keys.PrivateKey, keys.PublicKey)

	if !att.Verify() {
		t.Error("Valid attestation should verify")
	}
}

func TestAttestation_Verify_TamperedFromID(t *testing.T) {
	keys, _ := GenerateKeyPair()
	fromID := uuid.New()
	toID := uuid.New()

	att := SignAttestation(fromID, toID, "dht_ping", keys.PrivateKey, keys.PublicKey)

	// Tamper with FromSystemID
	att.FromSystemID = uuid.New()

	if att.Verify() {
		t.Error("Tampered FromSystemID should not verify")
	}
}

func TestAttestation_Verify_TamperedToID(t *testing.T) {
	keys, _ := GenerateKeyPair()
	fromID := uuid.New()
	toID := uuid.New()

	att := SignAttestation(fromID, toID, "dht_ping", keys.PrivateKey, keys.PublicKey)

	// Tamper with ToSystemID
	att.ToSystemID = uuid.New()

	if att.Verify() {
		t.Error("Tampered ToSystemID should not verify")
	}
}

func TestAttestation_Verify_TamperedTimestamp(t *testing.T) {
	keys, _ := GenerateKeyPair()
	fromID := uuid.New()
	toID := uuid.New()

	att := SignAttestation(fromID, toID, "dht_ping", keys.PrivateKey, keys.PublicKey)

	// Tamper with Timestamp
	att.Timestamp = att.Timestamp + 1000

	if att.Verify() {
		t.Error("Tampered Timestamp should not verify")
	}
}

func TestAttestation_Verify_TamperedMessageType(t *testing.T) {
	keys, _ := GenerateKeyPair()
	fromID := uuid.New()
	toID := uuid.New()

	att := SignAttestation(fromID, toID, "dht_ping", keys.PrivateKey, keys.PublicKey)

	// Tamper with MessageType
	att.MessageType = "dht_announce"

	if att.Verify() {
		t.Error("Tampered MessageType should not verify")
	}
}

func TestAttestation_Verify_InvalidSignature(t *testing.T) {
	keys, _ := GenerateKeyPair()
	fromID := uuid.New()
	toID := uuid.New()

	att := SignAttestation(fromID, toID, "dht_ping", keys.PrivateKey, keys.PublicKey)

	// Corrupt the signature
	att.Signature = "aW52YWxpZF9zaWduYXR1cmU=" // base64 of "invalid_signature"

	if att.Verify() {
		t.Error("Invalid signature should not verify")
	}
}

func TestAttestation_Verify_InvalidPublicKey(t *testing.T) {
	keys, _ := GenerateKeyPair()
	fromID := uuid.New()
	toID := uuid.New()

	att := SignAttestation(fromID, toID, "dht_ping", keys.PrivateKey, keys.PublicKey)

	// Use a different public key
	otherKeys, _ := GenerateKeyPair()
	att.PublicKey = base64.StdEncoding.EncodeToString(otherKeys.PublicKey)

	if att.Verify() {
		t.Error("Wrong public key should not verify")
	}
}

// =============================================================================
// TIMESTAMP VALIDATION TESTS
// =============================================================================

func TestAttestation_IsTimestampValid(t *testing.T) {
	keys, _ := GenerateKeyPair()
	att := SignAttestation(uuid.New(), uuid.New(), "dht_ping", keys.PrivateKey, keys.PublicKey)

	maxDrift := 5 * time.Minute

	// Fresh attestation should be valid
	if !att.IsTimestampValid(maxDrift) {
		t.Error("Fresh attestation timestamp should be valid")
	}
}

func TestAttestation_IsTimestampValid_TooOld(t *testing.T) {
	keys, _ := GenerateKeyPair()
	att := SignAttestation(uuid.New(), uuid.New(), "dht_ping", keys.PrivateKey, keys.PublicKey)

	// Make timestamp 10 minutes ago
	att.Timestamp = time.Now().Unix() - 600

	maxDrift := 5 * time.Minute

	if att.IsTimestampValid(maxDrift) {
		t.Error("10-minute old attestation should not be valid with 5-minute drift")
	}
}

func TestAttestation_IsTimestampValid_TooFuture(t *testing.T) {
	keys, _ := GenerateKeyPair()
	att := SignAttestation(uuid.New(), uuid.New(), "dht_ping", keys.PrivateKey, keys.PublicKey)

	// Make timestamp 10 minutes in the future
	att.Timestamp = time.Now().Unix() + 600

	maxDrift := 5 * time.Minute

	if att.IsTimestampValid(maxDrift) {
		t.Error("10-minute future attestation should not be valid with 5-minute drift")
	}
}

func TestAttestation_IsTimestampValid_AtThreshold(t *testing.T) {
	keys, _ := GenerateKeyPair()
	att := SignAttestation(uuid.New(), uuid.New(), "dht_ping", keys.PrivateKey, keys.PublicKey)

	maxDrift := 5 * time.Minute

	// Make timestamp exactly at the threshold (5 minutes ago)
	att.Timestamp = time.Now().Unix() - 300

	// Should be valid (within or at threshold)
	if !att.IsTimestampValid(maxDrift) {
		t.Error("Attestation at exactly max drift should be valid")
	}
}

// =============================================================================
// MESSAGE TYPE TESTS
// =============================================================================

func TestAttestation_MessageTypes(t *testing.T) {
	keys, _ := GenerateKeyPair()
	fromID := uuid.New()
	toID := uuid.New()

	messageTypes := []string{
		"dht_ping",
		"dht_ping_response",
		"dht_find_node",
		"dht_find_node_response",
		"dht_announce",
		"dht_announce_response",
	}

	for _, msgType := range messageTypes {
		t.Run(msgType, func(t *testing.T) {
			att := SignAttestation(fromID, toID, msgType, keys.PrivateKey, keys.PublicKey)

			if att.MessageType != msgType {
				t.Errorf("MessageType = %s, want %s", att.MessageType, msgType)
			}

			if !att.Verify() {
				t.Error("Attestation should verify regardless of message type")
			}
		})
	}
}

// =============================================================================
// SIGNABLE MESSAGE TESTS
// =============================================================================

func TestAttestation_GetSignableMessage_Deterministic(t *testing.T) {
	att := &Attestation{
		FromSystemID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		ToSystemID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Timestamp:    1234567890,
		MessageType:  "dht_ping",
	}

	msg1 := att.GetSignableMessage()
	msg2 := att.GetSignableMessage()

	if string(msg1) != string(msg2) {
		t.Error("GetSignableMessage should return deterministic results")
	}
}

func TestAttestation_GetSignableMessage_Different(t *testing.T) {
	att1 := &Attestation{
		FromSystemID: uuid.New(),
		ToSystemID:   uuid.New(),
		Timestamp:    time.Now().Unix(),
		MessageType:  "dht_ping",
	}

	att2 := &Attestation{
		FromSystemID: uuid.New(),
		ToSystemID:   uuid.New(),
		Timestamp:    time.Now().Unix(),
		MessageType:  "dht_ping",
	}

	msg1 := att1.GetSignableMessage()
	msg2 := att2.GetSignableMessage()

	if string(msg1) == string(msg2) {
		t.Error("Different attestations should have different signable messages")
	}
}
