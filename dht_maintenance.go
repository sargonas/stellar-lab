package main

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"
)

const (
	// LivenessSampleSize is the max number of peers to check per liveness cycle
	// At 5-min intervals, this keeps network overhead reasonable at any scale
	// With 50 peers/cycle, a 20K node network takes ~33 hours to fully cycle
	// But organic contact (announces, FIND_NODE) provides additional verification
	LivenessSampleSize = 50
)

// announceLoop periodically announces our presence to the network
func (dht *DHT) announceLoop() {
	defer dht.wg.Done()

	// Initial announce after short delay
	time.Sleep(10 * time.Second)
	dht.announceToNetwork()

	ticker := time.NewTicker(AnnounceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dht.shutdown:
			return
		case <-ticker.C:
			dht.announceToNetwork()
		}
	}
}

// peerLivenessLoop periodically checks if routing table peers are still alive
// This runs more frequently than announce to maintain connections
func (dht *DHT) peerLivenessLoop() {
	defer dht.wg.Done()

	// Wait for initial bootstrap
	time.Sleep(30 * time.Second)

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-dht.shutdown:
			return
		case <-ticker.C:
			dht.checkPeerLiveness()
		}
	}
}

// checkPeerLiveness pings a sample of peers to verify they're alive
// Uses sampling to scale to large networks (20K+ nodes)
// Organic contact (announces, FIND_NODE responses) provides additional verification
func (dht *DHT) checkPeerLiveness() {
	allNodes := dht.routingTable.GetAllRoutingTableNodes()
	if len(allNodes) == 0 {
		return
	}

	// Sample peers if we have more than LivenessSampleSize
	var nodes []*System
	if len(allNodes) <= LivenessSampleSize {
		nodes = allNodes
	} else {
		// Random sample without replacement
		nodes = make([]*System, LivenessSampleSize)
		perm := rand.Perm(len(allNodes))
		for i := 0; i < LivenessSampleSize; i++ {
			nodes[i] = allNodes[perm[i]]
		}
	}

	log.Printf("Checking liveness of %d peers (of %d total)...", len(nodes), len(allNodes))

	alive := 0
	dead := 0

	for _, sys := range nodes {
		if sys.PeerAddress == "" {
			continue
		}

		// Ping and announce in one go
		if err := dht.PingNode(sys); err != nil {
			dead++
		} else {
			// Also announce ourselves so they know we're alive
			dht.AnnounceToSystem(sys)
			alive++
		}
	}

	if dead > 0 {
		log.Printf("Peer liveness: %d alive, %d unreachable", alive, dead)
	}

	// Evict nodes that have failed too many times
	evicted := dht.routingTable.EvictDeadNodes()
	if evicted > 0 {
		log.Printf("Evicted %d dead nodes from routing table", evicted)
	}
}

// announceToNetwork announces ourselves to the K closest nodes
func (dht *DHT) announceToNetwork() {
	log.Printf("Announcing presence to network...")

	// Find K closest nodes to ourselves
	result := dht.FindNode(dht.localSystem.ID)

	announced := 0
	for _, sys := range result.ClosestNodes {
		if sys.ID == dht.localSystem.ID {
			continue // Don't announce to ourselves
		}
		if sys.PeerAddress == "" {
			continue
		}

		if err := dht.AnnounceToSystem(sys); err != nil {
			log.Printf("  Failed to announce to %s: %v", sys.Name, err)
			dht.routingTable.MarkFailed(sys.ID)
		} else {
			announced++
			dht.routingTable.MarkVerified(sys.ID)
		}
	}

	log.Printf("Announced to %d nodes", announced)
}

// cacheMaintenanceLoop periodically prunes the system cache
func (dht *DHT) cacheMaintenanceLoop() {
	defer dht.wg.Done()

	ticker := time.NewTicker(CachePruneInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dht.shutdown:
			return
		case <-ticker.C:
			dht.pruneCache()
		}
	}
}

// gossipValidationLoop periodically verifies systems learned via gossip
// This prevents "ghost" systems from persisting - systems that were mentioned
// in gossip but are actually dead or never existed.
func (dht *DHT) gossipValidationLoop() {
	defer dht.wg.Done()

	// Wait for initial bootstrap before validating
	time.Sleep(2 * time.Minute)

	// Run every 10 minutes - validate a batch of unverified systems
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-dht.shutdown:
			return
		case <-ticker.C:
			dht.validateGossipSystems()
		}
	}
}

