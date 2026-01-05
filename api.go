package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type API struct {
	transport  *StellarTransport
	storage    *Storage
	system     *System
	router     *mux.Router
}

// NewAPI creates a new API server
func NewAPI(system *System, transport *StellarTransport, storage *Storage) *API {
	api := &API{
		system:     system,
		transport:  transport,
		storage:    storage,
		router:     mux.NewRouter(),
	}

	api.setupRoutes()
	return api
}

func (api *API) setupRoutes() {
	// Web interface (root)
	api.router.HandleFunc("/", api.ServeWebInterface).Methods("GET")
	api.router.HandleFunc("/add-peer", api.HandleAddPeerForm).Methods("POST")
	api.router.HandleFunc("/map", api.ServeMapPage).Methods("GET")
	
	// JSON API endpoints (prefixed with /api)
	api.router.HandleFunc("/api/system", api.getSystemInfo).Methods("GET")
	api.router.HandleFunc("/api/peers", api.getPeers).Methods("GET")
	api.router.HandleFunc("/api/stats", api.getStats).Methods("GET")
	api.router.HandleFunc("/api/reputation", api.getReputation).Methods("GET")
	api.router.HandleFunc("/api/reputation/verify", api.verifyReputation).Methods("POST")
	api.router.HandleFunc("/api/version", api.getVersion).Methods("GET")
	api.router.HandleFunc("/api/map", api.getMapData).Methods("GET")
	api.router.HandleFunc("/api/topology", api.getTopology).Methods("GET")
	api.router.HandleFunc("/api/database/stats", api.getDatabaseStats).Methods("GET")

	// Legacy endpoints (for backward compatibility)
	api.router.HandleFunc("/system", api.getSystemInfo).Methods("GET")
	api.router.HandleFunc("/peers", api.getPeers).Methods("GET")
	api.router.HandleFunc("/stats", api.getStats).Methods("GET")
	api.router.HandleFunc("/reputation", api.getReputation).Methods("GET")
	api.router.HandleFunc("/reputation/verify", api.verifyReputation).Methods("POST")
	api.router.HandleFunc("/version", api.getVersion).Methods("GET")

	// Peer management
	api.router.HandleFunc("/peers/add", api.addPeer).Methods("POST")

	// Transport protocol endpoint (renamed from gossip)
	api.router.HandleFunc("/gossip", api.handleTransport).Methods("POST") // Keep old endpoint for compatibility

	// Health check
	api.router.HandleFunc("/health", api.healthCheck).Methods("GET")
}

// Start starts the API server
func (api *API) Start(address string) error {
	log.Printf("Starting API server on %s", address)
	return http.ListenAndServe(address, api.router)
}

// getSystemInfo returns information about this node's star system
func (api *API) getSystemInfo(w http.ResponseWriter, r *http.Request) {
	api.system.LastSeenAt = time.Now()
	respondJSON(w, http.StatusOK, api.system)
}

// getPeers returns the list of known peer systems
func (api *API) getPeers(w http.ResponseWriter, r *http.Request) {
	peers := api.transport.GetPeers()
	
	response := map[string]interface{}{
		"count": len(peers),
		"peers": peers,
	}
	
	respondJSON(w, http.StatusOK, response)
}

// getStats returns network statistics
func (api *API) getStats(w http.ResponseWriter, r *http.Request) {
	stats, err := api.storage.GetStats()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get stats")
		return
	}

	// Add runtime stats
	stats["system_id"] = api.system.ID
	stats["system_name"] = api.system.Name
	stats["uptime_seconds"] = int(time.Since(api.system.CreatedAt).Seconds())
	stats["coordinates"] = map[string]float64{
		"x": api.system.X,
		"y": api.system.Y,
		"z": api.system.Z,
	}
	
	// Add star system info
	stats["star_count"] = api.system.Stars.Count
	stats["is_binary"] = api.system.Stars.IsBinary
	stats["is_trinary"] = api.system.Stars.IsTrinary
	stats["primary_star_class"] = api.system.Stars.Primary.Class

	respondJSON(w, http.StatusOK, stats)
}

