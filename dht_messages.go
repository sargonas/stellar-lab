package main

import (
	"time"

	"github.com/google/uuid"
)

// DHT Message Types
const (
	MessageTypePing     = "ping"
	MessageTypeFindNode = "find_node"
	MessageTypeAnnounce = "announce"
)

// DHTMessage is the unified message format for all DHT operations
type DHTMessage struct {
	Type        string       `json:"type"`                    // "ping", "find_node", "announce"
	Version     string       `json:"version"`                 // Protocol version (e.g., "1.0.0")
	FromSystem  *System      `json:"from_system"`             // Sender's full system info (always included)
	TargetID    *uuid.UUID   `json:"target_id,omitempty"`     // For find_node: the ID we're looking for
	ClosestNodes []*System   `json:"closest_nodes,omitempty"` // For find_node response: K closest nodes
	Attestation *Attestation `json:"attestation"`             // Cryptographic proof (required)
	Timestamp   time.Time    `json:"timestamp"`
	IsResponse  bool         `json:"is_response"`             // True if this is a response to a request
	RequestID   string       `json:"request_id,omitempty"`    // Correlates requests with responses
}

// DHTError represents an error response
type DHTError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error codes
const (
	ErrCodeInvalidMessage    = 400
	ErrCodeMissingAttestation = 401
	ErrCodeInvalidAttestation = 402
	ErrCodeIncompatibleVersion = 403
	ErrCodeInternalError     = 500
)

// NewPingRequest creates a new ping request message
func NewPingRequest(fromSystem *System, requestID string) (*DHTMessage, error) {
	if fromSystem.Keys == nil {
		return nil, ErrNoKeys
	}

	attestation := SignAttestation(
		fromSystem.ID,
		uuid.Nil, // Broadcast
		"dht_ping",
		fromSystem.Keys.PrivateKey,
		fromSystem.Keys.PublicKey,
	)

	return &DHTMessage{
		Type:        MessageTypePing,
		Version:     CurrentProtocolVersion.String(),
		FromSystem:  fromSystem,
		Attestation: attestation,
		Timestamp:   time.Now(),
		IsResponse:  false,
		RequestID:   requestID,
	}, nil
}

// NewPingResponse creates a ping response message
func NewPingResponse(fromSystem *System, requestID string) (*DHTMessage, error) {
	if fromSystem.Keys == nil {
		return nil, ErrNoKeys
	}

	attestation := SignAttestation(
		fromSystem.ID,
		uuid.Nil, // Broadcast
		"dht_ping_response",
		fromSystem.Keys.PrivateKey,
		fromSystem.Keys.PublicKey,
	)

	return &DHTMessage{
		Type:        MessageTypePing,
		Version:     CurrentProtocolVersion.String(),
		FromSystem:  fromSystem,
		Attestation: attestation,
		Timestamp:   time.Now(),
		IsResponse:  true,
		RequestID:   requestID,
	}, nil
}

// NewFindNodeRequest creates a new find_node request message
func NewFindNodeRequest(fromSystem *System, targetID uuid.UUID, requestID string) (*DHTMessage, error) {
	if fromSystem.Keys == nil {
		return nil, ErrNoKeys
	}

	attestation := SignAttestation(
		fromSystem.ID,
		uuid.Nil, // Broadcast
		"dht_find_node",
		fromSystem.Keys.PrivateKey,
		fromSystem.Keys.PublicKey,
	)

	return &DHTMessage{
		Type:        MessageTypeFindNode,
		Version:     CurrentProtocolVersion.String(),
		FromSystem:  fromSystem,
		TargetID:    &targetID,
		Attestation: attestation,
		Timestamp:   time.Now(),
		IsResponse:  false,
		RequestID:   requestID,
	}, nil
}

// NewFindNodeResponse creates a find_node response with closest nodes
func NewFindNodeResponse(fromSystem *System, closestNodes []*System, requestID string) (*DHTMessage, error) {
	if fromSystem.Keys == nil {
		return nil, ErrNoKeys
	}

	attestation := SignAttestation(
		fromSystem.ID,
		uuid.Nil, // Broadcast
		"dht_find_node_response",
		fromSystem.Keys.PrivateKey,
		fromSystem.Keys.PublicKey,
	)

	return &DHTMessage{
		Type:         MessageTypeFindNode,
		Version:      CurrentProtocolVersion.String(),
		FromSystem:   fromSystem,
		ClosestNodes: closestNodes,
		Attestation:  attestation,
		Timestamp:    time.Now(),
		IsResponse:   true,
		RequestID:    requestID,
	}, nil
}

// NewAnnounceRequest creates an announce request (node advertising itself)
func NewAnnounceRequest(fromSystem *System, requestID string) (*DHTMessage, error) {
	if fromSystem.Keys == nil {
		return nil, ErrNoKeys
	}

	attestation := SignAttestation(
		fromSystem.ID,
		uuid.Nil, // Broadcast
		"dht_announce",
		fromSystem.Keys.PrivateKey,
		fromSystem.Keys.PublicKey,
	)

	return &DHTMessage{
		Type:        MessageTypeAnnounce,
		Version:     CurrentProtocolVersion.String(),
		FromSystem:  fromSystem,
		Attestation: attestation,
		Timestamp:   time.Now(),
		IsResponse:  false,
		RequestID:   requestID,
	}, nil
}

// NewAnnounceResponse creates an announce response
func NewAnnounceResponse(fromSystem *System, requestID string) (*DHTMessage, error) {
	if fromSystem.Keys == nil {
		return nil, ErrNoKeys
	}

	attestation := SignAttestation(
		fromSystem.ID,
		uuid.Nil, // Broadcast
		"dht_announce_response",
		fromSystem.Keys.PrivateKey,
		fromSystem.Keys.PublicKey,
	)

	return &DHTMessage{
		Type:        MessageTypeAnnounce,
		Version:     CurrentProtocolVersion.String(),
		FromSystem:  fromSystem,
		Attestation: attestation,
		Timestamp:   time.Now(),
		IsResponse:  true,
		RequestID:   requestID,
	}, nil
}

// Validate checks if a DHT message is valid
func (msg *DHTMessage) Validate() error {
	if msg.FromSystem == nil {
		return &DHTError{Code: ErrCodeInvalidMessage, Message: "missing from_system"}
	}

	if msg.Attestation == nil {
		return &DHTError{Code: ErrCodeMissingAttestation, Message: "missing attestation"}
	}

	if !msg.Attestation.Verify() {
		return &DHTError{Code: ErrCodeInvalidAttestation, Message: "invalid attestation signature"}
	}

	switch msg.Type {
	case MessageTypePing:
		// No additional validation needed
	case MessageTypeFindNode:
		if !msg.IsResponse && msg.TargetID == nil {
			return &DHTError{Code: ErrCodeInvalidMessage, Message: "find_node request requires target_id"}
		}
	case MessageTypeAnnounce:
		// No additional validation needed
	default:
		return &DHTError{Code: ErrCodeInvalidMessage, Message: "unknown message type: " + msg.Type}
	}

	return nil
}

// Error implements the error interface for DHTError
func (e *DHTError) Error() string {
	return e.Message
}

// Custom errors
var (
	ErrNoKeys = &DHTError{Code: ErrCodeInternalError, Message: "no cryptographic keys available"}
)