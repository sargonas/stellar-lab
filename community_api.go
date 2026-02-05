package main

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"
)

// CommunityAPI handles public read-only API endpoints for third-party developers
// These endpoints are served on the protocol port (:7867) alongside DHT endpoints
type CommunityAPI struct {
	dht     *DHT
	storage *Storage

	// Rate limiting: 60 requests/minute per IP
	rateLimiter *ipRateLimiter
}

// NewCommunityAPI creates a new community API handler
func NewCommunityAPI(dht *DHT, storage *Storage) *CommunityAPI {
	api := &CommunityAPI{
		dht:         dht,
		storage:     storage,
		rateLimiter: newIPRateLimiter(60, time.Minute),
	}
	return api
}

// RegisterRoutes adds community API routes to the given mux
func (api *CommunityAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/galaxy", api.withCORSAndRateLimit(api.handleGalaxy))
	mux.HandleFunc("/api/network", api.withCORSAndRateLimit(api.handleNetwork))
	mux.HandleFunc("/api/system-profile", api.withCORSAndRateLimit(api.handleSystemProfile))
}

// withCORSAndRateLimit wraps a handler with CORS headers and rate limiting
func (api *CommunityAPI) withCORSAndRateLimit(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers for all responses
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Only allow GET requests
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Rate limiting by IP
		ip := getClientIP(r)
		if !api.rateLimiter.allow(ip) {
			http.Error(w, "Rate limit exceeded (60 requests/minute)", http.StatusTooManyRequests)
			return
		}

		handler(w, r)
	}
}

// =============================================================================
// RESPONSE TYPES
// =============================================================================

// GalaxyResponse is the response for GET /api/galaxy
type GalaxyResponse struct {
	NodeID       string          `json:"node_id"`
	NodeName     string          `json:"node_name"`
	GeneratedAt  int64           `json:"generated_at"`
	Systems      []GalaxySystem  `json:"systems"`
	Connections  []GalaxyConnection `json:"connections"`
	TotalSystems int             `json:"total_systems"`
}

// GalaxySystem represents a system in the galaxy response
type GalaxySystem struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Coordinates GalaxyCoordinates `json:"coordinates"`
	Stars       GalaxyStars       `json:"stars"`
	MaxPeers    int               `json:"max_peers"`
	Credits     *GalaxyCredits    `json:"credits"` // nil for remote systems
	LearnedAt   int64             `json:"learned_at"`
	IsSelf      bool              `json:"is_self"`
}

// GalaxyCoordinates represents 3D coordinates
type GalaxyCoordinates struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// GalaxyStars represents star information
type GalaxyStars struct {
	Primary   GalaxyStar `json:"primary"`
	IsBinary  bool       `json:"is_binary"`
	IsTrinary bool       `json:"is_trinary"`
}

