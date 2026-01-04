package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TransportMessage represents information exchanged between nodes
// In protocol v1.0.0+, attestations are REQUIRED for security
type TransportMessage struct {
	Type        string       `json:"type"`                  // "announce", "peer_list", "heartbeat"
	System      *System      `json:"system"`                // sender's system info
	Peers       []*Peer      `json:"peers"`                 // list of known peers
	Attestation *Attestation `json:"attestation"`           // cryptographic proof (REQUIRED in v1.0.0+)
	Version     *VersionInfo `json:"version,omitempty"`     // version info (optional for legacy)
	Timestamp   time.Time    `json:"timestamp"`
}

// StellarTransport handles peer-to-peer communication and network maintenance
type StellarTransport struct {
	localSystem   *System
	storage       *Storage
	peers         map[uuid.UUID]*Peer
	peerVersions  map[uuid.UUID]*ProtocolVersion // Track peer versions for compatibility
	mu            sync.RWMutex
	httpClient    *http.Client
	listenAddr   string
}

// NewStellarTransport creates a new transport protocol handler
func NewStellarTransport(system *System, storage *Storage, listenAddr string) *StellarTransport {
	g := &StellarTransport{
		localSystem:  system,
		storage:      storage,
		peers:        make(map[uuid.UUID]*Peer),
		peerVersions: make(map[uuid.UUID]*ProtocolVersion),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		listenAddr: listenAddr,
	}

	// Load existing peers from storage
	if peers, err := storage.GetPeers(); err == nil {
		for _, peer := range peers {
			g.peers[peer.SystemID] = peer
		}
	}

	return g
}

// Start begins the gossip protocol loops and peer transport server
func (g *StellarTransport) Start() {
	// Start peer-to-peer HTTP server on separate port
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/transport", g.HandleIncomingMessage)

		log.Printf("Peer transport listening on %s", g.listenAddr)
		if err := http.ListenAndServe(g.listenAddr, mux); err != nil {
			log.Printf("Peer transport server error: %v", err)
		}
	}()

	// Periodic gossip with random peers
	go g.gossipLoop(30 * time.Second)

	// Periodic peer list exchange
	go g.peerExchangeLoop(60 * time.Second)

	// Periodic cleanup of dead peers
	go g.cleanupLoop(5 * time.Minute)
}

// gossipLoop periodically sends heartbeats to random peers
func (g *StellarTransport) gossipLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		g.mu.RLock()
		peers := make([]*Peer, 0, len(g.peers))
		for _, peer := range g.peers {
			peers = append(peers, peer)
		}
		g.mu.RUnlock()

		if len(peers) == 0 {
			continue
		}

		// Select random peer
		peer := peers[rand.Intn(len(peers))]
		if err := g.sendHeartbeat(peer); err != nil {
			log.Printf("Failed to send heartbeat to %s: %v", peer.Address, err)
		}
	}
}

// peerExchangeLoop periodically exchanges peer lists with neighbors
func (g *StellarTransport) peerExchangeLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		g.mu.RLock()
		peers := make([]*Peer, 0, len(g.peers))
		for _, peer := range g.peers {
			peers = append(peers, peer)
		}
		g.mu.RUnlock()

		// Exchange with up to 3 random peers
		count := min(3, len(peers))
		for i := 0; i < count; i++ {
			peer := peers[rand.Intn(len(peers))]
			if err := g.exchangePeers(peer); err != nil {
				log.Printf("Failed to exchange peers with %s: %v", peer.Address, err)
			}
		}
	}
}

// cleanupLoop periodically removes dead peers
func (g *StellarTransport) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		threshold := 10 * time.Minute
		cutoff := time.Now().Add(-threshold)

		g.mu.Lock()
		for id, peer := range g.peers {
			if peer.LastSeenAt.Before(cutoff) {
				delete(g.peers, id)
			}
		}
		g.mu.Unlock()

		// Also prune from storage
		if err := g.storage.PruneDeadPeers(threshold); err != nil {
			log.Printf("Failed to prune dead peers: %v", err)
		}
	}
}

// AddPeer adds a new peer to the network
func (g *StellarTransport) AddPeer(systemID uuid.UUID, address string) error {
	peer := &Peer{
		SystemID:   systemID,
		Address:    address,
		LastSeenAt: time.Now(),
	}

	g.mu.Lock()
	g.peers[systemID] = peer
	g.mu.Unlock()

	return g.storage.SavePeer(peer)
}

// GetPeers returns a copy of the current peer list
func (g *StellarTransport) GetPeers() []*Peer {
	g.mu.RLock()
	defer g.mu.RUnlock()

	peers := make([]*Peer, 0, len(g.peers))
	for _, peer := range g.peers {
		peers = append(peers, peer)
	}
	return peers
}

// sendHeartbeat sends a heartbeat message to a peer with cryptographic attestation
// Attestations are MANDATORY in v1.0.0+ for security
func (g *StellarTransport) sendHeartbeat(peer *Peer) error {
	// Get current version info
	versionInfo := GetVersionInfo()
	
	// ALWAYS create and send attestation - it's required in v1.0.0+
	if g.localSystem.Keys == nil {
		return fmt.Errorf("cannot send message: no cryptographic keys")
	}
	
	attestation := SignAttestation(
		g.localSystem.ID,
		peer.SystemID,
		"heartbeat",
		g.localSystem.Keys.PrivateKey,
		g.localSystem.Keys.PublicKey,
	)
	
	msg := TransportMessage{
		Type:        "heartbeat",
		System:      g.localSystem,
		Version:     &versionInfo,
		Attestation: attestation,
		Timestamp:   time.Now(),
	}

	return g.sendMessage(peer.Address, msg)
}

