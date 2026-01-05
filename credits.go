package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// STELLAR CREDITS SYSTEM
// =============================================================================
//
// Design principles:
// 1. SLOW PROGRESSION: Ranks change over weeks/months, not hours/days
// 2. NORMALIZED EARNING: All nodes earn at same rate regardless of peer count
// 3. TRANSFER-READY: Credits are a balance that can be sent to other systems
//
// Credit earning:
// - Base rate: 1 credit per hour of verified uptime (24/day, ~720/month)
// - Normalized: A node with 13 peers earns the same as one with 5
// - Attestations prove uptime, but don't directly equal credits
//
// Rank thresholds (designed for weeks/months progression):
// - Unranked:  0 credits       (new node)
// - Bronze:    168 credits     (~1 week)
// - Silver:    720 credits     (~1 month)
// - Gold:      2160 credits    (~3 months)
// - Platinum:  4320 credits    (~6 months)
// - Diamond:   8640 credits    (~1 year)
//
// =============================================================================

// CreditBalance represents a system's stellar credit balance
type CreditBalance struct {
	SystemID      uuid.UUID `json:"system_id"`
	Balance       int64     `json:"balance"`         // Current spendable credits
	TotalEarned   int64     `json:"total_earned"`    // Lifetime earned (for stats)
	TotalSent     int64     `json:"total_sent"`      // Lifetime sent to others
	TotalReceived int64     `json:"total_received"`  // Lifetime received from others
	LastUpdated   int64     `json:"last_updated"`    // Unix timestamp
}

// CreditRank represents the rank thresholds
type CreditRank struct {
	Name      string `json:"name"`
	Threshold int64  `json:"threshold"`
	Color     string `json:"color"` // For UI display
}

// Credit ranks with thresholds designed for slow progression
var CreditRanks = []CreditRank{
	{Name: "Diamond", Threshold: 8640, Color: "#b9f2ff"},   // ~1 year
	{Name: "Platinum", Threshold: 4320, Color: "#e5e4e2"}, // ~6 months
	{Name: "Gold", Threshold: 2160, Color: "#ffd700"},     // ~3 months
	{Name: "Silver", Threshold: 720, Color: "#c0c0c0"},    // ~1 month
	{Name: "Bronze", Threshold: 168, Color: "#cd7f32"},    // ~1 week
	{Name: "Unranked", Threshold: 0, Color: "#666666"},    // New
}

// GetRank returns the rank for a given credit balance
func GetRank(balance int64) CreditRank {
	for _, rank := range CreditRanks {
		if balance >= rank.Threshold {
			return rank
		}
	}
	return CreditRanks[len(CreditRanks)-1] // Unranked
}

// GetNextRank returns the next rank and credits needed
func GetNextRank(balance int64) (CreditRank, int64) {
	for i := len(CreditRanks) - 1; i >= 0; i-- {
		if balance < CreditRanks[i].Threshold {
			return CreditRanks[i], CreditRanks[i].Threshold - balance
		}
	}
	// Already at max rank
	return CreditRanks[0], 0
}

// =============================================================================
// NORMALIZED CREDIT CALCULATION
// =============================================================================

// CreditCalculator handles normalized credit earning
type CreditCalculator struct {
	// Credits earned per hour of verified uptime
	// This is the BASE RATE - same for all nodes regardless of peer count
	CreditsPerHour float64
	
	// Grace period for restarts/updates (no penalty for gaps up to this duration)
	GracePeriod time.Duration
	
	// Minimum uptime ratio to earn credits (after grace period consideration)
	MinUptimeRatio float64
}

// NewCreditCalculator creates a calculator with default settings
func NewCreditCalculator() *CreditCalculator {
	return &CreditCalculator{
		CreditsPerHour: 1.0,              // Base: 1 credit per hour
		GracePeriod:    15 * time.Minute, // 15 min grace for updates
		MinUptimeRatio: 0.5,              // Need 50%+ uptime to earn
	}
}

