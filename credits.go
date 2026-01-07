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
// Bonuses (applied to base rate):
// - Bridge bonus: Up to +50% for being critical to network connectivity
// - Longevity bonus: +1% per week continuous uptime, max +52% at 1 year
//   (resets after 30 min downtime)
// - Pioneer bonus: Up to +30% when network is small (<20 nodes)
// - Reciprocity bonus: Up to +5% for healthy bidirectional peer relationships
//
// Grace period: 15 minutes - short gaps don't count as downtime
// Longevity reset: 30 minutes - longer gaps reset your streak
//
// Rank thresholds:
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
	SystemID        uuid.UUID `json:"system_id"`
	Balance         int64     `json:"balance"`           // Current spendable credits
	PendingCredits  float64   `json:"pending_credits"`   // Fractional credits carried forward
	TotalEarned     int64     `json:"total_earned"`      // Lifetime earned (for stats)
	TotalSent       int64     `json:"total_sent"`        // Lifetime sent to others
	TotalReceived   int64     `json:"total_received"`    // Lifetime received from others
	LastUpdated     int64     `json:"last_updated"`      // Unix timestamp
	LongevityStart  int64     `json:"longevity_start"`   // When current uptime streak began
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
	CreditsPerHour float64
	
	// Grace period for restarts/updates (no penalty for gaps up to this duration)
	GracePeriod time.Duration
	
	// Longevity reset threshold (gaps longer than this reset your streak)
	LongevityResetThreshold time.Duration
	
	// Minimum uptime ratio to earn credits (after grace period consideration)
	MinUptimeRatio float64
}

// NewCreditCalculator creates a calculator with default settings
func NewCreditCalculator() *CreditCalculator {
	return &CreditCalculator{
		CreditsPerHour:          1.0,              // Base: 1 credit per hour
		GracePeriod:             15 * time.Minute, // 15 min grace for updates
		LongevityResetThreshold: 30 * time.Minute, // 30 min gap resets streak
		MinUptimeRatio:          0.5,              // Need 50%+ uptime to earn
	}
}

// CreditBonuses holds the individual bonus multipliers for transparency
type CreditBonuses struct {
	Bridge     float64 `json:"bridge"`     // 0.0 to 0.50
	Longevity  float64 `json:"longevity"`  // 0.0 to 0.52
	Pioneer    float64 `json:"pioneer"`    // 0.0 to 0.30
	Reciprocity float64 `json:"reciprocity"` // 0.0 to 0.05
	Total      float64 `json:"total"`      // Sum of all bonuses
}

// CalculationInput holds all inputs for credit calculation
type CalculationInput struct {
	Attestations     []*Attestation
	PeerCount        int
	LastCalculation  int64
	LongevityStart   int64   // When current uptime streak began
	BridgeScore      float64 // 0.0 to 1.0
	GalaxySize       int     // Total nodes in network
	ReciprocityRatio float64 // 0.0 to 1.0, fraction of peers that attest back
}

// CalculationResult holds the result with breakdown
type CalculationResult struct {
	CreditsEarned     float64       `json:"credits_earned"`     // Now float64 for fractional credits
	BaseCredits       float64       `json:"base_credits"`
	Bonuses           CreditBonuses `json:"bonuses"`
	LongevityBroken   bool          `json:"longevity_broken"`   // True if streak was reset
	NewLongevityStart int64         `json:"new_longevity_start"`
}

