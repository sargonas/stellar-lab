package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DHT Configuration
const (
	// Alpha is the parallelism factor for iterative lookups
	Alpha = 3

	// K is the default number of nodes to return in FIND_NODE
	K = 5

	// RequestTimeout is how long to wait for a DHT response
	RequestTimeout = 5 * time.Second

	// AnnounceInterval is how often to re-announce ourselves
	AnnounceInterval = 30 * time.Minute

	// RefreshInterval is how often to refresh stale buckets
	RefreshInterval = 60 * time.Minute

	// CachePruneInterval is how often to prune stale cache entries
	CachePruneInterval = 6 * time.Hour

	// CacheMaxAge is how long to keep unverified systems
	CacheMaxAge = 24 * time.Hour
)

// DHT is the main coordinator for distributed hash table operations
type DHT struct {
	localSystem  *System
	routingTable *RoutingTable
	storage      *Storage
	httpClient   *http.Client
	listenAddr   string

	// Pending requests awaiting responses
	pendingRequests map[string]chan *DHTMessage
	pendingMu       sync.RWMutex

	// Shutdown coordination
	shutdown chan struct{}
	wg       sync.WaitGroup
}

// NewDHT creates a new DHT instance
func NewDHT(localSystem *System, storage *Storage, listenAddr string) *DHT {
	dht := &DHT{
		localSystem:     localSystem,
		storage:         storage,
		listenAddr:      listenAddr,
		pendingRequests: make(map[string]chan *DHTMessage),
		shutdown:        make(chan struct{}),
		httpClient: &http.Client{
			Timeout: RequestTimeout,
		},
	}

	// Create routing table
	dht.routingTable = NewRoutingTable(localSystem, storage)

	return dht
}

// Start begins the DHT server and maintenance loops
// Returns an error if the server fails to bind
func (dht *DHT) Start() error {
	// Try to bind the listener BEFORE starting goroutines
	listener, err := net.Listen("tcp", dht.listenAddr)
	if err != nil {
		return fmt.Errorf("DHT failed to bind to %s: %w", dht.listenAddr, err)
	}

	// Start HTTP server for DHT messages
	go dht.serveHTTP(listener)

	// Start maintenance loops
	dht.wg.Add(5)
	go dht.announceLoop()
	go dht.refreshLoop()
	go dht.cacheMaintenanceLoop()
	go dht.peerLivenessLoop()
	go dht.creditCalculationLoop()

	log.Printf("DHT started for %s (%s)", dht.localSystem.Name, dht.localSystem.ID)
	return nil
}

// Stop gracefully shuts down the DHT
func (dht *DHT) Stop() {
	close(dht.shutdown)
	dht.wg.Wait()
	log.Printf("DHT stopped")
}

// serveHTTP runs the HTTP server on an existing listener
func (dht *DHT) serveHTTP(listener net.Listener) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dht", dht.handleDHTMessage)
	mux.HandleFunc("/system", dht.handleSystemInfo)
	mux.HandleFunc("/api/discovery", dht.handleDiscoveryInfo)

	log.Printf("DHT listening on %s", dht.listenAddr)
	if err := http.Serve(listener, mux); err != nil {
		log.Printf("DHT server error: %v", err)
	}
}