// CalculateEarnedCredits computes credits from attestations with normalization
// 
// We measure UPTIME, not raw attestation count.
// A node with more peers generates more attestations per hour, but that
// doesn't mean they've been online longer.
//
// Features:
// - Normalized earning (peer count doesn't matter)
// - 15-minute grace period for restarts/updates
// - Bridge bonus for critical connectivity
//
func (cc *CreditCalculator) CalculateEarnedCredits(
	attestations []*Attestation,
	peerCount int,
	lastCalculation int64,
	bridgeScore float64, // 0.0 to 1.0, how critical this node is for connectivity
) int64 {
	if len(attestations) == 0 || peerCount == 0 {
		return 0
	}

	// Find time bounds
	var oldest, newest int64 = attestations[0].Timestamp, attestations[0].Timestamp
	for _, att := range attestations {
		if att.Timestamp < oldest {
			oldest = att.Timestamp
		}
		if att.Timestamp > newest {
			newest = att.Timestamp
		}
	}

	// Only count time since last calculation
	if oldest < lastCalculation {
		oldest = lastCalculation
	}

	// Time span in hours
	spanSeconds := newest - oldest
	if spanSeconds <= 0 {
		return 0
	}
	spanHours := float64(spanSeconds) / 3600.0

	// Expected attestations per hour based on peer count
	// Liveness loop runs every 5 minutes = 12 times per hour
	// Each run pings all peers, so expected = peerCount * 12 per hour
	expectedPerHour := float64(peerCount) * 12.0

	// Count attestations and identify gaps for grace period
	actualCount := 0
	var timestamps []int64
	for _, att := range attestations {
		if att.Timestamp >= oldest && att.Verify() {
			actualCount++
			timestamps = append(timestamps, att.Timestamp)
		}
	}

	// Sort timestamps to find gaps
	sortInt64s(timestamps)
	
	// Calculate effective uptime considering grace period
	// Gaps under 15 minutes don't count against you
	gracePeriodSec := int64(cc.GracePeriod.Seconds())
	totalGapTime := int64(0)
	for i := 1; i < len(timestamps); i++ {
		gap := timestamps[i] - timestamps[i-1]
		// Expected gap is ~5 minutes (300 sec) between liveness checks
		// If gap is larger than grace period, count the excess as downtime
		if gap > gracePeriodSec {
			totalGapTime += gap - gracePeriodSec
		}
	}

	// Effective online time = span - excess gaps
	effectiveSeconds := spanSeconds - totalGapTime
	if effectiveSeconds < 0 {
		effectiveSeconds = 0
	}
	effectiveHours := float64(effectiveSeconds) / 3600.0

	// Calculate uptime ratio based on attestation density
	expectedTotal := expectedPerHour * spanHours
	uptimeRatio := float64(actualCount) / expectedTotal
	if uptimeRatio > 1.0 {
		uptimeRatio = 1.0
	}

	// Require minimum uptime to earn anything
	if uptimeRatio < cc.MinUptimeRatio {
		return 0
	}

	// Base credits = effective hours * rate * uptime_ratio
	baseCredits := effectiveHours * cc.CreditsPerHour * uptimeRatio

	// ==========================================================================
	// BRIDGE BONUS
	// ==========================================================================
	// Nodes that serve as critical bridges (connecting otherwise isolated clusters)
	// earn bonus credits. This rewards nodes that maintain connectivity when
	// other nodes go offline.
	//
	// bridgeScore ranges from 0.0 (not critical) to 1.0 (highly critical)
	// Bonus: up to 50% extra credits for being a critical bridge
	//
	bridgeBonus := baseCredits * bridgeScore * 0.5

	totalCredits := baseCredits + bridgeBonus

	return int64(totalCredits)
}

// sortInt64s sorts a slice of int64 in ascending order
func sortInt64s(a []int64) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// =============================================================================
// BRIDGE SCORE CALCULATION
// =============================================================================

// CalculateBridgeScore determines how critical a node is for network connectivity
// Returns 0.0 to 1.0 based on:
// - How many of your peers have few other connections
// - Whether you connect otherwise-isolated clusters
// - How long your peers have been relying on you
//
func CalculateBridgeScore(
	myPeerCount int,
	peerConnectivity []int, // How many connections each of your peers has
	avgNetworkConnectivity float64,
) float64 {
	if myPeerCount == 0 || len(peerConnectivity) == 0 {
		return 0.0
	}

	// Count peers that are "dependent" - have below-average connectivity
	dependentPeers := 0
	criticallyDependent := 0 // Peers with very few connections
	
	for _, peerConns := range peerConnectivity {
		if float64(peerConns) < avgNetworkConnectivity {
			dependentPeers++
		}
		if peerConns <= 2 {
			criticallyDependent++
		}
	}

	// Base score: what fraction of your peers are dependent on you
	dependencyRatio := float64(dependentPeers) / float64(len(peerConnectivity))
	
	// Critical multiplier: extra weight for peers with very few connections
	criticalRatio := float64(criticallyDependent) / float64(len(peerConnectivity))
	
	// Combined score (weighted average)
	// Critically dependent peers count more
	score := dependencyRatio*0.6 + criticalRatio*0.4
	
	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}
	
	return score
}

