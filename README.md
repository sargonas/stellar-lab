# Stellar Lab

A silly program for home labbers and others to run when you have a few spare megs of ram. It is a decentralized peer-to-peer mesh network where each node represents a star system in a shared galaxy. Nodes discover each other organically, exchange cryptographically signed attestations to prove participation, and build verifiable reputation over time, based on your time spent as part of the galaxy.

## Overview

Stellar Lab creates a virtual galaxy where each participant runs a node representing a star system. Systems have procedurally generated identities—star types, binary/trinary compositions, and 3D coordinates—all derived deterministically from a cryptographic seed. Nodes build reputation through cryptographically signed attestations of their interactions.

**Key Properties:**
- **Guaranteed discovery**: Any node can find any other node in O(log n) hops
- **Full galaxy awareness**: Every node eventually learns about every other node over time
- **Organic clustering**: New systems spawn near a sponsor system with an available connection, forming natural clusters
- **Scalable**: Designed for tens of thousands of nodes

## Features

- **Unique Identity**: Your UUID is generated from hardware fingerprint
- **Multi-Star Systems**: Single (50%), Binary (40%), and Trinary (10%) systems
- **Star Classification**: Realistic distribution (O, B, A, F, G, K, M classes) based on real world ratios
- **Peer Capacity**: Star class determines max connections (M-class: 5, scaling to an O-class: 12+)
- **Spatial Clustering**: New nodes spawn 100-500 units from their initial sponsor
- **DHT Routing**: Kademlia-style k-bucket routing with XOR distance metric
- **Cryptographic Identity**: Ed25519 keypairs for authentication
- **Attestation System**: Signed proofs of every peer interaction
- **Stellar Credits**: Earn credits for uptime with bonuses for network contribution
- **Web Interface**: A simple dashboard with galaxy map visualization
- **Persistent Storage**: SQLite with automatic compaction

## Stellar Credits

Nodes earn Stellar Credits based on verified uptime, with bonuses that reward healthy network participation.

### Base Rate
- **1 credit per hour** of verified uptime
- Normalized across all star types (an M-class earns the same as an O-class)

### Bonuses

| Bonus | Max | Description |
|-------|-----|-------------|
| **Bridge** | +50% | Being critical for network connectivity (peers with few connections depend on you) |
| **Longevity** | +52% | +1% per week of continuous uptime (resets after 30-min gap) |
| **Pioneer** | +30% | Participating when the network is small (<20 nodes) |
| **Reciprocity** | +5% | Healthy bidirectional relationships with peers |

### Grace Periods
- **15 minutes**: Short gaps (restarts, updates) don't affect credits earnings for that hour
- **30 minutes**: Gaps below this will not reset your longevity streak

### Ranks

| Rank | Credits | Approximate Time |
|------|---------|------------------|
| Unranked | 0 | New node |
| Bronze | 168 | ~1 week |
| Silver | 720 | ~1 month |
| Gold | 2,160 | ~3 months |
| Platinum | 4,320 | ~6 months |
| Diamond | 8,640 | ~1 year |

## Quick Start

### Docker

The recomended method is to spin up a docker container. A sample compose file is included to assist with this.

## Building from source, or providing Development support

### Prerequisites

- Go 1.18+
- GCC (for SQLite CGO)
  - **macOS**: `xcode-select --install`
  - **Linux**: `sudo apt-get install build-essential`
  - **Windows**: MinGW or WSL

### Build

```bash
git clone https://github.com/sargonas/stellar-lab.git
cd stellar-lab
go mod tidy
go build -o stellar-lab
```

### Run Your First Node

```bash
./stellar-lab -name "Sol" -public-address "your-domain.com:7867"
```

Visit http://localhost:8080 for the web interface.

### Join the Network

Nodes discover the network automatically via seeder nodes:

Or you can manually bootstrap from a specific peer:

```bash
./stellar-lab -name "Alpha Centauri" -public-address "my-server.com:7867" -bootstrap "192.168.1.100:7867"
```

### Multi-Node Local Testing

