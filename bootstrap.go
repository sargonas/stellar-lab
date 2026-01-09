package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// BootstrapConfig holds configuration for the bootstrap process
type BootstrapConfig struct {
	SeedNodes       []string      // Seed node addresses to try
	BootstrapPeer   string        // Direct peer to bootstrap from
	Timeout         time.Duration // Timeout for bootstrap operations
	MinInitialPeers int           // Minimum peers before considering bootstrap complete
}

// DefaultBootstrapConfig sets our defaults
func DefaultBootstrapConfig() BootstrapConfig {
	return BootstrapConfig{
		Timeout:         30 * time.Second,
		MinInitialPeers: 2,
	}
}

// Bootstrap joins the DHT network
func (dht *DHT) Bootstrap(config BootstrapConfig) error {
	log.Printf("Starting DHT bootstrap...")

	// First, try to rejoin using cached peers from previous sessions
	cachedPeers := dht.routingTable.GetAllRoutingTableNodes()
	if len(cachedPeers) > 0 {
		log.Printf("Found %d cached peers from previous session, attempting to rejoin...", len(cachedPeers))
		connected := 0
		for _, peer := range cachedPeers {
			if peer.PeerAddress == "" {
				continue
			}
			log.Printf("  Pinging cached peer: %s (%s)", peer.Name, peer.PeerAddress)
			if err := dht.PingNode(peer); err != nil {
				log.Printf("    Failed: %v", err)
				continue
			}
			log.Printf("    Connected!")
			connected++
		}
		if connected > 0 {
			log.Printf("Rejoined network via %d cached peers", connected)
			return dht.completeBootstrap()
		}
		log.Printf("Could not reach any cached peers, falling back to bootstrap...")
	}

	// If we have a direct bootstrap peer by cli parameter, try that
	if config.BootstrapPeer != "" {
		err := dht.bootstrapFromPeer(config.BootstrapPeer)
		if err != nil {
			// In isolated mode, failing to bootstrap means we're the genesis node
			if isolatedMode != nil && *isolatedMode {
				log.Printf("Isolated mode: becoming genesis node")
				dht.becomeGenesisNode()
				return nil
			}
			return fmt.Errorf("direct bootstrap peer failed: %w", err)
		}
		log.Printf("Successfully bootstrapped from direct peer")
		return dht.completeBootstrap()
	}

	// In isolated mode with no bootstrap peer, become genesis immediately
	if isolatedMode != nil && *isolatedMode {
		log.Printf("Isolated mode: no bootstrap peer specified, starting as genesis")
		dht.becomeGenesisNode()
		return nil
	}

	// Try the seed nodes
	if len(config.SeedNodes) > 0 {
		for _, seedAddr := range config.SeedNodes {
			log.Printf("Trying seed node: %s", seedAddr)
			if err := dht.bootstrapFromSeed(seedAddr); err != nil {
				log.Printf("  Failed: %v", err)
				continue
			}
			log.Printf("Successfully bootstrapped from seed node")
			return dht.completeBootstrap()
		}
	}

	// Try fetching seed nodes from GitHub (only if not in isolated mode)
	if isolatedMode == nil || !*isolatedMode {
		log.Printf("Fetching seed nodes from GitHub...")
		seedNodes := FetchSeedNodes()
		for _, seedAddr := range seedNodes {
			log.Printf("Trying seed node: %s", seedAddr)
			if err := dht.bootstrapFromSeed(seedAddr); err != nil {
				log.Printf("  Failed: %v", err)
				continue
			}
			log.Printf("Successfully bootstrapped from seed node")
			return dht.completeBootstrap()
		}
	}

	// When no bootstrap sources available
	rtSize := dht.routingTable.GetRoutingTableSize()
	if rtSize > 0 {
		log.Printf("Warning: Could not contact any seed nodes, but have %d cached peers", rtSize)
		return dht.completeBootstrap()
	}

	log.Printf("Warning: Could not find any bootstrap peers")
	log.Printf("  Your node is running but isolated")
	log.Printf("  Other nodes can still connect to you directly")
	return nil
}