// validateGossipSystems attempts to verify systems that were learned via gossip
// but never directly contacted. This closes the loop on gossip propagation.
func (dht *DHT) validateGossipSystems() {
	unverified := dht.routingTable.GetUnverifiedCachedSystems()
	if len(unverified) == 0 {
		return
	}

	// Validate up to 10 systems per cycle to avoid flooding
	maxValidate := 10
	if len(unverified) < maxValidate {
		maxValidate = len(unverified)
	}

	log.Printf("Validating %d of %d unverified gossip systems...", maxValidate, len(unverified))

	verified := 0
	removed := 0

	for i := 0; i < maxValidate; i++ {
		sys := unverified[i]
		if sys.PeerAddress == "" {
			// Can't verify without address - remove from cache
			dht.routingTable.RemoveFromCache(sys.ID)
			removed++
			continue
		}

		// Try to ping the system directly
		_, err := dht.Ping(sys.PeerAddress)
		if err != nil {
			// Failed to contact - check if this is the expected system
			// The Ping function already handles UUID mismatches
			// For now, just mark that we tried (fail count is tracked elsewhere)
			log.Printf("  %s (%s): unreachable - %v", sys.Name, sys.ID.String()[:8], err)

			// If we've never verified this system and it's unreachable,
			// remove it from cache to prevent ghost propagation
			cached := dht.routingTable.GetCachedSystemMeta(sys.ID)
			if cached != nil && cached.LastVerified.IsZero() {
				// Never verified at all - likely a ghost, remove it
				dht.routingTable.RemoveFromCache(sys.ID)
				removed++
			}
		} else {
			verified++
			log.Printf("  %s: verified", sys.Name)
		}
	}

	if verified > 0 || removed > 0 {
		log.Printf("Gossip validation: %d verified, %d removed as ghosts", verified, removed)
	}
}

// pruneCache removes stale entries from the system cache and storage
func (dht *DHT) pruneCache() {
	// Prune in-memory cache
	pruned := dht.routingTable.PruneCache(CacheMaxAge)
	if pruned > 0 {
		log.Printf("Pruned %d stale entries from system cache", pruned)
	}

	// Prune peer_systems table
	prunedSystems, err := dht.storage.PrunePeerSystems(CacheMaxAge)
	if err != nil {
		log.Printf("Error pruning peer systems: %v", err)
	} else if prunedSystems > 0 {
		log.Printf("Pruned %d stale entries from peer_systems table", prunedSystems)
	}

	// Prune peer_connections table
	prunedConns, err := dht.storage.PrunePeerConnections(CacheMaxAge)
	if err != nil {
		log.Printf("Error pruning peer connections: %v", err)
	} else if prunedConns > 0 {
		log.Printf("Pruned %d stale entries from peer_connections table", prunedConns)
	}
}

// GetNetworkStats returns statistics about the DHT network
func (dht *DHT) GetNetworkStats() map[string]interface{} {
	rtSize := dht.routingTable.GetRoutingTableSize()
	cacheSize := dht.routingTable.GetCacheSize()
	unverifiedCount := dht.routingTable.GetUnverifiedCount()

	return map[string]interface{}{
		"local_id":           dht.localSystem.ID.String(),
		"local_name":         dht.localSystem.Name,
		"routing_table_size": rtSize,
		"cache_size":         cacheSize,
		"verified_peers":     rtSize,
		"unverified_peers":   unverifiedCount,
		"max_peers":          MaxPeers,
	}
}

// =============================================================================
// STELLAR CREDITS CALCULATION
// =============================================================================

// creditCalculationLoop periodically calculates earned credits
func (dht *DHT) creditCalculationLoop() {
	defer dht.wg.Done()

	// Calculate every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Initial calculation after 5 minutes
	select {
	case <-dht.shutdown:
		return
	case <-time.After(5 * time.Minute):
		dht.calculateCredits()
	}

	for {
		select {
		case <-dht.shutdown:
			return
		case <-ticker.C:
			dht.calculateCredits()
		}
	}
}