// getReputation returns network contribution based on verified cryptographic attestations
func (api *API) getReputation(w http.ResponseWriter, r *http.Request) {
	// Get all verified attestations for this system
	attestations, err := api.storage.GetAttestations(api.system.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get attestations")
		return
	}
	
	// Build cryptographic proof
	proof := BuildAttestationProof(api.system.ID, attestations)
	
	// Calculate verifiable reputation from proof
	reputationScore := CalculateVerifiableReputation(proof)
	rank := GetVerifiableRank(reputationScore)
	
	// Calculate uptime from oldest attestation
	var uptimeSeconds int64 = 0
	var uptimeHours, uptimeDays int
	if proof.OldestProof > 0 {
		uptimeSeconds = time.Now().Unix() - proof.OldestProof
		uptimeHours = int(uptimeSeconds / 3600)
		uptimeDays = uptimeHours / 24
	}
	
	// Build response
	contribution := &VerifiableNetworkContribution{
		SystemID:       api.system.ID,
		PublicKey:      base64.StdEncoding.EncodeToString(api.system.Keys.PublicKey),
		Proof:          proof,
		ReputationScore: reputationScore,
		Rank:           rank,
		LastCalculated: time.Now(),
	}
	
	summary := map[string]interface{}{
		"rank":                rank,
		"reputation_points":   int(reputationScore),
		"uptime_hours":        uptimeHours,
		"uptime_days":         uptimeDays,
		"verified_attestations": proof.TotalProofs,
		"unique_peers":        proof.UniquePeers,
		"is_critical_bridge":  false, // TODO: Implement bridge detection from attestation graph
	}
	
	response := map[string]interface{}{
		"contribution": contribution,
		"summary":      summary,
		"public_key":   contribution.PublicKey,
		"verified":     true, // This reputation is cryptographically verified
	}
	
	respondJSON(w, http.StatusOK, response)
}

// addPeer manually adds a peer to the network
func (api *API) addPeer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address string `json:"address"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Address == "" {
		respondError(w, http.StatusBadRequest, "Address is required")
		return
	}

	// Fetch system info from the peer
	resp, err := http.Get("http://" + req.Address + "/system")
	if err != nil {
		respondError(w, http.StatusBadRequest, "Failed to connect to peer")
		return
	}
	defer resp.Body.Close()

	var peerSystem System
	if err := json.NewDecoder(resp.Body).Decode(&peerSystem); err != nil {
		respondError(w, http.StatusBadRequest, "Failed to decode peer response")
		return
	}

	// Add to transport network
	if err := api.transport.AddPeer(peerSystem.ID, req.Address); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to add peer")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Peer added successfully",
		"peer": map[string]interface{}{
			"id":      peerSystem.ID,
			"name":    peerSystem.Name,
			"address": req.Address,
		},
	})
}

// handleTransport processes incoming transport protocol messages
func (api *API) handleTransport(w http.ResponseWriter, r *http.Request) {
	var msg TransportMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid transport message")
		return
	}

	if err := api.transport.HandleMessage(msg); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to process transport message")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// healthCheck returns the health status of the node
func (api *API) healthCheck(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "healthy",
		"timestamp": time.Now(),
	})
}

// verifyReputation allows any node to verify another node's reputation proof
// This enables trustless, decentralized verification
func (api *API) verifyReputation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AttestationProof *AttestationProof `json:"proof"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	if req.AttestationProof == nil {
		respondError(w, http.StatusBadRequest, "Proof is required")
		return
	}
	
	// Verify all attestations cryptographically
	validCount := 0
	invalidCount := 0
	
	for _, att := range req.AttestationProof.Attestations {
		if att.Verify() {
			validCount++
		} else {
			invalidCount++
		}
	}
	
	// Calculate reputation from verified attestations
	reputationScore := CalculateVerifiableReputation(req.AttestationProof)
	rank := GetVerifiableRank(reputationScore)
	
	// Is this proof trustworthy?
	trustworthy := invalidCount == 0 && validCount > 0
	
	response := map[string]interface{}{
		"verified":          true,
		"trustworthy":       trustworthy,
		"valid_attestations": validCount,
		"invalid_attestations": invalidCount,
		"total_attestations": len(req.AttestationProof.Attestations),
		"calculated_reputation": reputationScore,
		"calculated_rank":   rank,
		"unique_peers":      req.AttestationProof.UniquePeers,
		"oldest_proof_timestamp": req.AttestationProof.OldestProof,
	}
	
	respondJSON(w, http.StatusOK, response)
}