// exchangePeers exchanges peer lists with another node
// Attestations are MANDATORY in v1.0.0+
func (g *StellarTransport) exchangePeers(peer *Peer) error {
	g.mu.RLock()
	peerList := make([]*Peer, 0, len(g.peers))
	for _, p := range g.peers {
		peerList = append(peerList, p)
	}
	g.mu.RUnlock()

	// Get current version info
	versionInfo := GetVersionInfo()

	// ALWAYS create attestation - required in v1.0.0+
	if g.localSystem.Keys == nil {
		return fmt.Errorf("cannot send message: no cryptographic keys")
	}

	attestation := SignAttestation(
		g.localSystem.ID,
		peer.SystemID,
		"peer_exchange",
		g.localSystem.Keys.PrivateKey,
		g.localSystem.Keys.PublicKey,
	)

	msg := TransportMessage{
		Type:        "peer_list",
		System:      g.localSystem,
		Peers:       peerList,
		Version:     &versionInfo,
		Attestation: attestation,
		Timestamp:   time.Now(),
	}

	return g.sendMessage(peer.Address, msg)
}

// sendMessage sends a transport protocol message to an address
func (g *StellarTransport) sendMessage(address string, msg TransportMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s/transport", address)
	resp, err := g.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// HandleMessage processes an incoming transport message with version negotiation
func (g *StellarTransport) HandleMessage(msg TransportMessage) error {
	// Store peer version for future feature negotiation
	if msg.Version != nil {
		if peerVersion, err := ParseVersion(msg.Version.Protocol); err == nil {
			g.mu.Lock()
			if msg.System != nil {
				g.peerVersions[msg.System.ID] = &peerVersion
			}
			g.mu.Unlock()
			
			// Log version compatibility
			if !CurrentProtocolVersion.IsCompatibleWith(peerVersion) {
				log.Printf("WARNING: Incompatible protocol version from %s: %s (we are %s)",
					msg.System.ID, peerVersion.String(), CurrentProtocolVersion.String())
				return fmt.Errorf("incompatible protocol version")
			}
		}
	}
	
	// CRITICAL: Attestations are MANDATORY in v1.0.0+
	// This is a fundamental security requirement, not optional
	if msg.Attestation == nil {
		log.Printf("ERROR: Missing attestation from %s - rejecting message (required in v1.0.0+)", msg.System.ID)
		return fmt.Errorf("attestations are required in protocol v1.0.0+")
	}
	
	if !msg.Attestation.Verify() {
		log.Printf("ERROR: Invalid attestation from %s - rejecting message", msg.System.ID)
		return fmt.Errorf("invalid attestation signature")
	}
	
	// Store verified attestation as proof
	if err := g.storage.SaveAttestation(msg.Attestation); err != nil {
		log.Printf("Failed to save attestation: %v", err)
	} else {
		log.Printf("Verified and stored attestation from %s (%s) (type: %s)",
			msg.Attestation.FromSystemID, msg.System.Name, msg.Attestation.MessageType)
	}

	// Cache peer's system info for map visualization
	if msg.System != nil {
		g.storage.SavePeerSystem(msg.System)
	}
	
	// Update peer last seen time
	if msg.System != nil {
		peer := &Peer{
			SystemID:   msg.System.ID,
			Address:    msg.System.Address,
			LastSeenAt: time.Now(),
		}

		g.mu.Lock()
		g.peers[peer.SystemID] = peer
		g.mu.Unlock()

		g.storage.SavePeer(peer)
	}

	// Handle peer list exchange
	if msg.Type == "peer_list" && len(msg.Peers) > 0 {
		for _, peer := range msg.Peers {
			// Don't add ourselves
			if peer.SystemID == g.localSystem.ID {
				continue
			}

			g.mu.Lock()
			if _, exists := g.peers[peer.SystemID]; !exists {
				peer.LastSeenAt = time.Now()
				g.peers[peer.SystemID] = peer
				g.storage.SavePeer(peer)
			}
			g.mu.Unlock()
		}
	}

	return nil
}

// HandleIncomingMessage handles HTTP requests from peers
func (g *StellarTransport) HandleIncomingMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg TransportMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "Invalid message", http.StatusBadRequest)
		return
	}

	// Process the message using existing HandleMessage logic
	if err := g.HandleMessage(msg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Send success response
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// GetRandomPeerSystem returns a random peer's system info for clustering
func (g *StellarTransport) GetRandomPeerSystem() *System {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.peers) == 0 {
		return nil
	}

	// Get a random peer
	peers := make([]*Peer, 0, len(g.peers))
	for _, peer := range g.peers {
		peers = append(peers, peer)
	}
	
	if len(peers) == 0 {
		return nil
	}

	// For now, just return nil - we'd need to fetch the peer's system info
	// This is a placeholder for future enhancement where we cache peer system info
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