// becomeGenesisNode converts this node into the genesis black hole for an isolated network
func (dht *DHT) becomeGenesisNode() {
	// Set coordinates to origin
	dht.localSystem.X = 0
	dht.localSystem.Y = 0
	dht.localSystem.Z = 0
	dht.localSystem.SponsorID = nil

	// Convert to Class X black hole
	dht.localSystem.Stars = MultiStarSystem{
		Primary: StarType{
			Class:       "X",
			Description: "Supermassive Black Hole",
			Color:       "#000000",
			Temperature: 0,
			Luminosity:  0,
		},
		IsBinary:  false,
		IsTrinary: false,
		Count:     1,
	}

	// Save to database
	if err := dht.storage.SaveSystem(dht.localSystem); err != nil {
		log.Printf("Warning: failed to save genesis state: %v", err)
	}

	log.Printf("✦ Now operating as genesis black hole at origin (0,0,0) ✦")
}

// bootstrapFromPeer bootstraps from a known peer address
func (dht *DHT) bootstrapFromPeer(address string) error {
	// If we don't have a sponsor yet, we need to get peer info BEFORE pinging
	// because the ping will fail coordinate validation without valid coordinates
	if dht.localSystem.SponsorID == nil && dht.localSystem.Stars.Primary.Class != "X" {
		// Get peer's system info via HTTP api call (not DHT ping)
		systemURL := fmt.Sprintf("http://%s/system", address)
		resp, err := http.Get(systemURL)
		if err != nil {
			return fmt.Errorf("failed to get peer system info: %w", err)
		}
		defer resp.Body.Close()

		var peerSys System
		if err := json.NewDecoder(resp.Body).Decode(&peerSys); err != nil {
			return fmt.Errorf("failed to parse peer system info: %w", err)
		}

		// Don't bootstrap from ourselves - this happens when a node points to itself
		// for isolation (e.g., dev cluster node 1). Fail cleanly without state mutation.
		if peerSys.ID == dht.localSystem.ID {
			return fmt.Errorf("cannot bootstrap from self (isolated mode)")
		}

		// Generate our deterministic coordinates based on this sponsor
		dht.localSystem.GenerateClusteredCoordinates(&peerSys)

		log.Printf("  Assigned sponsor: %s (%s)", peerSys.Name, peerSys.ID.String()[:8])
		log.Printf("  New coordinates: (%.2f, %.2f, %.2f)",
			dht.localSystem.X, dht.localSystem.Y, dht.localSystem.Z)

		// Save updated coordinates to database
		if err := dht.storage.SaveSystem(dht.localSystem); err != nil {
			log.Printf("  Warning: failed to save coordinates: %v", err)
		}
	}

	// Now we can ping with valid coordinates
	sys, err := dht.Ping(address)
	if err != nil {
		return fmt.Errorf("failed to ping bootstrap peer: %w", err)
	}

	log.Printf("  Connected to %s (%s)", sys.Name, sys.ID.String()[:8])

	// Add to routing table (proper Kademlia LRS-ping if bucket full)
	dht.updateRoutingTable(sys)
	dht.routingTable.MarkVerified(sys.ID)

	// Query for nodes close to us - now we know who we're talking to
	closest, err := dht.FindNodeDirectToSystem(sys, dht.localSystem.ID)
	if err != nil {
		return fmt.Errorf("failed to find_node from bootstrap peer: %w", err)
	}

	for _, node := range closest {
		dht.updateRoutingTable(node)
	}

	log.Printf("  Learned about %d nodes", len(closest))
	return nil
}

