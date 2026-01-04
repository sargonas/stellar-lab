package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Command-line flags
	var (
		name       = flag.String("name", "", "Name for this star system (required)")
		address    = flag.String("address", "localhost:8080", "Address to bind API server (host:port)")
		dbPath     = flag.String("db", "stellar-mesh.db", "Path to SQLite database file")
		bootstrap  = flag.String("bootstrap", "", "Bootstrap peer address (host:port)")
		systemSeed = flag.String("seed", "", "Optional seed for semi-deterministic UUID (same seed = same UUID on this hardware)")
		useRandom  = flag.Bool("random-uuid", false, "Use completely random UUID instead of hardware-based")
	)
	flag.Parse()

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
		system = &System{
			ID:         newID,
			Name:       *name,
			Address:    *address,
			CreatedAt:  time.Now(),
			LastSeenAt: time.Now(),
		}
		system.GenerateCoordinates(nearbySystem)
		system.GenerateMultiStarSystem()
		
		if err := storage.SaveSystem(system); err != nil {
			log.Fatalf("Failed to save system: %v", err)
		}
		
		log.Printf("System ID: %s", system.ID)
		
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
		
		// Display planetary system
		if system.Planets != nil {
			log.Printf("Planetary System: %d planets (%d habitable)", 
				system.Planets.TotalPlanets, system.Planets.HabitablePlanets)
			for _, planet := range system.Planets.Planets {
				habitableMarker := ""
				if planet.Habitable {
					habitableMarker = " [HABITABLE]"
				}
				log.Printf("  - %s: %s at %.2f AU%s", 
					planet.Name, planet.Type, planet.OrbitAU, habitableMarker)
			}
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
		
		// Display planetary system
		if system.Planets != nil {
			log.Printf("Planetary System: %d planets (%d habitable)", 
				system.Planets.TotalPlanets, system.Planets.HabitablePlanets)
		}
		
		// Update address if changed
		if system.Address != *address {
			system.Address = *address
			storage.SaveSystem(system)
		}
	}

	// Initialize stellar transport protocol
	transport := NewStellarTransport(system, storage)
	transport.Start()
	log.Println("Stellar transport protocol started")
	
	// Initialize decentralized reputation system
	reputation, err := NewDecentralizedReputation(system.ID)
	if err != nil {
		log.Fatalf("Failed to initialize reputation system: %v", err)
	}
	log.Printf("Reputation system initialized (Public Key: %s...)", reputation.PublicKey[:16])

	// Connect to bootstrap peer if provided
	if *bootstrap != "" {
		log.Printf("Attempting to connect to bootstrap peer: %s", *bootstrap)
		if err := transport.AddPeer(system.ID, *bootstrap); err != nil {
			log.Printf("Warning: Failed to add bootstrap peer: %v", err)
		}
	}

	// Initialize and start API server
	api := NewAPI(system, transport, storage, reputation)
	
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
	log.Printf("Star system '%s' is now online at %s", system.Name, *address)
	log.Printf("API endpoints available at http://%s", *address)
	if err := api.Start(*address); err != nil {
		log.Fatalf("API server failed: %v", err)
	}
}
