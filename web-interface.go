package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "html/template"
    "log"
    "net"
    "net/http"
    "time"
)

// WebInterface handles the web UI and API endpoints
type WebInterface struct {
    dht      *DHT
    storage  *Storage
    addr     string
}

// WebInterfaceData holds data for the web template
type WebInterfaceData struct {
    System            *System
    Peers             []*System
    PeerIDs           []string
    PeerCount         int
    MaxPeers          int
    PeerCapacityDesc  string
    KnownSystems      []*System
    TotalSystems      int
    ProtocolVersion   string
    AttestationCount  int
    DatabaseSize      string
    NodeHealth        string
    NodeHealthClass   string
    RoutingTableSize  int
    CacheSize         int
    // Stellar Credits
    CreditBalance     int64
    CreditRank        string
    CreditRankColor   string
    NextRank          string
    CreditsToNextRank int64
    LongevityWeeks       float64
    LongevityBonus       float64
    LongevityBonusPct    float64
    LongevityProgressPct float64
}

// NewWebInterface creates a new web interface
func NewWebInterface(dht *DHT, storage *Storage, addr string) *WebInterface {
    return &WebInterface{
        dht:     dht,
        storage: storage,
        addr:    addr,
    }
}

// Start begins the web server
// Returns an error if the server fails to bind
func (w *WebInterface) Start() error {
    // Try to bind BEFORE starting goroutine
    listener, err := net.Listen("tcp", w.addr)
    if err != nil {
        return fmt.Errorf("Web server failed to bind to %s: %w", w.addr, err)
    }

    mux := http.NewServeMux()

    // Web UI
    mux.HandleFunc("/", w.handleIndex)

    // API endpoints
    mux.HandleFunc("/api/system", w.handleSystemAPI)
    mux.HandleFunc("/api/peers", w.handlePeersAPI)
    mux.HandleFunc("/api/known-systems", w.handleKnownSystemsAPI)
    mux.HandleFunc("/api/stats", w.handleStatsAPI)
    mux.HandleFunc("/api/credits", w.handleCreditsAPI)
    mux.HandleFunc("/api/version", w.handleVersionAPI)
    mux.HandleFunc("/api/connections", w.handleConnectionsAPI)

    log.Printf("Web interface listening on %s", w.addr)
    go func() {
        if err := http.Serve(listener, mux); err != nil {
            log.Printf("Web server error: %v", err)
        }
    }()

    return nil
}

// handleIndex serves the main web page
func (w *WebInterface) handleIndex(rw http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/" {
        http.NotFound(rw, r)
        return
    }

    data := w.buildTemplateData()

    tmpl := template.Must(template.New("index").Parse(indexTemplate))
    
    var buf bytes.Buffer
    if err := tmpl.Execute(&buf, data); err != nil {
        http.Error(rw, err.Error(), http.StatusInternalServerError)
        return
    }
    
    rw.Header().Set("Content-Type", "text/html; charset=utf-8")
    buf.WriteTo(rw)
}

// buildTemplateData gathers all data for the web template
func (w *WebInterface) buildTemplateData() WebInterfaceData {
    sys := w.dht.GetLocalSystem()
    rt := w.dht.GetRoutingTable()

    // Get routing table nodes (active peers)
    peers := rt.GetAllRoutingTableNodes()

    // Get all cached systems (known galaxy)
    allSystems := rt.GetAllCachedSystems()

    // Get attestation count (use GetDatabaseStats)
    dbStats, _ := w.storage.GetDatabaseStats()
    attestationCount := 0
    if count, ok := dbStats["attestation_count"].(int); ok {
        attestationCount = count
    }

    // Get database size
    dbSizeStr := "unknown"
    if sizeBytes, ok := dbStats["database_size_bytes"].(int64); ok {
        dbSizeStr = formatBytes(sizeBytes)
    }

    // Determine node health
    rtSize := rt.GetRoutingTableSize()
    var health, healthClass string
    if rtSize >= 2 {
        health = "Healthy"
        healthClass = "health-healthy"
    } else if rtSize == 1 {
        health = "Low Connectivity"
        healthClass = "health-warning"
    } else {
        health = "Isolated"
        healthClass = "health-critical"
    }

    // Peer capacity description
    capacityDesc := fmt.Sprintf("%s-class", sys.Stars.Primary.Class)
    if sys.Stars.IsBinary {
        capacityDesc = fmt.Sprintf("%s/%s binary", sys.Stars.Primary.Class, sys.Stars.Secondary.Class)
    } else if sys.Stars.IsTrinary {
        capacityDesc = "trinary system"
    }

    // Get credit balance and longevity
    var creditBalance int64
    var creditRank, creditRankColor, nextRank string
    var creditsToNext int64
    var longevityWeeks, longevityBonus float64
    
    balance, err := w.storage.GetCreditBalance(sys.ID)
    if err == nil {
        creditBalance = balance.Balance
        rank := GetRank(balance.Balance)
        creditRank = rank.Name
        creditRankColor = rank.Color
        next, needed := GetNextRank(balance.Balance)
        if needed > 0 {
            nextRank = next.Name
            creditsToNext = needed
        }
        // Calculate longevity
        if balance.LongevityStart > 0 {
            longevitySeconds := time.Now().Unix() - balance.LongevityStart
            longevityWeeks = float64(longevitySeconds) / (7 * 24 * 3600)
            longevityBonus = min(longevityWeeks * 0.01, 0.52)
        }
    }

    // Calculate percentages for display
    longevityBonusPct := longevityBonus * 100
    longevityProgressPct := min((longevityWeeks / 52) * 100, 100)

    // Build peer ID list for JS
    peerIDs := make([]string, len(peers))
    for i, p := range peers {
        peerIDs[i] = p.ID.String()
    }

    return WebInterfaceData{
        System:           sys,
        Peers:            peers,
        PeerIDs:          peerIDs,
        PeerCount:        rtSize,
        MaxPeers:         sys.GetMaxPeers(),
        PeerCapacityDesc: capacityDesc,
        KnownSystems:     allSystems,
        TotalSystems:     len(allSystems) + 1, // +1 for self
        ProtocolVersion:  CurrentProtocolVersion.String(),
        AttestationCount: attestationCount,
        DatabaseSize:     dbSizeStr,
        NodeHealth:       health,
        NodeHealthClass:  healthClass,
        RoutingTableSize: rtSize,
        CacheSize:        rt.GetCacheSize(),
        // Credits
        CreditBalance:     creditBalance,
        CreditRank:        creditRank,
        CreditRankColor:      creditRankColor,
        NextRank:             nextRank,
        CreditsToNextRank:    creditsToNext,
        LongevityWeeks:       longevityWeeks,
        LongevityBonus:       longevityBonus,
        LongevityBonusPct:    longevityBonusPct,
        LongevityProgressPct: longevityProgressPct,
    }
}

