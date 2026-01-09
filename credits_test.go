package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// RANK TESTS
// =============================================================================

func TestGetRank(t *testing.T) {
	tests := []struct {
		name     string
		balance  int64
		wantRank string
	}{
		{"zero balance", 0, "Unranked"},
		{"just under bronze", 167, "Unranked"},
		{"exactly bronze", 168, "Bronze"},
		{"between bronze and silver", 500, "Bronze"},
		{"exactly silver", 720, "Silver"},
		{"between silver and gold", 1500, "Silver"},
		{"exactly gold", 2160, "Gold"},
		{"between gold and platinum", 3000, "Gold"},
		{"exactly platinum", 4320, "Platinum"},
		{"between platinum and diamond", 6000, "Platinum"},
		{"exactly diamond", 8640, "Diamond"},
		{"way above diamond", 100000, "Diamond"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRank(tt.balance)
			if got.Name != tt.wantRank {
				t.Errorf("GetRank(%d) = %s, want %s", tt.balance, got.Name, tt.wantRank)
			}
		})
	}
}

func TestGetNextRank(t *testing.T) {
	tests := []struct {
		name         string
		balance      int64
		wantNextRank string
		wantNeeded   int64
	}{
		{"unranked needs bronze", 0, "Bronze", 168},
		{"halfway to bronze", 84, "Bronze", 84},
		{"bronze needs silver", 168, "Silver", 552},
		{"silver needs gold", 720, "Gold", 1440},
		{"gold needs platinum", 2160, "Platinum", 2160},
		{"platinum needs diamond", 4320, "Diamond", 4320},
		{"at diamond max rank", 8640, "Diamond", 0},
		{"above diamond max rank", 10000, "Diamond", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRank, gotNeeded := GetNextRank(tt.balance)
			if gotRank.Name != tt.wantNextRank {
				t.Errorf("GetNextRank(%d) rank = %s, want %s", tt.balance, gotRank.Name, tt.wantNextRank)
			}
			if gotNeeded != tt.wantNeeded {
				t.Errorf("GetNextRank(%d) needed = %d, want %d", tt.balance, gotNeeded, tt.wantNeeded)
			}
		})
	}
}

// =============================================================================
// PIONEER BONUS TESTS
// =============================================================================

// TestCalculatePioneerBonus verifies the pioneer bonus formula from credits.go:
// - <20 nodes: 30% flat
// - 20-49 nodes: linear interpolation from 30% to 15%: 0.30 - ((size-20)/30)*0.15
// - 50-99 nodes: linear interpolation from 15% to 0%: 0.15 - ((size-50)/50)*0.15
// - 100+ nodes: 0%
func TestCalculatePioneerBonus(t *testing.T) {
	tests := []struct {
		name       string
		galaxySize int
		wantBonus  float64
		formula    string // Document how we calculated expected value
	}{
		{"tiny network (1 node)", 1, 0.30, "flat 30% for <20"},
		{"small network (10 nodes)", 10, 0.30, "flat 30% for <20"},
		{"at 19 nodes", 19, 0.30, "flat 30% for <20"},
		// Formula: 0.30 - ((size-20)/30)*0.15
		{"exactly 20 nodes", 20, 0.30, "0.30 - ((20-20)/30)*0.15 = 0.30"},
		{"21 nodes", 21, 0.295, "0.30 - ((21-20)/30)*0.15 = 0.30 - 0.005 = 0.295"},
		{"35 nodes", 35, 0.225, "0.30 - ((35-20)/30)*0.15 = 0.30 - 0.075 = 0.225"},
		{"49 nodes", 49, 0.155, "0.30 - ((49-20)/30)*0.15 = 0.30 - 0.145 = 0.155"},
		// Formula: 0.15 - ((size-50)/50)*0.15
		{"exactly 50 nodes", 50, 0.15, "0.15 - ((50-50)/50)*0.15 = 0.15"},
		{"51 nodes", 51, 0.147, "0.15 - ((51-50)/50)*0.15 = 0.15 - 0.003 = 0.147"},
		{"75 nodes", 75, 0.075, "0.15 - ((75-50)/50)*0.15 = 0.15 - 0.075 = 0.075"},
		{"99 nodes", 99, 0.003, "0.15 - ((99-50)/50)*0.15 = 0.15 - 0.147 = 0.003"},
		{"exactly 100 nodes", 100, 0.0, "0% for 100+"},
		{"large network (500 nodes)", 500, 0.0, "0% for 100+"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculatePioneerBonus(tt.galaxySize)
			// Use tight tolerance - these are exact formula results
			if math.Abs(got-tt.wantBonus) > 0.001 {
				t.Errorf("calculatePioneerBonus(%d) = %v, want %v\nFormula: %s",
					tt.galaxySize, got, tt.wantBonus, tt.formula)
			}
		})
	}
}