// calculateCredits computes and stores earned credits based on attestations
func (dht *DHT) calculateCredits() {
	log.Printf("Calculating stellar credits...")

	// Get current balance
	balance, err := dht.storage.GetCreditBalance(dht.localSystem.ID)
	if err != nil {
		log.Printf("  ERROR: Failed to get credit balance: %v", err)
		return
	}

	log.Printf("  Current state: balance=%d, pending=%.3f, last_calculated=%d, longevity_start=%d",
		balance.Balance, balance.PendingCredits, balance.LastUpdated, balance.LongevityStart)

	// Get attestations since last calculation
	attestations, err := dht.storage.GetAttestationsSince(dht.localSystem.ID, balance.LastUpdated)
	if err != nil {
		log.Printf("  ERROR: Failed to get attestations: %v", err)
		return
	}

	log.Printf("  Found %d attestations since last calculation", len(attestations))

	if len(attestations) == 0 {
		log.Printf("  No new attestations - skipping calculation")
		return
	}

	// Get current peer count for normalization
	peerCount := dht.routingTable.GetRoutingTableSize()
	if peerCount == 0 {
		peerCount = 1 // Avoid division by zero
	}

	log.Printf("  Current peer count: %d", peerCount)

	// Calculate inputs for credit calculation
	bridgeScore := dht.calculateBridgeScore()
	galaxySize := dht.routingTable.GetCacheSize() + 1 // +1 for self
	reciprocityRatio := dht.calculateReciprocityRatio(attestations)

	log.Printf("  Inputs: bridge_score=%.3f, galaxy_size=%d, reciprocity=%.3f",
		bridgeScore, galaxySize, reciprocityRatio)

	// Build calculation input
	input := CalculationInput{
		Attestations:     attestations,
		PeerCount:        peerCount,
		LastCalculation:  balance.LastUpdated,
		LongevityStart:   balance.LongevityStart,
		BridgeScore:      bridgeScore,
		GalaxySize:       galaxySize,
		ReciprocityRatio: reciprocityRatio,
	}

	// Calculate earned credits with all bonuses
	calculator := NewCreditCalculator()
	result := calculator.CalculateEarnedCredits(input)

	log.Printf("  Calculation result: earned=%.3f, base=%.3f",
		result.CreditsEarned, result.BaseCredits)

	if result.CreditsEarned > 0 || result.BaseCredits > 0 {
		// Add earned credits to pending balance
		pending := balance.PendingCredits + result.CreditsEarned
		
		// Extract whole credits
		wholeCredits := int64(pending)
		
		// Keep fractional part for next time
		balance.PendingCredits = pending - float64(wholeCredits)
		
		// Update balance with whole credits
		balance.Balance += wholeCredits
		balance.TotalEarned += wholeCredits
		balance.LastUpdated = time.Now().Unix()
		balance.LongevityStart = result.NewLongevityStart

		if err := dht.storage.SaveCreditBalance(balance); err != nil {
			log.Printf("  ERROR: Failed to save credit balance: %v", err)
			return
		}

		rank := GetRank(balance.Balance)

		// Build bonus summary for logging
		bonusParts := []string{}
		if result.Bonuses.Bridge > 0.001 {
			bonusParts = append(bonusParts, fmt.Sprintf("bridge:+%.0f%%", result.Bonuses.Bridge*100))
		}
		if result.Bonuses.Longevity > 0.001 {
			bonusParts = append(bonusParts, fmt.Sprintf("longevity:+%.0f%%", result.Bonuses.Longevity*100))
		}
		if result.Bonuses.Pioneer > 0.001 {
			bonusParts = append(bonusParts, fmt.Sprintf("pioneer:+%.0f%%", result.Bonuses.Pioneer*100))
		}
		if result.Bonuses.Reciprocity > 0.001 {
			bonusParts = append(bonusParts, fmt.Sprintf("reciprocity:+%.1f%%", result.Bonuses.Reciprocity*100))
		}

		if wholeCredits > 0 {
			if len(bonusParts) > 0 {
				log.Printf("  ✦ Earned %d stellar credits [%s] (total: %d, rank: %s, pending: %.3f)",
					wholeCredits, strings.Join(bonusParts, ", "), balance.Balance, rank.Name, balance.PendingCredits)
			} else {
				log.Printf("  ✦ Earned %d stellar credits (total: %d, rank: %s, pending: %.3f)",
					wholeCredits, balance.Balance, rank.Name, balance.PendingCredits)
			}
		} else {
			// No whole credits this cycle, but fractional credits accumulated
			log.Printf("  Accumulated %.3f credits (pending: %.3f, need %.3f more for next credit)",
				result.CreditsEarned, balance.PendingCredits, 1.0-balance.PendingCredits)
		}

		if result.LongevityBroken {
			log.Printf("  Longevity streak reset due to >30min gap")
		}
	} else {
		log.Printf("  No credits earned this cycle (base=%.2f)",
			result.BaseCredits)
	}
}

// calculateBridgeScore determines how critical this node is for network connectivity
func (dht *DHT) calculateBridgeScore() float64 {
	peers := dht.routingTable.GetAllRoutingTableNodes()
	if len(peers) == 0 {
		return 0.0
	}

	// For each peer, estimate their connectivity
	peerConnectivity := make([]int, 0, len(peers))
	totalConnectivity := 0

	for _, peer := range peers {
		// Estimate peer's connectivity based on their max peers (star class)
		estimatedConns := peer.GetMaxPeers() / 2
		if estimatedConns < 1 {
			estimatedConns = 1
		}
		peerConnectivity = append(peerConnectivity, estimatedConns)
		totalConnectivity += estimatedConns
	}

	avgConnectivity := float64(totalConnectivity) / float64(len(peers))
	if avgConnectivity < 1 {
		avgConnectivity = 1
	}

	return CalculateBridgeScore(len(peers), peerConnectivity, avgConnectivity)
}

// calculateReciprocityRatio determines what fraction of our peers attest back to us
func (dht *DHT) calculateReciprocityRatio(attestations []*Attestation) float64 {
	peers := dht.routingTable.GetAllRoutingTableNodes()
	if len(peers) == 0 {
		return 0.0
	}

	// Build set of peers we've heard FROM
	// These attestations were already filtered by GetAttestationsSince to ones
	// we received (via received_by column), so just track who sent them
	heardFrom := make(map[string]bool)

	for _, att := range attestations {
		heardFrom[att.FromSystemID.String()] = true
	}

	// Count how many of our routing table peers have attested to us
	reciprocal := 0
	for _, peer := range peers {
		if heardFrom[peer.ID.String()] {
			reciprocal++
		}
	}

	return float64(reciprocal) / float64(len(peers))
}