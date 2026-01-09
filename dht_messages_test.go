package main

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// MESSAGE CREATION TESTS
// =============================================================================

func createTestSystem() *System {
	keys, _ := GenerateKeyPair()
	sys := &System{
		ID:   uuid.New(),
		Name: "TestSystem",
		Keys: keys,
		Stars: MultiStarSystem{
			Primary: StarType{Class: "G"},
			Count:   1,
		},
	}
	sys.GenerateMultiStarSystem() // Ensure valid star config
	return sys
}

func TestNewPingRequest(t *testing.T) {
	sys := createTestSystem()
	toID := uuid.New()

	msg, err := NewPingRequest(sys, toID, "req-123")

	if err != nil {
		t.Fatalf("NewPingRequest() error = %v", err)
	}

	if msg.Type != MessageTypePing {
		t.Errorf("Type = %s, want %s", msg.Type, MessageTypePing)
	}
	if msg.IsResponse {
		t.Error("Request should not be marked as response")
	}
	if msg.RequestID != "req-123" {
		t.Errorf("RequestID = %s, want req-123", msg.RequestID)
	}
	if msg.FromSystem != sys {
		t.Error("FromSystem should be the provided system")
	}
	if msg.Attestation == nil {
		t.Error("Attestation should not be nil")
	}
	if msg.Attestation.MessageType != "dht_ping" {
		t.Errorf("Attestation.MessageType = %s, want dht_ping", msg.Attestation.MessageType)
	}
	if msg.Attestation.ToSystemID != toID {
		t.Error("Attestation.ToSystemID should match toID")
	}
}

func TestNewPingResponse(t *testing.T) {
	sys := createTestSystem()
	toID := uuid.New()

	msg, err := NewPingResponse(sys, toID, "req-123")

	if err != nil {
		t.Fatalf("NewPingResponse() error = %v", err)
	}

	if msg.Type != MessageTypePing {
		t.Errorf("Type = %s, want %s", msg.Type, MessageTypePing)
	}
	if !msg.IsResponse {
		t.Error("Response should be marked as response")
	}
	if msg.Attestation.MessageType != "dht_ping_response" {
		t.Errorf("Attestation.MessageType = %s, want dht_ping_response", msg.Attestation.MessageType)
	}
}

func TestNewFindNodeRequest(t *testing.T) {
	sys := createTestSystem()
	toID := uuid.New()
	targetID := uuid.New()

	msg, err := NewFindNodeRequest(sys, toID, targetID, "req-456")

	if err != nil {
		t.Fatalf("NewFindNodeRequest() error = %v", err)
	}

	if msg.Type != MessageTypeFindNode {
		t.Errorf("Type = %s, want %s", msg.Type, MessageTypeFindNode)
	}
	if msg.TargetID == nil || *msg.TargetID != targetID {
		t.Error("TargetID should be set")
	}
	if msg.Attestation.MessageType != "dht_find_node" {
		t.Errorf("Attestation.MessageType = %s, want dht_find_node", msg.Attestation.MessageType)
	}
}

func TestNewFindNodeResponse(t *testing.T) {
	sys := createTestSystem()
	toID := uuid.New()
	closestNodes := []*System{createTestSystem(), createTestSystem()}

	msg, err := NewFindNodeResponse(sys, toID, closestNodes, "req-456")

	if err != nil {
		t.Fatalf("NewFindNodeResponse() error = %v", err)
	}

	if msg.Type != MessageTypeFindNode {
		t.Errorf("Type = %s, want %s", msg.Type, MessageTypeFindNode)
	}
	if !msg.IsResponse {
		t.Error("Response should be marked as response")
	}
	if len(msg.ClosestNodes) != 2 {
		t.Errorf("ClosestNodes length = %d, want 2", len(msg.ClosestNodes))
	}
	if msg.Attestation.MessageType != "dht_find_node_response" {
		t.Errorf("Attestation.MessageType = %s, want dht_find_node_response", msg.Attestation.MessageType)
	}
}

