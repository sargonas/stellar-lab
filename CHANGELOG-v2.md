# Stellar Mesh - Major Update v2.0

## 🌟 What's New

### 1. Multi-Star Systems (Binary & Trinary)

**Real Galaxy Distribution:**
- 50% Single stars
- 40% Binary systems  
- 10% Trinary systems

Each system now has realistic stellar composition:
- Primary star (always present)
- Secondary star (binary/trinary systems)
- Tertiary star (trinary systems only)

Secondary and tertiary stars are typically smaller than the primary, matching real astronomy.

**Example Output:**
```
Binary Star System:
  Primary:   G (Yellow Dwarf)
  Secondary: M (Red Dwarf)
```

### 2. Semi-Deterministic UUIDs

UUIDs can now be generated from hardware identifiers:

**Hardware-Based** (default):
- Uses MAC address + hostname + machine-id
- Same hardware = same UUID (useful for rebuilding)
- Different hardware = different UUID

**Seed-Based**:
```bash
./stellar-mesh -name "My System" -seed "my-secret-seed"
```
- Same hardware + same seed = same UUID
- Allows deterministic testing and development

**Fully Random**:
```bash
./stellar-mesh -name "My System" -random-uuid
```

### 3. Network Reputation System

Nodes are now rewarded for being critical connectors!

**Metrics Tracked:**
- **Uptime**: How long you've been running
- **Betweenness Centrality**: How many network paths go through you
- **Bridge Score**: Are you a critical connector between network segments?
- **Reputation Points**: Accumulated score based on all factors

**Contribution Ranks:**
- Unranked: < 100 points
- Bronze: 100+ points
- Silver: 500+ points
- Gold: 2,000+ points
- Platinum: 5,000+ points
- Diamond: 10,000+ points

**Earning Reputation:**
- 1 point per hour of uptime
- Bonus points for high betweenness (being on many paths)
- MASSIVE bonus for being a bridge (50+ points per bridge connection)
- Points for peer count (with diminishing returns)

**API Endpoint:**
```bash
curl http://localhost:8080/reputation
```

Response:
```json
{
  "contribution": {
    "system_id": "...",
    "uptime_seconds": 86400,
    "betweenness_score": 45.0,
    "bridge_score": 150.0,
    "reputation_points": 245.5,
    "contribution_rank": "Bronze"
  },
  "summary": {
    "rank": "Bronze",
    "reputation_points": 245,
    "uptime_hours": 24,
    "uptime_days": 1,
    "is_critical_bridge": true,
    "network_centrality": 45.0
  }
}
```

### 4. Renamed Gossip → Stellar Transport

For fun, the gossip protocol is now called "Stellar Transport Protocol"
- Code is cleaner and more thematic
- Endpoints: `/transport` (new) and `/gossip` (legacy compatibility)

## Breaking Changes

⚠️ **Database Schema Changed**:
- Multi-star columns added
- Reputation table added
- Old databases won't work

**Migration:** Delete old .db files and restart fresh.

## New Command-Line Flags

```bash
-seed string           # Optional seed for semi-deterministic UUID
-random-uuid           # Use completely random UUID
```

## Updated API Endpoints

### GET /system
Now includes full multi-star system data:
```json
{
  "id": "...",
  "name": "Sol System",
  "stars": {
    "primary": {
      "class": "G",
      "description": "Yellow Dwarf",
      "color": "#fff4ea",
      "temperature": 5778,
      "luminosity": 1.0
    },
    "secondary": {
      "class": "M",
      "description": "Red Dwarf",
      "color": "#ffcc6f",
      "temperature": 3200,
      "luminosity": 0.05
    },
    "is_binary": true,
    "is_trinary": false,
    "count": 2
  }
}
```

### GET /reputation (NEW)
Returns network contribution metrics and rank.

### GET /stats
Now includes:
- `star_count`: Number of stars in system
- `is_binary`: Boolean
- `is_trinary`: Boolean
- `primary_star_class`: Primary star classification

## Usage Examples

### Create a system with seed (deterministic):
```bash
./stellar-mesh -name "Development Node" -seed "test123" -address "localhost:8080"
# Rebuilding with same seed on same hardware = same UUID
```

### Create a system with random UUID:
```bash
./stellar-mesh -name "Random Node" -random-uuid -address "localhost:8081"
```

### Check your reputation:
```bash
curl http://localhost:8080/reputation | jq '.summary'
```

### See your star system:
```bash
curl http://localhost:8080/system | jq '.stars'
```

## Multi-Star Statistics

In a sample of 1000 systems, you'll see approximately:
- 500 single-star systems
- 400 binary systems
- 100 trinary systems

Star class distribution remains realistic (76% M-type red dwarfs, etc.)

## How Reputation Rewards Work

**Scenario 1: Bridge Node**
- You connect two network clusters
- If you go offline, they're disconnected
- Bridge score: +100 points
- Rank up quickly!

**Scenario 2: High Uptime**
- Run for 30 days = 720 points from uptime alone
- Reach Silver rank (500) in ~21 days
- Reach Gold rank (2000) in ~83 days

**Scenario 3: Central Hub**
- Many peers connect through you
- High betweenness centrality
- +5 points per peer, +10 if they have few others
- Popular nodes = valuable nodes

## Future Applications

The reputation system enables:
- **Access control**: High-rank nodes get priority features
- **Resource allocation**: Reputation as "currency"
- **Network health**: Identify and reward critical infrastructure
- **Gamification**: Leaderboards, achievements, badges
- **Economic layer**: Trade reputation for resources

## Technical Details

**UUID Generation:**
- Hardware ID: SHA256(MAC + hostname + machine-id)
- Seed-based: SHA256(hardware-id + user-seed)
- Consistent across restarts with same inputs

**Reputation Calculation:**
```
reputation = uptime_hours + betweenness_score + (bridge_score * 2) + log10(peers) * 10
```

**Bridge Detection:**
For each pair of your peers, check if they have alternate paths to each other.
If you're their ONLY connection → massive bridge bonus.

## Files Changed

- `system.go` - Multi-star system generation
- `hardware.go` - NEW - Hardware ID and semi-deterministic UUIDs
- `reputation.go` - NEW - Network contribution tracking
- `storage.go` - Updated schema for multi-stars and reputation
- `api.go` - Added /reputation endpoint
- `stellar-transport.go` - Renamed from gossip.go
- `main.go` - Semi-deterministic UUID support, multi-star display
- `README.md` - Updated documentation

## Star System Examples

You might see systems like:
- "Alpha Centauri" - G/M binary (like the real one!)
- "Sirius" - A/M binary (also like the real one!)
- "Polaris" - F/F/M trinary
- "Proxima" - M single
- "Rigel" - B single (rare!)

Every system is deterministic from its UUID, so the same UUID always generates the same stellar composition.
