package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"
)

const webInterfaceHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.SystemName}} - Stellar Mesh</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: monospace;
            background: #000;
            color: #0f0;
            padding: 20px;
            line-height: 1.6;
        }
        .container { max-width: 900px; margin: 0 auto; }
        h1 { color: #0ff; border-bottom: 2px solid #0ff; padding-bottom: 10px; margin-bottom: 20px; }
        h2 { color: #0f0; margin: 20px 0 10px 0; font-size: 1.2em; }
        .box {
            border: 1px solid #0f0;
            padding: 15px;
            margin: 15px 0;
            background: #001100;
        }
        .stat-row { display: flex; justify-content: space-between; padding: 5px 0; }
        .stat-label { color: #0ff; }
        .stat-value { color: #fff; }
        .rank {
            display: inline-block;
            padding: 5px 10px;
            border: 2px solid;
            font-weight: bold;
            margin: 5px 0;
        }
        .rank-diamond { color: #00ffff; border-color: #00ffff; }
        .rank-platinum { color: #e5e4e2; border-color: #e5e4e2; }
        .rank-gold { color: #ffd700; border-color: #ffd700; }
        .rank-silver { color: #c0c0c0; border-color: #c0c0c0; }
        .rank-bronze { color: #cd7f32; border-color: #cd7f32; }
        .rank-unranked { color: #666; border-color: #666; }
        .peer-list { list-style: none; }
        .peer-item {
            padding: 8px;
            margin: 5px 0;
            border-left: 3px solid #0f0;
            background: #002200;
        }
        form { margin: 15px 0; }
        input[type="text"] {
            background: #002200;
            border: 1px solid #0f0;
            color: #0f0;
            padding: 8px;
            width: 300px;
            font-family: monospace;
        }
        button {
            background: #003300;
            border: 2px solid #0f0;
            color: #0f0;
            padding: 8px 16px;
            cursor: pointer;
            font-family: monospace;
            font-weight: bold;
        }
        button:hover { background: #004400; }
        .stars { margin: 10px 0; }
        .star { padding: 5px; margin: 3px 0; border-left: 3px solid; }
        .refresh-link { color: #0ff; text-decoration: none; float: right; }
        .refresh-link:hover { text-decoration: underline; }
    </style>
</head>
<body>
	<div class="nav" style="background: #001100; padding: 10px 20px; border-bottom: 1px solid #0f0; margin-bottom: 20px;">
        <a href="/" style="color: #0f0; background: #0f0; color: #000; text-decoration: none; margin-right: 20px; padding: 5px 10px;">System Info</a>
        <a href="/map" style="color: #0f0; text-decoration: none; margin-right: 20px; padding: 5px 10px;">Galactic Map</a>
    </div>
    <div class="container">
        <h1>⭐ {{.SystemName}} <a href="/" class="refresh-link">↻ Refresh</a></h1>
        
        <div class="box">
            <h2>System Information</h2>
            <div class="stat-row">
                <span class="stat-label">System ID:</span>
                <span class="stat-value">{{.SystemID}}</span>
            </div>
            <div class="stat-row">
                <span class="stat-label">Coordinates:</span>
                <span class="stat-value">X: {{printf "%.2f" .X}} | Y: {{printf "%.2f" .Y}} | Z: {{printf "%.2f" .Z}}</span>
            </div>
            <div class="stat-row">
                <span class="stat-label">Uptime:</span>
                <span class="stat-value">{{.Uptime}}</span>
            </div>
            <div class="stat-row">
                <span class="stat-label">Created:</span>
                <span class="stat-value">{{.Created}}</span>
            </div>
        </div>

        <div class="box">
            <h2>Stellar Composition ({{.StarCount}} Star{{if ne .StarCount 1}}s{{end}})</h2>
            <div class="stars">
                {{range .Stars}}
                <div class="star" style="border-left-color: {{.Color}}">
                    <strong>{{.Label}}:</strong> {{.Class}} - {{.Description}} ({{.Temperature}}K, {{printf "%.2f" .Luminosity}}L☉)
                </div>
                {{end}}
            </div>
        </div>

        <div class="box">
            <h2>Reputation & Network Contribution</h2>
            <div class="rank rank-{{.RankClass}}">{{.Rank}}</div>
            <div class="stat-row">
                <span class="stat-label">Verified Points:</span>
                <span class="stat-value">{{.VerifiedPoints}}</span>
            </div>
            <div class="stat-row">
                <span class="stat-label">Attestations:</span>
                <span class="stat-value">{{.TotalAttestations}} ({{.UptimeAttestations}} uptime, {{.BridgeAttestations}} bridge, {{.RelayAttestations}} relay)</span>
            </div>
            <div class="stat-row">
                <span class="stat-label">Last Attestation:</span>
                <span class="stat-value">{{.LastAttestation}}</span>
            </div>
            <div class="stat-row">
                <span class="stat-label">Public Key:</span>
                <span class="stat-value">{{.PublicKey}}</span>
            </div>
        </div>

        <div class="box">
            <h2>Connected Systems ({{.PeerCount}})</h2>
            {{if .Peers}}
            <ul class="peer-list">
                {{range .Peers}}
				<li class="peer-item">
				    <strong>{{.Name}}</strong><br>
				    {{.StarInfo}}<br>
				    ID: {{.ID}}<br>
				    Address: {{.Address}} | Last seen: {{.LastSeen}}
				</li>
				{{end}}
            </ul>
            {{else}}
            <p>No peers connected yet.</p>
            {{end}}
        </div>

        <div class="box">
            <h2>Galaxy Statistics</h2>
            <div class="stat-row">
                <span class="stat-label">Total Systems in Network:</span>
                <span class="stat-value">{{.TotalSystems}} (known)</span>
            </div>
            <div class="stat-row">
                <span class="stat-label">Direct Connections:</span>
                <span class="stat-value">{{.PeerCount}}</span>
            </div>
        </div>

        <div class="box">
            <h2>Add Peer System</h2>
            <form method="POST" action="/add-peer">
                <input type="text" name="address" placeholder="hostname:port" required>
                <button type="submit">Connect</button>
            </form>
        </div>

        <div class="box">
            <h2>Quick Actions</h2>
            <a href="/api/system" style="color: #0ff">View JSON (System Info)</a> |
            <a href="/api/reputation" style="color: #0ff">View JSON (Reputation)</a> |
            <a href="/api/peers" style="color: #0ff">View JSON (Peers)</a>
        </div>
    </div>
</body>
</html>`

type WebInterfaceData struct {
	SystemName         string
	SystemID           string
	X, Y, Z            float64
	Uptime             string
	Created            string
	StarCount          int
	Stars              []StarDisplay
	Rank               string
	RankClass          string
	VerifiedPoints     int
	TotalAttestations  int
	UptimeAttestations int
	BridgeAttestations int
	RelayAttestations  int
	LastAttestation    string
	PublicKey          string
	PeerCount          int
	Peers              []PeerDisplay
	TotalSystems       int
}

type StarDisplay struct {
	Label       string
	Class       string
	Description string
	Color       string
	Temperature int
	Luminosity  float64
}

type PeerDisplay struct {
	Name     string
	StarInfo string
	ID       string
	Address  string
	LastSeen string
}


// ServeWebInterface handles the main web UI
func (api *API) ServeWebInterface(w http.ResponseWriter, r *http.Request) {
	// Gather all data
	peers := api.transport.GetPeers()
	
	// Build peer displays with system info fetches
	peerDisplays := make([]PeerDisplay, 0)
	for _, peer := range peers {
		name := peer.SystemID.String()[:8] + "..."
		starInfo := "Unknown star type"

		// Try to get cached system info
		if peerSys, err := api.storage.GetPeerSystem(peer.SystemID); err == nil && peerSys != nil {
			name = peerSys.Name
			starInfo = peerSys.Stars.Primary.Class + " - " + peerSys.Stars.Primary.Description
		}

		peerDisplays = append(peerDisplays, PeerDisplay{
			Name:     name,
			StarInfo: starInfo,
			ID:       peer.SystemID.String(),
			Address:  peer.Address,
			LastSeen: formatDuration(time.Since(peer.LastSeenAt)),
		})
	}
	
	// Get reputation from attestations
	attestations, _ := api.storage.GetAttestations(api.system.ID)
	proof := BuildAttestationProof(api.system.ID, attestations)
	reputationScore := CalculateVerifiableReputation(proof)
	rank := GetVerifiableRank(reputationScore)
	
	// Calculate time since last attestation
	lastAttestationAgo := "Never"
	if len(attestations) > 0 {
		lastTime := time.Unix(attestations[len(attestations)-1].Timestamp, 0)
		lastAttestationAgo = formatDuration(time.Since(lastTime)) + " ago"
	}
	
	// Get public key
	publicKey := ""
	if api.system.Keys != nil {
		publicKey = base64.StdEncoding.EncodeToString(api.system.Keys.PublicKey)[:16] + "..."
	}
	
	repSummary := map[string]interface{}{
		"rank":                  rank,
		"reputation_points":     int(reputationScore),
		"verified_points":       int(reputationScore),
		"verified_attestations": proof.TotalProofs,
		"total_attestations":    proof.TotalProofs,
		"uptime_attestations":   proof.TotalProofs,
		"bridge_attestations":   0,
		"relay_attestations":    0,
		"unique_peers":          proof.UniquePeers,
		"last_attestation_ago":  lastAttestationAgo,
		"public_key":            publicKey,
	}
	
	// Build star displays
	starDisplays := make([]StarDisplay, 0)
	starDisplays = append(starDisplays, StarDisplay{
		Label:       "Primary",
		Class:       api.system.Stars.Primary.Class,
		Description: api.system.Stars.Primary.Description,
		Color:       api.system.Stars.Primary.Color,
		Temperature: api.system.Stars.Primary.Temperature,
		Luminosity:  api.system.Stars.Primary.Luminosity,
	})
	if api.system.Stars.Secondary != nil {
		starDisplays = append(starDisplays, StarDisplay{
			Label:       "Secondary",
			Class:       api.system.Stars.Secondary.Class,
			Description: api.system.Stars.Secondary.Description,
			Color:       api.system.Stars.Secondary.Color,
			Temperature: api.system.Stars.Secondary.Temperature,
			Luminosity:  api.system.Stars.Secondary.Luminosity,
		})
	}
	if api.system.Stars.Tertiary != nil {
		starDisplays = append(starDisplays, StarDisplay{
			Label:       "Tertiary",
			Class:       api.system.Stars.Tertiary.Class,
			Description: api.system.Stars.Tertiary.Description,
			Color:       api.system.Stars.Tertiary.Color,
			Temperature: api.system.Stars.Tertiary.Temperature,
			Luminosity:  api.system.Stars.Tertiary.Luminosity,
		})
	}
	
	rankClass := "unranked"
	rank = repSummary["rank"].(string)
	switch rank {
	case "Diamond":
		rankClass = "diamond"
	case "Platinum":
		rankClass = "platinum"
	case "Gold":
		rankClass = "gold"
	case "Silver":
		rankClass = "silver"
	case "Bronze":
		rankClass = "bronze"
	}
	
	data := WebInterfaceData{
		SystemName:         api.system.Name,
		SystemID:           api.system.ID.String(),
		X:                  api.system.X,
		Y:                  api.system.Y,
		Z:                  api.system.Z,
		Uptime:             formatDuration(time.Since(api.system.CreatedAt)),
		Created:            api.system.CreatedAt.Format("2006-01-02 15:04:05"),
		StarCount:          api.system.Stars.Count,
		Stars:              starDisplays,
		Rank:               rank,
		RankClass:          rankClass,
		VerifiedPoints:     repSummary["verified_points"].(int),
		TotalAttestations:  repSummary["total_attestations"].(int),
		UptimeAttestations: repSummary["uptime_attestations"].(int),
		BridgeAttestations: repSummary["bridge_attestations"].(int),
		RelayAttestations:  repSummary["relay_attestations"].(int),
		LastAttestation:    repSummary["last_attestation_ago"].(string),
		PublicKey:          repSummary["public_key"].(string),
		PeerCount:          len(peers),
		Peers:              peerDisplays,
		TotalSystems:       api.storage.CountKnownSystems() + 1, // All known systems + self
	}
	
	tmpl, err := template.New("web").Parse(webInterfaceHTML)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, data)
}

// HandleAddPeerForm processes the add peer form submission
func (api *API) HandleAddPeerForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	
	address := r.FormValue("address")
	if address == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	
	// Try to connect to the peer
	resp, err := http.Get("http://" + address + "/system")
	if err != nil {
		// Failed, but redirect anyway
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	defer resp.Body.Close()
	
	var peerSystem System
	if err := json.NewDecoder(resp.Body).Decode(&peerSystem); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	
	// Add the peer
	api.transport.AddPeer(peerSystem.ID, address)
	
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else {
		return fmt.Sprintf("%dm", minutes)
	}
}

// ServeMapPage serves the galactic map visualization
func (api *API) ServeMapPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("map").Parse(mapPageHTML)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	data := struct {
		SystemName string
	}{
		SystemName: api.system.Name,
	}

	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, data)
}

const mapPageHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.SystemName}} - Galactic Map</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            background: #000;
            color: #0f0;
            font-family: 'Courier New', monospace;
            min-height: 100vh;
        }
        .nav {
            background: #001100;
            padding: 10px 20px;
            border-bottom: 1px solid #0f0;
        }
        .nav a {
            color: #0f0;
            text-decoration: none;
            margin-right: 20px;
            padding: 5px 10px;
        }
        .nav a:hover, .nav a.active {
            background: #0f0;
            color: #000;
        }
        .container {
            padding: 20px;
        }
        h1 {
            margin-bottom: 20px;
            color: #0f0;
        }

        /* 2D Map Styles */
        .map-2d-container {
            background: #000a00;
            border: 1px solid #0f0;
            border-radius: 5px;
            padding: 10px;
            margin-bottom: 20px;
        }
        .map-2d-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 10px;
        }
        .map-2d-header h2 {
            color: #0f0;
            font-size: 1.2em;
        }
        .view-toggle {
            display: flex;
            gap: 10px;
        }
        .view-toggle button {
            background: #001100;
            color: #0f0;
            border: 1px solid #0f0;
            padding: 5px 15px;
            cursor: pointer;
            font-family: inherit;
        }
        .view-toggle button:hover, .view-toggle button.active {
            background: #0f0;
            color: #000;
        }
        #map-2d {
            width: 100%;
            height: 400px;
            background: #000;
        }

        /* Star styling in SVG */
        .star {
            cursor: pointer;
            transition: r 0.2s;
        }
        .star:hover {
            filter: brightness(1.5);
        }
        .star-local {
            filter: drop-shadow(0 0 10px currentColor);
        }
        .star-label {
            font-size: 10px;
            fill: #0f0;
            pointer-events: none;
        }
        .connection {
            stroke: #003300;
            stroke-width: 1;
        }
        .grid-line {
            stroke: #001a00;
            stroke-width: 0.5;
        }

        /* 3D Map Styles */
        .map-3d-container {
            background: #000a00;
            border: 1px solid #0f0;
            border-radius: 5px;
            padding: 10px;
        }
        .map-3d-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 10px;
        }
        .map-3d-header h2 {
            color: #0f0;
            font-size: 1.2em;
        }
        #map-3d-placeholder {
            width: 100%;
            height: 400px;
            background: #000;
            display: flex;
            flex-direction: column;
            justify-content: center;
            align-items: center;
            border: 1px dashed #003300;
        }
        #map-3d-placeholder p {
            color: #006600;
            margin-bottom: 20px;
        }
        #enable-3d-btn {
            background: #001100;
            color: #0f0;
            border: 1px solid #0f0;
            padding: 10px 30px;
            cursor: pointer;
            font-family: inherit;
            font-size: 1em;
        }
        #enable-3d-btn:hover {
            background: #0f0;
            color: #000;
        }
        #map-3d {
            width: 100%;
            height: 400px;
            display: none;
        }
        #disable-3d-btn {
            display: none;
            background: #110000;
            color: #f00;
            border: 1px solid #f00;
            padding: 5px 15px;
            cursor: pointer;
            font-family: inherit;
        }
        #disable-3d-btn:hover {
            background: #f00;
            color: #000;
        }

        /* Tooltip */
        .tooltip {
            position: absolute;
            background: #001100;
            border: 1px solid #0f0;
            padding: 10px;
            border-radius: 5px;
            pointer-events: none;
            display: none;
            z-index: 1000;
        }
        .tooltip h3 {
            margin-bottom: 5px;
            color: #0f0;
        }
        .tooltip p {
            color: #0a0;
            font-size: 0.9em;
        }

        /* Legend */
        .legend {
            margin-top: 10px;
            padding: 10px;
            background: #000a00;
            border: 1px solid #003300;
            border-radius: 5px;
        }
        .legend h3 {
            margin-bottom: 10px;
            color: #0f0;
        }
        .legend-item {
            display: inline-flex;
            align-items: center;
            margin-right: 20px;
            margin-bottom: 5px;
        }
        .legend-color {
            width: 12px;
            height: 12px;
            border-radius: 50%;
            margin-right: 8px;
        }
    </style>
</head>
<body>
    <div class="nav">
        <a href="/">System Info</a>
        <a href="/map" class="active">Galactic Map</a>
    </div>

    <div class="container">
        <h1>🌌 Galactic Map</h1>

        <!-- 2D Map Section -->
        <div class="map-2d-container">
            <div class="map-2d-header">
                <h2>2D View</h2>
                <div class="view-toggle">
                    <button id="view-top" class="active" onclick="setView('top')">Top (X-Z)</button>
                    <button id="view-side" onclick="setView('side')">Side (X-Y)</button>
                    <button id="view-front" onclick="setView('front')">Front (Z-Y)</button>
                </div>
            </div>
            <svg id="map-2d" viewBox="-5000 -5000 10000 10000">
                <!-- Grid lines will be added by JS -->
                <g id="grid"></g>
                <!-- Connections drawn first (behind stars) -->
                <g id="connections"></g>
                <!-- Stars drawn on top -->
                <g id="stars"></g>
            </svg>

            <div class="legend">
                <h3>Star Types</h3>
                <div class="legend-item"><span class="legend-color" style="background: #9bb0ff;"></span> O - Blue Giant</div>
                <div class="legend-item"><span class="legend-color" style="background: #aabfff;"></span> B - Blue-White</div>
                <div class="legend-item"><span class="legend-color" style="background: #cad7ff;"></span> A - White</div>
                <div class="legend-item"><span class="legend-color" style="background: #f8f7ff;"></span> F - Yellow-White</div>
                <div class="legend-item"><span class="legend-color" style="background: #fff4ea;"></span> G - Yellow (Sol-like)</div>
                <div class="legend-item"><span class="legend-color" style="background: #ffd2a1;"></span> K - Orange</div>
                <div class="legend-item"><span class="legend-color" style="background: #ffcc6f;"></span> M - Red Dwarf</div>
            </div>
        </div>

        <!-- 3D Map Section -->
        <div class="map-3d-container">
            <div class="map-3d-header">
                <h2>3D View</h2>
                <button id="disable-3d-btn" onclick="disable3D()">Disable 3D</button>
            </div>
            <div id="map-3d-placeholder">
                <p>3D visualization uses WebGL and may impact performance</p>
                <button id="enable-3d-btn" onclick="enable3D()">🚀 Enable 3D View</button>
            </div>
            <div id="map-3d"></div>
        </div>
    </div>

    <!-- Tooltip -->
    <div class="tooltip" id="tooltip">
        <h3 id="tooltip-name"></h3>
        <p id="tooltip-class"></p>
        <p id="tooltip-coords"></p>
    </div>

    <script>
        // Global state
        let mapData = null;
        let currentView = 'top';
        let three3DEnabled = false;
        let threeScene, threeCamera, threeRenderer, threeControls;
        let animationFrameId = null;

        // Fetch map data from API
        async function fetchMapData() {
            try {
                const response = await fetch('/api/map');
                mapData = await response.json();
                render2DMap();
            } catch (error) {
                console.error('Failed to fetch map data:', error);
            }
        }

        // Calculate view bounds based on data
        function calculateBounds() {
            if (!mapData || mapData.systems.length === 0) {
                return { minX: -5000, maxX: 5000, minY: -5000, maxY: 5000 };
            }

            let coords = [];
            mapData.systems.forEach(sys => {
                switch(currentView) {
                    case 'top':   coords.push({x: sys.x, y: sys.z}); break;
                    case 'side':  coords.push({x: sys.x, y: sys.y}); break;
                    case 'front': coords.push({x: sys.z, y: sys.y}); break;
                }
            });

            let minX = Math.min(...coords.map(c => c.x)) - 500;
            let maxX = Math.max(...coords.map(c => c.x)) + 500;
            let minY = Math.min(...coords.map(c => c.y)) - 500;
            let maxY = Math.max(...coords.map(c => c.y)) + 500;

            // Keep aspect ratio and ensure minimum size
            let rangeX = Math.max(maxX - minX, 1000);
            let rangeY = Math.max(maxY - minY, 1000);
            let range = Math.max(rangeX, rangeY);

            let centerX = (minX + maxX) / 2;
            let centerY = (minY + maxY) / 2;

            return {
                minX: centerX - range/2,
                maxX: centerX + range/2,
                minY: centerY - range/2,
                maxY: centerY + range/2
            };
        }

        // Render 2D SVG map
        function render2DMap() {
            if (!mapData) return;

            const svg = document.getElementById('map-2d');
            const starsGroup = document.getElementById('stars');
            const connectionsGroup = document.getElementById('connections');
            const gridGroup = document.getElementById('grid');

            // Clear existing content
            starsGroup.innerHTML = '';
            connectionsGroup.innerHTML = '';
            gridGroup.innerHTML = '';

            // Calculate bounds
            const bounds = calculateBounds();
            const range = bounds.maxX - bounds.minX;
            svg.setAttribute('viewBox',
                bounds.minX + ' ' + bounds.minY + ' ' + range + ' ' + range);

            // Draw grid
            const gridSpacing = range / 10;
            for (let i = 0; i <= 10; i++) {
                // Vertical lines
                const vLine = document.createElementNS('http://www.w3.org/2000/svg', 'line');
                vLine.setAttribute('x1', bounds.minX + i * gridSpacing);
                vLine.setAttribute('y1', bounds.minY);
                vLine.setAttribute('x2', bounds.minX + i * gridSpacing);
                vLine.setAttribute('y2', bounds.maxY);
                vLine.setAttribute('class', 'grid-line');
                gridGroup.appendChild(vLine);

                // Horizontal lines
                const hLine = document.createElementNS('http://www.w3.org/2000/svg', 'line');
                hLine.setAttribute('x1', bounds.minX);
                hLine.setAttribute('y1', bounds.minY + i * gridSpacing);
                hLine.setAttribute('x2', bounds.maxX);
                hLine.setAttribute('y2', bounds.minY + i * gridSpacing);
                hLine.setAttribute('class', 'grid-line');
                gridGroup.appendChild(hLine);
            }

            // Create lookup for system positions
            const systemPositions = {};
            mapData.systems.forEach(sys => {
                let x, y;
                switch(currentView) {
                    case 'top':   x = sys.x; y = sys.z; break;
                    case 'side':  x = sys.x; y = sys.y; break;
                    case 'front': x = sys.z; y = sys.y; break;
                }
                systemPositions[sys.id] = {x, y, sys};
            });

            // Draw connections
            mapData.connections.forEach(conn => {
                const from = systemPositions[conn.from];
                const to = systemPositions[conn.to];
                if (from && to) {
                    const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
                    line.setAttribute('x1', from.x);
                    line.setAttribute('y1', from.y);
                    line.setAttribute('x2', to.x);
                    line.setAttribute('y2', to.y);
                    line.setAttribute('class', 'connection');
                    connectionsGroup.appendChild(line);
                }
            });

            // Draw stars
            Object.values(systemPositions).forEach(({x, y, sys}) => {
                const group = document.createElementNS('http://www.w3.org/2000/svg', 'g');

                // Star circle
                const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
                circle.setAttribute('cx', x);
                circle.setAttribute('cy', y);
                circle.setAttribute('r', sys.is_local ? range/80 : range/100);
                circle.setAttribute('fill', sys.star_color);
                circle.setAttribute('class', 'star' + (sys.is_local ? ' star-local' : ''));

                // Tooltip events
                circle.addEventListener('mouseenter', (e) => showTooltip(e, sys));
                circle.addEventListener('mouseleave', hideTooltip);
                circle.addEventListener('mousemove', moveTooltip);

                group.appendChild(circle);

                // Label for local system
                if (sys.is_local) {
                    const label = document.createElementNS('http://www.w3.org/2000/svg', 'text');
                    label.setAttribute('x', x);
                    label.setAttribute('y', y - range/60);
                    label.setAttribute('text-anchor', 'middle');
                    label.setAttribute('class', 'star-label');
                    label.setAttribute('font-size', range/50);
                    label.textContent = sys.name;
                    group.appendChild(label);
                }

                starsGroup.appendChild(group);
            });
        }

        // View toggle
        function setView(view) {
            currentView = view;
            document.querySelectorAll('.view-toggle button').forEach(btn => {
                btn.classList.remove('active');
            });
            document.getElementById('view-' + view).classList.add('active');
            render2DMap();
        }

        // Tooltip functions
        function showTooltip(event, sys) {
            const tooltip = document.getElementById('tooltip');
            document.getElementById('tooltip-name').textContent = sys.name;
            document.getElementById('tooltip-class').textContent = 'Class ' + sys.star_class + ' Star';
            document.getElementById('tooltip-coords').textContent =
                'Coordinates: (' + sys.x.toFixed(1) + ', ' + sys.y.toFixed(1) + ', ' + sys.z.toFixed(1) + ')';
            tooltip.style.display = 'block';
            moveTooltip(event);
        }

        function hideTooltip() {
            document.getElementById('tooltip').style.display = 'none';
        }

        function moveTooltip(event) {
            const tooltip = document.getElementById('tooltip');
            tooltip.style.left = (event.pageX + 15) + 'px';
            tooltip.style.top = (event.pageY + 15) + 'px';
        }

        // 3D Functions
        function enable3D() {
            if (three3DEnabled) return;

            // Load Three.js dynamically
            const script = document.createElement('script');
            script.src = 'https://cdnjs.cloudflare.com/ajax/libs/three.js/r128/three.min.js';
            script.onload = () => {
                // Load OrbitControls
                const controlsScript = document.createElement('script');
                controlsScript.src = 'https://cdn.jsdelivr.net/npm/three@0.128.0/examples/js/controls/OrbitControls.js';
                controlsScript.onload = init3D;
                document.head.appendChild(controlsScript);
            };
            document.head.appendChild(script);
        }

        function init3D() {
            three3DEnabled = true;

            // Hide placeholder, show 3D container
            document.getElementById('map-3d-placeholder').style.display = 'none';
            document.getElementById('map-3d').style.display = 'block';
            document.getElementById('disable-3d-btn').style.display = 'inline-block';

            const container = document.getElementById('map-3d');
            const width = container.clientWidth;
            const height = container.clientHeight;

            // Scene
            threeScene = new THREE.Scene();
            threeScene.background = new THREE.Color(0x000000);

            // Camera
            threeCamera = new THREE.PerspectiveCamera(75, width / height, 1, 50000);
            threeCamera.position.set(
                mapData.center.x + 1000,
                mapData.center.y + 1000,
                mapData.center.z + 1000
            );

            // Renderer
            threeRenderer = new THREE.WebGLRenderer({ antialias: true });
            threeRenderer.setSize(width, height);
            container.appendChild(threeRenderer.domElement);

            // Controls
            threeControls = new THREE.OrbitControls(threeCamera, threeRenderer.domElement);
            threeControls.target.set(mapData.center.x, mapData.center.y, mapData.center.z);
            threeControls.enableDamping = true;
            threeControls.dampingFactor = 0.05;

            // Add stars
            mapData.systems.forEach(sys => {
                const geometry = new THREE.SphereGeometry(sys.is_local ? 30 : 20, 16, 16);
                const material = new THREE.MeshBasicMaterial({
                    color: sys.star_color
                });
                const star = new THREE.Mesh(geometry, material);
                star.position.set(sys.x, sys.y, sys.z);
                threeScene.add(star);

                // Add glow for local system
                if (sys.is_local) {
                    const glowGeometry = new THREE.SphereGeometry(50, 16, 16);
                    const glowMaterial = new THREE.MeshBasicMaterial({
                        color: sys.star_color,
                        transparent: true,
                        opacity: 0.3
                    });
                    const glow = new THREE.Mesh(glowGeometry, glowMaterial);
                    glow.position.set(sys.x, sys.y, sys.z);
                    threeScene.add(glow);
                }
            });

            // Add connection lines
            const lineMaterial = new THREE.LineBasicMaterial({
                color: 0x003300,
                transparent: true,
                opacity: 0.6
            });

            mapData.connections.forEach(conn => {
                const fromSys = mapData.systems.find(s => s.id === conn.from);
                const toSys = mapData.systems.find(s => s.id === conn.to);

                if (fromSys && toSys) {
                    const points = [
                        new THREE.Vector3(fromSys.x, fromSys.y, fromSys.z),
                        new THREE.Vector3(toSys.x, toSys.y, toSys.z)
                    ];
                    const geometry = new THREE.BufferGeometry().setFromPoints(points);
                    const line = new THREE.Line(geometry, lineMaterial);
                    threeScene.add(line);
                }
            });

            // Add ambient grid
            const gridHelper = new THREE.GridHelper(10000, 20, 0x001a00, 0x001a00);
            gridHelper.position.y = mapData.center.y - 500;
            threeScene.add(gridHelper);

            // Animation loop
            function animate() {
                animationFrameId = requestAnimationFrame(animate);
                threeControls.update();
                threeRenderer.render(threeScene, threeCamera);
            }
            animate();

            // Handle resize
            window.addEventListener('resize', on3DResize);
        }

        function on3DResize() {
            if (!three3DEnabled) return;
            const container = document.getElementById('map-3d');
            const width = container.clientWidth;
            const height = container.clientHeight;
            threeCamera.aspect = width / height;
            threeCamera.updateProjectionMatrix();
            threeRenderer.setSize(width, height);
        }

        function disable3D() {
            if (!three3DEnabled) return;

            // Stop animation
            if (animationFrameId) {
                cancelAnimationFrame(animationFrameId);
                animationFrameId = null;
            }

            // Clean up Three.js
            const container = document.getElementById('map-3d');
            if (threeRenderer) {
                container.removeChild(threeRenderer.domElement);
                threeRenderer.dispose();
            }

            // Clear references
            threeScene = null;
            threeCamera = null;
            threeRenderer = null;
            threeControls = null;
            three3DEnabled = false;

            // Show placeholder again
            document.getElementById('map-3d').style.display = 'none';
            document.getElementById('map-3d-placeholder').style.display = 'flex';
            document.getElementById('disable-3d-btn').style.display = 'none';

            window.removeEventListener('resize', on3DResize);
        }

        // Auto-refresh every 30 seconds
        setInterval(fetchMapData, 30000);

        // Initial load
        fetchMapData();
    </script>
</body>
</html>
`