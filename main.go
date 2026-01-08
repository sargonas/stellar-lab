package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// Global flag for isolated mode (accessible from bootstrap.go)
var isolatedMode *bool

func main() {
	// Parse command line flags (CLI args override environment variables)
	name := flag.String("name", getEnv("STELLAR_NAME", ""), "Name for this star system")
	seed := flag.String("seed", getEnv("STELLAR_SEED", ""), "Seed for deterministic UUID generation (optional)")
	dbPath := flag.String("db", getEnv("STELLAR_DB", "/data/stellar-lab.db"), "Path to SQLite database")
	address := flag.String("address", getEnv("STELLAR_ADDRESS", "0.0.0.0:8080"), "Address to bind web UI server (host:port)")
	publicAddr := flag.String("public-address", getEnv("STELLAR_PUBLIC_ADDRESS", ""), "Public address for peer connections (host:port)")
	bootstrapPeer := flag.String("bootstrap", getEnv("STELLAR_BOOTSTRAP", ""), "Bootstrap peer address (host:port)")
	isolatedMode = flag.Bool("isolated", false, "Isolated network mode (skips seed nodes, first node becomes genesis)")
	flag.Parse()

	// Clean and validate the star system name
	cleanName := sanitizeStarName(*name)
	if err := validateStarName(cleanName); err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Validate public address
	if *publicAddr == "" {
		log.Fatal("Error: -public-address or STELLAR_PUBLIC_ADDRESS is required (e.g., \"myhost.com:7867\")")
	}

	peerAddr := *publicAddr

	// Extract port from public address for local binding
	peerPort := "7867" // default
	if idx := strings.LastIndex(*publicAddr, ":"); idx != -1 {
		peerPort = (*publicAddr)[idx+1:]
	}
	listenAddr := "0.0.0.0:" + peerPort

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
		log.Printf("Creating new star system: %s", cleanName)

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
			Name:        cleanName,
			CreatedAt:   time.Now(),
			LastSeenAt:  time.Now(),
			Address:     webAddr,
			PeerAddress: peerAddr,
			Keys:        keys,
		}

		// Generate star system
		system.GenerateMultiStarSystem()

		// New nodes start at origin with no sponsor
		// Real coordinates assigned during bootstrap when we find a sponsor
		system.X = 0
		system.Y = 0
		system.Z = 0
		system.SponsorID = nil

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

	// Set InfoVersion to current timestamp (milliseconds) on every startup
	// This ensures our info is considered "fresh" and prevents stale gossip
	// from overwriting our current state
	system.InfoVersion = time.Now().UnixMilli()

	// Log system info
	log.Printf("System ID: %s", system.ID)
	log.Printf("Public Key: %s...", truncateKey(system.Keys.PublicKey))
	logStarSystem(system)
	log.Printf("Coordinates: (%.2f, %.2f, %.2f)", system.X, system.Y, system.Z)

	// Create DHT (listenAddr for binding, peerAddr is already set on system)
	dht := NewDHT(system, storage, listenAddr)

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
	log.Printf("  DHT: %s (listening on %s)", peerAddr, listenAddr)

	// Bootstrap into the network
	go func() {
		time.Sleep(2 * time.Second) // Wait for servers to start

		config := DefaultBootstrapConfig()
		if *bootstrapPeer != "" {
			config.BootstrapPeer = *bootstrapPeer
		}

		// In isolated mode, never fetch seed nodes
		if !*isolatedMode {
			config.SeedNodes = FetchSeedNodes()
		}

		if err := dht.Bootstrap(config); err != nil {
			log.Printf("Bootstrap warning: %v", err)
		}

		// If this is a new node at origin (0,0,0), update coordinates near a peer
		// Exception: Class X (genesis black hole) stays at origin
		if system.X == 0 && system.Y == 0 && system.Z == 0 && system.Stars.Primary.Class != "X" {
			peers := dht.GetRoutingTable().GetAllRoutingTableNodes()
			if len(peers) > 0 {
				sponsor := peers[0]
				log.Printf("Updating coordinates to cluster near %s", sponsor.Name)
				system.GenerateCoordinates(sponsor)
				system.SponsorID = &sponsor.ID
				storage.SaveSystem(system)
				log.Printf("New coordinates: (%.2f, %.2f, %.2f), sponsored by %s", system.X, system.Y, system.Z, sponsor.Name)
			}
		}
	}()

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
	// Special case for the genesis black hole
	if sys.Stars.Primary.Class == "X" {
		log.Printf("✦ Supermassive Black Hole - Galactic Core ✦")
		return
	}

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

// getEnv returns environment variable value or default if not set
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// sanitizeStarName cleans up a star name by removing quotes and extra whitespace
func sanitizeStarName(name string) string {
	// Trim whitespace
	name = strings.TrimSpace(name)
	// Remove surrounding quotes (single or double) that may have been passed through
	name = strings.Trim(name, "\"'`")
	// Trim again in case there was whitespace inside quotes
	name = strings.TrimSpace(name)
	return name
}

// validateStarName checks if a star name is valid
func validateStarName(name string) error {
	// Check for empty name
	if name == "" {
		return fmt.Errorf("STELLAR_NAME is required - please set a unique name for your star system")
	}

	// Check for placeholder values that users forgot to change
	placeholders := []string{
		"CHANGE_ME",
		"YOUR_STAR_NAME",
		"YOUR_STAR_NAME_HERE",
		"CHANGE_ME_TO_YOUR_STAR_NAME",
		"MyStarSystem",
		"example",
		"test",
		"default",
		"placeholder",
	}

	nameLower := strings.ToLower(name)
	for _, p := range placeholders {
		if nameLower == strings.ToLower(p) {
			return fmt.Errorf("please change STELLAR_NAME from '%s' to a unique name for your star system", name)
		}
	}

	// Check minimum length
	if len(name) < 2 {
		return fmt.Errorf("star system name must be at least 2 characters long")
	}

	// Check maximum length
	if len(name) > 64 {
		return fmt.Errorf("star system name must be 64 characters or less")
	}

	return nil
}