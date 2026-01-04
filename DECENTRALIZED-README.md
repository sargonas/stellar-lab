# Stellar Mesh - Decentralized Version

## Critical Changes

### ✅ Reputation is Now Decentralized & Trustless

**Before**: Each node self-reported reputation (easily faked)
**Now**: Peers cryptographically sign attestations about your contributions

- Ed25519 signatures
- Verifiable by anyone
- Expiring proofs (must be renewed)
- Cannot self-attest

### ✅ Planetary Systems Added

Every star system has deterministic planets:
- 0-9 planets depending on star type
- Habitable planets in goldilocks zone
- Moons, temperatures, masses all deterministic from UUID

### ✅ Minimal Web Interface

Dead-simple UI at http://localhost:8080
- Shows system info, planets, reputation, peers
- Terminal aesthetic, no JavaScript
- Works on any browser

## How Decentralized Reputation Works

1. **Uptime Challenges**: Peers send random challenges, you sign them, they attest
2. **Bridge Detection**: Multiple peers confirm you're a critical connector
3. **Relay Attestations**: Peers attest when you relay messages

All attestations are signed with Ed25519 private keys and verifiable by public keys.

## Quick Start

```bash
go build
./stellar-mesh -name "Sol" -address "localhost:8080"

# Open browser: http://localhost:8080
```

## File Structure

- `decentralized-reputation.go` - Cryptographic attestations
- `planets.go` - Deterministic planetary generation  
- `web-interface.go` - Minimal web UI
- (all other files updated)

Total: ~2,500 lines of Go