When testing locally, you can run multiple nodes from one install base by specifying unique database files, ports, and custom seeds.
```bash
# Terminal 1 - Seed node
./stellar-lab -name "Sol" -seed "sol" -public-address "localhost:7867" -db "sol.db"

# Terminal 2
./stellar-lab -name "Alpha" -seed "alpha" \
  -public-address "localhost:7868" -address "0.0.0.0:8081" -db "alpha.db"

# Terminal 3
./stellar-lab -name "Beta" -seed "beta" \
  -public-address "localhost:7869" -address "0.0.0.0:8082" -db "beta.db"
```

## Configuration

All settings can be configured via command-line flags or environment variables. CLI flags take precedence over environment variables.

### Command-Line Flags

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `-name` | `STELLAR_NAME` | (required) | Name of your star system |
| `-public-address` | `STELLAR_PUBLIC_ADDRESS` | (required) | Public address for peer connections (host:port) |
| `-seed` | `STELLAR_SEED` | (random) | Seed for deterministic UUID generation in development |
| `-address` | `STELLAR_ADDRESS` | `0.0.0.0:8080` | Web UI bind address (host:port) |
| `-db` | `STELLAR_DB` | `/data/stellar-lab.db` | SQLite database file path |
| `-bootstrap` | `STELLAR_BOOTSTRAP` | | Specific peer to bootstrap from |

## Architecture

### DHT Design

Stellar Lab uses a Kademlia-inspired Distributed Hash Table:

- **128-bit Node IDs**: Derived from system UUIDs via SHA-256
- **XOR Distance Metric**: Determines "closeness" in the network
- **K-Buckets**: 128 buckets, capacity varies by star class
- **Iterative Lookups**: Parallel queries for O(log n) discovery

### Dual-Port Design

Each node runs two HTTP servers:
- **Web UI** (default :8080): Dashboard and JSON APIs for users
- **DHT Protocol** (default :7867): Node-to-node DHT communication

### DHT Operations

| Operation | Description |
|-----------|-------------|
| `PING` | Liveness check, exchange system info |
| `FIND_NODE` | Request K closest nodes to a target ID |
| `ANNOUNCE` | Register presence with closest nodes |

### Maintenance Loops

| Loop | Interval | Purpose |
|------|----------|---------|
| Announce | 30 min | Re-announce to K closest nodes |
| Refresh | 60 min | Refresh stale k-buckets |
| Liveness | 5 min | Ping routing table peers |
| Cache Prune | 6 hours | Remove stale cache entries |
| Compaction | Daily 3 AM | Aggregate attestations older than 14 days |
| Credits | 1 hour | Calculate and store earned credits |

### Star Types & Peer Capacity

Star class determines maximum routing table connections:

| Class | Type | Distribution | Base Peers |
|-------|------|--------------|------------|
| X | Supermassive Black Hole | Unique | 20 |
| O | Blue Supergiant | 0.003% | 12 |
| B | Blue Giant | 0.13% | 10 |
| A | White Star | 0.6% | 9 |
| F | Yellow-White | 3% | 8 |
| G | Yellow Dwarf | 8% | 7 |
| K | Orange Dwarf | 12% | 6 |
| M | Red Dwarf | 76% | 5 |

**Multi-star bonuses:** Binary +3, Trinary +5

**Note:** The X-class Supermassive Black Hole exists only at the galactic core (0,0,0) and serves as the genesis seed node.

### Spatial Coordinates

- **First node**: Deterministic from UUID (range: -10,000 to +10,000)
- **Joining nodes**: Cluster 100-500 units from their bootstrap sponsor
- Creates organic galaxy growth with natural clustering

## API Endpoints

### Web UI Server (:8080)

```bash
GET /                      # Web dashboard
GET /api/system            # Local system info
GET /api/peers             # Routing table peers
GET /api/known-systems     # All cached systems
GET /api/stats             # Network statistics
GET /api/credits           # Credit balance and rank info
```

### Credits API Response

