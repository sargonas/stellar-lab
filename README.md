# Stellar Lab

A silly program for homelabbers and others to run when you have a few spare megs of ram and storage. It is a decentralized peer-to-peer mesh network where each node represents a star system in a shared galaxy. Nodes discover each other organically, exchange cryptographically signed attestations to prove participation, and build verifiable reputation over time based on your time spent as part of the galaxy.

## Technical Overview

Stellar Lab creates a virtual galaxy where each participant runs a node representing their star system. Systems have procedurally generated identities—star types, binary/trinary compositions, and 3D coordinates—all derived deterministically from cryptographic seeds. Nodes build reputation through cryptographically signed attestations of their interactions with peers.

**Key Properties:**
- **Guaranteed discovery**: Any node can find any other node in O(log n) hops via DHT (Distributed Hash Table)
- **Full galaxy awareness**: Every node eventually learns about every other node over time
- **Organic clustering**: New systems spawn near their sponsor system, forming natural clusters
- **Scalable**: Designed for tens of thousands of nodes but *can* go higher easily
- **Tamper-resistant**: Identity binding, coordinate validation, and attestation verification prevent common spoofing attacks, though this isn't a blockchain and isn't meant to be one and there may be gaps in my design!

## Features

- **Unique Identity**: UUID generated from hardware fingerprint, cryptographically bound to your keypair
- **Multi-Star Systems**: Single (50%), Binary (40%), and Trinary (10%) system probabilities
- **Star Classification**: Semi-Realistic distribution (O, B, A, F, G, K, M classes) adjusted for practical network sizes
- **Peer Capacity**: Star class determines max connections (M-class: 10, scaling up to O-class: 18+)
- **Spatial Clustering**: New nodes spawn 100-500 units from their sponsor system
- **DHT Routing**: Kademlia-style k-bucket routing with XOR distance metric
- **Cryptographic Identity**: Ed25519 keypairs for authentication
- **Attestation System**: Signed proofs of every peer interaction
- **Stellar Credits**: Earn credits for uptime with bonuses for network contribution
- **Web Interface**: Dashboard with interactive galaxy map visualization
- **Persistent Storage**: SQLite database preserves identity across restarts

## Stellar Credits

Nodes earn Stellar Credits based on verified uptime, with bonuses that reward healthy network participation.

### Base Rate
- **1 credit per hour** of verified uptime
- Normalized across all star types (an M-class earns the same as an O-class despite more peers)

### Bonuses

| Bonus | Max | Description |
|-------|-----|-------------|
| **Bridge** | +50% | Being critical for network connectivity (peers depend on you to reach the galaxy) |
| **Longevity** | +52% | +1% per week of continuous uptime, capping at 1 year |
| **Pioneer** | +30% | Participating when the network is small (scales down as network grows past 20 nodes, reaches 0% at 100+) |
| **Reciprocity** | +5% | Healthy bidirectional relationships with peers |

### Grace Periods
- **15 minutes**: Short gaps (restarts, updates) don't affect credit earnings for that hour
- **30 minutes**: Gaps below this won't reset your longevity streak
- **60 minutes**: Gaps longer than this drop you from peer routing tables and map, but your position is preserved when you return, your UUID deterministically places you back where you belong

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

### Docker (Recommended)

```bash
docker run -d \
  --name stellar-lab \
  -p 8080:8080 \
  -p 7867:7867 \
  -v stellar-data:/data \
  -e STELLAR_NAME="YourSystemName" \
  -e STELLAR_PUBLIC_ADDRESS="your-domain.com:7867" \
  ghcr.io/sargonas/stellar-lab:latest
```

A sample `docker-compose.yml` is included in the repository.

**Important**: If running multiple nodes on the same host, each needs unique internal *AND* external ports:
```yaml
# Node 1
ports: ["8080:8080", "7867:7867"]
STELLAR_PUBLIC_ADDRESS: "your-domain.com:7867"

# Node 2  
ports: ["8081:8081", "7868:7868"]
STELLAR_PUBLIC_ADDRESS: "your-domain.com:7868"
```

## Building from Source

### Prerequisites

