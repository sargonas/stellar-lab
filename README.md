# Stellar Mesh

A decentralized peer-to-peer mesh network where each node represents a star system in a shared galaxy. Nodes discover each other organically, exchange cryptographically signed attestations to prove participation, and build verifiable reputation over time.

## Overview

Stellar Mesh creates a virtual galaxy where:
- Each node is a **star system** with unique characteristics (star type, coordinates, binary/trinary composition)
- Nodes connect to **peers** and exchange heartbeats to prove liveness
- All interactions are secured with **Ed25519 cryptographic signatures**
- Participation builds **verifiable reputation** that can be independently validated
- The network topology is visualized as an interactive **3D galactic map**

## Features

### Identity & Generation
- **Unique Identity**: Each node gets a UUID (hardware-based, seed-based, or random)
- **Cryptographic Keys**: Ed25519 keypair for signing all network messages
- **Multi-Star Systems**: Single (50%), Binary (40%), and trinary (10%) star systems with realistic composition
- **Star Classification**: Deterministic star types (O, B, A, F, G, K, M) matching real galaxy distribution
- **Spatial Clustering**: New nodes spawn 100-500 units from their sponsor, creating organic galaxy growth

### Network Protocol
- **Stellar Transport Protocol**: Automatic peer discovery and mesh maintenance
- **Cryptographic Attestations**: Every message includes a signed proof of interaction
- **Protocol Versioning**: Semantic versioning with compatibility negotiation
- **Variable Peer Limits**: Star type determines max connections (M-class: 5 ranging to O-class: 12, +3 for binary, +5 for trinary)
- **Attestation-Rate Normalization**: Larger hub nodes don't earn faster than small nodes

### Reputation System
- **Verifiable Reputation**: Score based on cryptographic attestation count
- **Rank Progression**: Unranked → Bronze → Silver → Gold → Platinum → Diamond
- **Independent Verification**: Any node can verify another's reputation proof
- **No Central Authority**: Reputation is computed from attestations, not assigned

### Visualization
- **Web Dashboard**: Real-time system info, peer list, and reputation display
- **2D Galaxy Map**: SVG-based map with multiple projection views (X-Y, X-Z, Y-Z)
- **3D Galaxy Map**: Interactive Three.js visualization with orbit controls
- **Topology View**: See actual mesh connections inferred from attestations

## Quick Start

### Prerequisites

- Go 1.21 or higher
- GCC (for SQLite CGO compilation)
  - **macOS**: `xcode-select --install`
  - **Linux**: `sudo apt-get install build-essential`
  - **Windows**: Install MinGW or use WSL

### Build

```bash
git clone https://github.com/your-org/stellar-mesh
cd stellar-mesh
go mod tidy
go build -o stellar-mesh
```

### Run Your First Node

```bash
./stellar-mesh -name "Sol" -port 8080 -peer-port 7867
```

Visit http://localhost:8080 to see the web interface.

### Join the Network

**Automatic Discovery (recommended):**
```bash
./stellar-mesh -name "Alpha Centauri" -port 8081 -peer-port 7868
```

The node automatically discovers peers via seed nodes listed in `SEED-NODES.txt`.

**Manual Bootstrap:**
```bash
./stellar-mesh -name "Alpha Centauri" -port 8081 -peer-port 7868 -bootstrap "localhost:7867"
```

### Multi-Node Local Testing

```bash
# Terminal 1 - Seed node
./stellar-mesh -name "Sol" -port 8080 -peer-port 7867 -db sol.db

# Terminal 2
./stellar-mesh -name "Alpha" -port 8081 -peer-port 7868 -db alpha.db -bootstrap "localhost:7867"

# Terminal 3
./stellar-mesh -name "Beta" -port 8082 -peer-port 7869 -db beta.db -bootstrap "localhost:7867"
```

### Docker

