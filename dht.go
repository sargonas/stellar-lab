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
	K = 20

	// RequestTimeout is how long to wait for a DHT response
	RequestTimeout = 5 * time.Second

	// AnnounceInterval is how often to re-announce ourselves
	AnnounceInterval = 30 * time.Minute

	// CachePruneInterval is how often to prune stale cache entries
	CachePruneInterval = 2 * time.Hour

	// CacheMaxAge is how long to keep systems that haven't been seen
	CacheMaxAge = 48 * time.Hour

	// VerificationCutoff is how long before a system is considered "stale" for full-sync
	// Extended from 24h to 36h to avoid missing alive-but-quiet nodes
	VerificationCutoff = 36 * time.Hour
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

	// Inbound connection tracking (for outbound-only detection)
	startTime           time.Time
	hasReceivedInbound  bool
	inboundMu           sync.RWMutex
	lastInboundWarning  time.Time

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
		startTime:       time.Now(),
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
	go dht.cacheMaintenanceLoop()
	go dht.peerLivenessLoop()
	go dht.gossipValidationLoop()
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

// markInboundReceived records that we've received an inbound connection
func (dht *DHT) markInboundReceived() {
	dht.inboundMu.Lock()
	dht.hasReceivedInbound = true
	dht.inboundMu.Unlock()
}

// checkInboundStatus logs a warning if no inbound connections after startup period
func (dht *DHT) checkInboundStatus() {
	dht.inboundMu.RLock()
	hasInbound := dht.hasReceivedInbound
	lastWarning := dht.lastInboundWarning
	dht.inboundMu.RUnlock()

	if hasInbound {
		return
	}

	// Only warn after 10 minutes of uptime
	if time.Since(dht.startTime) < 10*time.Minute {
		return
	}

	// Repeat warning every 6 hours
	if !lastWarning.IsZero() && time.Since(lastWarning) < 6*time.Hour {
		return
	}

	dht.inboundMu.Lock()
	dht.lastInboundWarning = time.Now()
	dht.inboundMu.Unlock()

	log.Printf("WARNING: No inbound connections received after 10 minutes.")
	log.Printf("  Your node may be in outbound-only mode (can see network but others can't reach you).")
	log.Printf("  Check that port %s is open and forwarded correctly, as UPnP may have failed.", dht.listenAddr)
}

// updateRoutingTable adds a node to the peer cache
// Simplified from Kademlia - we just cache all peers we hear about
func (dht *DHT) updateRoutingTable(sys *System) {
	if sys == nil {
		return
	}
	dht.routingTable.Update(sys)
}

// serveHTTP runs the HTTP server on an existing listener
func (dht *DHT) serveHTTP(listener net.Listener) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dht", dht.handleDHTMessage)
	mux.HandleFunc("/system", dht.handleSystemInfo)
	mux.HandleFunc("/api/discovery", dht.handleDiscoveryInfo)
	mux.HandleFunc("/api/full-sync", dht.handleFullSync)

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

	// Store attestation with our local ID as the receiver
	// Note: We pass our ID separately to preserve the original signed attestation
	// The attestation's ToSystemID (uuid.Nil) stays unchanged so signature remains valid
	if err := dht.storage.SaveAttestation(msg.Attestation, dht.localSystem.ID); err != nil {
		log.Printf("Failed to save attestation: %v", err)
	}

	// Update routing table with sender's info (proper Kademlia LRS-ping if bucket full)
	dht.updateRoutingTable(msg.FromSystem)

	// Handle based on message type
	var response *DHTMessage
	var err error

	if msg.IsResponse {
		// This is a response to one of our requests
		dht.handleResponse(&msg)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Mark that we've received an inbound request (not a response)
	dht.markInboundReceived()

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

	// Check if sender is using old protocol (no targeted attestation)
	dht.warnIfOldProtocol(msg)

	return NewPingResponse(dht.localSystem, msg.FromSystem.ID, msg.RequestID)
}

// handleFindNode processes a find_node request
func (dht *DHT) handleFindNode(msg *DHTMessage) (*DHTMessage, error) {
	if msg.TargetID == nil {
		return nil, fmt.Errorf("find_node requires target_id")
	}

	// Mark the sender as verified since they successfully contacted us
	dht.routingTable.MarkVerified(msg.FromSystem.ID)

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

	return NewFindNodeResponse(dht.localSystem, msg.FromSystem.ID, closest, msg.RequestID)
}