- Go 1.18+
- GCC (for SQLite CGO)
  - **macOS**: `xcode-select --install`
  - **Linux**: `sudo apt-get install build-essential`
  - **Windows**: Use WSL

### Building

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

Nodes discover the network automatically via seed nodes listed in `SEED-NODES.txt`, fetched from GitHub at startup.

You can also bootstrap from a specific peer instead (note: if specified and it fails, it will NOT fall back to the seed list, by design):

```bash
./stellar-lab -name "Alpha Centauri" -public-address "my-server.com:7867" -bootstrap "192.168.1.100:7867"
```

### Multi-Node Local Testing

```bash
# Terminal 1 - First node (becomes sponsor for others)
./stellar-lab -name "Sol" -seed "sol" -public-address "localhost:7867" -db "sol.db"

# Terminal 2
./stellar-lab -name "Alpha" -seed "alpha" \
  -public-address "localhost:7868" -address "0.0.0.0:8081" -db "alpha.db"

# Terminal 3
./stellar-lab -name "Beta" -seed "beta" \
  -public-address "localhost:7869" -address "0.0.0.0:8082" -db "beta.db"
```

## Configuration

All settings can be configured via command-line flags or environment variables. CLI flags take precedence.

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `-name` | `STELLAR_NAME` | (required) | Name of your star system |
| `-public-address` | `STELLAR_PUBLIC_ADDRESS` | (required) | Public address for peer connections (host:port) |
| `-seed` | `STELLAR_SEED` | (random) | Seed for deterministic UUID (development only) |
| `-address` | `STELLAR_ADDRESS` | `0.0.0.0:8080` | Web UI bind address |
| `-db` | `STELLAR_DB` | `/data/stellar-lab.db` | SQLite database path |
| `-bootstrap` | `STELLAR_BOOTSTRAP` | | Specific peer to bootstrap from |

## Architecture

### DHT Design

Stellar Lab uses a Kademlia-inspired Distributed Hash Table:

- **128-bit Node IDs**: Derived from system UUIDs via SHA-256
- **XOR Distance Metric**: Determines "closeness" in the network
- **K-Buckets**: 128 buckets with capacity varying by star class
- **Iterative Lookups**: Parallel queries for O(log n) discovery

### Dual-Port Design

Each node runs two HTTP servers:
- **Web UI** (default :8080): Dashboard and JSON APIs for users
- **DHT Protocol** (default :7867): Node-to-node communication

### DHT Operations

| Operation | Description |
|-----------|-------------|
| `PING` | Liveness check with system info exchange |
| `FIND_NODE` | Request K closest nodes to a target ID |
| `ANNOUNCE` | Register presence with closest nodes |

### Background Processes

| Process | Interval | Purpose |
|---------|----------|---------|
| Announce | 30 min | Re-announce to K closest nodes |
| Refresh | 60 min | Refresh stale routing buckets |
| Liveness | 5 min | Ping peers, evict unresponsive nodes |
| Cache Prune | 6 hours | Remove stale cache entries (>24h) |
| Compaction | Daily 3 AM | Aggregate old attestations into summaries |
| Credits | 1 hour | Calculate and award earned credits |

### Star Types & Peer Capacity

Star class determines maximum routing table connections:

| Class | Type | Distribution | Max Peers |
|-------|------|--------------|-----------|
| X | Supermassive Black Hole | Unique (Genesis only) | 20 |
| O | Blue Supergiant | 0.5% | 18 |
| B | Blue Giant | 2% | 16 |
| A | White Star | 5% |15 |
| F | Yellow-White | 10% | 14 |
| G | Yellow Dwarf | 17.5% | 12 |
| K | Orange Dwarf | 25% | 11 |
| M | Red Dwarf | 40% | 10 |

**Multi-star bonuses:** Binary systems +3 max peers, Trinary +5

**Note:** The X-class Supermassive Black Hole exists only at the galactic core (0,0,0) and serves as the genesis node.

### Spatial Coordinates

- **Genesis**: The galactic core at coordinates (0,0,0)
- **New nodes**: Assigned coordinates 100-500 units from their sponsor during bootstrap
- **Deterministic**: Position is derived from `Hash(YourUUID + SponsorUUID)`, making it permanent and verifiable

