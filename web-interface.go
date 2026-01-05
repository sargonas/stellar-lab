package main

import (
    "encoding/json"
    "fmt"
    "html/template"
    "log"
    "net"
    "net/http"
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
    ActiveBuckets     int
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
func (w *WebInterface) Start() {
    mux := http.NewServeMux()

    // Web UI
    mux.HandleFunc("/", w.handleIndex)

    // API endpoints
    mux.HandleFunc("/api/system", w.handleSystemAPI)
    mux.HandleFunc("/api/peers", w.handlePeersAPI)
    mux.HandleFunc("/api/known-systems", w.handleKnownSystemsAPI)
    mux.HandleFunc("/api/stats", w.handleStatsAPI)

    // Create listener
    listener, err := net.Listen("tcp", w.addr)
    if err != nil {
        log.Fatalf("Web server failed to bind to %s: %v", w.addr, err)
    }

    log.Printf("Web interface listening on %s", w.addr)
    if err := http.Serve(listener, mux); err != nil {
        log.Fatalf("Web server error: %v", err)
    }
}

// handleIndex serves the main web page
func (w *WebInterface) handleIndex(rw http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/" {
        http.NotFound(rw, r)
        return
    }

    data := w.buildTemplateData()

    tmpl := template.Must(template.New("index").Parse(indexTemplate))
    if err := tmpl.Execute(rw, data); err != nil {
        http.Error(rw, err.Error(), http.StatusInternalServerError)
    }
}

// buildTemplateData gathers all data for the web template
func (w *WebInterface) buildTemplateData() WebInterfaceData {
    sys := w.dht.GetLocalSystem()
    rt := w.dht.GetRoutingTable()

    // Get routing table nodes (active peers)
    peers := rt.GetAllRoutingTableNodes()

    // Get all cached systems (known galaxy)
    allSystems := rt.GetAllCachedSystems()

    // Count active buckets
    activeBuckets := 0
    for i := 0; i < IDBits; i++ {
        if len(rt.GetBucketNodes(i)) > 0 {
            activeBuckets++
        }
    }

    // Get attestation count (use GetDatabaseStats)
    dbStats, _ := w.storage.GetDatabaseStats()
    attestationCount := 0
    if count, ok := dbStats["attestation_count"].(int); ok {
        attestationCount = count
    }

    // Get database size
    dbSizeStr := "unknown"
    if size, ok := dbStats["database_size"].(string); ok {
        dbSizeStr = size
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

    return WebInterfaceData{
        System:           sys,
        Peers:            peers,
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
        ActiveBuckets:    activeBuckets,
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
    <title>{{.System.Name}} - Stellar Mesh DHT</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #0a0a1a 0%, #1a1a3a 100%);
            color: #e0e0e0;
            min-height: 100vh;
            padding: 20px;
        }
        .container { max-width: 1200px; margin: 0 auto; }
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
            grid-template-columns: repeat(auto-fit, minmax(350px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
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
            height: 400px;
            background: rgba(0,0,0,0.3);
            border-radius: 12px;
            position: relative;
            overflow: hidden;
        }
        .map-dot {
            position: absolute;
            width: 8px;
            height: 8px;
            border-radius: 50%;
            transform: translate(-50%, -50%);
            cursor: pointer;
        }
        .map-dot.self {
            width: 12px;
            height: 12px;
            box-shadow: 0 0 10px #60a5fa;
            z-index: 10;
        }
        .version-badge {
            display: inline-block;
            padding: 4px 8px;
            background: rgba(167, 139, 250, 0.2);
            border-radius: 4px;
            font-size: 0.9em;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>{{.System.Name}}</h1>
        <p class="subtitle">
            Stellar Mesh DHT Node
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
                <h2>DHT Statistics</h2>
                <div class="stat-row">
                    <span class="stat-label">Routing Table</span>
                    <span class="stat-value">{{.RoutingTableSize}} nodes</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">Active Buckets</span>
                    <span class="stat-value">{{.ActiveBuckets}} / 128</span>
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

            <div class="card">
                <h2>Galaxy Map ({{.TotalSystems}} systems)</h2>
                <div id="galaxy-map"></div>
            </div>
        </div>
    </div>

    <script>
        const knownSystems = [
            {{range .KnownSystems}}
            {id: "{{.ID}}", name: "{{.Name}}", x: {{.X}}, y: {{.Y}}, z: {{.Z}}, color: "{{.Stars.Primary.Color}}"},
            {{end}}
        ];
        const selfSystem = {
            id: "{{.System.ID}}",
            name: "{{.System.Name}}",
            x: {{.System.X}},
            y: {{.System.Y}},
            z: {{.System.Z}},
            color: "{{.System.Stars.Primary.Color}}"
        };

        function renderGalaxyMap() {
            const map = document.getElementById('galaxy-map');
            if (!map) return;

            const width = map.clientWidth;
            const height = map.clientHeight;

            // Don't render if container has no size yet
            if (width === 0 || height === 0) {
                setTimeout(renderGalaxyMap, 100);
                return;
            }

            const allSystems = [selfSystem, ...knownSystems];
            if (allSystems.length === 0) return;

            let minX = Infinity, maxX = -Infinity;
            let minY = Infinity, maxY = -Infinity;

            allSystems.forEach(s => {
                if (s.x < minX) minX = s.x;
                if (s.x > maxX) maxX = s.x;
                if (s.y < minY) minY = s.y;
                if (s.y > maxY) maxY = s.y;
            });

            // Handle single system or systems at same location
            let rangeX = maxX - minX;
            let rangeY = maxY - minY;

            if (rangeX < 1) {
                minX -= 500;
                maxX += 500;
                rangeX = 1000;
            }
            if (rangeY < 1) {
                minY -= 500;
                maxY += 500;
                rangeY = 1000;
            }

            // Add padding
            const padX = rangeX * 0.1;
            const padY = rangeY * 0.1;
            minX -= padX; maxX += padX;
            minY -= padY; maxY += padY;
            rangeX = maxX - minX;
            rangeY = maxY - minY;

            map.innerHTML = '';

            allSystems.forEach(s => {
                const x = ((s.x - minX) / rangeX) * (width - 20) + 10;
                const y = ((s.y - minY) / rangeY) * (height - 20) + 10;

                const dot = document.createElement('div');
                dot.className = 'map-dot' + (s.id === selfSystem.id ? ' self' : '');
                dot.style.left = x + 'px';
                dot.style.top = y + 'px';
                dot.style.background = s.color || '#60a5fa';
                dot.title = s.name + '\n(' + s.x.toFixed(0) + ', ' + s.y.toFixed(0) + ', ' + s.z.toFixed(0) + ')';
                map.appendChild(dot);
            });
        }

        // Wait for DOM and layout to be ready
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', renderGalaxyMap);
        } else {
            setTimeout(renderGalaxyMap, 50);
        }
        window.addEventListener('resize', renderGalaxyMap);
        setTimeout(() => location.reload(), 30000);
    </script>
</body>
</html>`