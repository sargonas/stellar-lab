# Stellar Mesh v2.0 - Quick Reference

## 🚀 Quick Start

### Basic Node
```bash
./stellar-mesh -name "Sol System" -address "localhost:8080"
```

### With Seed (Deterministic UUID)
```bash
./stellar-mesh -name "Alpha Centauri" -seed "my-seed" -address "localhost:8081" -bootstrap "localhost:8080"
```

### Random UUID
```bash
./stellar-mesh -name "Random Star" -random-uuid -address "localhost:8082" -bootstrap "localhost:8080"
```

## 📊 Multi-Star Systems

### Distribution (Matches Real Galaxy)
- 50% Single stars
- 40% Binary systems
- 10% Trinary systems

### Examples
```
Sol System          → G (Yellow Dwarf)                    [Single]
Alpha Centauri      → G + M (Yellow Dwarf + Red Dwarf)   [Binary]
Polaris             → F + F + M                           [Trinary]
Proxima             → M (Red Dwarf)                       [Single]
```

## 🏆 Reputation System

### Ranks
```
Unranked:  <100 points
Bronze:    100+
Silver:    500+
Gold:      2,000+
Platinum:  5,000+
Diamond:   10,000+
```

### How to Earn Points
- ⏰ 1 point per hour uptime
- 🌐 5 points per peer connection
- 🔗 50+ points per critical bridge connection
- 📈 Bonus for high network centrality

### Bridge Bonus Example
```
You connect Cluster A ←→ Cluster B
If you go offline, they're disconnected
Bridge Score: +150 points!
```

## 🔧 API Endpoints

### GET /system
```json
{
  "id": "uuid",
  "name": "Sol System",
  "stars": {
    "primary": {"class": "G", "description": "Yellow Dwarf"},
    "is_binary": false,
    "count": 1
  },
  "x": 1234.56,
  "y": -2345.67,
  "z": 3456.78
}
```

### GET /reputation
```json
{
  "summary": {
    "rank": "Gold",
    "reputation_points": 2450,
    "uptime_days": 42,
    "is_critical_bridge": true
  }
}
```

### GET /peers
List of connected peer systems

### GET /stats
System statistics + star info

## 🌌 UUID Generation

### Hardware-Based (Default)
```bash
# Uses: MAC address + hostname + machine-id
./stellar-mesh -name "My System"
```
- Same hardware = same UUID
- Useful for rebuilding after crash

### Seed-Based
```bash
./stellar-mesh -name "Dev Node" -seed "test123"
```
- Same hardware + same seed = same UUID
- Perfect for development/testing

### Random
```bash
./stellar-mesh -name "Prod Node" -random-uuid
```
- New UUID every time
- Maximum uniqueness

## 📈 Star Type Distribution

```
M Type (Red Dwarf):        76% - Most common
K Type (Orange Dwarf):     12% - Common
G Type (Yellow Dwarf):      8% - Like our Sun
F Type (Yellow-White):      3% - Uncommon
A Type (White Star):      0.6% - Rare
B Type (Blue Giant):     0.13% - Very rare
O Type (Blue Supergiant): 0.003% - Extremely rare
```

## 🔄 Stellar Transport Protocol

Formerly "gossip protocol" - renamed for thematic consistency

### Behaviors
- Heartbeat every 30s to random peer
- Peer exchange every 60s with 3 random peers
- Dead peer cleanup every 5 minutes
- Auto-discovery of new nodes

## 💡 Pro Tips

### Maximize Reputation
1. Stay online (uptime = points)
2. Connect to isolated peers (bridge bonus)
3. Maintain many peer connections (centrality)
4. Be first in a new cluster (critical connector)

### Best Practices
- Use -seed for development environments
- Use hardware-based for stable production
- Use -random-uuid for temporary/test nodes
- Check /reputation regularly to track contribution

### Testing Multi-Star Distribution
```bash
cd cmd/test-multistar
go run main.go
```
Generates 1000 systems and shows distribution stats

## 🗃️ Database Schema

### Tables
- `system` - Your star system (multi-star support)
- `peers` - Known peer nodes
- `reputation` - Network contribution tracking

### Breaking Change
Old databases incompatible - delete .db files

## 🌟 Coming Soon Ideas

- Resource trading between systems
- System features (habitability, resources)
- Faction/alliance support
- Economic layer using reputation
- Visual galaxy map
- Mobile app integration
