package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
)

func main() {
	// Parse command line flags
	name := flag.String("name", "", "Name for this star system")
	seed := flag.String("seed", "", "Seed for deterministic UUID generation (optional)")
	dbPath := flag.String("db", "stellar-mesh.db", "Path to SQLite database")
	address := flag.String("address", "0.0.0.0:8080", "Address to bind web UI server (host:port)")
	peerPort := flag.String("peer-port", "7867", "Port for DHT peer communication")
	bootstrapPeer := flag.String("bootstrap", "", "Bootstrap peer address (host:port)")
	flag.Parse()

	// Validate required flags
	if *name == "" {
		log.Fatal("Error: -name flag is required")
	}

	// Construct peer address using same host as web UI but specified peer port
	webHost := *address
	if idx := strings.LastIndex(webHost, ":"); idx != -1 {
		webHost = webHost[:idx]
	}
	peerAddr := webHost + ":" + *peerPort

	// Generate addresses
	webAddr := *address

	// Initialize storage
	storage, err := NewStorage(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	// Try to load existing system or create new one
	system, err := storage.LoadSystem()
	if err != nil {
		// Create new system
		log.Printf("Creating new star system: %s", *name)

		// Generate UUID (deterministic if seed provided)
		var systemID uuid.UUID
		if *seed != "" {
			log.Printf("Using semi-deterministic UUID (seed: %s)", *seed)
			systemID = generateDeterministicUUID(*seed)
		} else {
			systemID = uuid.New()
		}

		// Generate cryptographic keys
		keys, err := GenerateKeyPair()
		if err != nil {
			log.Fatalf("Failed to generate keys: %v", err)
		}

		system = &System{
			ID:          systemID,
			Name:        *name,
			CreatedAt:   time.Now(),
			LastSeenAt:  time.Now(),
			Address:     webAddr,
			PeerAddress: peerAddr,
			Keys:        keys,
		}

		// Generate star system
		system.GenerateMultiStarSystem()

		// Save to database
		if err := storage.SaveSystem(system); err != nil {
			log.Fatalf("Failed to save system: %v", err)
		}
	} else {
		log.Printf("Loaded existing star system: %s", system.Name)
		// Update addresses in case ports changed
		system.Address = webAddr
		system.PeerAddress = peerAddr
		storage.SaveSystem(system)
	}

	// Log system info
	log.Printf("System ID: %s", system.ID)
	log.Printf("Public Key: %s...", truncateKey(system.Keys.PublicKey))
	logStarSystem(system)
	log.Printf("Coordinates: (%.2f, %.2f, %.2f)", system.X, system.Y, system.Z)

	// Create DHT
	dht := NewDHT(system, storage, peerAddr)

	// Create web interface
	webInterface := NewWebInterface(dht, storage, webAddr)

	// Start DHT (HTTP server + maintenance loops)
	if err := dht.Start(); err != nil {
		log.Fatalf("Failed to start DHT: %v", err)
	}

	// Start web interface
	if err := webInterface.Start(); err != nil {
		log.Fatalf("Failed to start web interface: %v", err)
	}

	log.Printf("DHT protocol started")
	log.Printf("Star system '%s' is now online", system.Name)
	log.Printf("  Web UI: http://%s", webAddr)
	log.Printf("  DHT port: %s", peerAddr)

	// Bootstrap into the network
	go func() {
		time.Sleep(2 * time.Second) // Wait for servers to start

		config := DefaultBootstrapConfig()
		if *bootstrapPeer != "" {
			config.BootstrapPeer = *bootstrapPeer
		}
		config.SeedNodes = FetchSeedNodes()

		if err := dht.Bootstrap(config); err != nil {
			log.Printf("Bootstrap warning: %v", err)
		}
	}()

	// Schedule attestation compaction
	scheduleCompaction(storage)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Printf("Shutting down...")
	dht.Stop()
	log.Printf("Goodbye!")
}

// generateDeterministicUUID creates a UUID from a seed string
func generateDeterministicUUID(seed string) uuid.UUID {
	// Include hardware fingerprint for uniqueness
	fingerprint := GetHardwareFingerprint()
	data := seed + fingerprint

	hash := sha256.Sum256([]byte(data))

	// Use first 16 bytes as UUID
	var id uuid.UUID
	copy(id[:], hash[:16])

	// Set version 5 (SHA-1 name-based) and variant bits
	id[6] = (id[6] & 0x0f) | 0x50 // Version 5
	id[8] = (id[8] & 0x3f) | 0x80 // Variant 1

	return id
}

// truncateKey returns a truncated base64 representation of a key
func truncateKey(key []byte) string {
	if len(key) == 0 {
		return "(none)"
	}
	encoded := hex.EncodeToString(key)
	if len(encoded) > 16 {
		return encoded[:16]
	}
	return encoded
}

// logStarSystem logs the star configuration
func logStarSystem(sys *System) {
	if sys.Stars.IsTrinary {
		log.Printf("Trinary Star System:")
		log.Printf("  Primary:   %s (%s)", sys.Stars.Primary.Class, sys.Stars.Primary.Description)
		if sys.Stars.Secondary != nil {
			log.Printf("  Secondary: %s (%s)", sys.Stars.Secondary.Class, sys.Stars.Secondary.Description)
		}
		if sys.Stars.Tertiary != nil {
			log.Printf("  Tertiary:  %s (%s)", sys.Stars.Tertiary.Class, sys.Stars.Tertiary.Description)
		}
	} else if sys.Stars.IsBinary {
		log.Printf("Binary Star System:")
		log.Printf("  Primary:   %s (%s)", sys.Stars.Primary.Class, sys.Stars.Primary.Description)
		if sys.Stars.Secondary != nil {
			log.Printf("  Secondary: %s (%s)", sys.Stars.Secondary.Class, sys.Stars.Secondary.Description)
		}
	} else {
		log.Printf("Single Star: %s (%s)", sys.Stars.Primary.Class, sys.Stars.Primary.Description)
	}
}

// scheduleCompaction schedules periodic attestation compaction
func scheduleCompaction(storage *Storage) {
	// Run at 3 AM daily
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	if next.Before(now) {
		next = next.Add(24 * time.Hour)
	}

	duration := next.Sub(now)
	log.Printf("Next compaction scheduled for %s (in %v)", next.Format(time.RFC3339), duration.Round(time.Second))

	go func() {
		time.Sleep(duration)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			log.Printf("Running attestation compaction...")
			if _, err := storage.CompactAttestations(7); err != nil {
				log.Printf("Compaction error: %v", err)
			} else {
				log.Printf("Compaction complete")
			}

			<-ticker.C
		}
	}()
}