func TestNewAnnounceRequest(t *testing.T) {
	sys := createTestSystem()
	toID := uuid.New()

	msg, err := NewAnnounceRequest(sys, toID, "req-789")

	if err != nil {
		t.Fatalf("NewAnnounceRequest() error = %v", err)
	}

	if msg.Type != MessageTypeAnnounce {
		t.Errorf("Type = %s, want %s", msg.Type, MessageTypeAnnounce)
	}
	if msg.Attestation.MessageType != "dht_announce" {
		t.Errorf("Attestation.MessageType = %s, want dht_announce", msg.Attestation.MessageType)
	}
}

func TestNewAnnounceResponse(t *testing.T) {
	sys := createTestSystem()
	toID := uuid.New()

	msg, err := NewAnnounceResponse(sys, toID, "req-789")

	if err != nil {
		t.Fatalf("NewAnnounceResponse() error = %v", err)
	}

	if msg.Type != MessageTypeAnnounce {
		t.Errorf("Type = %s, want %s", msg.Type, MessageTypeAnnounce)
	}
	if !msg.IsResponse {
		t.Error("Response should be marked as response")
	}
	if msg.Attestation.MessageType != "dht_announce_response" {
		t.Errorf("Attestation.MessageType = %s, want dht_announce_response", msg.Attestation.MessageType)
	}
}

// =============================================================================
// MESSAGE CREATION ERROR TESTS
// =============================================================================

func TestNewPingRequest_NoKeys(t *testing.T) {
	sys := &System{ID: uuid.New(), Keys: nil}

	_, err := NewPingRequest(sys, uuid.New(), "req-123")

	if err == nil {
		t.Error("Expected error when system has no keys")
	}
	if err != ErrNoKeys {
		t.Errorf("Expected ErrNoKeys, got %v", err)
	}
}

func TestNewPingResponse_NoKeys(t *testing.T) {
	sys := &System{ID: uuid.New(), Keys: nil}

	_, err := NewPingResponse(sys, uuid.New(), "req-123")

	if err != ErrNoKeys {
		t.Errorf("Expected ErrNoKeys, got %v", err)
	}
}

func TestNewFindNodeRequest_NoKeys(t *testing.T) {
	sys := &System{ID: uuid.New(), Keys: nil}

	_, err := NewFindNodeRequest(sys, uuid.New(), uuid.New(), "req-123")

	if err != ErrNoKeys {
		t.Errorf("Expected ErrNoKeys, got %v", err)
	}
}

func TestNewAnnounceRequest_NoKeys(t *testing.T) {
	sys := &System{ID: uuid.New(), Keys: nil}

	_, err := NewAnnounceRequest(sys, uuid.New(), "req-123")

	if err != ErrNoKeys {
		t.Errorf("Expected ErrNoKeys, got %v", err)
	}
}

// =============================================================================
// MESSAGE VALIDATION TESTS
// =============================================================================

func TestDHTMessage_Validate_Valid(t *testing.T) {
	sys := createTestSystem()
	msg, _ := NewPingRequest(sys, uuid.New(), "req-123")

	err := msg.Validate()

	if err != nil {
		t.Errorf("Valid message should not return error: %v", err)
	}
}

func TestDHTMessage_Validate_MissingFromSystem(t *testing.T) {
	msg := &DHTMessage{
		Type:        MessageTypePing,
		FromSystem:  nil,
		Attestation: &Attestation{},
	}

	err := msg.Validate()

	if err == nil {
		t.Error("Expected error for missing FromSystem")
	}
	if dhtErr, ok := err.(*DHTError); ok {
		if dhtErr.Code != ErrCodeInvalidMessage {
			t.Errorf("Expected code %d, got %d", ErrCodeInvalidMessage, dhtErr.Code)
		}
	}
}

func TestDHTMessage_Validate_MissingAttestation(t *testing.T) {
	sys := createTestSystem()
	msg := &DHTMessage{
		Type:        MessageTypePing,
		FromSystem:  sys,
		Attestation: nil,
	}

	err := msg.Validate()

	if err == nil {
		t.Error("Expected error for missing Attestation")
	}
	if dhtErr, ok := err.(*DHTError); ok {
		if dhtErr.Code != ErrCodeMissingAttestation {
			t.Errorf("Expected code %d, got %d", ErrCodeMissingAttestation, dhtErr.Code)
		}
	}
}