## API Endpoints

### Web UI Server (:8080)

| Endpoint | Description |
|----------|-------------|
| `GET /` | Web dashboard |
| `GET /api/system` | Local system info |
| `GET /api/peers` | Routing table peers |
| `GET /api/known-systems` | All cached systems |
| `GET /api/stats` | Network statistics |
| `GET /api/credits` | Credit balance and rank |
| `GET /api/connections` | Peer connection topology |

### DHT Protocol Server (:7867)

| Endpoint | Description |
|----------|-------------|
| `GET /api/discovery` | Bootstrap discovery info |
| `POST /dht` | DHT message handler |
| `GET /system` | System info for peers |

## Web Interface

The dashboard displays:

- **System Info**: Name, UUID, star classification, coordinates
- **DHT Statistics**: Routing table size, known systems, cache size
- **Stellar Credits**: Balance, rank, and progress to next rank
- **Health Status**: Connectivity indicator (Healthy/Warning/Isolated)
- **Peer List**: Connected systems with coordinates
- **Galaxy Map**: Interactive 2D visualization with connection lines
  - Drag to pan, scroll to zoom
  - Hover for system details
  - Your system highlighted in blue

## Database

### Tables

| Table | Purpose |
|-------|---------|
| `system` | Local node identity, keypair, coordinates |
| `peers` | Known peer addresses |
| `peer_systems` | Cached remote system info |
| `peer_connections` | Tracks peer relationships |
| `identity_bindings` | UUID to public key mapping (spoofing prevention) |
| `attestations` | Recent signed interaction proofs |
| `attestation_summaries` | Compacted historical data |
| `credit_balance` | Stellar credits and streak tracking |
| `credit_transfers` | Transfer history (future feature) |
| `verified_transfers` | Validated transfers (future feature) |

### Backup

Your identity lives in the database file. Back it up to preserve your UUID, keypair, coordinates, and credit balance across hardware changes.

## Troubleshooting

### Node stays isolated

```bash
# Check if seed nodes are reachable
curl http://localhost:7867/api/discovery

# Verify SEED-NODES.txt is accessible
curl https://raw.githubusercontent.com/sargonas/stellar-lab/main/SEED-NODES.txt

# Try explicit bootstrap
./stellar-lab -name "Test" -public-address "you.com:7867" -bootstrap "known-peer:7867"
```

### Port conflicts

```bash
lsof -i :8080
lsof -i :7867

# Use different ports
./stellar-lab -name "Test" -address "0.0.0.0:8090" -public-address "you.com:7877"
```

### Multiple nodes on same host

Each node needs unique ports for BOTH the web UI (-address) AND the DHT (-public-address). The internal port is extracted from your public address.

### Database errors

```bash
# Reset and start fresh (loses identity!)
rm stellar-lab.db
./stellar-lab -name "Sol" -public-address "you.com:7867"
```

### You made a deployment mistake and now have duplicate systems!

Oh no, you deployed your system with a bad name or other config error, fixed it, and re deployed it and now there are two on the map and in the tables? FEAR NOT! After about an hour or so the system's housekeeping will drop them off the tables, and after a day or so they will be gone entirely from maps!

## Contributing

1. Fork the repository
2. Create a feature branch
3. Submit a pull request

To add your node as a seed:
1. Ensure stable uptime and connectivity
2. Add your peer address to `SEED-NODES.txt`
3. Submit a PR

## To-Do

1. MUCH better Web UI. It should always be lightweight and simple, but it can be far better than what we have at the moment!
2. Improve the actual map on the Web UI
3. API improvements. Right now it is the same port as the WebUI. I want to either split more of the API between Web and protocol, so that people can keep the webUI and some endpoints behind a firewall and expose others, or make a completely separate API port with public and private endpoints
4. Beacon system. I would like to trigger a "beacon" once every 24 hours in a random system, and every system on the shortest path between the previous beacon and the new one gets a credit bonus.
5. Eventually, let people send and recieve stellar credits between systems, but this is WAY out there for now.

## License

MIT