// bootstrapFromSeed bootstraps using a seed node's discovery endpoint
func (dht *DHT) bootstrapFromSeed(seedAddr string) error {
	// First try the discovery endpoint
	discoveryURL := fmt.Sprintf("http://%s/api/discovery", seedAddr)
	resp, err := http.Get(discoveryURL)
	if err != nil {
		return fmt.Errorf("failed to contact seed: %w", err)
	}
	defer resp.Body.Close()

	var systems []DiscoverySystem
	if err := json.NewDecoder(resp.Body).Decode(&systems); err != nil {
		return fmt.Errorf("failed to parse discovery response: %w", err)
	}

	if len(systems) == 0 {
		return fmt.Errorf("seed returned no systems")
	}

	log.Printf("  Seed returned %d systems", len(systems))

	// If we don't have a sponsor yet (new node), set one before pinging
	// This is required for coordinate validation
	if dht.localSystem.SponsorID == nil && dht.localSystem.Stars.Primary.Class != "X" {
		// Find a suitable sponsor from the discovery list
		var sponsor *DiscoverySystem
		for i := range systems {
			if systems[i].ID != dht.localSystem.ID.String() {
				sponsor = &systems[i]
				break
			}
		}

		if sponsor != nil {
			sponsorID, err := uuid.Parse(sponsor.ID)
			if err == nil {
				// Create a temporary System struct for coordinate generation
				sponsorSys := &System{
					ID: sponsorID,
					X:  sponsor.X,
					Y:  sponsor.Y,
					Z:  sponsor.Z,
				}

				// Generate our deterministic coordinates based on sponsor
				dht.localSystem.GenerateClusteredCoordinates(sponsorSys)

				log.Printf("  Assigned sponsor: %s (%s)", sponsor.Name, sponsor.ID[:8])
				log.Printf("  New coordinates: (%.2f, %.2f, %.2f)",
					dht.localSystem.X, dht.localSystem.Y, dht.localSystem.Z)

				// Save updated coordinates to database
				if err := dht.storage.SaveSystem(dht.localSystem); err != nil {
					log.Printf("  Warning: failed to save coordinates: %v", err)
				}
			}
		}
	}

	// Try to connect to systems with capacity
	connected := 0
	for _, sys := range systems {
		if sys.ID == dht.localSystem.ID.String() {
			continue // Skip ourselves
		}

		// Prefer systems with capacity
		if !sys.HasCapacity && connected > 0 {
			continue
		}

		// Ping to establish connection and get full system info
		fullSys, err := dht.Ping(sys.PeerAddress)
		if err != nil {
			log.Printf("    Failed to ping %s: %v", sys.Name, err)
			continue
		}

		dht.updateRoutingTable(fullSys)
		dht.routingTable.MarkVerified(fullSys.ID)
		connected++
		log.Printf("    Connected to %s", fullSys.Name)

		// Also do a find_node to learn more peers - now we know the system
		closest, err := dht.FindNodeDirectToSystem(fullSys, dht.localSystem.ID)
		if err == nil {
			for _, node := range closest {
				dht.updateRoutingTable(node)
			}
		}

		if connected >= 3 {
			break // Enough initial connections
		}
	}

	if connected == 0 {
		return fmt.Errorf("could not connect to any systems from seed")
	}

	return nil
}

// completeBootstrap finishes the bootstrap process
func (dht *DHT) completeBootstrap() error {
	log.Printf("Completing bootstrap process...")

	// Step 1: Do an iterative lookup for ourselves
	// This populates our routing table with relevant nodes
	log.Printf("  Looking up our own ID to populate routing table...")
	result := dht.FindNode(dht.localSystem.ID)
	log.Printf("  Found %d close nodes in %d hops", len(result.ClosestNodes), result.Hops)

	// Step 2: Refresh random buckets to learn about diverse parts of the network
	log.Printf("  Refreshing random buckets...")
	refreshed := 0
	for i := 0; i < IDBits; i++ {
		// Only refresh some buckets to avoid flooding
		if i%10 == 0 || i < 10 {
			randomID := dht.routingTable.RandomIDInBucket(i)
			dht.FindNode(randomID)
			refreshed++
		}
	}
	log.Printf("  Refreshed %d buckets", refreshed)

	// Step 3: Announce ourselves to closest nodes
	log.Printf("  Announcing to closest nodes...")
	announced := 0
	for _, sys := range result.ClosestNodes {
		if sys.ID == dht.localSystem.ID || sys.PeerAddress == "" {
			continue
		}
		if err := dht.AnnounceToSystem(sys); err == nil {
			announced++
		}
	}
	log.Printf("  Announced to %d nodes", announced)

	// Report final state
	rtSize := dht.routingTable.GetRoutingTableSize()
	cacheSize := dht.routingTable.GetCacheSize()
	log.Printf("Bootstrap complete: %d nodes in routing table, %d in cache", rtSize, cacheSize)

	return nil
}

// BootstrapFromAddress is a convenience function to bootstrap from a single address
func (dht *DHT) BootstrapFromAddress(address string) error {
	config := DefaultBootstrapConfig()
	config.BootstrapPeer = address
	return dht.Bootstrap(config)
}

// BootstrapFromSeedNodes is a convenience function to bootstrap from seed nodes
func (dht *DHT) BootstrapFromSeedNodes(seeds []string) error {
	config := DefaultBootstrapConfig()
	config.SeedNodes = seeds
	return dht.Bootstrap(config)
}