// GalaxyStar represents a single star
type GalaxyStar struct {
	Class       string `json:"class"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

// GalaxyCredits represents credit information (only populated for self)
type GalaxyCredits struct {
	Balance   int64  `json:"balance"`
	Rank      string `json:"rank"`
	RankColor string `json:"rank_color"`
}

// GalaxyConnection represents a connection between systems
type GalaxyConnection struct {
	FromID        string `json:"from_id"`
	ToID          string `json:"to_id"`
	Bidirectional bool   `json:"bidirectional"`
}

// NetworkResponse is the response for GET /api/network
type NetworkResponse struct {
	Self                NetworkSelf   `json:"self"`
	KnownPeers          []NetworkPeer `json:"known_peers"`
	SeedNodesURL        string        `json:"seed_nodes_url"`
	NetworkSizeEstimate int           `json:"network_size_estimate"`
	Timestamp           int64         `json:"timestamp"`
}

// NetworkSelf represents this node's identity
type NetworkSelf struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	PublicAddress   string `json:"public_address"`
	ProtocolVersion string `json:"protocol_version"`
}

// NetworkPeer represents a known peer
type NetworkPeer struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	PublicAddress  string `json:"public_address"`
	LastSeen       int64  `json:"last_seen"`
	InRoutingTable bool   `json:"in_routing_table"`
}

// SystemProfileResponse is the response for GET /api/system-profile
type SystemProfileResponse struct {
	ID           string                   `json:"id"`
	Name         string                   `json:"name"`
	Coordinates  GalaxyCoordinates        `json:"coordinates"`
	Stars        GalaxyStars              `json:"stars"`
	MaxPeers     int                      `json:"max_peers"`
	CurrentPeers int                      `json:"current_peers"`
	Credits      SystemProfileCredits     `json:"credits"`
	Version      SystemProfileVersion     `json:"version"`
}

// SystemProfileCredits includes detailed credit info
type SystemProfileCredits struct {
	Balance          int64   `json:"balance"`
	Rank             string  `json:"rank"`
	RankColor        string  `json:"rank_color"`
	LongevityWeeks   float64 `json:"longevity_weeks"`
	LongevityBonusPct float64 `json:"longevity_bonus_pct"`
}

// SystemProfileVersion includes version info
type SystemProfileVersion struct {
	Software string `json:"software"`
	Build    string `json:"build"`
	Protocol string `json:"protocol"`
}

// =============================================================================
// ENDPOINT HANDLERS
// =============================================================================

// handleGalaxy returns the full galaxy snapshot
func (api *CommunityAPI) handleGalaxy(w http.ResponseWriter, r *http.Request) {
	self := api.dht.GetLocalSystem()
	rt := api.dht.GetRoutingTable()

	// Build systems list
	systems := make([]GalaxySystem, 0)

	// Add self with credits
	selfCredits := api.getSelfCredits()
	systems = append(systems, GalaxySystem{
		ID:   self.ID.String(),
		Name: self.Name,
		Coordinates: GalaxyCoordinates{
			X: self.X,
			Y: self.Y,
			Z: self.Z,
		},
		Stars: GalaxyStars{
			Primary: GalaxyStar{
				Class:       self.Stars.Primary.Class,
				Description: self.Stars.Primary.Description,
				Color:       self.Stars.Primary.Color,
			},
			IsBinary:  self.Stars.IsBinary,
			IsTrinary: self.Stars.IsTrinary,
		},
		MaxPeers:  self.GetMaxPeers(),
		Credits:   selfCredits,
		LearnedAt: self.CreatedAt.Unix(),
		IsSelf:    true,
	})

	// Add all cached systems (remote systems, no credits)
	cachedSystems := rt.GetAllCachedSystemsWithMeta()
	for _, cached := range cachedSystems {
		sys := cached.System
		systems = append(systems, GalaxySystem{
			ID:   sys.ID.String(),
			Name: sys.Name,
			Coordinates: GalaxyCoordinates{
				X: sys.X,
				Y: sys.Y,
				Z: sys.Z,
			},
			Stars: GalaxyStars{
				Primary: GalaxyStar{
					Class:       sys.Stars.Primary.Class,
					Description: sys.Stars.Primary.Description,
					Color:       sys.Stars.Primary.Color,
				},
				IsBinary:  sys.Stars.IsBinary,
				IsTrinary: sys.Stars.IsTrinary,
			},
			MaxPeers:  sys.GetMaxPeers(),
			Credits:   nil, // We don't know remote systems' credits
			LearnedAt: cached.LearnedAt.Unix(),
			IsSelf:    false,
		})
	}

	// Build connections with bidirectional detection
	connections := api.buildConnections()

	response := GalaxyResponse{
		NodeID:       self.ID.String(),
		NodeName:     self.Name,
		GeneratedAt:  time.Now().Unix(),
		Systems:      systems,
		Connections:  connections,
		TotalSystems: len(systems),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleNetwork returns network crawling/discovery info
func (api *CommunityAPI) handleNetwork(w http.ResponseWriter, r *http.Request) {
	self := api.dht.GetLocalSystem()
	rt := api.dht.GetRoutingTable()

	// Get routing table peers (active, verified)
	rtPeers := rt.GetAllRoutingTableNodesWithMeta()
	rtPeerIDs := make(map[string]bool)
	for _, p := range rtPeers {
		rtPeerIDs[p.System.ID.String()] = true
	}

	// Build known peers list
	knownPeers := make([]NetworkPeer, 0)

	// Add routing table peers first
	for _, cached := range rtPeers {
		sys := cached.System
		if sys.PeerAddress == "" {
			continue // Skip if no known address
		}
		knownPeers = append(knownPeers, NetworkPeer{
			ID:             sys.ID.String(),
			Name:           sys.Name,
			PublicAddress:  sys.PeerAddress,
			LastSeen:       cached.LastVerified.Unix(),
			InRoutingTable: true,
		})
	}

	// Add cached systems not in routing table
	allCached := rt.GetAllCachedSystemsWithMeta()
	for _, cached := range allCached {
		sys := cached.System
		if rtPeerIDs[sys.ID.String()] {
			continue // Already added as routing table peer
		}
		if sys.PeerAddress == "" {
			continue // Skip if no known address
		}
		knownPeers = append(knownPeers, NetworkPeer{
			ID:             sys.ID.String(),
			Name:           sys.Name,
			PublicAddress:  sys.PeerAddress,
			LastSeen:       cached.LearnedAt.Unix(),
			InRoutingTable: false,
		})
	}

	response := NetworkResponse{
		Self: NetworkSelf{
			ID:              self.ID.String(),
			Name:            self.Name,
			PublicAddress:   self.PeerAddress,
			ProtocolVersion: CurrentProtocolVersion.String(),
		},
		KnownPeers:          knownPeers,
		SeedNodesURL:        SeedNodeListURL,
		NetworkSizeEstimate: len(allCached) + 1, // +1 for self
		Timestamp:           time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSystemProfile returns this node's profile
func (api *CommunityAPI) handleSystemProfile(w http.ResponseWriter, r *http.Request) {
	self := api.dht.GetLocalSystem()
	rt := api.dht.GetRoutingTable()

	// Get credit info
	balance, err := api.storage.GetCreditBalance(self.ID)
	var credits SystemProfileCredits
	if err == nil {
		rank := GetRank(balance.Balance)
		var longevityWeeks float64
		if balance.LongevityStart > 0 {
			longevitySeconds := time.Now().Unix() - balance.LongevityStart
			longevityWeeks = float64(longevitySeconds) / (7 * 24 * 3600)
		}
		longevityBonus := min(longevityWeeks*0.01, 0.52)

		credits = SystemProfileCredits{
			Balance:           balance.Balance,
			Rank:              rank.Name,
			RankColor:         rank.Color,
			LongevityWeeks:    longevityWeeks,
			LongevityBonusPct: longevityBonus * 100,
		}
	}

	response := SystemProfileResponse{
		ID:   self.ID.String(),
		Name: self.Name,
		Coordinates: GalaxyCoordinates{
			X: self.X,
			Y: self.Y,
			Z: self.Z,
		},
		Stars: GalaxyStars{
			Primary: GalaxyStar{
				Class:       self.Stars.Primary.Class,
				Description: self.Stars.Primary.Description,
				Color:       self.Stars.Primary.Color,
			},
			IsBinary:  self.Stars.IsBinary,
			IsTrinary: self.Stars.IsTrinary,
		},
		MaxPeers:     self.GetMaxPeers(),
		CurrentPeers: rt.GetRoutingTableSize(),
		Credits:      credits,
		Version: SystemProfileVersion{
			Software: "stellar-lab",
			Build:    BuildVersion,
			Protocol: CurrentProtocolVersion.String(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// =============================================================================
// HELPER METHODS
// =============================================================================

// getSelfCredits retrieves this node's credit info
func (api *CommunityAPI) getSelfCredits() *GalaxyCredits {
	self := api.dht.GetLocalSystem()
	balance, err := api.storage.GetCreditBalance(self.ID)
	if err != nil {
		return nil
	}
	rank := GetRank(balance.Balance)
	return &GalaxyCredits{
		Balance:   balance.Balance,
		Rank:      rank.Name,
		RankColor: rank.Color,
	}
}

// buildConnections builds connection list with bidirectional detection
func (api *CommunityAPI) buildConnections() []GalaxyConnection {
	// Get connections from storage
	storedConns, err := api.storage.GetAllConnections(time.Hour)
	if err != nil {
		storedConns = []TopologyEdge{}
	}

	// Also add our direct connections (routing table peers)
	selfID := api.dht.GetLocalSystem().ID.String()
	peers := api.dht.GetRoutingTable().GetAllRoutingTableNodes()

	// Build edge set for deduplication and bidirectional detection
	// Key: "fromID:toID"
	edgeSet := make(map[string]bool)

	// Add stored connections
	for _, c := range storedConns {
		edgeSet[c.FromID+":"+c.ToID] = true
	}

	// Add our direct peer connections (both directions since routing table peers are bidirectional)
	for _, peer := range peers {
		peerID := peer.ID.String()
		edgeSet[selfID+":"+peerID] = true
		edgeSet[peerID+":"+selfID] = true
	}

	// Build final connection list with bidirectional flag
	// Use canonical ordering (smaller ID first) to avoid duplicates
	seenPairs := make(map[string]bool)
	connections := make([]GalaxyConnection, 0)

	for edge := range edgeSet {
		var fromID, toID string
		for i, c := range edge {
			if c == ':' {
				fromID = edge[:i]
				toID = edge[i+1:]
				break
			}
		}

		// Create canonical key (smaller ID first)
		var canonicalKey string
		if fromID < toID {
			canonicalKey = fromID + ":" + toID
		} else {
			canonicalKey = toID + ":" + fromID
		}

		if seenPairs[canonicalKey] {
			continue
		}
		seenPairs[canonicalKey] = true

		// Check if bidirectional
		reverseKey := toID + ":" + fromID
		bidirectional := edgeSet[edge] && edgeSet[reverseKey]

		// Use canonical ordering for output
		if fromID > toID {
			fromID, toID = toID, fromID
		}

		connections = append(connections, GalaxyConnection{
			FromID:        fromID,
			ToID:          toID,
			Bidirectional: bidirectional,
		})
	}

	return connections
}

// =============================================================================
// RATE LIMITING
// =============================================================================

// ipRateLimiter implements a simple per-IP rate limiter
type ipRateLimiter struct {
	mu       sync.Mutex
	requests map[string]*rateLimitEntry
	limit    int
	window   time.Duration
}

type rateLimitEntry struct {
	count    int
	windowStart time.Time
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	rl := &ipRateLimiter{
		requests: make(map[string]*rateLimitEntry),
		limit:    limit,
		window:   window,
	}
	// Start cleanup goroutine
	go rl.cleanup()
	return rl
}

func (rl *ipRateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	entry, exists := rl.requests[ip]

	if !exists || now.Sub(entry.windowStart) > rl.window {
		// New window
		rl.requests[ip] = &rateLimitEntry{
			count:       1,
			windowStart: now,
		}
		return true
	}

	if entry.count >= rl.limit {
		return false
	}

	entry.count++
	return true
}

func (rl *ipRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, entry := range rl.requests {
			if now.Sub(entry.windowStart) > rl.window*2 {
				delete(rl.requests, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxied requests)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if idx := len(xff); idx > 0 {
			for i, c := range xff {
				if c == ',' {
					return xff[:i]
				}
			}
			return xff
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
