package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Attestation is a signed statement from one peer about another
type Attestation struct {
	FromSystemID   uuid.UUID `json:"from_system_id"`    // Who is making the attestation
	ToSystemID     uuid.UUID `json:"to_system_id"`      // Who it's about
	AttestationType string   `json:"attestation_type"`  // "uptime", "bridge", "relay"
	Value          float64   `json:"value"`             // How many points to award
	Timestamp      time.Time `json:"timestamp"`
	ExpiresAt      time.Time `json:"expires_at"`        // Attestations expire
	Signature      string    `json:"signature"`         // Ed25519 signature
	PublicKey      string    `json:"public_key"`        // Public key of attester
}

// AttestationProof contains the data that gets signed
type AttestationProof struct {
	FromSystemID   uuid.UUID `json:"from_system_id"`
	ToSystemID     uuid.UUID `json:"to_system_id"`
	AttestationType string   `json:"attestation_type"`
	Value          float64   `json:"value"`
	Timestamp      time.Time `json:"timestamp"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// UptimeChallenge is sent by peers to prove you're online
type UptimeChallenge struct {
	ChallengerID uuid.UUID `json:"challenger_id"`
	Challenge    string    `json:"challenge"` // Random string to sign
	Timestamp    time.Time `json:"timestamp"`
}

// UptimeResponse proves you're online by signing the challenge
type UptimeResponse struct {
	SystemID  uuid.UUID `json:"system_id"`
	Challenge string    `json:"challenge"`
	Signature string    `json:"signature"`
	PublicKey string    `json:"public_key"`
	Timestamp time.Time `json:"timestamp"`
}

// DecentralizedReputation tracks verifiable reputation
type DecentralizedReputation struct {
	SystemID           uuid.UUID      `json:"system_id"`
	PublicKey          string         `json:"public_key"`
	PrivateKey         string         `json:"-"` // Never expose in JSON
	Attestations       []*Attestation `json:"attestations"`
	TotalPoints        float64        `json:"total_points"`
	VerifiedPoints     float64        `json:"verified_points"` // Only from valid signatures
	LastChallengeTime  time.Time      `json:"last_challenge_time"`
	LastAttestationTime time.Time     `json:"last_attestation_time"`
}

// GenerateKeyPair creates Ed25519 keys for signing attestations
func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	return pub, priv, err
}

// NewDecentralizedReputation initializes reputation with crypto keys
func NewDecentralizedReputation(systemID uuid.UUID) (*DecentralizedReputation, error) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	return &DecentralizedReputation{
		SystemID:          systemID,
		PublicKey:         hex.EncodeToString(pub),
		PrivateKey:        hex.EncodeToString(priv),
		Attestations:      make([]*Attestation, 0),
		TotalPoints:       0,
		VerifiedPoints:    0,
		LastChallengeTime: time.Now(),
		LastAttestationTime: time.Now(),
	}, nil
}

// SignAttestation creates a signed attestation about another system
func (dr *DecentralizedReputation) SignAttestation(
	toSystemID uuid.UUID,
	attestationType string,
	value float64,
	duration time.Duration,
) (*Attestation, error) {
	
	proof := AttestationProof{
		FromSystemID:   dr.SystemID,
		ToSystemID:     toSystemID,
		AttestationType: attestationType,
		Value:          value,
		Timestamp:      time.Now(),
		ExpiresAt:      time.Now().Add(duration),
	}

	// Serialize proof for signing
	proofBytes, err := json.Marshal(proof)
	if err != nil {
		return nil, err
	}

	// Sign with private key
	privKey, err := hex.DecodeString(dr.PrivateKey)
	if err != nil {
		return nil, err
	}

	signature := ed25519.Sign(privKey, proofBytes)

	attestation := &Attestation{
		FromSystemID:   proof.FromSystemID,
		ToSystemID:     proof.ToSystemID,
		AttestationType: proof.AttestationType,
		Value:          proof.Value,
		Timestamp:      proof.Timestamp,
		ExpiresAt:      proof.ExpiresAt,
		Signature:      hex.EncodeToString(signature),
		PublicKey:      dr.PublicKey,
	}

	return attestation, nil
}

// VerifyAttestation checks if an attestation has a valid signature
func VerifyAttestation(att *Attestation) bool {
	// Reconstruct the proof that was signed
	proof := AttestationProof{
		FromSystemID:   att.FromSystemID,
		ToSystemID:     att.ToSystemID,
		AttestationType: att.AttestationType,
		Value:          att.Value,
		Timestamp:      att.Timestamp,
		ExpiresAt:      att.ExpiresAt,
	}

	proofBytes, err := json.Marshal(proof)
	if err != nil {
		return false
	}

	// Decode public key and signature
	pubKey, err := hex.DecodeString(att.PublicKey)
	if err != nil {
		return false
	}

	signature, err := hex.DecodeString(att.Signature)
	if err != nil {
		return false
	}

	// Verify signature
	return ed25519.Verify(pubKey, proofBytes, signature)
}

// AddAttestation adds and verifies an attestation
func (dr *DecentralizedReputation) AddAttestation(att *Attestation) bool {
	// Only accept attestations about us
	if att.ToSystemID != dr.SystemID {
		return false
	}

	// Check if expired
	if time.Now().After(att.ExpiresAt) {
		return false
	}

	// Verify signature
	if !VerifyAttestation(att) {
		return false
	}

	// Check for duplicates (same attester, same type, recent timestamp)
	for _, existing := range dr.Attestations {
		if existing.FromSystemID == att.FromSystemID &&
			existing.AttestationType == att.AttestationType &&
			existing.Timestamp.After(time.Now().Add(-1*time.Hour)) {
			// Already have recent attestation of this type from this peer
			return false
		}
	}

	// Add to attestations
	dr.Attestations = append(dr.Attestations, att)
	dr.LastAttestationTime = time.Now()

	// Recalculate verified points
	dr.RecalculatePoints()

	return true
}

// RecalculatePoints sums up verified attestations
func (dr *DecentralizedReputation) RecalculatePoints() {
	verified := 0.0
	total := 0.0

	for _, att := range dr.Attestations {
		// Skip expired
		if time.Now().After(att.ExpiresAt) {
			continue
		}

		total += att.Value

		// Only count verified
		if VerifyAttestation(att) {
			verified += att.Value
		}
	}

	dr.TotalPoints = total
	dr.VerifiedPoints = verified
}

// PruneExpiredAttestations removes old attestations
func (dr *DecentralizedReputation) PruneExpiredAttestations() int {
	pruned := 0
	validAttestations := make([]*Attestation, 0)

	for _, att := range dr.Attestations {
		if time.Now().Before(att.ExpiresAt) {
			validAttestations = append(validAttestations, att)
		} else {
			pruned++
		}
	}

	dr.Attestations = validAttestations
	dr.RecalculatePoints()

	return pruned
}

// CreateUptimeChallenge generates a challenge for another peer
func CreateUptimeChallenge(challengerID uuid.UUID) *UptimeChallenge {
	// Random challenge string
	randomBytes := make([]byte, 32)
	hash := sha256.Sum256([]byte(time.Now().String() + challengerID.String()))
	copy(randomBytes, hash[:])

	return &UptimeChallenge{
		ChallengerID: challengerID,
		Challenge:    hex.EncodeToString(randomBytes),
		Timestamp:    time.Now(),
	}
}

// RespondToChallenge signs a challenge to prove uptime
func (dr *DecentralizedReputation) RespondToChallenge(challenge *UptimeChallenge) (*UptimeResponse, error) {
	privKey, err := hex.DecodeString(dr.PrivateKey)
	if err != nil {
		return nil, err
	}

	// Sign the challenge string
	signature := ed25519.Sign(privKey, []byte(challenge.Challenge))

	return &UptimeResponse{
		SystemID:  dr.SystemID,
		Challenge: challenge.Challenge,
		Signature: hex.EncodeToString(signature),
		PublicKey: dr.PublicKey,
		Timestamp: time.Now(),
	}, nil
}

// VerifyUptimeResponse checks if a challenge response is valid
func VerifyUptimeResponse(challenge *UptimeChallenge, response *UptimeResponse) bool {
	// Check challenge matches
	if challenge.Challenge != response.Challenge {
		return false
	}

	// Decode public key and signature
	pubKey, err := hex.DecodeString(response.PublicKey)
	if err != nil {
		return false
	}

	signature, err := hex.DecodeString(response.Signature)
	if err != nil {
		return false
	}

	// Verify signature on challenge
	return ed25519.Verify(pubKey, []byte(challenge.Challenge), signature)
}

// GetReputationSummary returns human-readable reputation info
func (dr *DecentralizedReputation) GetReputationSummary() map[string]interface{} {
	// Count attestations by type
	uptimeCount := 0
	bridgeCount := 0
	relayCount := 0

	for _, att := range dr.Attestations {
		if time.Now().After(att.ExpiresAt) {
			continue
		}
		switch att.AttestationType {
		case "uptime":
			uptimeCount++
		case "bridge":
			bridgeCount++
		case "relay":
			relayCount++
		}
	}

	rank := "Unranked"
	if dr.VerifiedPoints >= 10000 {
		rank = "Diamond"
	} else if dr.VerifiedPoints >= 5000 {
		rank = "Platinum"
	} else if dr.VerifiedPoints >= 2000 {
		rank = "Gold"
	} else if dr.VerifiedPoints >= 500 {
		rank = "Silver"
	} else if dr.VerifiedPoints >= 100 {
		rank = "Bronze"
	}

	return map[string]interface{}{
		"verified_points":      int(dr.VerifiedPoints),
		"total_attestations":   len(dr.Attestations),
		"rank":                 rank,
		"uptime_attestations":  uptimeCount,
		"bridge_attestations":  bridgeCount,
		"relay_attestations":   relayCount,
		"public_key":           dr.PublicKey[:16] + "...", // Truncated for display
		"last_attestation_ago": time.Since(dr.LastAttestationTime).Round(time.Second).String(),
	}
}
