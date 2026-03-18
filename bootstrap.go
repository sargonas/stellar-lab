package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// === Seed Node Management ===

// SeedNodeListURL is the URL to fetch the seed node list from
const SeedNodeListURL = "https://raw.githubusercontent.com/sargonas/stellar-lab/main/SEED-NODES.txt"

// FallbackSeedNodes are used if GitHub is unreachable
var FallbackSeedNodes = []string{
	// Add stable fallback seeds here if needed
}

// FetchSeedNodes retrieves the current seed node list from GitHub
func FetchSeedNodes() []string {
	log.Printf("Fetching seed node list from GitHub...")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", SeedNodeListURL, nil)
	if err != nil {
		log.Printf("Warning: Could not create request: %v", err)
		log.Printf("Using fallback seed nodes")
		return FallbackSeedNodes
	}
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Warning: Could not fetch seed list from GitHub: %v", err)
		log.Printf("Using fallback seed nodes")
		return FallbackSeedNodes
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Warning: GitHub seed list returned status %d", resp.StatusCode)
		log.Printf("Using fallback seed nodes")
		return FallbackSeedNodes
	}

	var seeds []string
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		seeds = append(seeds, line)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Warning: Error reading seed list: %v", err)
		log.Printf("Using fallback seed nodes")
		return FallbackSeedNodes
	}

	if len(seeds) == 0 {
		log.Printf("Warning: No seeds found in GitHub list")
		log.Printf("Using fallback seed nodes")
		return FallbackSeedNodes
	}

	log.Printf("Loaded %d seed nodes from GitHub", len(seeds))
	return seeds
}

// === Full Sync ===

// tryFullSync attempts to get the complete galaxy state from a peer via /api/full-sync
// This is the preferred method for new nodes to learn about the entire network quickly.
// Returns the number of new systems learned, or error if full-sync is not available.
func (dht *DHT) tryFullSync(address string) (int, error) {
	fullSyncURL := fmt.Sprintf("http://%s/api/full-sync", address)

	client := &http.Client{Timeout: 30 * time.Second} // Longer timeout for full sync
	resp, err := client.Get(fullSyncURL)
	if err != nil {
		return 0, fmt.Errorf("full-sync request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return 0, fmt.Errorf("full-sync not supported (v1.8.x or older)")
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("full-sync returned status %d", resp.StatusCode)
	}

	var syncResp FullSyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return 0, fmt.Errorf("failed to parse full-sync response: %w", err)
	}

	log.Printf("  Full-sync received %d systems from %s (protocol v%s)",
		syncResp.TotalCount, address, syncResp.ProtocolVersion)

	// Cache all systems from the response
	newSystems := 0

	// Cache the local system from the sync response
	if syncResp.LocalSystem.ID != "" {
		localID, err := uuid.Parse(syncResp.LocalSystem.ID)
		if err == nil && localID != dht.localSystem.ID {
			sys := &System{
				ID:          localID,
				Name:        syncResp.LocalSystem.Name,
				X:           syncResp.LocalSystem.X,
				Y:           syncResp.LocalSystem.Y,
				Z:           syncResp.LocalSystem.Z,
				PeerAddress: syncResp.LocalSystem.PeerAddress,
				InfoVersion: syncResp.LocalSystem.InfoVersion,
			}
			// Assign star type from class (simplified)
			sys.Stars = assignStarFromClass(syncResp.LocalSystem.StarClass)

			if existing := dht.routingTable.GetCachedSystem(localID); existing == nil {
				newSystems++
			}
			dht.routingTable.CacheSystem(sys, localID, true) // Mark as verified since we got it directly
		}
	}

	// Cache all other systems
	for _, syncSys := range syncResp.Systems {
		sysID, err := uuid.Parse(syncSys.ID)
		if err != nil {
			continue
		}
		if sysID == dht.localSystem.ID {
			continue // Skip ourselves
		}

		sys := &System{
			ID:          sysID,
			Name:        syncSys.Name,
			X:           syncSys.X,
			Y:           syncSys.Y,
			Z:           syncSys.Z,
			PeerAddress: syncSys.PeerAddress,
			InfoVersion: syncSys.InfoVersion,
		}
		sys.Stars = assignStarFromClass(syncSys.StarClass)

		if existing := dht.routingTable.GetCachedSystem(sysID); existing == nil {
			newSystems++
		}

		// Mark as verified only if the source says they verified it recently
		verified := syncSys.LastSeen > 0 && time.Since(time.Unix(syncSys.LastSeen, 0)) < VerificationCutoff
		dht.routingTable.CacheSystem(sys, uuid.Nil, verified)
	}

	return newSystems, nil
}

// assignStarFromClass creates a MultiStarSystem from a star class string
func assignStarFromClass(class string) MultiStarSystem {
	// Map class to star type - simplified version
	starTypes := map[string]StarType{
		"O": {Class: "O", Description: "Blue Giant", Color: "#9bb0ff", Temperature: 40000, Luminosity: 30000},
		"B": {Class: "B", Description: "Blue-White Star", Color: "#aabfff", Temperature: 20000, Luminosity: 2000},
		"A": {Class: "A", Description: "White Star", Color: "#cad7ff", Temperature: 9000, Luminosity: 25},
		"F": {Class: "F", Description: "Yellow-White Star", Color: "#f8f7ff", Temperature: 7000, Luminosity: 5},
		"G": {Class: "G", Description: "Yellow Star", Color: "#fff4ea", Temperature: 5500, Luminosity: 1},
		"K": {Class: "K", Description: "Orange Star", Color: "#ffd2a1", Temperature: 4500, Luminosity: 0.4},
		"M": {Class: "M", Description: "Red Dwarf", Color: "#ffcc6f", Temperature: 3000, Luminosity: 0.05},
		"X": {Class: "X", Description: "Supermassive Black Hole", Color: "#000000", Temperature: 0, Luminosity: 0},
	}

	star, ok := starTypes[class]
	if !ok {
		star = starTypes["G"] // Default to G-class
	}

	return MultiStarSystem{
		Primary:   star,
		IsBinary:  false,
		IsTrinary: false,
		Count:     1,
	}
}

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

	// Step 1: Try full-sync from routing table peers (v1.9.0+ feature)
	// This immediately gets the complete galaxy instead of iterative discovery
	peers := dht.routingTable.GetAllRoutingTableNodes()
	fullSyncSuccess := false
	for _, peer := range peers {
		if peer.PeerAddress == "" {
			continue
		}
		newSystems, err := dht.tryFullSync(peer.PeerAddress)
		if err != nil {
			log.Printf("  Full-sync from %s failed: %v (falling back to Kademlia)", peer.Name, err)
			continue
		}
		log.Printf("  Full-sync from %s: learned %d new systems", peer.Name, newSystems)
		fullSyncSuccess = true
		break // One successful full-sync is enough
	}

	// Step 2: If full-sync failed or unavailable, fall back to peer discovery
	if !fullSyncSuccess {
		log.Printf("  Full-sync unavailable, using peer discovery...")

		// Do a lookup for ourselves to discover more peers
		log.Printf("  Discovering peers via FIND_NODE...")
		result := dht.FindNode(dht.localSystem.ID)
		log.Printf("  Found %d peers", len(result.ClosestNodes))
	}

	// Step 3: Announce ourselves to closest nodes (always do this)
	log.Printf("  Announcing to closest nodes...")
	result := dht.FindNode(dht.localSystem.ID)
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