// handleDHTMessage processes incoming DHT HTTP requests
func (dht *DHT) handleDHTMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		dht.sendError(w, ErrCodeInvalidMessage, "method not allowed")
		return
	}

	// Limit request body size to 1MB for security
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var msg DHTMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		dht.sendError(w, ErrCodeInvalidMessage, "invalid JSON: "+err.Error())
		return
	}

	// Reject messages claiming our own UUID (impersonation attempt)
	if msg.FromSystem != nil && msg.FromSystem.ID == dht.localSystem.ID {
		dht.sendError(w, ErrCodeInvalidMessage, "cannot impersonate local system")
		return
	}

	// Validate message
	if err := msg.Validate(); err != nil {
		if dhtErr, ok := err.(*DHTError); ok {
			dht.sendError(w, dhtErr.Code, dhtErr.Message)
		} else {
			dht.sendError(w, ErrCodeInvalidMessage, err.Error())
		}
		return
	}

	// Validate identity binding (UUID must always map to same public key)
	if msg.FromSystem != nil && msg.FromSystem.Keys != nil {
		pubKeyStr := base64.StdEncoding.EncodeToString(msg.FromSystem.Keys.PublicKey)
		valid, isNew, err := dht.storage.ValidateIdentityBinding(msg.FromSystem.ID, pubKeyStr)
		if err != nil {
			log.Printf("Identity binding check failed: %v", err)
			dht.sendError(w, ErrCodeInternalError, "identity validation error")
			return
		}
		if !valid {
			log.Printf("UUID spoofing attempt detected: %s", msg.FromSystem.ID)
			dht.sendError(w, ErrCodeInvalidMessage, "identity mismatch: UUID bound to different key")
			return
		}
		if isNew {
			log.Printf("New identity bound: %s", msg.FromSystem.ID)
		}
	}

	// Validate coordinates match expected position based on UUID + Sponsor
	lookupSponsor := func(sponsorID uuid.UUID) *System {
		// Check routing table cache first
		if cached := dht.routingTable.GetCachedSystem(sponsorID); cached != nil {
			return cached
		}
		// Try storage
		if stored, err := dht.storage.GetPeerSystem(sponsorID); err == nil {
			return stored
		}
		return nil
	}
	if !ValidateCoordinates(msg.FromSystem, lookupSponsor) {
		dht.sendError(w, ErrCodeInvalidMessage, "coordinates invalid for UUID and sponsor")
		return
	}

	// Store attestation
	if err := dht.storage.SaveAttestation(msg.Attestation); err != nil {
		log.Printf("Failed to save attestation: %v", err)
	}

	// Update routing table with sender's info
	dht.routingTable.Update(msg.FromSystem)

	// Handle based on message type
	var response *DHTMessage
	var err error

	if msg.IsResponse {
		// This is a response to one of our requests
		dht.handleResponse(&msg)
		w.WriteHeader(http.StatusOK)
		return
	}

	switch msg.Type {
	case MessageTypePing:
		response, err = dht.handlePing(&msg)
	case MessageTypeFindNode:
		response, err = dht.handleFindNode(&msg)
	case MessageTypeAnnounce:
		response, err = dht.handleAnnounce(&msg)
	default:
		dht.sendError(w, ErrCodeInvalidMessage, "unknown message type")
		return
	}

	if err != nil {
		dht.sendError(w, ErrCodeInternalError, err.Error())
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handlePing processes a ping request
func (dht *DHT) handlePing(msg *DHTMessage) (*DHTMessage, error) {
	log.Printf("PING from %s (%s) [v%s]", msg.FromSystem.Name, msg.FromSystem.ID, msg.Version)

	// Mark the sender as verified since they successfully contacted us
	dht.routingTable.MarkVerified(msg.FromSystem.ID)

	return NewPingResponse(dht.localSystem, msg.RequestID)
}

// handleFindNode processes a find_node request
func (dht *DHT) handleFindNode(msg *DHTMessage) (*DHTMessage, error) {
	if msg.TargetID == nil {
		return nil, fmt.Errorf("find_node requires target_id")
	}

	// Only log FIND_NODE at debug level (commented out to reduce noise)
	// log.Printf("FIND_NODE for %s from %s", msg.TargetID.String()[:8], msg.FromSystem.Name)

	// Get K closest nodes to the target
	closest := dht.routingTable.GetClosest(*msg.TargetID, K)

	// Include ourselves if we're close enough
	selfIncluded := false
	for _, sys := range closest {
		if sys.ID == dht.localSystem.ID {
			selfIncluded = true
			break
		}
	}

	// Add self if we're one of the K closest and not already included
	if !selfIncluded && len(closest) < K {
		closest = append(closest, dht.localSystem)
	}

	return NewFindNodeResponse(dht.localSystem, closest, msg.RequestID)
}

// handleAnnounce processes an announce request
func (dht *DHT) handleAnnounce(msg *DHTMessage) (*DHTMessage, error) {
	log.Printf("ANNOUNCE from %s (%s) [v%s]", msg.FromSystem.Name, msg.FromSystem.ID, msg.Version)

	// Cache the announcing system (already done in handleDHTMessage via Update)
	// Mark as verified since they're actively announcing
	dht.routingTable.CacheSystem(msg.FromSystem, msg.FromSystem.ID, true)

	return NewAnnounceResponse(dht.localSystem, msg.RequestID)
}

// handleResponse processes a response to a pending request
func (dht *DHT) handleResponse(msg *DHTMessage) {
	dht.pendingMu.RLock()
	ch, exists := dht.pendingRequests[msg.RequestID]
	dht.pendingMu.RUnlock()

	if exists {
		select {
		case ch <- msg:
		default:
			// Channel full or closed, ignore
		}
	}
}

// sendError sends an error response
func (dht *DHT) sendError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": DHTError{Code: code, Message: message},
	})
}

// handleSystemInfo returns this node's system info
func (dht *DHT) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dht.localSystem)
}