// TestCalculatePioneerBonus_WouldCatchBugs verifies our tests would catch common bugs
func TestCalculatePioneerBonus_WouldCatchBugs(t *testing.T) {
	// If someone accidentally used 25% instead of 30% for small networks,
	// our test would catch it
	result := calculatePioneerBonus(10)
	if math.Abs(result-0.25) < 0.001 {
		t.Error("BUG: Pioneer bonus should be 30% for small networks, not 25%")
	}

	// If someone forgot to cap at 0%, our test would catch it
	result = calculatePioneerBonus(150)
	if result < 0 {
		t.Error("BUG: Pioneer bonus went negative - should be capped at 0%")
	}
	if result > 0.001 {
		t.Error("BUG: Pioneer bonus should be 0% for networks >= 100 nodes")
	}
}

// =============================================================================
// BRIDGE SCORE TESTS
// =============================================================================

// TestCalculateBridgeScore verifies the bridge score formula from credits.go:
// score = dependencyRatio*0.6 + criticalRatio*0.4
// where:
//   - dependencyRatio = (peers with connectivity < avgNetworkConnectivity) / totalPeers
//   - criticalRatio = (peers with connectivity <= 2) / totalPeers
func TestCalculateBridgeScore(t *testing.T) {
	tests := []struct {
		name                   string
		myPeerCount            int
		peerConnectivity       []int
		avgNetworkConnectivity float64
		wantScore              float64
		calculation            string
	}{
		{
			name:                   "no peers",
			myPeerCount:            0,
			peerConnectivity:       []int{},
			avgNetworkConnectivity: 5.0,
			wantScore:              0.0,
			calculation:            "0 peers = 0 score",
		},
		{
			name:                   "all peers well connected (above avg, >2)",
			myPeerCount:            5,
			peerConnectivity:       []int{10, 8, 12, 9, 11}, // all > 5 avg, all > 2
			avgNetworkConnectivity: 5.0,
			wantScore:              0.0,
			calculation:            "0/5 dependent, 0/5 critical = 0*0.6 + 0*0.4 = 0",
		},
		{
			name:                   "all peers critically dependent (<=2 connections)",
			myPeerCount:            5,
			peerConnectivity:       []int{1, 2, 1, 2, 1}, // all <= 2, all < 5
			avgNetworkConnectivity: 5.0,
			wantScore:              1.0,
			calculation:            "5/5 dependent, 5/5 critical = 1.0*0.6 + 1.0*0.4 = 1.0",
		},
		{
			name:                   "mixed: 3 below avg, 1 critical",
			myPeerCount:            4,
			peerConnectivity:       []int{2, 4, 3, 10}, // 2,4,3 < 5 (3 dependent); only 2 is <=2 (1 critical)
			avgNetworkConnectivity: 5.0,
			wantScore:              0.55,
			calculation:            "3/4 dependent, 1/4 critical = 0.75*0.6 + 0.25*0.4 = 0.45 + 0.10 = 0.55",
		},
		{
			name:                   "half peers dependent and critical",
			myPeerCount:            4,
			peerConnectivity:       []int{1, 1, 10, 10}, // 1,1 < 5 (2 dependent); 1,1 <= 2 (2 critical)
			avgNetworkConnectivity: 5.0,
			wantScore:              0.50,
			calculation:            "2/4 dependent, 2/4 critical = 0.5*0.6 + 0.5*0.4 = 0.30 + 0.20 = 0.50",
		},
		{
			name:                   "all dependent but none critical",
			myPeerCount:            3,
			peerConnectivity:       []int{3, 4, 4}, // all < 5 (3 dependent); none <= 2 (0 critical)
			avgNetworkConnectivity: 5.0,
			wantScore:              0.60,
			calculation:            "3/3 dependent, 0/3 critical = 1.0*0.6 + 0*0.4 = 0.60",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateBridgeScore(tt.myPeerCount, tt.peerConnectivity, tt.avgNetworkConnectivity)
			// Exact formula, tight tolerance
			if math.Abs(got-tt.wantScore) > 0.001 {
				t.Errorf("CalculateBridgeScore() = %v, want %v\nCalculation: %s",
					got, tt.wantScore, tt.calculation)
			}
		})
	}
}

