package main

import (
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
        .habitable { color: #00ff00; font-weight: bold; }
        .planet { padding: 8px; margin: 5px 0; background: #001100; border-left: 3px solid #666; }
        .planet.habitable { border-left-color: #00ff00; }
        .stars { margin: 10px 0; }
        .star { padding: 5px; margin: 3px 0; border-left: 3px solid; }
        .refresh-link { color: #0ff; text-decoration: none; float: right; }
        .refresh-link:hover { text-decoration: underline; }
    </style>
</head>
<body>
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

        {{if .Planets}}
        <div class="box">
            <h2>Planetary System ({{.TotalPlanets}} Planet{{if ne .TotalPlanets 1}}s{{end}}{{if .HabitablePlanets}}, {{.HabitablePlanets}} Habitable{{end}})</h2>
            <div class="stat-row">
                <span class="stat-label">Habitable Zone:</span>
                <span class="stat-value">{{printf "%.2f" .HZMin}} - {{printf "%.2f" .HZMax}} AU</span>
            </div>
            {{range .Planets}}
            <div class="planet{{if .Habitable}} habitable{{end}}">
                <strong>{{.Name}}</strong> - {{.Type}}{{if .Habitable}} 🌍 HABITABLE{{end}}<br>
                Orbit: {{printf "%.2f" .OrbitAU}} AU | Mass: {{printf "%.2f" .Mass}}M⊕ | Temp: {{.Temperature}}K | Moons: {{.Moons}}
            </div>
            {{end}}
        </div>
        {{end}}

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
	Planets            []PlanetDisplay
	TotalPlanets       int
	HabitablePlanets   int
	HZMin, HZMax       float64
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
	ID       string
	Address  string
	LastSeen string
}

type PlanetDisplay struct {
	Name        string
	Type        string
	OrbitAU     float64
	Mass        float64
	Temperature int
	Moons       int
	Habitable   bool
}

// ServeWebInterface handles the main web UI
func (api *API) ServeWebInterface(w http.ResponseWriter, r *http.Request) {
	// Gather all data
	peers := api.transport.GetPeers()
	
	// Build peer displays with system info fetches
	peerDisplays := make([]PeerDisplay, 0)
	for _, peer := range peers {
		peerDisplays = append(peerDisplays, PeerDisplay{
			Name:     peer.SystemID.String()[:8] + "...", // Will be replaced with actual name if we can fetch it
			ID:       peer.SystemID.String(),
			Address:  peer.Address,
			LastSeen: formatDuration(time.Since(peer.LastSeenAt)),
		})
	}
	
	// Get reputation (use simplified version for now)
	repSummary := api.reputation.GetReputationSummary()
	
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
	
	// Build planet displays
	planetDisplays := make([]PlanetDisplay, 0)
	if api.system.Planets != nil {
		for _, planet := range api.system.Planets.Planets {
			planetDisplays = append(planetDisplays, PlanetDisplay{
				Name:        planet.Name,
				Type:        string(planet.Type),
				OrbitAU:     planet.OrbitAU,
				Mass:        planet.Mass,
				Temperature: planet.Temperature,
				Moons:       planet.Moons,
				Habitable:   planet.Habitable,
			})
		}
	}
	
	rankClass := "unranked"
	rank := repSummary["rank"].(string)
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
		Planets:            planetDisplays,
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
		TotalSystems:       len(peers) + 1, // Simplified: we + known peers
	}
	
	if api.system.Planets != nil {
		data.TotalPlanets = api.system.Planets.TotalPlanets
		data.HabitablePlanets = api.system.Planets.HabitablePlanets
		data.HZMin = api.system.Planets.HabitableZoneMin
		data.HZMax = api.system.Planets.HabitableZoneMax
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
