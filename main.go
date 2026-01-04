package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// GitHub URL for community-maintained seed node list
const SeedNodeListURL = "https://raw.githubusercontent.com/sargonas/stellar-lab/main/SEED-NODES.txt"

// FallbackSeedNodes are used if GitHub is unreachable
// These should be stable, long-running community nodes
var FallbackSeedNodes = []string{
	// Add 1-2 very stable fallback seeds here
	// Example: "seed.stellar-mesh.io:8080",
}

// FetchSeedNodes retrieves the current seed node list from GitHub
func FetchSeedNodes() []string {
	log.Printf("Fetching seed node list from GitHub...")
	
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	resp, err := client.Get(SeedNodeListURL)
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

// DiscoverNetworkViaSeedNodes attempts to connect to seed nodes and discover peers
func DiscoverNetworkViaSeedNodes(transport *StellarTransport, systemID uuid.UUID) {
	// Fetch current seed list
	seedNodes := FetchSeedNodes()
	
	if len(seedNodes) == 0 {
		log.Println("No seed nodes available. You can:")
		log.Println("  1. Use -bootstrap flag to connect to a known peer")
		log.Println("  2. Submit a PR to add seed nodes to SEED-NODES.txt")
		log.Println("  3. Wait for other nodes to connect to you")
		return
	}

	connectedToSeed := false
	
	// Try to connect to at least one seed node
	for _, seedAddr := range seedNodes {
		log.Printf("Trying seed node: %s", seedAddr)
		
		// Try to fetch system info from seed to verify it's alive
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("http://" + seedAddr + "/api/system")
		if err != nil {
			log.Printf("  Seed node %s unavailable: %v", seedAddr, err)
			continue
		}
		resp.Body.Close()
		
		// Add the seed as a peer
		if err := transport.AddPeer(systemID, seedAddr); err != nil {
			log.Printf("  Failed to add seed node %s: %v", seedAddr, err)
			continue
		}
		
		log.Printf("  Connected to seed node: %s", seedAddr)
		connectedToSeed = true
		
		// Wait a moment for peer exchange to happen
		time.Sleep(2 * time.Second)
		
		// Check if we discovered other peers
		peers := transport.GetPeers()
		discoveredCount := 0
		for _, peer := range peers {
			if peer.Address != seedAddr {
				discoveredCount++
			}
		}
		
		if discoveredCount > 0 {
			log.Printf("  Discovered %d peers from seed node", discoveredCount)
			
			// We have other peers now, we can disconnect from seed if we want
			// (Optional: uncomment to disconnect from seeds after discovery)
			// transport.RemovePeer(seedAddr)
			// log.Printf("  Disconnected from seed node (now connected to discovered peers)")
		}
		
		break // Successfully connected to one seed, that's enough
	}
	
	if !connectedToSeed {
		log.Println("Warning: Could not connect to any seed nodes")
		log.Println("  Your node is running but isolated")
		log.Println("  Other nodes can still connect to you directly")
	}
}

// StartCompactionScheduler runs periodic database compaction
func StartCompactionScheduler(storage *Storage, keepDays int) {
    // Run immediately on startup if database is large
    go func() {
        // Wait a bit for system to stabilize
        time.Sleep(1 * time.Minute)

        // Check if compaction is needed
        stats, _ := storage.GetDatabaseStats()
        if count, ok := stats["attestation_count"].(int); ok && count > 10000 {
            log.Println("Database has many attestations, running initial compaction...")
            runCompaction(storage, keepDays)
        }
    }()

    // Schedule daily compaction at 3 AM local time
    go func() {
        for {
            now := time.Now()
            // Calculate next 3 AM
            next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
            if next.Before(now) {
                next = next.Add(24 * time.Hour)
            }

            sleepDuration := next.Sub(now)
            log.Printf("Next compaction scheduled for %s (in %s)", next.Format(time.RFC3339), sleepDuration.Round(time.Minute))

            time.Sleep(sleepDuration)
            runCompaction(storage, keepDays)
        }
    }()
}

func runCompaction(storage *Storage, keepDays int) {
    log.Printf("Starting attestation compaction (keeping %d days of detail)...", keepDays)

    stats, err := storage.CompactAttestations(keepDays)
    if err != nil {
        log.Printf("Compaction failed: %v", err)
        return
    }

    log.Printf("Compaction complete:")
    log.Printf("  - Attestations processed: %d", stats.AttestationsProcessed)
    log.Printf("  - Summaries created: %d", stats.SummariesCreated)
    log.Printf("  - Attestations deleted: %d", stats.AttestationsDeleted)
    if stats.SpaceReclaimed > 0 {
        log.Printf("  - Space reclaimed: %d bytes (%.2f MB)", stats.SpaceReclaimed, float64(stats.SpaceReclaimed)/1024/1024)
    }
}

func main() {
	// Command-line flags
	var (
		name       = flag.String("name", "", "Name for this star system (required)")
		address    = flag.String("address", "0.0.0.0:8080", "Address to bind web UI server (host:port)")
		peerPort   = flag.String("peer-port", "7867", "Port for peer-to-peer mesh communication")
		dbPath     = flag.String("db", "stellar-mesh.db", "Path to SQLite database file")
		bootstrap  = flag.String("bootstrap", "", "Bootstrap peer address (host:port)")
		systemSeed = flag.String("seed", "", "Optional seed for semi-deterministic UUID (same seed = same UUID on this hardware)")
		useRandom  = flag.Bool("random-uuid", false, "Use completely random UUID instead of hardware-based")
		compact = flag.Bool("compact", false, "Run database compaction and exit")
		compactDays = flag.Int("compact-days", 7, "Days of attestations to keep when compacting")
	)
	flag.Parse()

	// Construct peer address using same host as web UI but different port
	webHost := *address
	if colonIdx := strings.LastIndex(webHost, ":"); colonIdx != -1 {
	    webHost = webHost[:colonIdx]
	}
	peerAddress := webHost + ":" + *peerPort

	// Handle manual compaction mode
	if *compact {
	    storage, err := NewStorage(*dbPath)
	    if err != nil {
	        log.Fatalf("Failed to open database: %v", err)
	    }

	    // Show stats before
	    statsBefore, _ := storage.GetDatabaseStats()
	    log.Println("Database stats before compaction:")
	    log.Printf("  - Attestations: %v", statsBefore["attestation_count"])
	    log.Printf("  - Summaries: %v", statsBefore["summary_count"])
	    log.Printf("  - Size: %v bytes", statsBefore["database_size_bytes"])

	    // Run compaction
	    runCompaction(storage, *compactDays)

	    // Show stats after
	    statsAfter, _ := storage.GetDatabaseStats()
	    log.Println("Database stats after compaction:")
	    log.Printf("  - Attestations: %v", statsAfter["attestation_count"])
	    log.Printf("  - Summaries: %v", statsAfter["summary_count"])
	    log.Printf("  - Size: %v bytes", statsAfter["database_size_bytes"])

	    storage.Close()
	    os.Exit(0)
	}

	if *name == "" {
		log.Fatal("System name is required (use -name flag)")
	}

	// Show hardware fingerprint
	hwFingerprint := GetHardwareFingerprint()
	log.Printf("Hardware fingerprint: %s", hwFingerprint)

	// Initialize storage
	storage, err := NewStorage(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer storage.Close()

	// Start background compaction scheduler (keep 7 days of full attestations)
	StartCompactionScheduler(storage, 7)

	// Load or create system
	var system *System
	existing, err := storage.LoadSystem()
	if err != nil {
		// Try to get a nearby system from bootstrap peer if provided
		var nearbySystem *System
		if *bootstrap != "" {
			log.Printf("Fetching bootstrap peer info from %s", *bootstrap)
			resp, err := http.Get("http://" + *bootstrap + "/system")
			if err == nil {
				defer resp.Body.Close()
				var bootstrapSystem System
				if json.NewDecoder(resp.Body).Decode(&bootstrapSystem) == nil {
					nearbySystem = &bootstrapSystem
					log.Printf("Will cluster near bootstrap system: %s at (%.2f, %.2f, %.2f)",
						nearbySystem.Name, nearbySystem.X, nearbySystem.Y, nearbySystem.Z)
				}
			} else {
				log.Printf("Warning: Could not fetch bootstrap peer info: %v", err)
			}
		}

		// Create new system (clustered if we got bootstrap info)
		log.Printf("Creating new star system: %s", *name)
		
		// Create system with semi-deterministic or random UUID
		var newID uuid.UUID
		if *useRandom {
			newID = GenerateRandomUUID()
			log.Printf("Using random UUID")
		} else if *systemSeed != "" {
			var err error
			newID, err = GenerateSemiDeterministicUUID(*systemSeed)
			if err != nil {
				log.Printf("Warning: Could not generate semi-deterministic UUID, using random: %v", err)
				newID = GenerateRandomUUID()
			} else {
				log.Printf("Using semi-deterministic UUID (seed: %s)", *systemSeed)
			}
		} else {
			var err error
			newID, err = GenerateSemiDeterministicUUID("")
			if err != nil {
				log.Printf("Warning: Could not generate hardware-based UUID, using random: %v", err)
				newID = GenerateRandomUUID()
			} else {
				log.Printf("Using hardware-based UUID")
			}
		}
		
		// Create system manually with our chosen UUID
		keys, err := GenerateKeyPair()
		if err != nil {
			log.Fatalf("Failed to generate cryptographic keys: %v", err)
		}
		
		system = &System{
			ID:         newID,
			Name:       *name,
			Address:    *address,
			CreatedAt:  time.Now(),
			LastSeenAt: time.Now(),
			Keys:       keys,
		}
		system.GenerateCoordinates(nearbySystem)
		system.GenerateMultiStarSystem()
		
		if err := storage.SaveSystem(system); err != nil {
			log.Fatalf("Failed to save system: %v", err)
		}
		
		log.Printf("System ID: %s", system.ID)
		log.Printf("Public Key: %s...", base64.StdEncoding.EncodeToString(system.Keys.PublicKey)[:16])
		
		// Display star system composition
		if system.Stars.IsTrinary {
			log.Printf("Trinary Star System:")
			log.Printf("  Primary:   %s (%s)", system.Stars.Primary.Class, system.Stars.Primary.Description)
			log.Printf("  Secondary: %s (%s)", system.Stars.Secondary.Class, system.Stars.Secondary.Description)
			log.Printf("  Tertiary:  %s (%s)", system.Stars.Tertiary.Class, system.Stars.Tertiary.Description)
		} else if system.Stars.IsBinary {
			log.Printf("Binary Star System:")
			log.Printf("  Primary:   %s (%s)", system.Stars.Primary.Class, system.Stars.Primary.Description)
			log.Printf("  Secondary: %s (%s)", system.Stars.Secondary.Class, system.Stars.Secondary.Description)
		} else {
			log.Printf("Single Star System:")
			log.Printf("  Star: %s (%s)", system.Stars.Primary.Class, system.Stars.Primary.Description)
		}
		
		log.Printf("Coordinates: (%.2f, %.2f, %.2f)", system.X, system.Y, system.Z)
		if nearbySystem != nil {
			log.Printf("Distance from bootstrap: %.2f units", system.DistanceTo(nearbySystem))
		}
	} else {
		// Use existing system
		system = existing
		log.Printf("Loaded existing system: %s (ID: %s)", system.Name, system.ID)
		
		// Display star system composition
		if system.Stars.IsTrinary {
			log.Printf("Trinary Star System:")
			log.Printf("  Primary:   %s (%s)", system.Stars.Primary.Class, system.Stars.Primary.Description)
			log.Printf("  Secondary: %s (%s)", system.Stars.Secondary.Class, system.Stars.Secondary.Description)
			log.Printf("  Tertiary:  %s (%s)", system.Stars.Tertiary.Class, system.Stars.Tertiary.Description)
		} else if system.Stars.IsBinary {
			log.Printf("Binary Star System:")
			log.Printf("  Primary:   %s (%s)", system.Stars.Primary.Class, system.Stars.Primary.Description)
			log.Printf("  Secondary: %s (%s)", system.Stars.Secondary.Class, system.Stars.Secondary.Description)
		} else {
			log.Printf("Single Star System:")
			log.Printf("  Star: %s (%s)", system.Stars.Primary.Class, system.Stars.Primary.Description)
		}
		
		log.Printf("Coordinates: (%.2f, %.2f, %.2f)", system.X, system.Y, system.Z)
		
		// Log public key
		if system.Keys != nil {
			log.Printf("Public Key: %s...", base64.StdEncoding.EncodeToString(system.Keys.PublicKey)[:16])
		}
		
		// Update address if changed
		if system.Address != *address {
			system.Address = *address
			storage.SaveSystem(system)
		}
	}

	// Initialize stellar transport protocol
	transport := NewStellarTransport(system, storage, peerAddress)
	transport.Start()
	log.Println("Stellar transport protocol started")

	// Connect to bootstrap peer if provided, otherwise try seed nodes
	if *bootstrap != "" {
		log.Printf("Attempting to connect to bootstrap peer: %s", *bootstrap)
		if err := transport.AddPeer(system.ID, *bootstrap); err != nil {
			log.Printf("Warning: Failed to add bootstrap peer: %v", err)
		}
	} else {
		// No bootstrap provided, try seed nodes
		log.Println("No bootstrap peer provided, discovering network via seed nodes...")
		DiscoverNetworkViaSeedNodes(transport, system.ID)
	}

	// Initialize and start API server
	api := NewAPI(system, transport, storage)
	
	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		log.Println("Shutting down...")
		storage.Close()
		os.Exit(0)
	}()

	// Start API server (blocking)
	log.Printf("Star system '%s' is now online", system.Name)
	log.Printf("  Web UI: http://%s", *address)
	log.Printf("  Peer port: %s", peerAddress)
	log.Printf("API endpoints available at http://%s", *address)
	if err := api.Start(*address); err != nil {
		log.Fatalf("API server failed: %v", err)
	}
}