// TestCalculateBridgeScore_WouldCatchBugs proves our tests detect incorrect implementations
func TestCalculateBridgeScore_WouldCatchBugs(t *testing.T) {
	// Bug: If someone swapped the weights (0.4 for dependency, 0.6 for critical)
	// For case: 3/4 dependent, 1/4 critical
	// Correct: 0.75*0.6 + 0.25*0.4 = 0.55
	// Buggy:   0.75*0.4 + 0.25*0.6 = 0.45
	result := CalculateBridgeScore(4, []int{2, 4, 3, 10}, 5.0)
	if math.Abs(result-0.45) < 0.01 {
		t.Error("BUG: Weights appear swapped - dependency should be 0.6, critical 0.4")
	}

	// Bug: If critical threshold was 3 instead of 2
	// For connectivity [3, 3, 10, 10] with avg 5:
	// Correct (<=2 critical): 0/4 critical
	// Buggy (<=3 critical): 2/4 critical
	result = CalculateBridgeScore(4, []int{3, 3, 10, 10}, 5.0)
	// 2/4 dependent (3,3 < 5), 0/4 critical (3 > 2)
	// score = 0.5*0.6 + 0*0.4 = 0.30
	if math.Abs(result-0.30) > 0.01 {
		t.Errorf("BUG: Critical threshold might be wrong. Got %v, expected 0.30", result)
	}
}

// =============================================================================
// CREDIT CALCULATOR TESTS
// =============================================================================

func TestCreditCalculator_NoAttestations(t *testing.T) {
	cc := NewCreditCalculator()

	result := cc.CalculateEarnedCredits(CalculationInput{
		Attestations:    nil,
		PeerCount:       5,
		LastCalculation: time.Now().Add(-1 * time.Hour).Unix(),
	})

	if result.CreditsEarned != 0 {
		t.Errorf("Expected 0 credits with no attestations, got %v", result.CreditsEarned)
	}
}

func TestCreditCalculator_NoPeers(t *testing.T) {
	cc := NewCreditCalculator()

	result := cc.CalculateEarnedCredits(CalculationInput{
		Attestations:    []*Attestation{{Timestamp: time.Now().Unix()}},
		PeerCount:       0,
		LastCalculation: time.Now().Add(-1 * time.Hour).Unix(),
	})

	if result.CreditsEarned != 0 {
		t.Errorf("Expected 0 credits with no peers, got %v", result.CreditsEarned)
	}
}

func TestCreditCalculator_BaseCredits(t *testing.T) {
	cc := NewCreditCalculator()
	now := time.Now().Unix()
	oneHourAgo := now - 3600

	// Create attestations spanning one hour with enough density
	// With 5 peers and 4 expected attestations per peer per hour = 20 expected
	// We'll create 20 attestations to get ~100% uptime
	var attestations []*Attestation
	for i := 0; i < 20; i++ {
		attestations = append(attestations, createTestAttestation(
			oneHourAgo + int64(i*180), // Every 3 minutes
		))
	}

	result := cc.CalculateEarnedCredits(CalculationInput{
		Attestations:    attestations,
		PeerCount:       5,
		LastCalculation: oneHourAgo,
		LongevityStart:  oneHourAgo,
		BridgeScore:     0.0,
		GalaxySize:      100, // No pioneer bonus
		ReciprocityRatio: 0.0,
	})

	// Should earn close to 1 credit (base rate) for 1 hour
	// Allow some tolerance due to gap calculations
	if result.BaseCredits < 0.5 || result.BaseCredits > 1.5 {
		t.Errorf("Expected base credits ~1.0 for 1 hour, got %v", result.BaseCredits)
	}
}