// CalculateEarnedCredits computes credits with all bonuses
func (cc *CreditCalculator) CalculateEarnedCredits(input CalculationInput) CalculationResult {
	result := CalculationResult{
		NewLongevityStart: input.LongevityStart,
	}

	if len(input.Attestations) == 0 || input.PeerCount == 0 {
		return result
	}

	// Find time bounds
	var oldest, newest int64 = input.Attestations[0].Timestamp, input.Attestations[0].Timestamp
	for _, att := range input.Attestations {
		if att.Timestamp < oldest {
			oldest = att.Timestamp
		}
		if att.Timestamp > newest {
			newest = att.Timestamp
		}
	}

	// Only count time since last calculation
	if oldest < input.LastCalculation {
		oldest = input.LastCalculation
	}

	// Time span
	spanSeconds := newest - oldest
	if spanSeconds <= 0 {
		return result
	}
	spanHours := float64(spanSeconds) / 3600.0

	// Expected attestations per hour based on peer count
	// Peers send: liveness pings (~1 per 5min = 12/hr, but only when THEY check us)
	// Plus announces (~2/hr) and occasional find_node requests
	// Realistically expect ~4-6 attestations per peer per hour
	expectedPerHour := float64(input.PeerCount) * 4.0

	// Count attestations and collect timestamps for gap analysis
	actualCount := 0
	var timestamps []int64
	for _, att := range input.Attestations {
		if att.Timestamp >= oldest && att.Verify() {
			actualCount++
			timestamps = append(timestamps, att.Timestamp)
		}
	}

	// Sort timestamps to find gaps
	sortInt64s(timestamps)

	// Check for longevity-breaking gaps (>30 min)
	gracePeriodSec := int64(cc.GracePeriod.Seconds())
	longevityResetSec := int64(cc.LongevityResetThreshold.Seconds())
	totalGapTime := int64(0)
	
	for i := 1; i < len(timestamps); i++ {
		gap := timestamps[i] - timestamps[i-1]
		
		// Check if this gap breaks longevity streak
		if gap > longevityResetSec {
			result.LongevityBroken = true
			result.NewLongevityStart = timestamps[i] // Streak restarts from here
		}
		
		// Count excess gap time beyond grace period
		if gap > gracePeriodSec {
			totalGapTime += gap - gracePeriodSec
		}
	}

	// If no longevity start set, start now
	if result.NewLongevityStart == 0 {
		if input.LongevityStart == 0 {
			result.NewLongevityStart = oldest
		}
	}

	// Effective online time
	effectiveSeconds := spanSeconds - totalGapTime
	if effectiveSeconds < 0 {
		effectiveSeconds = 0
	}
	effectiveHours := float64(effectiveSeconds) / 3600.0

	// Calculate uptime ratio
	expectedTotal := expectedPerHour * spanHours
	uptimeRatio := float64(actualCount) / expectedTotal
	if uptimeRatio > 1.0 {
		uptimeRatio = 1.0
	}

	// Require minimum uptime
	if uptimeRatio < cc.MinUptimeRatio {
		return result
	}

	// ==========================================================================
	// BASE CREDITS
	// ==========================================================================
	result.BaseCredits = effectiveHours * cc.CreditsPerHour * uptimeRatio

	// ==========================================================================
	// BONUSES
	// ==========================================================================

	// 1. BRIDGE BONUS - Up to +50% for network criticality
	result.Bonuses.Bridge = input.BridgeScore * 0.50

	// 2. LONGEVITY BONUS - +1% per week, max +52% at 1 year
	longevityWeeks := float64(0)
	if !result.LongevityBroken && result.NewLongevityStart > 0 {
		longevitySeconds := newest - result.NewLongevityStart
		longevityWeeks = float64(longevitySeconds) / (7 * 24 * 3600)
	}
	result.Bonuses.Longevity = min(longevityWeeks * 0.01, 0.52)

	// 3. PIONEER BONUS - Up to +30% for small networks
	result.Bonuses.Pioneer = calculatePioneerBonus(input.GalaxySize)

	// 4. RECIPROCITY BONUS - Up to +5% for bidirectional relationships
	result.Bonuses.Reciprocity = input.ReciprocityRatio * 0.05

	// Total bonus multiplier
	result.Bonuses.Total = result.Bonuses.Bridge + 
		result.Bonuses.Longevity + 
		result.Bonuses.Pioneer + 
		result.Bonuses.Reciprocity

	// Apply bonuses - return full float64 value (no truncation here)
	result.CreditsEarned = result.BaseCredits * (1.0 + result.Bonuses.Total)

	return result
}

// calculatePioneerBonus returns bonus for small network participation
// +30% at <20 nodes, +15% at <50 nodes, +5% at <100 nodes, 0% at 100+
func calculatePioneerBonus(galaxySize int) float64 {
	if galaxySize < 20 {
		return 0.30
	} else if galaxySize < 50 {
		// Linear interpolation from 30% to 15%
		return 0.30 - (float64(galaxySize-20)/30.0)*0.15
	} else if galaxySize < 100 {
		// Linear interpolation from 15% to 0%
		return 0.15 - (float64(galaxySize-50)/50.0)*0.15
	}
	return 0.0
}