// getVersion returns version and compatibility information
func (api *API) getVersion(w http.ResponseWriter, r *http.Request) {
	versionInfo := GetVersionInfo()
	
	// Get peer version summary
	peers := api.transport.GetPeers()
	compatibleCount := 0
	incompatibleCount := 0
	unknownCount := 0
	
	api.transport.mu.RLock()
	for _, peer := range peers {
		if peerVersion, exists := api.transport.peerVersions[peer.SystemID]; exists {
			if CurrentProtocolVersion.IsCompatibleWith(*peerVersion) {
				compatibleCount++
			} else {
				incompatibleCount++
			}
		} else {
			unknownCount++
		}
	}
	api.transport.mu.RUnlock()
	
	response := map[string]interface{}{
		"protocol_version":   versionInfo.Protocol,
		"application_version": versionInfo.Application,
		"supported_features": versionInfo.Features,
		"peer_compatibility": map[string]interface{}{
			"compatible_peers":   compatibleCount,
			"incompatible_peers": incompatibleCount,
			"unknown_peers":      unknownCount,
			"total_peers":        len(peers),
		},
	}
	
	respondJSON(w, http.StatusOK, response)
}

// Helper functions for JSON responses
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

// getMapData returns all known systems and connections for map visualization
func (api *API) getMapData(w http.ResponseWriter, r *http.Request) {
	type MapSystem struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		X         float64 `json:"x"`
		Y         float64 `json:"y"`
		Z         float64 `json:"z"`
		StarColor string  `json:"star_color"`
		StarClass string  `json:"star_class"`
		IsLocal   bool    `json:"is_local"`
	}

	type MapConnection struct {
		From string `json:"from"`
		To   string `json:"to"`
	}

	type MapData struct {
		Systems     []MapSystem     `json:"systems"`
		Connections []MapConnection `json:"connections"`
		Center      MapSystem       `json:"center"`
	}

	systems := []MapSystem{}
	connections := []MapConnection{}

	// Add local system
	localSys := MapSystem{
		ID:        api.system.ID.String(),
		Name:      api.system.Name,
		X:         api.system.X,
		Y:         api.system.Y,
		Z:         api.system.Z,
		StarColor: api.system.Stars.Primary.Color,
		StarClass: api.system.Stars.Primary.Class,
		IsLocal:   true,
	}
	systems = append(systems, localSys)

	// Track which systems we've added (to avoid duplicates)
	addedSystems := make(map[string]bool)
	addedSystems[api.system.ID.String()] = true

	// Add all known peer systems from cache
	allPeerSystems, err := api.storage.GetAllPeerSystems()
	if err == nil {
		for _, peerSys := range allPeerSystems {
			if addedSystems[peerSys.ID.String()] {
				continue
			}

			starColor := "#FFFFFF"
			starClass := "?"
			if peerSys.Stars.Primary.Color != "" {
				starColor = peerSys.Stars.Primary.Color
				starClass = peerSys.Stars.Primary.Class
			}

			systems = append(systems, MapSystem{
				ID:        peerSys.ID.String(),
				Name:      peerSys.Name,
				X:         peerSys.X,
				Y:         peerSys.Y,
				Z:         peerSys.Z,
				StarColor: starColor,
				StarClass: starClass,
				IsLocal:   false,
			})
			addedSystems[peerSys.ID.String()] = true
		}
	}

	// Add connections for direct peers only (we only know our own connections)
	peers := api.transport.GetPeers()
	for _, peer := range peers {
		connections = append(connections, MapConnection{
			From: api.system.ID.String(),
			To:   peer.SystemID.String(),
		})
	}

	mapData := MapData{
		Systems:     systems,
		Connections: connections,
		Center:      localSys,
	}

	respondJSON(w, http.StatusOK, mapData)
}

// getDatabaseStats returns database statistics
func (api *API) getDatabaseStats(w http.ResponseWriter, r *http.Request) {
    stats, err := api.storage.GetDatabaseStats()
    if err != nil {
        respondError(w, http.StatusInternalServerError, "Failed to get stats")
        return
    }
    respondJSON(w, http.StatusOK, stats)
}

// getTopology returns the inferred network topology based on recent attestations
func (api *API) getTopology(w http.ResponseWriter, r *http.Request) {
	type TopologyEdge struct {
		FromID   string `json:"from_id"`
		FromName string `json:"from_name"`
		ToID     string `json:"to_id"`
		ToName   string `json:"to_name"`
	}

	type TopologyResponse struct {
		LocalID   string         `json:"local_id"`
		LocalName string         `json:"local_name"`
		Edges     []TopologyEdge `json:"edges"`
	}

	edges, err := api.storage.GetRecentTopology(5 * time.Minute)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get topology")
		return
	}

	response := TopologyResponse{
		LocalID:   api.system.ID.String(),
		LocalName: api.system.Name,
		Edges:     edges,
	}

	respondJSON(w, http.StatusOK, response)
}