// handleDiscoveryInfo returns discovery info for bootstrapping
func (dht *DHT) handleDiscoveryInfo(w http.ResponseWriter, r *http.Request) {
	systems := []DiscoverySystem{}
	seenIDs := make(map[uuid.UUID]bool)

	// Add self
	rtSize := dht.routingTable.GetRoutingTableSize()
	selfHasCapacity := rtSize < dht.localSystem.GetMaxPeers()

	systems = append(systems, DiscoverySystem{
		ID:           dht.localSystem.ID.String(),
		Name:         dht.localSystem.Name,
		X:            dht.localSystem.X,
		Y:            dht.localSystem.Y,
		Z:            dht.localSystem.Z,
		PeerAddress:  dht.localSystem.PeerAddress,
		CurrentPeers: rtSize,
		MaxPeers:     dht.localSystem.GetMaxPeers(),
		HasCapacity:  selfHasCapacity,
	})
	seenIDs[dht.localSystem.ID] = true

	// Add nodes from routing table
	for _, sys := range dht.routingTable.GetAllRoutingTableNodes() {
		systems = append(systems, DiscoverySystem{
			ID:          sys.ID.String(),
			Name:        sys.Name,
			X:           sys.X,
			Y:           sys.Y,
			Z:           sys.Z,
			PeerAddress: sys.PeerAddress,
			MaxPeers:    sys.GetMaxPeers(),
			HasCapacity: true, // Assume yes, they'll reject if not
		})
		seenIDs[sys.ID] = true
	}

	// Also add verified cached systems not already included
	for _, sys := range dht.routingTable.GetVerifiedCachedSystems() {
		if !seenIDs[sys.ID] {
			systems = append(systems, DiscoverySystem{
				ID:          sys.ID.String(),
				Name:        sys.Name,
				X:           sys.X,
				Y:           sys.Y,
				Z:           sys.Z,
				PeerAddress: sys.PeerAddress,
				MaxPeers:    sys.GetMaxPeers(),
				HasCapacity: true,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(systems)
}

// === Outbound Operations ===

// sendRequest sends a DHT request and waits for response
func (dht *DHT) sendRequest(address string, msg *DHTMessage) (*DHTMessage, error) {
	// Generate request ID if not set
	if msg.RequestID == "" {
		msg.RequestID = uuid.New().String()
	}

	// Register pending request
	respCh := make(chan *DHTMessage, 1)
	dht.pendingMu.Lock()
	dht.pendingRequests[msg.RequestID] = respCh
	dht.pendingMu.Unlock()

	defer func() {
		dht.pendingMu.Lock()
		delete(dht.pendingRequests, msg.RequestID)
		dht.pendingMu.Unlock()
	}()

	// Send request
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("http://%s/dht", address)
	resp, err := dht.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error DHTError `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, &errResp.Error
	}

	// Parse response
	var response DHTMessage
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	// Validate response
	if err := response.Validate(); err != nil {
		return nil, err
	}

	// Update routing table with responder's info
	if response.FromSystem != nil {
		dht.routingTable.Update(response.FromSystem)
		dht.routingTable.MarkVerified(response.FromSystem.ID)
	}

	// Cache any systems in the response
	for _, sys := range response.ClosestNodes {
		dht.routingTable.CacheSystem(sys, response.FromSystem.ID, false)
		dht.routingTable.Update(sys)
	}

	return &response, nil
}

// Ping sends a ping to a node and returns their system info
func (dht *DHT) Ping(address string) (*System, error) {
	msg, err := NewPingRequest(dht.localSystem, "")
	if err != nil {
		return nil, err
	}

	resp, err := dht.sendRequest(address, msg)
	if err != nil {
		return nil, err
	}

	return resp.FromSystem, nil
}

// PingNode pings a node by system (using its address)
func (dht *DHT) PingNode(sys *System) error {
	if sys.PeerAddress == "" {
		return fmt.Errorf("no peer address for %s", sys.Name)
	}

	_, err := dht.Ping(sys.PeerAddress)
	if err != nil {
		dht.routingTable.MarkFailed(sys.ID)
		return err
	}

	dht.routingTable.MarkVerified(sys.ID)
	return nil
}

// FindNode performs a single find_node query to a specific address
func (dht *DHT) FindNodeDirect(address string, targetID uuid.UUID) ([]*System, error) {
	msg, err := NewFindNodeRequest(dht.localSystem, targetID, "")
	if err != nil {
		return nil, err
	}

	resp, err := dht.sendRequest(address, msg)
	if err != nil {
		return nil, err
	}

	return resp.ClosestNodes, nil
}

// Announce sends an announce message to a specific node
func (dht *DHT) AnnounceTo(address string) error {
	msg, err := NewAnnounceRequest(dht.localSystem, "")
	if err != nil {
		return err
	}

	_, err = dht.sendRequest(address, msg)
	return err
}

// === Accessors ===

// GetRoutingTable returns the routing table
func (dht *DHT) GetRoutingTable() *RoutingTable {
	return dht.routingTable
}

// GetLocalSystem returns the local system
func (dht *DHT) GetLocalSystem() *System {
	return dht.localSystem
}

// GetStorage returns the storage
func (dht *DHT) GetStorage() *Storage {
	return dht.storage
}