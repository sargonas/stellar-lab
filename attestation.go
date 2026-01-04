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
	MessageType  string    `json:"message_type"`   // "heartbeat", "peer_exchange", etc.
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

// AttestationProof represents proof of network participation
type AttestationProof struct {
	SystemID          uuid.UUID      `json:"system_id"`
	Attestations      []*Attestation `json:"attestations"`      // All signed proofs
	TotalProofs       int            `json:"total_proofs"`      // Count of verified interactions
	OldestProof       int64          `json:"oldest_proof"`      // Timestamp of oldest proof (uptime indicator)
	UniquePeers       int            `json:"unique_peers"`      // Number of unique peers attested
	VerificationScore float64        `json:"verification_score"` // Score based on verified attestations
}

// CalculateVerifiableReputation computes reputation from cryptographic proofs
// with tolerance for brief downtimes (updates, maintenance)
func CalculateVerifiableReputation(proof *AttestationProof) float64 {
	if len(proof.Attestations) == 0 {
		return 0.0
	}

	// Verify all attestations
	validCount := 0
	for _, att := range proof.Attestations {
		if att.Verify() {
			validCount++
		}
	}

	// Points from verified interactions (1 point per verified attestation)
	verifiedPoints := float64(validCount)

	// Uptime bonus (based on oldest proof) with downtime tolerance
	if proof.OldestProof > 0 {
		// Calculate total time since first attestation
		totalTime := time.Now().Unix() - proof.OldestProof
		totalHours := float64(totalTime) / 3600.0
		
		// DOWNTIME TOLERANCE: Assume 99% uptime is realistic
		// Brief downtimes (updates, restarts) shouldn't heavily penalize
		// Give 0.5 points per hour for first 99% of time, reduced for gaps
		
		// Expected attestations: ~2 per minute (heartbeat every 30s, exchanges every 60s)
		// = ~120 per hour
		expectedPerHour := 120.0
		expectedTotal := totalHours * expectedPerHour
		
		// Actual vs expected ratio
		uptimeRatio := float64(validCount) / expectedTotal
		if uptimeRatio > 1.0 {
			uptimeRatio = 1.0 // Cap at 100%
		}
		
		// Apply uptime multiplier
		// 100% uptime = full 0.5 points/hour
		// 90% uptime = 0.45 points/hour (minor penalty)
		// <80% uptime = more significant penalty
		uptimeMultiplier := uptimeRatio
		if uptimeRatio >= 0.90 {
			// Minimal penalty for 90%+ uptime (covers brief restarts)
			uptimeMultiplier = 0.90 + (uptimeRatio-0.90)*1.0
		}
		
		uptimePoints := totalHours * 0.5 * uptimeMultiplier
		verifiedPoints += uptimePoints
	}

	// Unique peer bonus (diverse connections)
	verifiedPoints += float64(proof.UniquePeers) * 10.0

	return verifiedPoints
}

// CalculateFullReputation calculates reputation from both recent attestations and historical summaries
func CalculateFullReputation(proof *AttestationProof, summaries []AttestationSummary) float64 {
    // Recent attestations (full weight)
    recentScore := float64(proof.TotalProofs) * 1.0

    // Unique peers from recent attestations
    uniquePeers := make(map[string]bool)
    for _, att := range proof.Attestations {
        uniquePeers[att.FromSystemID.String()] = true
        uniquePeers[att.ToSystemID.String()] = true
    }

    // Historical summaries (decayed weight based on age)
    historicalScore := 0.0
    for _, summary := range summaries {
        // Track unique peers from history too
        uniquePeers[summary.PeerSystemID] = true

        // Calculate age-based decay
        periodEnd := time.Unix(summary.PeriodEnd, 0)
        ageInDays := time.Since(periodEnd).Hours() / 24

        // Decay formula: 100% for first 30 days, then linear decay to 50% floor at 1 year
        var decay float64
        if ageInDays <= 30 {
            decay = 1.0
        } else if ageInDays >= 365 {
            decay = 0.5
        } else {
            // Linear interpolation between 30 days (1.0) and 365 days (0.5)
            decay = 1.0 - ((ageInDays - 30) / (365 - 30) * 0.5)
        }

        totalCount := float64(summary.TotalCount())
        historicalScore += totalCount * decay
    }

    // Peer diversity bonus (more unique peers = more valuable)
    diversityBonus := float64(len(uniquePeers)) * 10.0

    // Combine scores
    totalScore := recentScore + historicalScore + diversityBonus

    return totalScore
}

// VerifiableNetworkContribution is the decentralized replacement for NetworkContribution
type VerifiableNetworkContribution struct {
	SystemID       uuid.UUID        `json:"system_id"`
	PublicKey      string           `json:"public_key"`      // Base64 encoded public key
	Proof          *AttestationProof `json:"proof"`           // Cryptographic proof
	ReputationScore float64          `json:"reputation_score"` // Calculated from verified proofs
	Rank           string           `json:"rank"`            // Rank based on verifiable score
	LastCalculated time.Time        `json:"last_calculated"`
}

// BuildAttestationProof collects all attestations and builds proof
func BuildAttestationProof(systemID uuid.UUID, attestations []*Attestation) *AttestationProof {
	if len(attestations) == 0 {
		return &AttestationProof{
			SystemID:    systemID,
			Attestations: []*Attestation{},
		}
	}

	// Find unique peers
	uniquePeers := make(map[uuid.UUID]bool)
	var oldestProof int64 = time.Now().Unix()

	for _, att := range attestations {
		// Track unique peers
		if att.FromSystemID == systemID {
			uniquePeers[att.ToSystemID] = true
		} else {
			uniquePeers[att.FromSystemID] = true
		}

		// Find oldest proof
		if att.Timestamp < oldestProof {
			oldestProof = att.Timestamp
		}
	}

	proof := &AttestationProof{
		SystemID:     systemID,
		Attestations: attestations,
		TotalProofs:  len(attestations),
		OldestProof:  oldestProof,
		UniquePeers:  len(uniquePeers),
	}

	proof.VerificationScore = CalculateVerifiableReputation(proof)

	return proof
}

// GetVerifiableRank determines rank from verifiable score
func GetVerifiableRank(score float64) string {
	switch {
	case score >= 10000:
		return "Diamond"
	case score >= 5000:
		return "Platinum"
	case score >= 2000:
		return "Gold"
	case score >= 500:
		return "Silver"
	case score >= 100:
		return "Bronze"
	default:
		return "Unranked"
	}
}
