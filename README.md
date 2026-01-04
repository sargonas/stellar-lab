# Stellar Mesh

A lightweight, distributed mesh network where each node represents a star system in a shared galaxy.

## Features

- **Unique Identity**: Each node gets a UUID (hardware-based, seed-based, or random)
- **Multi-Star Systems**: Binary (40%) and trinary (10%) star systems with realistic composition
- **Star Classification**: Each system has deterministic star types (O, B, A, F, G, K, M) with realistic distribution
- **Spatial Clustering**: New nodes cluster near existing ones (100-500 units away) for organic galaxy growth
- **Stellar Transport Protocol**: Automatic peer discovery and network maintenance (formerly "gossip")
- **Network Reputation**: Earn points for uptime and being a critical network connector
- **Low Resource**: Designed to run efficiently in Docker containers
- **HTTP API**: Query system info, peers, stats, and reputation
- **Persistent Storage**: SQLite for local data persistence
- **Future-Proof**: UUID-based deterministic seed for generating system features

## Quick Start

### Build

```bash
go mod download
go build -o stellar-mesh
```

### Run Your First Node

```bash
./stellar-mesh -name "Sol System" -address "localhost:8080"
```

### Run a Second Node (Connected to First)

```bash
./stellar-mesh -name "Alpha Centauri" -address "localhost:8081" -bootstrap "localhost:8080"
```

## Command-Line Flags

- `-name`: Name of your star system (required)
- `-address`: Host:port to bind the API server (default: localhost:8080)
- `-db`: Path to SQLite database file (default: stellar-mesh.db)
- `-bootstrap`: Address of a peer to connect to (optional)

## API Endpoints

### GET /system
Returns information about this node's star system.

```bash
curl http://localhost:8080/system
```

Response:
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "name": "Sol System",
  "x": 1234.56,
  "y": -2345.67,
  "z": 3456.78,
  "star_type": {
    "class": "G",
    "description": "Yellow Dwarf",
    "color": "#fff4ea",
    "temperature": 5778,
    "luminosity": 1.0
  },
  "created_at": "2025-01-04T12:00:00Z",
  "last_seen_at": "2025-01-04T12:05:00Z",
  "address": "localhost:8080"
}
```

### GET /peers
Returns list of known peer systems.

```bash
curl http://localhost:8080/peers
```

### GET /stats
Returns network statistics.

```bash
curl http://localhost:8080/stats
```

### POST /peers/add
Manually add a peer to the network.

```bash
curl -X POST http://localhost:8080/peers/add \
  -H "Content-Type: application/json" \
  -d '{"address": "localhost:8081"}'
```

### GET /health
Health check endpoint.

```bash
curl http://localhost:8080/health
```

## Docker

### Build Image

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN go build -o stellar-mesh

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/stellar-mesh .
EXPOSE 8080
ENTRYPOINT ["./stellar-mesh"]
```

Build:
```bash
docker build -t stellar-mesh .
```

### Run Container

```bash
docker run -d \
  -p 8080:8080 \
  -v stellar-data:/root \
  stellar-mesh \
  -name "My System" \
  -address "0.0.0.0:8080"
```

## Architecture

### Components

1. **System**: Represents a star system with UUID, name, 3D coordinates, and star classification
2. **Gossip**: Handles peer discovery and network maintenance
3. **Storage**: SQLite persistence for system info and peer list
4. **API**: HTTP interface for querying and interaction

### Star Types

Stars are classified using the Morgan-Keenan system with realistic distribution:

- **O Type** (0.003%) - Blue Supergiants - 30,000-50,000K - Extremely rare
- **B Type** (0.13%) - Blue Giants - 10,000-20,000K - Very rare  
- **A Type** (0.6%) - White Stars - 7,500-10,000K - Rare
- **F Type** (3%) - Yellow-White Stars - 6,000-7,500K - Uncommon
- **G Type** (8%) - Yellow Dwarfs - 5,200-6,000K - Like our Sun
- **K Type** (12%) - Orange Dwarfs - 3,700-5,200K - Common
- **M Type** (76%) - Red Dwarfs - 2,400-3,700K - Most common

Each star type is deterministically generated from the system's UUID and includes:
- Spectral class
- Description
- Color (hex code for visualization)
- Temperature (Kelvin)
- Luminosity (relative to Sol)

### Spatial Clustering

When a new node joins the network:

1. **With Bootstrap**: Fetches the bootstrap peer's coordinates and clusters 100-500 units away
2. **Without Bootstrap**: Uses deterministic coordinates from UUID (for first node)

This creates an organically growing galaxy where systems naturally group together rather than being randomly scattered.

### Gossip Protocol

- **Heartbeat**: Every 30 seconds to random peer
- **Peer Exchange**: Every 60 seconds with up to 3 random peers
- **Cleanup**: Every 5 minutes, removes peers not seen in 10 minutes

### Deterministic Features

The UUID can be used to deterministically generate any future system features:

```go
// Example: Generate a "star type" from UUID
seed := system.DeterministicSeed("star_type")
starType := seed % 10 // 0-9 representing different star classes
```

This ensures every node will always derive the same features from its UUID.

## Spatial Coordinates & Clustering

### Coordinate System
- 3D space with X, Y, Z axes
- Range: Approximately -10,000 to +10,000 units per axis (but can extend beyond based on clustering)
- Distance measured in arbitrary "galactic units"

### How Clustering Works

**First Node (Bootstrap)**:
- Generates deterministic coordinates from UUID
- Placed randomly in galactic coordinate space
- Acts as anchor point for network

**Subsequent Nodes**:
- Fetch coordinates from bootstrap peer
- Generate deterministic offset from UUID (100-500 units)
- Position themselves near the bootstrap system
- Creates organic, interconnected clusters

This means:
- Galaxy grows naturally as nodes join
- Systems stay relatively close together (easier to find peers)
- Network topology reflects spatial proximity
- Still deterministic (same UUID = same offset pattern)

## Future Expansion Ideas

- **System Features**: Star type, resources, habitability (all derived from UUID)
- **Trade Routes**: Connections based on spatial proximity
- **Factions**: Voluntary node grouping
- **Events**: Time-based occurrences in the galaxy
- **Resource Exchange**: Simple token/value transfer between systems

## Development

### Project Structure

```
stellar-mesh/
├── main.go       # Application entry point
├── system.go     # System model and coordinate generation
├── storage.go    # SQLite persistence layer
├── gossip.go     # Gossip protocol implementation
├── api.go        # HTTP API server
└── go.mod        # Go module definition
```

### Dependencies

- `github.com/google/uuid` - UUID generation
- `github.com/mattn/go-sqlite3` - SQLite driver
- `github.com/gorilla/mux` - HTTP router

## License

MIT