```bash
docker-compose up -d

# Access nodes at:
# http://localhost:8080 (node1)
# http://localhost:8081 (node2)
# http://localhost:8082 (node3)
```

## Command-Line Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-name` | Name of your star system | Required |
| `-port` | HTTP API port | 8080 |
| `-peer-port` | Peer mesh protocol port | 7867 |
| `-db` | SQLite database path | stellar-mesh.db |
| `-bootstrap` | Peer address to bootstrap from | (uses seed nodes) |
| `-seed` | Custom seed for deterministic UUID | (random) |

## Architecture

### Dual-Port Design

Each node runs two servers:
- **API Server** (default 8080): Web interface and JSON API for users
- **Peer Server** (default 7867): Stellar Transport Protocol for node-to-node communication

### Components

```
stellar-mesh/
├── main.go              # Entry point, CLI flags, bootstrap logic
├── system.go            # Star system model, coordinate generation, GetMaxPeers()
├── stellar-transport.go # Peer protocol, heartbeats, peer exchange
├── storage.go           # SQLite persistence, attestation storage, compaction
├── attestation.go       # Cryptographic signing/verification, reputation calculation
├── api.go               # HTTP API endpoints
├── web-interface.go     # HTML templates for dashboard and map
├── version.go           # Protocol versioning and compatibility
└── hardware.go          # Hardware-based UUID generation
```

### Network Protocol

**Message Types:**
- `heartbeat`: Periodic liveness proof (every 30 seconds to 3 random peers)
- `peer_exchange`: Share peer lists (every 60 seconds)

**All messages include:**
- Sender's system info
- Protocol version
- Cryptographic attestation (Ed25519 signature)

**Attestation Structure:**
```json
{
  "from_system_id": "uuid",
  "to_system_id": "uuid",
  "message_type": "heartbeat",
  "timestamp": 1704412800,
  "signature": "base64...",
  "public_key": "base64..."
}
```

### Peer Limits by Star Type

Larger/rarer stars can maintain more connections, acting as natural network hubs:

| Star Class | Base Peers | With Binary (+3) | With Trinary (+5) |
|------------|------------|------------------|-------------------|
| M (Red Dwarf) | 5 | 8 | 10 |
| K (Orange) | 6 | 9 | 11 |
| G (Yellow/Sol) | 7 | 10 | 12 |
| F (Yellow-White) | 8 | 11 | 13 |
| A (White) | 9 | 12 | 14 |
| B (Blue Giant) | 10 | 13 | 15 |
| O (Blue Supergiant) | 12 | 15 | 17 |

Note: Attestation rate is normalized - all nodes earn reputation at the same rate regardless of peer count.

### Reputation & Ranks

Reputation is calculated from verified attestation count:

| Rank | Required Attestations | Approximate Time |
|------|----------------------|------------------|
| Unranked | 0 | - |
| Bronze | 1,000 | ~6 hours |
| Silver | 10,000 | ~2.5 days |
| Gold | 50,000 | ~12 days |
| Platinum | 150,000 | ~5 weeks |
| Diamond | 500,000 | ~4 months |

Reputation is **verifiable** - any node can validate another's proof by checking attestation signatures.

## API Reference

### System Information

```bash
# Get system info
curl http://localhost:8080/api/system

# Get network statistics
curl http://localhost:8080/api/stats

# Get version and compatibility info
curl http://localhost:8080/api/version
```

### Peers

```bash
# List connected peers
curl http://localhost:8080/api/peers

# Manually add a peer
curl -X POST http://localhost:8080/peers/add \
  -H "Content-Type: application/json" \
  -d '{"address": "localhost:7868"}'
```

### Reputation

```bash
# Get this node's reputation with cryptographic proof
curl http://localhost:8080/api/reputation

# Verify another node's reputation proof
curl -X POST http://localhost:8080/api/reputation/verify \
  -H "Content-Type: application/json" \
  -d '{"proof": {...}}'
```

### Map & Topology