func TestDHTMessage_Validate_InvalidSignature(t *testing.T) {
	sys := createTestSystem()
	msg, _ := NewPingRequest(sys, uuid.New(), "req-123")

	// Corrupt the signature
	msg.Attestation.Signature = "invalid_signature"

	err := msg.Validate()

	if err == nil {
		t.Error("Expected error for invalid signature")
	}
	if dhtErr, ok := err.(*DHTError); ok {
		if dhtErr.Code != ErrCodeInvalidAttestation {
			t.Errorf("Expected code %d, got %d", ErrCodeInvalidAttestation, dhtErr.Code)
		}
	}
}

func TestDHTMessage_Validate_SenderMismatch(t *testing.T) {
	sys := createTestSystem()
	msg, _ := NewPingRequest(sys, uuid.New(), "req-123")

	// Change the attestation sender to not match FromSystem
	msg.Attestation.FromSystemID = uuid.New()

	err := msg.Validate()

	if err == nil {
		t.Error("Expected error for sender mismatch")
	}
}

func TestDHTMessage_Validate_ExpiredTimestamp(t *testing.T) {
	sys := createTestSystem()
	msg, _ := NewPingRequest(sys, uuid.New(), "req-123")

	// Make attestation timestamp 10 minutes old (> 5 min threshold)
	msg.Attestation.Timestamp = time.Now().Unix() - 600

	err := msg.Validate()

	if err == nil {
		t.Error("Expected error for expired timestamp")
	}
}

func TestDHTMessage_Validate_FutureTimestamp(t *testing.T) {
	sys := createTestSystem()
	msg, _ := NewPingRequest(sys, uuid.New(), "req-123")

	// Make attestation timestamp 10 minutes in the future
	msg.Attestation.Timestamp = time.Now().Unix() + 600

	err := msg.Validate()

	if err == nil {
		t.Error("Expected error for future timestamp")
	}
}

func TestDHTMessage_Validate_NameTooLong(t *testing.T) {
	sys := createTestSystem()
	sys.Name = "This name is way too long and should exceed the 64 character limit that is set in the validation"

	msg, _ := NewPingRequest(sys, uuid.New(), "req-123")

	err := msg.Validate()

	if err == nil {
		t.Error("Expected error for name too long")
	}
}

func TestDHTMessage_Validate_UnknownMessageType(t *testing.T) {
	sys := createTestSystem()
	msg, _ := NewPingRequest(sys, uuid.New(), "req-123")
	msg.Type = "unknown_type"

	err := msg.Validate()

	if err == nil {
		t.Error("Expected error for unknown message type")
	}
}

func TestDHTMessage_Validate_FindNodeMissingTarget(t *testing.T) {
	sys := createTestSystem()
	msg, _ := NewFindNodeRequest(sys, uuid.New(), uuid.New(), "req-123")

	// Remove the target ID (simulating invalid request)
	msg.TargetID = nil

	err := msg.Validate()

	if err == nil {
		t.Error("Expected error for find_node without target_id")
	}
}

// =============================================================================
// TARGETED ATTESTATION TESTS
// =============================================================================

func TestDHTMessage_HasTargetedAttestation(t *testing.T) {
	sys := createTestSystem()

	// With specific recipient
	msg1, _ := NewPingRequest(sys, uuid.New(), "req-123")
	if !msg1.HasTargetedAttestation() {
		t.Error("Message with specific recipient should have targeted attestation")
	}

	// With uuid.Nil recipient (first contact)
	msg2, _ := NewPingRequest(sys, uuid.Nil, "req-456")
	if msg2.HasTargetedAttestation() {
		t.Error("Message with Nil recipient should not have targeted attestation")
	}
}

// =============================================================================
// ERROR TYPE TESTS
// =============================================================================

func TestDHTError_Error(t *testing.T) {
	err := &DHTError{Code: 400, Message: "test error"}

	if err.Error() != "test error" {
		t.Errorf("Error() = %s, want 'test error'", err.Error())
	}
}