// =============================================================================
// CREDIT TRANSFERS (Future-ready structure)
// =============================================================================

// CreditTransfer represents a transfer of credits between systems
// This is designed for future implementation but the structure is ready
type CreditTransfer struct {
	ID            uuid.UUID `json:"id"`              // Unique transfer ID
	FromSystemID  uuid.UUID `json:"from_system_id"`  // Sender
	ToSystemID    uuid.UUID `json:"to_system_id"`    // Recipient
	Amount        int64     `json:"amount"`          // Credits transferred
	Timestamp     int64     `json:"timestamp"`       // When transfer occurred
	Memo          string    `json:"memo,omitempty"`  // Optional message
	Signature     string    `json:"signature"`       // Sender's signature
	PublicKey     string    `json:"public_key"`      // Sender's public key
}

// NewCreditTransfer creates a new signed credit transfer
func NewCreditTransfer(
	fromSystem *System,
	toSystemID uuid.UUID,
	amount int64,
	memo string,
) *CreditTransfer {
	transfer := &CreditTransfer{
		ID:           uuid.New(),
		FromSystemID: fromSystem.ID,
		ToSystemID:   toSystemID,
		Amount:       amount,
		Timestamp:    time.Now().Unix(),
		Memo:         memo,
		PublicKey:    base64.StdEncoding.EncodeToString(fromSystem.Keys.PublicKey),
	}

	// Sign the transfer
	transfer.Sign(fromSystem.Keys.PrivateKey)

	return transfer
}

// SignatureData returns the data that gets signed
func (t *CreditTransfer) SignatureData() []byte {
	data := fmt.Sprintf("%s:%s:%s:%d:%d:%s",
		t.ID.String(),
		t.FromSystemID.String(),
		t.ToSystemID.String(),
		t.Amount,
		t.Timestamp,
		t.Memo,
	)
	hash := sha256.Sum256([]byte(data))
	return hash[:]
}

// Sign signs the transfer with the sender's private key
func (t *CreditTransfer) Sign(privateKey ed25519.PrivateKey) {
	signature := ed25519.Sign(privateKey, t.SignatureData())
	t.Signature = base64.StdEncoding.EncodeToString(signature)
}

// Verify verifies the transfer signature
func (t *CreditTransfer) Verify() bool {
	pubKeyBytes, err := base64.StdEncoding.DecodeString(t.PublicKey)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return false
	}

	sigBytes, err := base64.StdEncoding.DecodeString(t.Signature)
	if err != nil {
		return false
	}

	return ed25519.Verify(pubKeyBytes, t.SignatureData(), sigBytes)
}

// ToJSON serializes the transfer for network transmission
func (t *CreditTransfer) ToJSON() ([]byte, error) {
	return json.Marshal(t)
}

// =============================================================================
// CREDIT PROOF (for verification by other nodes)
// =============================================================================

// CreditProof is a verifiable proof of a system's credit balance
// Other nodes can verify this without trusting the claimer
type CreditProof struct {
	SystemID        uuid.UUID         `json:"system_id"`
	Balance         int64             `json:"balance"`
	TotalEarned     int64             `json:"total_earned"`
	Rank            string            `json:"rank"`
	
	// Sampling of recent attestations for verification
	SampleAttestations []*Attestation `json:"sample_attestations"`
	
	// Recent transfers affecting balance
	RecentTransfers []*CreditTransfer `json:"recent_transfers,omitempty"`
	
	Timestamp       int64             `json:"timestamp"`
	Signature       string            `json:"signature"`
	PublicKey       string            `json:"public_key"`
}

// GenerateCreditProof creates a verifiable proof of credit balance
func GenerateCreditProof(system *System, balance *CreditBalance, sampleAttestations []*Attestation) *CreditProof {
	proof := &CreditProof{
		SystemID:           system.ID,
		Balance:            balance.Balance,
		TotalEarned:        balance.TotalEarned,
		Rank:               GetRank(balance.Balance).Name,
		SampleAttestations: sampleAttestations,
		Timestamp:          time.Now().Unix(),
		PublicKey:          base64.StdEncoding.EncodeToString(system.Keys.PublicKey),
	}

	// Sign the proof
	data := fmt.Sprintf("%s:%d:%d:%d",
		proof.SystemID.String(),
		proof.Balance,
		proof.TotalEarned,
		proof.Timestamp,
	)
	hash := sha256.Sum256([]byte(data))
	signature := ed25519.Sign(system.Keys.PrivateKey, hash[:])
	proof.Signature = base64.StdEncoding.EncodeToString(signature)

	return proof
}