func TestCreditCalculator_BonusesApplied(t *testing.T) {
	cc := NewCreditCalculator()
	now := time.Now().Unix()
	oneHourAgo := now - 3600
	twoWeeksAgo := now - (14 * 24 * 3600)

	var attestations []*Attestation
	for i := 0; i < 20; i++ {
		attestations = append(attestations, createTestAttestation(
			oneHourAgo + int64(i*180),
		))
	}

	result := cc.CalculateEarnedCredits(CalculationInput{
		Attestations:     attestations,
		PeerCount:        5,
		LastCalculation:  oneHourAgo,
		LongevityStart:   twoWeeksAgo,
		BridgeScore:      0.5,       // 50% bridge = 25% bonus
		GalaxySize:       10,        // Pioneer bonus = 30%
		ReciprocityRatio: 1.0,       // Full reciprocity = 5%
	})

	// Check individual bonuses
	if math.Abs(result.Bonuses.Bridge-0.25) > 0.01 {
		t.Errorf("Expected bridge bonus ~0.25, got %v", result.Bonuses.Bridge)
	}
	if math.Abs(result.Bonuses.Pioneer-0.30) > 0.01 {
		t.Errorf("Expected pioneer bonus ~0.30, got %v", result.Bonuses.Pioneer)
	}
	if math.Abs(result.Bonuses.Reciprocity-0.05) > 0.01 {
		t.Errorf("Expected reciprocity bonus ~0.05, got %v", result.Bonuses.Reciprocity)
	}
	// Longevity: 2 weeks = 0.02
	if result.Bonuses.Longevity < 0.01 || result.Bonuses.Longevity > 0.03 {
		t.Errorf("Expected longevity bonus ~0.02 for 2 weeks, got %v", result.Bonuses.Longevity)
	}

	// Total credits should be base * (1 + total_bonus)
	expectedMultiplier := 1.0 + result.Bonuses.Total
	expectedCredits := result.BaseCredits * expectedMultiplier
	if math.Abs(result.CreditsEarned-expectedCredits) > 0.01 {
		t.Errorf("Credits calculation mismatch: got %v, expected %v", result.CreditsEarned, expectedCredits)
	}
}

func TestCreditCalculator_LongevityBroken(t *testing.T) {
	cc := NewCreditCalculator()
	now := time.Now().Unix()

	// Create attestations with a 45-minute gap (> 30 min threshold)
	attestations := []*Attestation{
		createTestAttestation(now - 7200), // 2 hours ago
		createTestAttestation(now - 5400), // 1.5 hours ago
		// Gap of 45 minutes
		createTestAttestation(now - 2700), // 45 min ago (after 45 min gap)
		createTestAttestation(now - 1800), // 30 min ago
		createTestAttestation(now - 900),  // 15 min ago
		createTestAttestation(now),        // now
	}

	result := cc.CalculateEarnedCredits(CalculationInput{
		Attestations:    attestations,
		PeerCount:       1,
		LastCalculation: now - 7200,
		LongevityStart:  now - (30 * 24 * 3600), // 30 days ago
	})

	if !result.LongevityBroken {
		t.Error("Expected longevity to be broken due to 45-min gap")
	}

	// New longevity start should be after the gap
	if result.NewLongevityStart <= now-5400 || result.NewLongevityStart > now-2700 {
		t.Errorf("NewLongevityStart should be around the post-gap attestation, got %d", result.NewLongevityStart)
	}
}

func TestCreditCalculator_LongevityMaxBonus(t *testing.T) {
	cc := NewCreditCalculator()
	now := time.Now().Unix()
	oneHourAgo := now - 3600
	twoYearsAgo := now - (2 * 365 * 24 * 3600)

	var attestations []*Attestation
	for i := 0; i < 20; i++ {
		attestations = append(attestations, createTestAttestation(
			oneHourAgo + int64(i*180),
		))
	}

	result := cc.CalculateEarnedCredits(CalculationInput{
		Attestations:    attestations,
		PeerCount:       5,
		LastCalculation: oneHourAgo,
		LongevityStart:  twoYearsAgo, // Way over 1 year
		GalaxySize:      100,
	})

	// Longevity bonus should be capped at 0.52 (52%)
	if result.Bonuses.Longevity != 0.52 {
		t.Errorf("Expected longevity bonus capped at 0.52, got %v", result.Bonuses.Longevity)
	}
}

func TestCreditCalculator_MinUptimeRatio(t *testing.T) {
	cc := NewCreditCalculator()
	now := time.Now().Unix()
	oneHourAgo := now - 3600

	// With 5 peers expecting ~20 attestations/hour, provide only 5 (25% uptime)
	attestations := []*Attestation{
		createTestAttestation(oneHourAgo),
		createTestAttestation(oneHourAgo + 900),
		createTestAttestation(oneHourAgo + 1800),
		createTestAttestation(oneHourAgo + 2700),
		createTestAttestation(now),
	}

	result := cc.CalculateEarnedCredits(CalculationInput{
		Attestations:    attestations,
		PeerCount:       5,
		LastCalculation: oneHourAgo,
		GalaxySize:      100,
	})

	// Should earn 0 credits because uptime ratio < 50%
	if result.CreditsEarned != 0 {
		t.Errorf("Expected 0 credits with low uptime ratio, got %v", result.CreditsEarned)
	}
}

// =============================================================================
// CREDIT TRANSFER SIGNATURE TESTS
// =============================================================================