// min returns the smaller of two float64 values
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
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
// Includes a proof that allows recipients to verify the sender has sufficient balance
type CreditTransfer struct {
	ID            uuid.UUID     `json:"id"`              // Unique transfer ID
	FromSystemID  uuid.UUID     `json:"from_system_id"`  // Sender
	ToSystemID    uuid.UUID     `json:"to_system_id"`    // Recipient
	Amount        int64         `json:"amount"`          // Credits transferred
	Timestamp     int64         `json:"timestamp"`       // When transfer occurred
	Memo          string        `json:"memo,omitempty"`  // Optional message
	Signature     string        `json:"signature"`       // Sender's signature
	PublicKey     string        `json:"public_key"`      // Sender's public key
	Proof         *CreditProof  `json:"proof"`           // Proof of sufficient balance
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

// CreditProof is a verifiable proof of earned credits
// Recipients can verify this by recalculating from the included attestations
type CreditProof struct {
	SystemID     uuid.UUID      `json:"system_id"`
	ClaimedTotal int64          `json:"claimed_total"`   // Total credits claimed to have earned
	AsOfTime     int64          `json:"as_of_time"`      // Timestamp this proof was generated
	
	// Attestations proving uptime - these are signed by OTHER nodes
	// Only attestations where ToSystemID == SystemID count (others attesting TO us)
	Attestations []*Attestation `json:"attestations"`
	
	// Previous transfers that affect available balance
	PriorTransfersSent int64 `json:"prior_transfers_sent"` // Total previously sent
	
	Signature string `json:"signature"` // Proof signed by the claimer
	PublicKey string `json:"public_key"`
}

// ProofHash returns a hash of the proof for reference/storage
func (p *CreditProof) ProofHash() string {
	data := fmt.Sprintf("%s:%d:%d:%d",
		p.SystemID.String(),
		p.ClaimedTotal,
		p.AsOfTime,
		len(p.Attestations),
	)
	hash := sha256.Sum256([]byte(data))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// GenerateCreditProof creates a verifiable proof of credit balance
// Includes attestations sufficient to prove the claimed amount
func GenerateCreditProof(
	system *System,
	claimedTotal int64,
	priorSent int64,
	attestations []*Attestation,
) *CreditProof {
	proof := &CreditProof{
		SystemID:           system.ID,
		ClaimedTotal:       claimedTotal,
		AsOfTime:           time.Now().Unix(),
		Attestations:       attestations,
		PriorTransfersSent: priorSent,
		PublicKey:          base64.StdEncoding.EncodeToString(system.Keys.PublicKey),
	}

	// Sign the proof
	data := fmt.Sprintf("%s:%d:%d:%d",
		proof.SystemID.String(),
		proof.ClaimedTotal,
		proof.AsOfTime,
		len(proof.Attestations),
	)
	hash := sha256.Sum256([]byte(data))
	signature := ed25519.Sign(system.Keys.PrivateKey, hash[:])
	proof.Signature = base64.StdEncoding.EncodeToString(signature)

	return proof
}

// =============================================================================
// TRANSFER VALIDATION
// =============================================================================

// CalculateCreditsFromAttestations deterministically calculates earned credits
// This is a simplified calculation that any node can verify:
// - 1 credit per hour of attested uptime
// - Only counts attestations FROM others TO the system (not self-attestations)
// - No bonuses (they depend on network state at calculation time)
func CalculateCreditsFromAttestations(attestations []*Attestation, systemID uuid.UUID) int64 {
	if len(attestations) == 0 {
		return 0
	}

	// Filter to only attestations TO this system (from others)
	var validAttestations []*Attestation
	for _, att := range attestations {
		// Must be TO us, not FROM us
		if att.ToSystemID != systemID {
			continue
		}
		// Must be FROM someone else
		if att.FromSystemID == systemID {
			continue
		}
		// Must have valid signature
		if !att.Verify() {
			continue
		}
		validAttestations = append(validAttestations, att)
	}

	if len(validAttestations) == 0 {
		return 0
	}

	// Find time bounds
	var oldest, newest int64 = validAttestations[0].Timestamp, validAttestations[0].Timestamp
	for _, att := range validAttestations {
		if att.Timestamp < oldest {
			oldest = att.Timestamp
		}
		if att.Timestamp > newest {
			newest = att.Timestamp
		}
	}

	// Calculate hours of uptime
	// Simple model: if you have attestations spanning N hours, you were online N hours
	spanSeconds := newest - oldest
	if spanSeconds <= 0 {
		// Single attestation = 1 hour minimum
		return 1
	}

	// 1 credit per hour
	hours := spanSeconds / 3600
	if hours < 1 {
		hours = 1
	}

	return hours
}

// ValidateTransferProof validates that a transfer has sufficient proven balance
// Returns nil if valid, error describing the problem if invalid
func ValidateTransferProof(transfer *CreditTransfer, knownTransfers []*CreditTransfer) error {
	// 1. Verify transfer signature
	if !transfer.Verify() {
		return fmt.Errorf("invalid transfer signature")
	}

	// 2. Verify proof exists
	if transfer.Proof == nil {
		return fmt.Errorf("missing credit proof")
	}

	// 3. Verify proof is for the sender
	if transfer.Proof.SystemID != transfer.FromSystemID {
		return fmt.Errorf("proof system ID doesn't match sender")
	}

	// 4. Verify all attestations in proof are from OTHER nodes (not self-signed)
	for _, att := range transfer.Proof.Attestations {
		// Must be TO the sender (proving they received attestation)
		if att.ToSystemID != transfer.FromSystemID {
			continue // Skip, doesn't help their case
		}
		// Must be FROM someone else
		if att.FromSystemID == transfer.FromSystemID {
			return fmt.Errorf("proof contains self-attestation")
		}
		// Must have valid signature from the other node
		if !att.Verify() {
			return fmt.Errorf("proof contains invalid attestation signature")
		}
	}

	// 5. Recalculate earned credits from attestations
	calculatedEarned := CalculateCreditsFromAttestations(
		transfer.Proof.Attestations,
		transfer.FromSystemID,
	)

	if transfer.Proof.ClaimedTotal > calculatedEarned {
		return fmt.Errorf("claimed earnings (%d) exceed provable amount (%d)",
			transfer.Proof.ClaimedTotal, calculatedEarned)
	}

	// 6. Calculate total previously sent (from known transfers)
	var totalPreviouslySent int64
	for _, t := range knownTransfers {
		if t.FromSystemID == transfer.FromSystemID && t.ID != transfer.ID {
			totalPreviouslySent += t.Amount
		}
	}

	// Also add what they claim to have sent (we trust proof for now)
	if transfer.Proof.PriorTransfersSent > totalPreviouslySent {
		totalPreviouslySent = transfer.Proof.PriorTransfersSent
	}

	// 7. Calculate total received (from known transfers)
	var totalReceived int64
	for _, t := range knownTransfers {
		if t.ToSystemID == transfer.FromSystemID {
			totalReceived += t.Amount
		}
	}

	// 8. Check balance: earned + received >= sent + this_transfer
	availableBalance := calculatedEarned + totalReceived - totalPreviouslySent
	if transfer.Amount > availableBalance {
		return fmt.Errorf("insufficient balance: need %d, have %d (earned %d + received %d - sent %d)",
			transfer.Amount, availableBalance,
			calculatedEarned, totalReceived, totalPreviouslySent)
	}

	return nil
}

// BuildMinimalProof creates a proof with just enough attestations to cover the transfer amount
// This keeps proof sizes manageable for large histories
func BuildMinimalProof(
	system *System,
	amount int64,
	priorSent int64,
	allAttestations []*Attestation,
) *CreditProof {
	// Sort attestations by timestamp (newest first for freshest proof)
	sorted := make([]*Attestation, len(allAttestations))
	copy(sorted, allAttestations)
	
	// Simple sort - newest first
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Timestamp > sorted[i].Timestamp {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Include attestations until we have enough to prove the needed balance
	needed := amount + priorSent
	var included []*Attestation
	var running int64

	for _, att := range sorted {
		// Only count attestations TO us from others
		if att.ToSystemID != system.ID || att.FromSystemID == system.ID {
			continue
		}
		if !att.Verify() {
			continue
		}

		included = append(included, att)
		running = CalculateCreditsFromAttestations(included, system.ID)

		if running >= needed {
			break
		}
	}

	return GenerateCreditProof(system, running, priorSent, included)
}