// API handlers

func (w *WebInterface) handleSystemAPI(rw http.ResponseWriter, r *http.Request) {
    rw.Header().Set("Content-Type", "application/json")
    json.NewEncoder(rw).Encode(w.dht.GetLocalSystem())
}

func (w *WebInterface) handlePeersAPI(rw http.ResponseWriter, r *http.Request) {
    peers := w.dht.GetRoutingTable().GetAllRoutingTableNodes()
    rw.Header().Set("Content-Type", "application/json")
    json.NewEncoder(rw).Encode(peers)
}

func (w *WebInterface) handleKnownSystemsAPI(rw http.ResponseWriter, r *http.Request) {
    systems := w.dht.GetRoutingTable().GetAllCachedSystems()
    rw.Header().Set("Content-Type", "application/json")
    json.NewEncoder(rw).Encode(systems)
}

func (w *WebInterface) handleStatsAPI(rw http.ResponseWriter, r *http.Request) {
    stats := w.dht.GetNetworkStats()
    rw.Header().Set("Content-Type", "application/json")
    json.NewEncoder(rw).Encode(stats)
}

func (w *WebInterface) handleCreditsAPI(rw http.ResponseWriter, r *http.Request) {
    sys := w.dht.GetLocalSystem()
    balance, err := w.storage.GetCreditBalance(sys.ID)
    if err != nil {
        http.Error(rw, "Failed to get credit balance", http.StatusInternalServerError)
        return
    }

    rank := GetRank(balance.Balance)
    nextRank, creditsNeeded := GetNextRank(balance.Balance)

    // Calculate current longevity streak in weeks
    var longevityWeeks float64
    if balance.LongevityStart > 0 {
        longevitySeconds := time.Now().Unix() - balance.LongevityStart
        longevityWeeks = float64(longevitySeconds) / (7 * 24 * 3600)
    }

    response := map[string]interface{}{
        "system_id":         sys.ID.String(),
        "balance":           balance.Balance,
        "total_earned":      balance.TotalEarned,
        "total_sent":        balance.TotalSent,
        "total_received":    balance.TotalReceived,
        "rank":              rank.Name,
        "rank_color":        rank.Color,
        "next_rank":         nextRank.Name,
        "credits_to_next":   creditsNeeded,
        "longevity_weeks":   longevityWeeks,
        "longevity_bonus":   min(longevityWeeks * 0.01, 0.52),
        "last_updated":      balance.LastUpdated,
    }

    rw.Header().Set("Content-Type", "application/json")
    json.NewEncoder(rw).Encode(response)
}

func (w *WebInterface) handleVersionAPI(rw http.ResponseWriter, r *http.Request) {
    response := map[string]interface{}{
        "version":  BuildVersion,
        "protocol": CurrentProtocolVersion.String(),
        "software": "stellar-lab",
    }
    rw.Header().Set("Content-Type", "application/json")
    json.NewEncoder(rw).Encode(response)
}

func (w *WebInterface) handleConnectionsAPI(rw http.ResponseWriter, r *http.Request) {
    // Get connections from peer_connections table (1 hour max age)
    connections, err := w.storage.GetAllConnections(time.Hour)
    if err != nil {
        connections = []TopologyEdge{} // Continue with empty if error
    }

    // Also add our direct connections (routing table peers)
    // These are definitely connected to us
    selfID := w.dht.GetLocalSystem().ID.String()
    selfName := w.dht.GetLocalSystem().Name
    peers := w.dht.GetRoutingTable().GetAllRoutingTableNodes()
    
    // Build a set of existing connections to avoid duplicates
    existingEdges := make(map[string]bool)
    for _, c := range connections {
        key1 := c.FromID + ":" + c.ToID
        key2 := c.ToID + ":" + c.FromID
        existingEdges[key1] = true
        existingEdges[key2] = true
    }
    
    // Add our direct peer connections
    for _, peer := range peers {
        peerID := peer.ID.String()
        key := selfID + ":" + peerID
        if !existingEdges[key] {
            connections = append(connections, TopologyEdge{
                FromID:   selfID,
                FromName: selfName,
                ToID:     peerID,
                ToName:   peer.Name,
            })
            existingEdges[key] = true
        }
    }

    rw.Header().Set("Content-Type", "application/json")
    json.NewEncoder(rw).Encode(connections)
}