```bash
# Get all known systems and connections for visualization
curl http://localhost:8080/api/map

# Get network topology inferred from recent attestations
curl http://localhost:8080/api/topology
```

### Database

```bash
# Get database statistics (table sizes, attestation counts)
curl http://localhost:8080/api/database/stats
```

### Health

```bash
curl http://localhost:8080/health
```

## Web Interface

### Dashboard (/)

The main dashboard shows:
- System identity (name, ID, coordinates)
- Star composition (class, temperature, luminosity)
- Current rank and reputation score
- Connected peers with their star info
- Network statistics

### Galactic Map (/map)

Interactive visualization featuring:
- **2D View**: SVG projection with view toggle (X-Y, X-Z, Y-Z planes)
- **3D View**: Three.js rendering with orbit controls
- **Topology**: Connection lines based on actual attestation data
- Stars colored by spectral class
- Local system highlighted

## Database Management

### Attestation Compaction

Attestations are periodically summarized to prevent database bloat:
- Raw attestations older than 24 hours are aggregated into hourly summaries
- Summaries preserve: peer ID, direction, message type counts, sample signature
- Compaction runs automatically; can also be triggered manually

### Storage Tables

- `system`: Local node identity and keys
- `peers`: Known peer addresses and last-seen times
- `peer_systems`: Cached system info for map visualization
- `attestations`: Raw cryptographic proofs (recent)
- `attestation_summaries`: Compacted historical data

## Star Classification

Stars follow realistic galactic distribution:

| Class | Probability | Description | Temperature | Example |
|-------|-------------|-------------|-------------|---------|
| O | 0.003% | Blue Supergiant | 30,000-50,000K | Extremely rare |
| B | 0.13% | Blue Giant | 10,000-20,000K | Very rare |
| A | 0.6% | White Star | 7,500-10,000K | Rare |
| F | 3% | Yellow-White | 6,000-7,500K | Uncommon |
| G | 8% | Yellow Dwarf | 5,200-6,000K | Like our Sun |
| K | 12% | Orange Dwarf | 3,700-5,200K | Common |
| M | 76% | Red Dwarf | 2,400-3,700K | Most common |

Multi-star systems:
- 50% single star
- 40% binary (two stars)
- 10% trinary (three stars)

## Security Model

### Threat Mitigations

- **Sybil Attacks**: Rate-limited by real-time participation requirement
- **Replay Attacks**: Timestamps and unique message signatures
- **Impersonation**: Ed25519 signatures tied to system identity
- **Attestation Forgery**: Cryptographic verification of all proofs

## Seed Nodes

Public seed nodes help new nodes discover the network. See [SEED-NODES.md](SEED-NODES.md) for:
- Current seed node list
- How to run your own seed node
- How to submit your node to the seed list

## Troubleshooting

### Build Errors

**"missing go.sum entry"**
```bash
go mod tidy && go build -o stellar-mesh
```

**"gcc: command not found"**
```bash
# macOS
xcode-select --install

# Ubuntu/Debian
sudo apt-get install build-essential
```

### Connection Issues

**"rejected: at max capacity"**
- The peer has reached its connection limit
- Try connecting to a different peer
- Network will route you to available nodes automatically

**Nodes not discovering each other**
```bash
# Check seed nodes are reachable
curl http://seed-node-address:7867/api/discovery

# Verify firewall allows peer port
sudo ufw allow 7867/tcp
```

### Database Issues

**"database locked"**
- Only one process can access a SQLite database
- Use different `-db` paths for multiple local nodes

**Database growing too large**
- Compaction runs automatically every 6 hours
- Check `/api/database/stats` for table sizes

## Contributing

1. Fork the repository
2. Create a feature branch
3. Submit a pull request

To add your node as a seed:
1. Ensure your node has high uptime and stable connectivity
2. Add your peer address to `SEED-NODES.txt`
3. Submit a PR

## License

MIT