```json
{
  "system_id": "40585bf2-5ccc-50fb-8da6-4f2e0135d5f7",
  "balance": 168,
  "total_earned": 168,
  "total_sent": 0,
  "total_received": 0,
  "rank": "Bronze",
  "rank_color": "#cd7f32",
  "next_rank": "Silver",
  "credits_to_next": 552,
  "longevity_weeks": 1.2,
  "longevity_bonus": 0.012,
  "last_updated": 1736121600
}
```

### DHT Protocol Server (:7867)

```bash
GET /api/discovery         # Bootstrap discovery info
POST /dht                  # DHT message handler
GET /system                # System info for peers
```

## Web Interface

The dashboard displays:

- **System Info**: Name, UUID, star classification, coordinates
- **DHT Statistics**: Routing table size, cache size, active buckets
- **Stellar Credits**: Balance, rank, and progress to next rank
- **Health Status**: Connectivity indicator (Healthy/Warning/Isolated)
- **Peer List**: Connected systems with coordinates and star types
- **Galaxy Map**: Interactive 2D visualization
  - Drag to pan
  - Scroll to zoom
  - Hover for system details
  - Your system highlighted in blue

## Network Discovery

### Seed Nodes

On startup, nodes fetch the seed list from GitHub:
```
https://raw.githubusercontent.com/sargonas/stellar-lab/main/SEED-NODES.txt
```

### Bootstrap Flow

1. Check for cached peers from previous session → ping and rejoin
2. Try `-bootstrap` peer if specified
3. Fetch and try seed nodes from GitHub
4. Perform self-lookup to populate routing table
5. Announce to K closest nodes
6. Update coordinates near sponsor (if new node at origin)

### Reconnection After Restart

- Cached peers are pinged immediately on startup
- Other nodes' liveness checks (every 5 min) rediscover you
- Full connectivity typically restored within 5 minutes

## File Structure

```
├── main.go              # Entry point, CLI flags
├── dht.go               # Core DHT coordinator
├── dht_messages.go      # PING, FIND_NODE, ANNOUNCE
├── dht_lookup.go        # Iterative lookup algorithm
├── dht_maintenance.go   # Background maintenance loops
├── routing_table.go     # K-bucket implementation
├── bootstrap.go         # Network join logic
├── system.go            # Star system model
├── attestation.go       # Cryptographic attestations
├── credits.go           # Stellar credits system
├── storage.go           # SQLite persistence & compaction
├── hardware.go          # Hardware fingerprinting
├── web-interface.go     # Web UI and APIs
├── seeds.go             # Seed node fetching
├── version.go           # Protocol versioning
├── SEED-NODES.txt       # Default seed nodes
└── web/
    └── index.html       # Fallback web template
```

## Database

### Tables

- `system` - Local node identity and keypair
- `peer_systems` - Cached remote system info
- `attestations` - Recent signed interaction proofs
- `attestation_summaries` - Compacted historical aggregates
- `credit_balance` - Stellar credits balance and streak tracking
- `credit_transfers` - Transfer history (for future use)

### Attestation Compaction

Runs daily at 3 AM:
- Aggregates attestations older than 14 days into weekly summaries
- Preserves: peer ID, direction, message counts, sample signature
- Prevents unbounded database growth

## Troubleshooting

### Node stays isolated

```bash
# Check if seed nodes are reachable
curl http://localhost:7867/api/discovery

# Verify SEED-NODES.txt is reachable
curl https://raw.githubusercontent.com/sargonas/stellar-lab/main/SEED-NODES.txt

# Try explicit bootstrap
./stellar-lab -name "Test" -bootstrap "known-peer:7867"
```

### Port conflicts

```bash
lsof -i :8080
lsof -i :7867

# Use different ports
./stellar-lab -name "Test" -address "0.0.0.0:8090" -peer-port "7877"
```

### All systems at (0,0,0)

Coordinates are assigned after successful bootstrap. If your node started isolated, coordinates remain at origin. Restart it once seed nodes are reachable.

### Database errors

```bash
# Reset and start fresh
rm stellar-lab.db
./stellar-lab -name "Sol"
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Submit a pull request

To add your node as a seed:
1. Ensure stable uptime and connectivity
2. Add your peer address to `SEED-NODES.txt`
3. Submit a PR

## Version

**v1.0.0** - Initial release

## License

MIT