// formatBytes formats a byte count as a human-readable string
func formatBytes(bytes int64) string {
    const unit = 1024
    if bytes < unit {
        return fmt.Sprintf("%d B", bytes)
    }
    div, exp := int64(unit), 0
    for n := bytes / unit; n >= unit; n /= unit {
        div *= unit
        exp++
    }
    return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// HTML template
const indexTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.System.Name}} - Stellar Lab</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #0a0a1a 0%, #1a1a3a 100%);
            color: #e0e0e0;
            min-height: 100vh;
            padding: 20px;
        }
        .container { max-width: 1600px; margin: 0 auto; }
        h1 {
            font-size: 2.5em;
            margin-bottom: 10px;
            background: linear-gradient(90deg, #60a5fa, #a78bfa);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        .subtitle { color: #888; margin-bottom: 30px; }
        .grid {
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 20px;
            margin-bottom: 20px;
        }
        .grid-full {
            grid-column: 1 / -1;
        }
        .card {
            background: rgba(255,255,255,0.05);
            border-radius: 12px;
            padding: 20px;
            border: 1px solid rgba(255,255,255,0.1);
        }
        .card h2 {
            font-size: 1.2em;
            margin-bottom: 15px;
            color: #a78bfa;
        }
        .stat-row {
            display: flex;
            justify-content: space-between;
            padding: 8px 0;
            border-bottom: 1px solid rgba(255,255,255,0.05);
        }
        .stat-row:last-child { border-bottom: none; }
        .stat-label { color: #888; }
        .stat-value { font-weight: 500; }
        .health-healthy { color: #4ade80; }
        .health-warning { color: #facc15; }
        .health-critical { color: #f87171; }
        .peer-list { max-height: 300px; overflow-y: auto; }
        .peer-item {
            padding: 10px;
            margin: 5px 0;
            background: rgba(255,255,255,0.03);
            border-radius: 8px;
        }
        .peer-name { font-weight: 500; color: #60a5fa; }
        .peer-id { font-size: 0.8em; color: #666; font-family: monospace; }
        .star-display { display: flex; align-items: center; gap: 10px; margin: 10px 0; }
        .star {
            width: 30px;
            height: 30px;
            border-radius: 50%;
            box-shadow: 0 0 20px currentColor;
        }
        .coords { font-family: monospace; color: #888; font-size: 0.9em; }
        #galaxy-map {
            width: 100%;
            height: 600px;
            background: radial-gradient(ellipse at center, #0a0a1a 0%, #000005 100%);
            border-radius: 12px;
            position: relative;
            overflow: hidden;
        }
        #galaxy-map canvas {
            border-radius: 12px;
        }
        .map-controls {
            position: absolute;
            top: 10px;
            right: 10px;
            z-index: 100;
            display: flex;
            gap: 8px;
        }
        .map-btn {
            background: rgba(96, 165, 250, 0.2);
            border: 1px solid rgba(96, 165, 250, 0.4);
            color: #60a5fa;
            padding: 8px 12px;
            border-radius: 6px;
            cursor: pointer;
            font-size: 12px;
            transition: all 0.2s;
        }
        .map-btn:hover {
            background: rgba(96, 165, 250, 0.3);
            border-color: rgba(96, 165, 250, 0.6);
        }
        .map-tooltip {
            position: absolute;
            background: rgba(0, 0, 0, 0.85);
            border: 1px solid rgba(96, 165, 250, 0.4);
            border-radius: 6px;
            padding: 8px 12px;
            color: #e0e0e0;
            font-size: 12px;
            pointer-events: none;
            z-index: 200;
            display: none;
        }
        .map-tooltip .tooltip-name {
            color: #60a5fa;
            font-weight: 500;
            margin-bottom: 4px;
        }
        .map-tooltip .tooltip-coords {
            color: #888;
            font-family: monospace;
            font-size: 11px;
        }
        .map-hint {
            position: absolute;
            bottom: 8px;
            right: 12px;
            font-size: 11px;
            color: #666;
            z-index: 100;
        }
        .version-badge {
            display: inline-block;
            padding: 4px 8px;
            background: rgba(167, 139, 250, 0.2);
            border-radius: 4px;
            font-size: 0.9em;
        }
        .longevity-bar {
            margin-top: 12px;
            padding: 12px;
            background: rgba(167, 139, 250, 0.1);
            border-radius: 8px;
        }
        .longevity-header {
            display: flex;
            justify-content: space-between;
            margin-bottom: 8px;
            font-size: 0.85em;
        }
        .longevity-label { color: #888; }
        .longevity-value { color: #a78bfa; font-weight: 500; }
        .longevity-track {
            height: 8px;
            background: rgba(255,255,255,0.1);
            border-radius: 4px;
            overflow: hidden;
        }
        .longevity-fill {
            height: 100%;
            background: linear-gradient(90deg, #60a5fa, #a78bfa);
            border-radius: 4px;
            transition: width 0.3s ease;
        }
        .longevity-note {
            margin-top: 6px;
            font-size: 0.75em;
            color: #666;
        }
        .map-tooltip .tooltip-class {
            color: #a78bfa;
            font-size: 11px;
            margin-bottom: 2px;
        }
        .map-tooltip .tooltip-distance {
            color: #4ade80;
            font-size: 11px;
            margin-top: 4px;
        }
        @media (max-width: 1400px) {
            .grid { grid-template-columns: repeat(2, 1fr); }
        }
        @media (max-width: 800px) {
            .grid { grid-template-columns: 1fr; }
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>{{.System.Name}}</h1>
        <p class="subtitle">
            Stellar Lab Node
            <span class="version-badge">v{{.ProtocolVersion}}</span>
        </p>

        <div class="grid">
            <div class="card">
                <h2>System Information</h2>
                <div class="stat-row">
                    <span class="stat-label">Status</span>
                    <span class="stat-value {{.NodeHealthClass}}">{{.NodeHealth}}</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">System ID</span>
                    <span class="stat-value" style="font-family: monospace; font-size: 0.8em; word-break: break-all;">{{.System.ID}}</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">Coordinates</span>
                    <span class="stat-value coords">({{printf "%.1f" .System.X}}, {{printf "%.1f" .System.Y}}, {{printf "%.1f" .System.Z}})</span>
                </div>
                <div class="star-display">
                    <div class="star" style="background: {{.System.Stars.Primary.Color}}; color: {{.System.Stars.Primary.Color}};"></div>
                    {{if .System.Stars.Secondary}}
                    <div class="star" style="background: {{.System.Stars.Secondary.Color}}; color: {{.System.Stars.Secondary.Color}}; width: 24px; height: 24px;"></div>
                    {{end}}
                    {{if .System.Stars.Tertiary}}
                    <div class="star" style="background: {{.System.Stars.Tertiary.Color}}; color: {{.System.Stars.Tertiary.Color}}; width: 18px; height: 18px;"></div>
                    {{end}}
                    <span>{{.System.Stars.Primary.Description}}</span>
                </div>
            </div>

            <div class="card">
                <h2>Stellar Transport</h2>
                <div class="stat-row">
                    <span class="stat-label">Routing Table</span>
                    <span class="stat-value">{{.RoutingTableSize}} nodes</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">System Cache</span>
                    <span class="stat-value">{{.CacheSize}} systems</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">Known Galaxy</span>
                    <span class="stat-value">{{.TotalSystems}} systems</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">Max Peers</span>
                    <span class="stat-value">{{.MaxPeers}} ({{.PeerCapacityDesc}})</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">Attestations</span>
                    <span class="stat-value">{{.AttestationCount}}</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">Database</span>
                    <span class="stat-value">{{.DatabaseSize}}</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">Uptime Streak</span>
                    <span class="stat-value">{{printf "%.1f" .LongevityWeeks}} weeks</span>
                </div>
            </div>

            <div class="card">
                <h2>Stellar Credits</h2>
                <div class="stat-row">
                    <span class="stat-label">Balance</span>
                    <span class="stat-value">{{.CreditBalance}} ✦</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">Rank</span>
                    <span class="stat-value" style="color: {{.CreditRankColor}};">{{.CreditRank}}</span>
                </div>
                {{if .NextRank}}
                <div class="stat-row">
                    <span class="stat-label">Next Rank</span>
                    <span class="stat-value">{{.NextRank}} ({{.CreditsToNextRank}} ✦ needed)</span>
                </div>
                {{end}}
                <div class="longevity-bar">
                    <div class="longevity-header">
                        <span class="longevity-label">Longevity Bonus</span>
                        <span class="longevity-value">+{{printf "%.1f" .LongevityBonusPct}}%</span>
                    </div>
                    <div class="longevity-track">
                        <div class="longevity-fill" style="width: {{printf "%.1f" .LongevityProgressPct}}%;"></div>
                    </div>
                    <div class="longevity-note">{{printf "%.1f" .LongevityWeeks}} / 52 weeks to max (+52%)</div>
                </div>
            </div>

            <div class="card">
                <h2>Routing Table ({{.RoutingTableSize}} nodes)</h2>
                <div class="peer-list">
                    {{range .Peers}}
                    <div class="peer-item">
                        <div class="peer-name">{{.Name}}</div>
                        <div class="peer-id">{{.ID}}</div>
                        <div class="coords">({{printf "%.1f" .X}}, {{printf "%.1f" .Y}}, {{printf "%.1f" .Z}})</div>
                    </div>
                    {{else}}
                    <p style="color: #666; padding: 20px; text-align: center;">No peers in routing table</p>
                    {{end}}
                </div>
            </div>

            <div class="card grid-full">
                <h2>Galaxy Map ({{.TotalSystems}} systems)</h2>
                <div id="galaxy-map"></div>
            </div>
        </div>
    </div>

    <script src="https://cdnjs.cloudflare.com/ajax/libs/three.js/r128/three.min.js"></script>
    <script>
        // OrbitControls inline (r128 compatible)
        THREE.OrbitControls = function(object, domElement) {
            this.object = object;
            this.domElement = domElement;
            this.enabled = true;
            this.target = new THREE.Vector3();
            this.minDistance = 0;
            this.maxDistance = Infinity;
            this.enableDamping = false;
            this.dampingFactor = 0.05;
            this.enableZoom = true;
            this.zoomSpeed = 1.0;
            this.enableRotate = true;
            this.rotateSpeed = 1.0;
            this.enablePan = true;
            this.panSpeed = 1.0;
            
            const scope = this;
            const STATE = { NONE: -1, ROTATE: 0, DOLLY: 1, PAN: 2 };
            let state = STATE.NONE;
            
            const rotateStart = new THREE.Vector2();
            const rotateEnd = new THREE.Vector2();
            const rotateDelta = new THREE.Vector2();
            const panStart = new THREE.Vector2();
            const panEnd = new THREE.Vector2();
            const panDelta = new THREE.Vector2();
            const dollyStart = new THREE.Vector2();
            const dollyEnd = new THREE.Vector2();
            const dollyDelta = new THREE.Vector2();
            
            let spherical = new THREE.Spherical();
            let sphericalDelta = new THREE.Spherical();
            let scale = 1;
            let panOffset = new THREE.Vector3();
            
            this.update = function() {
                const offset = new THREE.Vector3();
                const quat = new THREE.Quaternion().setFromUnitVectors(object.up, new THREE.Vector3(0, 1, 0));
                const quatInverse = quat.clone().invert();
                const lastPosition = new THREE.Vector3();
                
                const position = scope.object.position;
                offset.copy(position).sub(scope.target);
                offset.applyQuaternion(quat);
                spherical.setFromVector3(offset);
                spherical.theta += sphericalDelta.theta;
                spherical.phi += sphericalDelta.phi;
                spherical.phi = Math.max(0.01, Math.min(Math.PI - 0.01, spherical.phi));
                spherical.radius *= scale;
                spherical.radius = Math.max(scope.minDistance, Math.min(scope.maxDistance, spherical.radius));
                scope.target.add(panOffset);
                offset.setFromSpherical(spherical);
                offset.applyQuaternion(quatInverse);
                position.copy(scope.target).add(offset);
                scope.object.lookAt(scope.target);
                sphericalDelta.set(0, 0, 0);
                scale = 1;
                panOffset.set(0, 0, 0);
                return false;
            };
            
            function onMouseDown(event) {
                if (!scope.enabled) return;
                event.preventDefault();
                if (event.button === 0) {
                    state = STATE.ROTATE;
                    rotateStart.set(event.clientX, event.clientY);
                } else if (event.button === 1) {
                    state = STATE.DOLLY;
                    dollyStart.set(event.clientX, event.clientY);
                } else if (event.button === 2) {
                    state = STATE.PAN;
                    panStart.set(event.clientX, event.clientY);
                }
                document.addEventListener('mousemove', onMouseMove, false);
                document.addEventListener('mouseup', onMouseUp, false);
            }
            
            function onMouseMove(event) {
                if (!scope.enabled) return;
                event.preventDefault();
                if (state === STATE.ROTATE) {
                    rotateEnd.set(event.clientX, event.clientY);
                    rotateDelta.subVectors(rotateEnd, rotateStart).multiplyScalar(scope.rotateSpeed);
                    sphericalDelta.theta -= 2 * Math.PI * rotateDelta.x / domElement.clientHeight;
                    sphericalDelta.phi -= 2 * Math.PI * rotateDelta.y / domElement.clientHeight;
                    rotateStart.copy(rotateEnd);
                } else if (state === STATE.DOLLY) {
                    dollyEnd.set(event.clientX, event.clientY);
                    dollyDelta.subVectors(dollyEnd, dollyStart);
                    if (dollyDelta.y > 0) scale /= Math.pow(0.95, scope.zoomSpeed);
                    else if (dollyDelta.y < 0) scale *= Math.pow(0.95, scope.zoomSpeed);
                    dollyStart.copy(dollyEnd);
                } else if (state === STATE.PAN) {
                    panEnd.set(event.clientX, event.clientY);
                    panDelta.subVectors(panEnd, panStart).multiplyScalar(scope.panSpeed);
                    const offset = new THREE.Vector3();
                    offset.setFromMatrixColumn(scope.object.matrix, 0);
                    offset.multiplyScalar(-panDelta.x * 0.5);
                    panOffset.add(offset);
                    offset.setFromMatrixColumn(scope.object.matrix, 1);
                    offset.multiplyScalar(panDelta.y * 0.5);
                    panOffset.add(offset);
                    panStart.copy(panEnd);
                }
                scope.update();
            }
            
            function onMouseUp() {
                state = STATE.NONE;
                document.removeEventListener('mousemove', onMouseMove, false);
                document.removeEventListener('mouseup', onMouseUp, false);
            }
            
            function onWheel(event) {
                if (!scope.enabled || !scope.enableZoom) return;
                event.preventDefault();
                if (event.deltaY < 0) scale *= Math.pow(0.95, scope.zoomSpeed);
                else if (event.deltaY > 0) scale /= Math.pow(0.95, scope.zoomSpeed);
                scope.update();
            }
            
            domElement.addEventListener('mousedown', onMouseDown, false);
            domElement.addEventListener('wheel', onWheel, { passive: false });
            domElement.addEventListener('contextmenu', e => e.preventDefault(), false);
            this.update();
        };

        const knownSystems = [
            {{range .KnownSystems}}
            {id: "{{.ID}}", name: "{{.Name}}", x: {{.X}}, y: {{.Y}}, z: {{.Z}}, color: "{{.Stars.Primary.Color}}", starClass: "{{.Stars.Primary.Class}}", starDesc: "{{.Stars.Primary.Description}}"},
            {{end}}
        ];
        const selfSystem = {
            id: "{{.System.ID}}",
            name: "{{.System.Name}}",
            x: {{.System.X}},
            y: {{.System.Y}},
            z: {{.System.Z}},
            color: "{{.System.Stars.Primary.Color}}",
            starClass: "{{.System.Stars.Primary.Class}}",
            starDesc: "{{.System.Stars.Primary.Description}}"
        };
        const livePeerIDs = new Set([
            {{range .PeerIDs}}"{{.}}",{{end}}
        ]);

        let scene, camera, renderer, controls;
        let starMeshes = [];
        let connectionLines = [];
        let cachedConnections = [];

        async function fetchConnections() {
            try {
                const resp = await fetch('/api/connections');
                cachedConnections = await resp.json() || [];
            } catch (e) {
                console.error('Failed to fetch connections:', e);
                cachedConnections = [];
            }
        }

        function hexToRgb(hex) {
            const result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex);
            return result ? {
                r: parseInt(result[1], 16) / 255,
                g: parseInt(result[2], 16) / 255,
                b: parseInt(result[3], 16) / 255
            } : { r: 1, g: 1, b: 1 };
        }

        function createStarSprite(color, size, isSelf, isCached) {
            const canvas = document.createElement('canvas');
            canvas.width = 64;
            canvas.height = 64;
            const ctx = canvas.getContext('2d');
            
            const gradient = ctx.createRadialGradient(32, 32, 0, 32, 32, 32);
            let rgb = hexToRgb(color);
            
            // For cached (non-live) systems, desaturate and add reddish tint
            if (isCached && !isSelf) {
                const avg = (rgb.r + rgb.g + rgb.b) / 3;
                rgb.r = avg * 0.6 + 0.4; // Push toward red
                rgb.g = avg * 0.4;
                rgb.b = avg * 0.4;
            }
            
            const colorStr = 'rgb(' + Math.floor(rgb.r*255) + ',' + Math.floor(rgb.g*255) + ',' + Math.floor(rgb.b*255) + ')';
            
            gradient.addColorStop(0, isCached && !isSelf ? 'rgba(255,200,200,0.8)' : 'rgba(255,255,255,1)');
            gradient.addColorStop(0.1, colorStr);
            gradient.addColorStop(0.4, colorStr.replace('rgb', 'rgba').replace(')', ',' + (isCached ? '0.3' : '0.6') + ')'));
            gradient.addColorStop(1, 'rgba(0,0,0,0)');
            
            ctx.fillStyle = gradient;
            ctx.fillRect(0, 0, 64, 64);
            
            const texture = new THREE.CanvasTexture(canvas);
            const material = new THREE.SpriteMaterial({ 
                map: texture, 
                transparent: true,
                blending: THREE.AdditiveBlending,
                opacity: isCached && !isSelf ? 0.6 : 1.0
            });
            const sprite = new THREE.Sprite(material);
            sprite.scale.set(size, size, 1);
            return sprite;
        }

        function calculateDistance(sys1, sys2) {
            const dx = sys1.x - sys2.x;
            const dy = sys1.y - sys2.y;
            const dz = sys1.z - sys2.z;
            return Math.sqrt(dx*dx + dy*dy + dz*dz);
        }

        function centerOnSelf() {
            if (!controls || !camera) return;
            
            const targetPos = new THREE.Vector3(selfSystem.x, selfSystem.y, selfSystem.z);
            controls.target.copy(targetPos);
            
            // Position camera at a nice viewing angle
            const distance = 800;
            camera.position.set(
                targetPos.x + distance * 0.7,
                targetPos.y + distance * 0.5,
                targetPos.z + distance * 0.7
            );
            controls.update();
        }

        async function initGalaxyMap() {
            const container = document.getElementById('galaxy-map');
            if (!container) return;
            
            const width = container.clientWidth;
            const height = container.clientHeight;
            if (width === 0 || height === 0) {
                setTimeout(initGalaxyMap, 100);
                return;
            }

            // Fetch connections
            await fetchConnections();

            // Scene
            scene = new THREE.Scene();
            
            // Camera
            camera = new THREE.PerspectiveCamera(60, width / height, 1, 50000);
            
            // Renderer
            renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
            renderer.setSize(width, height);
            renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
            container.appendChild(renderer.domElement);
            
            // Controls
            controls = new THREE.OrbitControls(camera, renderer.domElement);
            controls.enableDamping = true;
            controls.dampingFactor = 0.05;
            controls.minDistance = 100;
            controls.maxDistance = 15000;
            controls.panSpeed = 0.8;
            controls.rotateSpeed = 0.6;
            
            // Add controls UI
            const controlsDiv = document.createElement('div');
            controlsDiv.className = 'map-controls';
            controlsDiv.innerHTML = '<button class="map-btn" onclick="centerOnSelf()">⌂ Center on Home</button>';
            container.appendChild(controlsDiv);
            
            // Add legend
            const legend = document.createElement('div');
            legend.style.cssText = 'position:absolute;top:10px;left:10px;background:rgba(0,0,0,0.7);border:1px solid rgba(255,255,255,0.1);border-radius:6px;padding:8px 12px;font-size:11px;z-index:100;';
            legend.innerHTML = 
                '<div style="margin-bottom:6px;color:#888;font-weight:500;">Legend</div>' +
                '<div style="display:flex;align-items:center;gap:6px;margin-bottom:4px;"><span style="color:#60a5fa;">●</span> You</div>' +
                '<div style="display:flex;align-items:center;gap:6px;margin-bottom:4px;"><span style="color:#4ade80;">●</span> Live peers</div>' +
                '<div style="display:flex;align-items:center;gap:6px;margin-bottom:4px;"><span style="color:#996666;">●</span> Cached</div>' +
                '<div style="display:flex;align-items:center;gap:6px;margin-bottom:4px;"><span style="color:#64c8ff;">―</span> Reciprocal</div>' +
                '<div style="display:flex;align-items:center;gap:6px;"><span style="color:#ffaa44;">┄</span> One-way</div>';
            container.appendChild(legend);
            
            // Add tooltip
            const tooltip = document.createElement('div');
            tooltip.className = 'map-tooltip';
            tooltip.id = 'map-tooltip';
            container.appendChild(tooltip);
            
            // Add hint
            const hint = document.createElement('div');
            hint.className = 'map-hint';
            hint.textContent = 'Left-drag: rotate • Right-drag: pan • Scroll: zoom';
            container.appendChild(hint);

            const allSystems = [selfSystem, ...knownSystems];
            const systemById = {};
            allSystems.forEach(s => { systemById[s.id] = s; });

            // Labels container
            const labelsContainer = document.createElement('div');
            labelsContainer.style.cssText = 'position:absolute;top:0;left:0;width:100%;height:100%;pointer-events:none;overflow:hidden;';
            container.appendChild(labelsContainer);
            
            const labelElements = [];

            // Add stars
            allSystems.forEach(sys => {
                const isSelf = sys.id === selfSystem.id;
                const isLive = livePeerIDs.has(sys.id);
                const isCached = !isSelf && !isLive;
                const size = isSelf ? 40 : (isLive ? 28 : 22);
                const star = createStarSprite(sys.color || '#ffffff', size, isSelf, isCached);
                star.position.set(sys.x, sys.y, sys.z);
                star.userData = { system: sys, isSelf: isSelf, isLive: isLive, isCached: isCached };
                scene.add(star);
                starMeshes.push(star);
                
                // Create HTML label
                const label = document.createElement('div');
                label.textContent = sys.name;
                label.style.cssText = 'position:absolute;font-size:11px;white-space:nowrap;transform:translateX(-50%);';
                if (isSelf) {
                    label.style.color = '#60a5fa';
                    label.style.fontWeight = '500';
                } else if (isLive) {
                    label.style.color = '#4ade80';
                } else {
                    label.style.color = '#666';
                }
                labelsContainer.appendChild(label);
                labelElements.push({ element: label, position: star.position, isSelf: isSelf });
            });

            // Build reciprocity map
            const edgeSet = new Set();
            if (cachedConnections) {
                cachedConnections.forEach(conn => {
                    edgeSet.add(conn.from_id + ':' + conn.to_id);
                });
            }
            
            function isReciprocal(fromId, toId) {
                return edgeSet.has(fromId + ':' + toId) && edgeSet.has(toId + ':' + fromId);
            }

            // Add connection lines
            const processedEdges = new Set();
            if (cachedConnections && cachedConnections.length > 0) {
                cachedConnections.forEach(conn => {
                    const from = systemById[conn.from_id];
                    const to = systemById[conn.to_id];
                    if (!from || !to) return;
                    
                    // Avoid drawing same edge twice
                    const edgeKey = [conn.from_id, conn.to_id].sort().join(':');
                    if (processedEdges.has(edgeKey)) return;
                    processedEdges.add(edgeKey);
                    
                    const reciprocal = isReciprocal(conn.from_id, conn.to_id);
                    const points = [
                        new THREE.Vector3(from.x, from.y, from.z),
                        new THREE.Vector3(to.x, to.y, to.z)
                    ];
                    const geometry = new THREE.BufferGeometry().setFromPoints(points);
                    
                    let line;
                    if (reciprocal) {
                        // Solid line for reciprocal connections
                        const material = new THREE.LineBasicMaterial({ 
                            color: 0x64c8ff, 
                            transparent: true, 
                            opacity: 0.35
                        });
                        line = new THREE.Line(geometry, material);
                    } else {
                        // Dashed line for one-way connections
                        const material = new THREE.LineDashedMaterial({ 
                            color: 0xffaa44, 
                            transparent: true, 
                            opacity: 0.3,
                            dashSize: 30,
                            gapSize: 20
                        });
                        line = new THREE.Line(geometry, material);
                        line.computeLineDistances();
                    }
                    line.userData = { fromId: conn.from_id, toId: conn.to_id, reciprocal: reciprocal };
                    scene.add(line);
                    connectionLines.push(line);
                });
            }

            // Add subtle starfield background
            const starfieldGeo = new THREE.BufferGeometry();
            const starfieldCount = 2000;
            const starfieldPositions = new Float32Array(starfieldCount * 3);
            for (let i = 0; i < starfieldCount * 3; i += 3) {
                starfieldPositions[i] = (Math.random() - 0.5) * 30000;
                starfieldPositions[i+1] = (Math.random() - 0.5) * 30000;
                starfieldPositions[i+2] = (Math.random() - 0.5) * 30000;
            }
            starfieldGeo.setAttribute('position', new THREE.BufferAttribute(starfieldPositions, 3));
            const starfieldMat = new THREE.PointsMaterial({ color: 0x444466, size: 2, transparent: true, opacity: 0.5 });
            const starfield = new THREE.Points(starfieldGeo, starfieldMat);
            scene.add(starfield);

            // Center on self initially
            centerOnSelf();

            // Raycaster for hover
            const raycaster = new THREE.Raycaster();
            raycaster.params.Sprite = { threshold: 20 };
            const mouse = new THREE.Vector2();
            let hoveredSystemId = null;
            
            function highlightConnections(systemId) {
                connectionLines.forEach(line => {
                    if (systemId && (line.userData.fromId === systemId || line.userData.toId === systemId)) {
                        line.material.opacity = line.userData.reciprocal ? 0.8 : 0.6;
                        if (line.userData.reciprocal) {
                            line.material.color.setHex(0x60a5fa);
                        }
                    } else {
                        line.material.opacity = line.userData.reciprocal ? 0.35 : 0.3;
                        if (line.userData.reciprocal) {
                            line.material.color.setHex(0x64c8ff);
                        }
                    }
                });
            }
            
            renderer.domElement.addEventListener('mousemove', (event) => {
                const rect = renderer.domElement.getBoundingClientRect();
                mouse.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
                mouse.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;
                
                raycaster.setFromCamera(mouse, camera);
                const intersects = raycaster.intersectObjects(starMeshes);
                
                const tooltip = document.getElementById('map-tooltip');
                if (intersects.length > 0) {
                    const userData = intersects[0].object.userData;
                    const sys = userData.system;
                    const isSelf = userData.isSelf;
                    const isLive = userData.isLive;
                    const distance = calculateDistance(selfSystem, sys);
                    
                    let statusLabel = '';
                    if (isSelf) statusLabel = ' <span style="color:#60a5fa">(You)</span>';
                    else if (isLive) statusLabel = ' <span style="color:#4ade80">(Live)</span>';
                    else statusLabel = ' <span style="color:#888">(Cached)</span>';
                    
                    tooltip.innerHTML = 
                        '<div class="tooltip-name">' + sys.name + statusLabel + '</div>' +
                        '<div class="tooltip-class">' + (sys.starDesc || sys.starClass + '-class star') + '</div>' +
                        '<div class="tooltip-coords">(' + sys.x.toFixed(1) + ', ' + sys.y.toFixed(1) + ', ' + sys.z.toFixed(1) + ')</div>' +
                        (isSelf ? '' : '<div class="tooltip-distance">' + distance.toFixed(1) + ' units away</div>');
                    tooltip.style.display = 'block';
                    tooltip.style.left = (event.clientX - rect.left + 15) + 'px';
                    tooltip.style.top = (event.clientY - rect.top + 15) + 'px';
                    renderer.domElement.style.cursor = 'pointer';
                    
                    // Highlight connections
                    if (hoveredSystemId !== sys.id) {
                        hoveredSystemId = sys.id;
                        highlightConnections(sys.id);
                    }
                } else {
                    tooltip.style.display = 'none';
                    renderer.domElement.style.cursor = 'grab';
                    if (hoveredSystemId !== null) {
                        hoveredSystemId = null;
                        highlightConnections(null);
                    }
                }
            });

            // Animation loop
            function animate() {
                requestAnimationFrame(animate);
                controls.update();
                
                // Update label positions
                const widthHalf = width / 2;
                const heightHalf = height / 2;
                labelElements.forEach(item => {
                    const pos = item.position.clone();
                    pos.project(camera);
                    
                    // Check if in front of camera
                    if (pos.z < 1) {
                        item.element.style.display = 'block';
                        item.element.style.left = (pos.x * widthHalf + widthHalf) + 'px';
                        item.element.style.top = (-pos.y * heightHalf + heightHalf + 15) + 'px';
                    } else {
                        item.element.style.display = 'none';
                    }
                });
                
                renderer.render(scene, camera);
            }
            animate();

            // Handle resize
            window.addEventListener('resize', () => {
                const w = container.clientWidth;
                const h = container.clientHeight;
                camera.aspect = w / h;
                camera.updateProjectionMatrix();
                renderer.setSize(w, h);
            });
        }

        document.addEventListener('DOMContentLoaded', initGalaxyMap);
        setTimeout(function() { location.reload(); }, 30000);
    </script>
</body>
</html>`