func TestCreditTransfer_SignAndVerify(t *testing.T) {
	// Create a test system with keys
	keys, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate keypair: %v", err)
	}

	system := &System{
		ID:   uuid.New(),
		Name: "TestSystem",
		Keys: keys,
	}

	// Create a transfer
	toSystemID := uuid.New()
	transfer := NewCreditTransfer(system, toSystemID, 100, "Test transfer")

	// Verify the transfer
	if !transfer.Verify() {
		t.Error("Transfer verification failed")
	}

	// Verify transfer fields
	if transfer.FromSystemID != system.ID {
		t.Error("FromSystemID mismatch")
	}
	if transfer.ToSystemID != toSystemID {
		t.Error("ToSystemID mismatch")
	}
	if transfer.Amount != 100 {
		t.Error("Amount mismatch")
	}
	if transfer.Memo != "Test transfer" {
		t.Error("Memo mismatch")
	}
}

func TestCreditTransfer_TamperedSignature(t *testing.T) {
	keys, _ := GenerateKeyPair()
	system := &System{
		ID:   uuid.New(),
		Name: "TestSystem",
		Keys: keys,
	}

	transfer := NewCreditTransfer(system, uuid.New(), 100, "Test")

	// Tamper with the amount
	transfer.Amount = 1000

	// Should fail verification
	if transfer.Verify() {
		t.Error("Tampered transfer should not verify")
	}
}

// =============================================================================
// ATTESTATION-BASED CREDIT CALCULATION TESTS
// =============================================================================

func TestCalculateCreditsFromAttestations_Empty(t *testing.T) {
	credits := CalculateCreditsFromAttestations(nil, uuid.New())
	if credits != 0 {
		t.Errorf("Expected 0 credits from empty attestations, got %d", credits)
	}
}

func TestCalculateCreditsFromAttestations_SelfAttestations(t *testing.T) {
	systemID := uuid.New()

	// Create self-attestations (should be ignored)
	keys, _ := GenerateKeyPair()
	attestations := []*Attestation{
		createSignedAttestation(systemID, systemID, keys), // self-attestation
	}

	credits := CalculateCreditsFromAttestations(attestations, systemID)
	if credits != 0 {
		t.Errorf("Self-attestations should not count, got %d credits", credits)
	}
}

func TestCalculateCreditsFromAttestations_ValidAttestations(t *testing.T) {
	systemID := uuid.New()
	otherID := uuid.New()
	now := time.Now().Unix()

	// Create valid attestations from another system spanning 3 hours
	keys, _ := GenerateKeyPair()
	attestations := []*Attestation{
		createSignedAttestationAt(otherID, systemID, keys, now-10800), // 3 hours ago
		createSignedAttestationAt(otherID, systemID, keys, now-7200),  // 2 hours ago
		createSignedAttestationAt(otherID, systemID, keys, now-3600),  // 1 hour ago
		createSignedAttestationAt(otherID, systemID, keys, now),       // now
	}

	credits := CalculateCreditsFromAttestations(attestations, systemID)

	// Should get ~3 credits for 3 hours of uptime
	if credits < 2 || credits > 4 {
		t.Errorf("Expected ~3 credits for 3-hour span, got %d", credits)
	}
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func createTestAttestation(timestamp int64) *Attestation {
	keys, _ := GenerateKeyPair()
	att := &Attestation{
		FromSystemID: uuid.New(),
		ToSystemID:   uuid.New(),
		Timestamp:    timestamp,
		MessageType:  "dht_ping",
		PublicKey:    base64.StdEncoding.EncodeToString(keys.PublicKey),
	}
	msg := att.GetSignableMessage()
	sig := ed25519.Sign(keys.PrivateKey, msg)
	att.Signature = base64.StdEncoding.EncodeToString(sig)
	return att
}

func createSignedAttestation(fromID, toID uuid.UUID, keys *KeyPair) *Attestation {
	return SignAttestation(fromID, toID, "dht_ping", keys.PrivateKey, keys.PublicKey)
}

func createSignedAttestationAt(fromID, toID uuid.UUID, keys *KeyPair, timestamp int64) *Attestation {
	// Create attestation with custom timestamp
	att := &Attestation{
		FromSystemID: fromID,
		ToSystemID:   toID,
		Timestamp:    timestamp,
		MessageType:  "dht_ping",
		PublicKey:    base64.StdEncoding.EncodeToString(keys.PublicKey),
	}
	// Sign it properly with the custom timestamp
	msg := att.GetSignableMessage()
	sig := ed25519.Sign(keys.PrivateKey, msg)
	att.Signature = base64.StdEncoding.EncodeToString(sig)
	return att
}