func TestErrorCodes(t *testing.T) {
	// Verify error codes are set correctly
	if ErrCodeInvalidMessage != 400 {
		t.Errorf("ErrCodeInvalidMessage = %d, want 400", ErrCodeInvalidMessage)
	}
	if ErrCodeMissingAttestation != 401 {
		t.Errorf("ErrCodeMissingAttestation = %d, want 401", ErrCodeMissingAttestation)
	}
	if ErrCodeInvalidAttestation != 402 {
		t.Errorf("ErrCodeInvalidAttestation = %d, want 402", ErrCodeInvalidAttestation)
	}
	if ErrCodeIncompatibleVersion != 403 {
		t.Errorf("ErrCodeIncompatibleVersion = %d, want 403", ErrCodeIncompatibleVersion)
	}
	if ErrCodeInternalError != 500 {
		t.Errorf("ErrCodeInternalError = %d, want 500", ErrCodeInternalError)
	}
}

// =============================================================================
// MESSAGE TYPE CONSTANTS TESTS
// =============================================================================

func TestMessageTypeConstants(t *testing.T) {
	if MessageTypePing != "ping" {
		t.Errorf("MessageTypePing = %s, want 'ping'", MessageTypePing)
	}
	if MessageTypeFindNode != "find_node" {
		t.Errorf("MessageTypeFindNode = %s, want 'find_node'", MessageTypeFindNode)
	}
	if MessageTypeAnnounce != "announce" {
		t.Errorf("MessageTypeAnnounce = %s, want 'announce'", MessageTypeAnnounce)
	}
}

// =============================================================================
// BUG DETECTION TESTS - Verify tests would catch common bugs
// =============================================================================

// TestValidation_WouldCatchBugs verifies validation tests catch common errors
func TestValidation_WouldCatchBugs(t *testing.T) {
	sys := createTestSystem()

	// Bug: If validation accepted nil attestation
	msgNoAtt := &DHTMessage{
		Type:        MessageTypePing,
		FromSystem:  sys,
		Attestation: nil,
	}
	if msgNoAtt.Validate() == nil {
		t.Error("BUG: Validation should reject nil attestation")
	}

	// Bug: If validation accepted nil FromSystem
	keys, _ := GenerateKeyPair()
	msgNoFrom := &DHTMessage{
		Type:       MessageTypePing,
		FromSystem: nil,
		Attestation: SignAttestation(uuid.New(), uuid.New(), "dht_ping",
			keys.PrivateKey, keys.PublicKey),
	}
	if msgNoFrom.Validate() == nil {
		t.Error("BUG: Validation should reject nil FromSystem")
	}

	// Bug: If timestamp validation was disabled
	validMsg, _ := NewPingRequest(sys, uuid.New(), "req-123")
	validMsg.Attestation.Timestamp = 0 // Unix epoch (1970)
	if validMsg.Validate() == nil {
		t.Error("BUG: Validation should reject timestamps from 1970")
	}
}

// TestAttestationTypes_WouldCatchBugs verifies attestation types are distinct
func TestAttestationTypes_WouldCatchBugs(t *testing.T) {
	sys := createTestSystem()
	toID := uuid.New()

	ping, _ := NewPingRequest(sys, toID, "1")
	pingResp, _ := NewPingResponse(sys, toID, "2")
	findNode, _ := NewFindNodeRequest(sys, toID, uuid.New(), "3")
	findNodeResp, _ := NewFindNodeResponse(sys, toID, nil, "4")
	announce, _ := NewAnnounceRequest(sys, toID, "5")
	announceResp, _ := NewAnnounceResponse(sys, toID, "6")

	types := map[string]bool{
		ping.Attestation.MessageType:         true,
		pingResp.Attestation.MessageType:     true,
		findNode.Attestation.MessageType:     true,
		findNodeResp.Attestation.MessageType: true,
		announce.Attestation.MessageType:     true,
		announceResp.Attestation.MessageType: true,
	}

	// Should have 6 distinct attestation types
	if len(types) != 6 {
		t.Errorf("BUG: Expected 6 distinct attestation types, got %d", len(types))
	}
}