// handleAnnounce processes an announce request
func (dht *DHT) handleAnnounce(msg *DHTMessage) (*DHTMessage, error) {
	log.Printf("ANNOUNCE from %s (%s) [v%s]", msg.FromSystem.Name, msg.FromSystem.ID, msg.Version)

	// Mark the sender as verified since they successfully contacted us
	dht.routingTable.MarkVerified(msg.FromSystem.ID)

	// Cache the announcing system (already done in handleDHTMessage via Update)
	// Mark as verified since they're actively announcing
	dht.routingTable.CacheSystem(msg.FromSystem, msg.FromSystem.ID, true)

	// Check if sender is using old protocol (no targeted attestation)
	dht.warnIfOldProtocol(msg)

	return NewAnnounceResponse(dht.localSystem, msg.FromSystem.ID, msg.RequestID)
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

	// Also add verified cached systems not already included (only recently verified)
	for _, sys := range dht.routingTable.GetVerifiedCachedSystems(24 * time.Hour) {
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

// FullSyncSystem represents a system in the full-sync response
type FullSyncSystem struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Z           float64 `json:"z"`
	PeerAddress string  `json:"peer_address"`
	StarClass   string  `json:"star_class"`
	InfoVersion int64   `json:"info_version"`
	LastSeen    int64   `json:"last_seen"` // Unix timestamp, 0 if never directly seen
}

// FullSyncResponse is the response from /api/full-sync
type FullSyncResponse struct {
	ProtocolVersion string           `json:"protocol_version"`
	Timestamp       int64            `json:"timestamp"`
	LocalSystem     FullSyncSystem   `json:"local_system"`
	Systems         []FullSyncSystem `json:"systems"`
	TotalCount      int              `json:"total_count"`
}

// handleFullSync returns ALL known systems for complete galaxy sync
// This endpoint enables new nodes to learn about the entire network in one request
// rather than iteratively discovering nodes through Kademlia lookups.
func (dht *DHT) handleFullSync(w http.ResponseWriter, r *http.Request) {
	// Build list of all known systems
	systems := []FullSyncSystem{}
	seenIDs := make(map[uuid.UUID]bool)

	// Add ourselves
	seenIDs[dht.localSystem.ID] = true

	// Cutoff for "recently verified" - only share systems we've actually talked to
	// within the cutoff period to prevent spreading stale/dead node info
	verificationCutoff := time.Now().Add(-VerificationCutoff)

	// Add routing table nodes first (these are actively maintained)
	for _, sys := range dht.routingTable.GetAllRoutingTableNodes() {
		if seenIDs[sys.ID] {
			continue
		}
		seenIDs[sys.ID] = true

		systems = append(systems, FullSyncSystem{
			ID:          sys.ID.String(),
			Name:        sys.Name,
			X:           sys.X,
			Y:           sys.Y,
			Z:           sys.Z,
			PeerAddress: sys.PeerAddress,
			StarClass:   sys.Stars.Primary.Class,
			InfoVersion: sys.InfoVersion,
			LastSeen:    time.Now().Unix(), // Routing table nodes are actively maintained
		})
	}

	// Add cached systems ONLY if they've been verified recently
	// This prevents spreading stale gossip about dead nodes
	for _, sys := range dht.routingTable.GetAllCachedSystems() {
		if seenIDs[sys.ID] {
			continue
		}

		// Get cache metadata to check verification status
		cached := dht.routingTable.GetCachedSystemMeta(sys.ID)
		if cached == nil {
			continue
		}

		// Only include verified systems with recent verification
		if !cached.Verified || cached.LastVerified.IsZero() || cached.LastVerified.Before(verificationCutoff) {
			continue // Skip unverified or stale systems
		}

		seenIDs[sys.ID] = true

		systems = append(systems, FullSyncSystem{
			ID:          sys.ID.String(),
			Name:        sys.Name,
			X:           sys.X,
			Y:           sys.Y,
			Z:           sys.Z,
			PeerAddress: sys.PeerAddress,
			StarClass:   sys.Stars.Primary.Class,
			InfoVersion: sys.InfoVersion,
			LastSeen:    cached.LastVerified.Unix(),
		})
	}

	response := FullSyncResponse{
		ProtocolVersion: CurrentProtocolVersion.String(),
		Timestamp:       time.Now().Unix(),
		LocalSystem: FullSyncSystem{
			ID:          dht.localSystem.ID.String(),
			Name:        dht.localSystem.Name,
			X:           dht.localSystem.X,
			Y:           dht.localSystem.Y,
			Z:           dht.localSystem.Z,
			PeerAddress: dht.localSystem.PeerAddress,
			StarClass:   dht.localSystem.Stars.Primary.Class,
			InfoVersion: dht.localSystem.InfoVersion,
			LastSeen:    time.Now().Unix(),
		},
		Systems:    systems,
		TotalCount: len(systems) + 1, // +1 for local system
	}

	log.Printf("FULL-SYNC: returning %d verified systems to %s", response.TotalCount, r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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

	// Update routing table with responder's info (proper Kademlia LRS-ping if bucket full)
	if response.FromSystem != nil {
		dht.updateRoutingTable(response.FromSystem)
		dht.routingTable.MarkVerified(response.FromSystem.ID)
	}

	// Cache any systems in the response and try to add to routing table
	for _, sys := range response.ClosestNodes {
		dht.routingTable.CacheSystem(sys, response.FromSystem.ID, false)
		dht.updateRoutingTable(sys)
	}

	return &response, nil
}

// Ping sends a ping to a node and returns their system info
// If the recipient's UUID is unknown (first contact), uses uuid.Nil
func (dht *DHT) Ping(address string) (*System, error) {
	// Try to look up recipient's UUID from routing table/cache
	recipientID := dht.routingTable.GetSystemIDByAddress(address)

	msg, err := NewPingRequest(dht.localSystem, recipientID, "")
	if err != nil {
		return nil, err
	}

	resp, err := dht.sendRequest(address, msg)
	if err != nil {
		return nil, err
	}

	return resp.FromSystem, nil
}

// PingNode pings a node by system (using its address and known UUID)
func (dht *DHT) PingNode(sys *System) error {
	if sys.PeerAddress == "" {
		return fmt.Errorf("no peer address for %s", sys.Name)
	}

	// We know the UUID since we have the System
	msg, err := NewPingRequest(dht.localSystem, sys.ID, "")
	if err != nil {
		return err
	}

	resp, err := dht.sendRequest(sys.PeerAddress, msg)
	if err != nil {
		dht.routingTable.MarkFailed(sys.ID)
		return err
	}

	// Check if the responding system matches who we expected
	if resp.FromSystem != nil && resp.FromSystem.ID != sys.ID {
		// Different node responded - the address now belongs to someone else
		log.Printf("UUID mismatch at %s: expected %s (%s), got %s (%s) - removing stale entry",
			sys.PeerAddress,
			sys.ID.String()[:8], sys.Name,
			resp.FromSystem.ID.String()[:8], resp.FromSystem.Name)

		// Remove stale entry from routing table and storage
		dht.routingTable.Remove(sys.ID)
		if err := dht.storage.DeletePeerSystem(sys.ID); err != nil {
			log.Printf("Warning: failed to delete stale peer system %s: %v", sys.ID.String()[:8], err)
		}

		// Don't return error - address is live, just different owner
		// sendRequest() already added the responder to our routing table
		return nil
	}

	dht.routingTable.MarkVerified(sys.ID)
	return nil
}

// FindNodeDirectToSystem performs a find_node query to a known system
func (dht *DHT) FindNodeDirectToSystem(sys *System, targetID uuid.UUID) ([]*System, error) {
	if sys.PeerAddress == "" {
		return nil, fmt.Errorf("no peer address for %s", sys.Name)
	}

	msg, err := NewFindNodeRequest(dht.localSystem, sys.ID, targetID, "")
	if err != nil {
		return nil, err
	}

	resp, err := dht.sendRequest(sys.PeerAddress, msg)
	if err != nil {
		return nil, err
	}

	return resp.ClosestNodes, nil
}

// AnnounceToSystem sends an announce message to a known system
func (dht *DHT) AnnounceToSystem(sys *System) error {
	if sys.PeerAddress == "" {
		return fmt.Errorf("no peer address for %s", sys.Name)
	}

	msg, err := NewAnnounceRequest(dht.localSystem, sys.ID, "")
	if err != nil {
		return err
	}

	_, err = dht.sendRequest(sys.PeerAddress, msg)
	return err
}

// === Protocol Compatibility ===

// Track which systems we've warned about old protocol (to avoid log spam)
var (
	oldProtocolWarned   = make(map[uuid.UUID]time.Time)
	oldProtocolWarnedMu sync.RWMutex
)

// warnIfOldProtocol logs a warning if the sender is using an old protocol version
// that doesn't include ToSystemID in attestations. Only warns once per hour per system.
func (dht *DHT) warnIfOldProtocol(msg *DHTMessage) {
	// Check if attestation has targeted recipient (v1.6.0+)
	if msg.HasTargetedAttestation() {
		return // New protocol, all good
	}

	// Rate limit warnings to once per hour per system
	oldProtocolWarnedMu.RLock()
	lastWarn, warned := oldProtocolWarned[msg.FromSystem.ID]
	oldProtocolWarnedMu.RUnlock()

	if warned && time.Since(lastWarn) < 1*time.Hour {
		return // Already warned recently
	}

	// Log warning
	log.Printf("⚠ %s (%s) is using old protocol v%s without targeted attestations. "+
		"They should upgrade to v1.6.0+.",
		msg.FromSystem.Name, msg.FromSystem.ID.String()[:8], msg.Version)

	// Update warning time
	oldProtocolWarnedMu.Lock()
	oldProtocolWarned[msg.FromSystem.ID] = time.Now()
	oldProtocolWarnedMu.Unlock()
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