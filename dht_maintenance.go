package main

import (
	"log"
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
