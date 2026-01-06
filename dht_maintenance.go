package main

import (
	"fmt"
	"log"
	"strings"
	"time"
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

// checkPeerLiveness pings all routing table peers to verify they're alive
// and announces ourselves to them
func (dht *DHT) checkPeerLiveness() {
	nodes := dht.routingTable.GetAllRoutingTableNodes()
	if len(nodes) == 0 {
		return
	}

	log.Printf("Checking liveness of %d peers...", len(nodes))
	
	alive := 0
	dead := 0

	for _, sys := range nodes {
		if sys.PeerAddress == "" {
			continue
		}

		// Ping and announce in one go
		if _, err := dht.Ping(sys.PeerAddress); err != nil {
			dht.routingTable.MarkFailed(sys.ID)
			dead++
		} else {
			dht.routingTable.MarkVerified(sys.ID)
			// Also announce ourselves so they know we're alive
			dht.AnnounceTo(sys.PeerAddress)
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

		if err := dht.AnnounceTo(sys.PeerAddress); err != nil {
			log.Printf("  Failed to announce to %s: %v", sys.Name, err)
			dht.routingTable.MarkFailed(sys.ID)
		} else {
			announced++
			dht.routingTable.MarkVerified(sys.ID)
		}
	}

	log.Printf("Announced to %d nodes", announced)
}

// refreshLoop periodically refreshes stale buckets
func (dht *DHT) refreshLoop() {
	defer dht.wg.Done()

	// Initial refresh after bootstrap
	time.Sleep(30 * time.Second)

	ticker := time.NewTicker(RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dht.shutdown:
			return
		case <-ticker.C:
			dht.refreshStaleBuckets()
		}
	}
}

// refreshStaleBuckets refreshes buckets that haven't been accessed recently
func (dht *DHT) refreshStaleBuckets() {
	log.Printf("Checking for stale buckets to refresh...")

	refreshed := 0
	for i := 0; i < IDBits; i++ {
		lastAccess := dht.routingTable.BucketLastAccess(i)

		// Skip empty buckets (lastAccess will be very recent from initialization)
		nodes := dht.routingTable.GetBucketNodes(i)
		if len(nodes) == 0 {
			continue
		}

		// Check if bucket is stale
		if time.Since(lastAccess) > RefreshInterval {
			// Generate random ID in this bucket's range and lookup
			randomID := dht.routingTable.RandomIDInBucket(i)
			log.Printf("  Refreshing bucket %d with lookup for %s", i, randomID.String()[:8])

			dht.FindNode(randomID)
			refreshed++
		}
	}

	if refreshed > 0 {
		log.Printf("Refreshed %d stale buckets", refreshed)
	}
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

// pruneCache removes stale entries from the system cache
func (dht *DHT) pruneCache() {
	pruned := dht.routingTable.PruneCache(CacheMaxAge)
	if pruned > 0 {
		log.Printf("Pruned %d stale entries from system cache", pruned)
	}
}

// verifyRoutingTableNodes periodically pings nodes in the routing table
// to verify they're still alive
func (dht *DHT) verifyRoutingTableNodes() {
	nodes := dht.routingTable.GetAllRoutingTableNodes()
	log.Printf("Verifying %d nodes in routing table...", len(nodes))

	verified := 0
	failed := 0

	for _, sys := range nodes {
		if sys.PeerAddress == "" {
			continue
		}

		if err := dht.PingNode(sys); err != nil {
			log.Printf("  %s (%s) - FAILED: %v", sys.Name, sys.ID.String()[:8], err)
			failed++
		} else {
			verified++
		}
	}

	log.Printf("Verification complete: %d verified, %d failed", verified, failed)
}

// GetNetworkStats returns statistics about the DHT network
func (dht *DHT) GetNetworkStats() map[string]interface{} {
	rtSize := dht.routingTable.GetRoutingTableSize()
	cacheSize := dht.routingTable.GetCacheSize()

	// Count active buckets
	activeBuckets := 0
	for i := 0; i < IDBits; i++ {
		if len(dht.routingTable.GetBucketNodes(i)) > 0 {
			activeBuckets++
		}
	}

	return map[string]interface{}{
		"local_id":         dht.localSystem.ID.String(),
		"local_name":       dht.localSystem.Name,
		"routing_table_size": rtSize,
		"cache_size":       cacheSize,
		"active_buckets":   activeBuckets,
		"bucket_k":         dht.routingTable.bucketK,
		"max_peers":        dht.localSystem.GetMaxPeers(),
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
	// Get current balance
	balance, err := dht.storage.GetCreditBalance(dht.localSystem.ID)
	if err != nil {
		log.Printf("Failed to get credit balance: %v", err)
		return
	}

	// Get attestations since last calculation
	attestations, err := dht.storage.GetAttestationsSince(dht.localSystem.ID, balance.LastUpdated)
	if err != nil {
		log.Printf("Failed to get attestations: %v", err)
		return
	}

	if len(attestations) == 0 {
		return
	}

	// Get current peer count for normalization
	peerCount := dht.routingTable.GetRoutingTableSize()
	if peerCount == 0 {
		peerCount = 1 // Avoid division by zero
	}

	// Calculate inputs for credit calculation
	bridgeScore := dht.calculateBridgeScore()
	galaxySize := dht.routingTable.GetCacheSize() + 1 // +1 for self
	reciprocityRatio := dht.calculateReciprocityRatio(attestations)

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

	if result.CreditsEarned > 0 {
		// Update balance
		balance.Balance += result.CreditsEarned
		balance.TotalEarned += result.CreditsEarned
		balance.LastUpdated = time.Now().Unix()
		balance.LongevityStart = result.NewLongevityStart

		if err := dht.storage.SaveCreditBalance(balance); err != nil {
			log.Printf("Failed to save credit balance: %v", err)
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

		if len(bonusParts) > 0 {
			log.Printf("Earned %d stellar credits [%s] (total: %d, rank: %s)", 
				result.CreditsEarned, strings.Join(bonusParts, ", "), balance.Balance, rank.Name)
		} else {
			log.Printf("Earned %d stellar credits (total: %d, rank: %s)", 
				result.CreditsEarned, balance.Balance, rank.Name)
		}

		if result.LongevityBroken {
			log.Printf("  Longevity streak reset due to >30min gap")
		}
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

	// Build set of peers we've heard FROM (they attested to us)
	heardFrom := make(map[string]bool)
	myID := dht.localSystem.ID.String()
	
	for _, att := range attestations {
		// If attestation is TO us (from a peer)
		if att.ToSystemID.String() == myID {
			heardFrom[att.FromSystemID.String()] = true
		}
	}

	// Count how many of our routing table peers have attested back
	reciprocal := 0
	for _, peer := range peers {
		if heardFrom[peer.ID.String()] {
			reciprocal++
		}
	}

	return float64(reciprocal) / float64(len(peers))
}