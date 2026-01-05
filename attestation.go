package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// KeyPair represents a node's cryptographic identity
type KeyPair struct {
	PublicKey  ed25519.PublicKey  `json:"public_key"`
	PrivateKey ed25519.PrivateKey `json:"-"` // Never serialized
}

// GenerateKeyPair creates a new Ed25519 keypair
func GenerateKeyPair() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &KeyPair{
		PublicKey:  pub,
		PrivateKey: priv,
	}, nil
}

// Attestation represents a cryptographically signed proof of peer interaction
type Attestation struct {
	FromSystemID uuid.UUID `json:"from_system_id"` // Who sent this
	ToSystemID   uuid.UUID `json:"to_system_id"`   // Who received it
	Timestamp    int64     `json:"timestamp"`      // Unix timestamp
	MessageType  string    `json:"message_type"`   // "ping", "find_node", "announce"
	Signature    string    `json:"signature"`      // Ed25519 signature (base64)
	PublicKey    string    `json:"public_key"`     // Sender's public key (base64)
}

// SignAttestation creates a signed attestation
func SignAttestation(fromID, toID uuid.UUID, msgType string, privKey ed25519.PrivateKey, pubKey ed25519.PublicKey) *Attestation {
	a := &Attestation{
		FromSystemID: fromID,
		ToSystemID:   toID,
		Timestamp:    time.Now().Unix(),
		MessageType:  msgType,
		PublicKey:    base64.StdEncoding.EncodeToString(pubKey),
	}

	// Create message to sign
	msg := a.GetSignableMessage()
	
	// Sign it
	sig := ed25519.Sign(privKey, msg)
	a.Signature = base64.StdEncoding.EncodeToString(sig)

	return a
}

// GetSignableMessage returns the canonical message to sign
func (a *Attestation) GetSignableMessage() []byte {
	msg := struct {
		From      string `json:"from"`
		To        string `json:"to"`
		Timestamp int64  `json:"timestamp"`
		Type      string `json:"type"`
	}{
		From:      a.FromSystemID.String(),
		To:        a.ToSystemID.String(),
		Timestamp: a.Timestamp,
		Type:      a.MessageType,
	}
	data, _ := json.Marshal(msg)
	return data
}

// Verify checks if an attestation has a valid signature
func (a *Attestation) Verify() bool {
	// Decode public key
	pubKeyBytes, err := base64.StdEncoding.DecodeString(a.PublicKey)
	if err != nil {
		return false
	}

	// Decode signature
	sigBytes, err := base64.StdEncoding.DecodeString(a.Signature)
	if err != nil {
		return false
	}

	// Verify signature
	msg := a.GetSignableMessage()
	return ed25519.Verify(pubKeyBytes, msg, sigBytes)
}
