# Stellar Mesh - Current State & Next Steps

## ✅ What's Been Built

### 1. Multi-Star Systems with Planets
- **Binary/Trinary stars** (50% single, 40% binary, 10% trinary)
- **Deterministic planetary systems** based on star type
- Planets have: type (rocky/gas/ice/lava), orbit, mass, habitability
- G-type stars can have 8 planets like our solar system
- Binary/trinary systems have fewer planets (unstable orbits)

### 2. Cryptographic Foundation
- **Ed25519 keypairs** for each node (`attestation.go`)
- **Signed attestations** for every peer interaction
- **Verification logic** - any node can verify another's claims
- Foundation for **trustless, decentralized reputation**

### 3. Semi-Deterministic UUIDs
- Hardware-based (MAC + hostname + machine-id)
- Seed-based (for testing/dev)
- Random (for maximum uniqueness)

### 4. Minimal Web UI
- Terminal aesthetic (black/green)
- Shows: system name, stars, planets, coordinates
- Displays: reputation, peers, network stats
- Auto-refreshes every 30s
- Located in `web/index.html`

## ⚠️ CRITICAL ISSUE YOU IDENTIFIED

**The reputation system is currently UNTRUSTWORTHY because:**
- Nodes self-report reputation (can lie)
- No cryptographic verification
- Not decentralized
- Anyone can edit their database and claim 1M points

**Your question: "are credits and reputation decentralized?"**
**Answer: NO - and that's a problem I need to fix.**

## 🔧 How to Fix It (Decentralized Reputation)

### The Solution: Cryptographic Attestations

**Every peer interaction gets signed by both parties:**

```
Node A ----heartbeat----> Node B
   |                         |
   |-- Signs attestation     |
   |   "I sent to B at T"    |
   |                         |
   |<----- Node B stores <---|
          Proof that A exists
```

**Both nodes store the signed attestation = mutual proof**

**Reputation = count of verified attestations (can't be faked)**

### Implementation Status

**✅ Created** (`attestation.go`):
- `SignAttestation()` - Sign peer interactions
- `Verify()` - Verify signatures
- `BuildAttestationProof()` - Collect all proofs
- `CalculateVerifiableReputation()` - Score from verified proofs

**❌ Need to Integrate**:
1. Add attestation signing to transport protocol
2. Store attestations in database
3. Update reputation endpoint to use verified attestations
4. Add network verification endpoint
5. Save/load keys securely

**📖 Full Guide**: See `DECENTRALIZED-REPUTATION-GUIDE.md`

## 🌐 Minimal Web UI Status

**What it shows:**
- System name (editable via flag)
- Reputation & rank
- Number of neighboring systems
- Total systems in galaxy (estimated)
- Star composition
- Planetary details
- Uptime

**Access**: `http://localhost:8080/`

**How barebones can we go?**

Current UI is already pretty minimal:
- Plain HTML (no JavaScript frameworks)
- Terminal aesthetic
- ~200 lines total
- Auto-refreshes data from API

**Could be even simpler:**
- Plain text output (no styling)
- Just data, no formatting
- But current version is already quite minimal

## 📊 How Reputation "Ticks" Work

### Current System (Wrong)

Reputation calculated on-demand when you hit `/reputation`
- Not verified
- Not trustworthy
- Can be edited

### Correct System (With Attestations)

**Every 30 seconds** (existing heartbeat):
1. Node A sends heartbeat to random peer
2. **NEW**: Includes signed attestation
3. Peer B receives and verifies signature
4. Peer B stores attestation as proof
5. Both nodes now have proof of interaction

**Reputation calculated from**:
- Count of verified attestations (1 point each)
- Uptime (based on oldest attestation)
- Unique peers (bonus points)
- Bridge status (bonus if critical connector)

**No central clock needed** - each node independently:
- Sends heartbeats every 30s
- Stores received attestations
- Calculates reputation from proofs
- Network reaches consensus

## 🪐 Planetary Systems

**How it works:**
```
G-type star (like Sun)
  ├─ Sol I (Rocky, 0.4 AU, NOT habitable - too close)
  ├─ Sol II (Rocky, 0.72 AU, NOT habitable - too hot)
  ├─ Sol III (Rocky, 1.0 AU, HABITABLE! - Earth-like)
  ├─ Sol IV (Rocky, 1.52 AU, NOT habitable - too cold)
  ├─ Sol V (Gas Giant, 5.2 AU)
  ├─ Sol VI (Gas Giant, 9.5 AU)
  ├─ Sol VII (Ice Giant, 19.2 AU)
  └─ Sol VIII (Ice Giant, 30.1 AU)
```

**Deterministic:**
- Same UUID = same planets every time
- Star type determines max planets
- Habitable zone based on star luminosity
- Binary/trinary = fewer planets

## 🔍 What Needs to Happen Next

### Priority 1: Fix Reputation (Decentralized)

**File**: `stellar-transport.go`
```go
// Add to sendHeartbeat()
attestation := SignAttestation(...)
msg.Attestation = attestation
```

**File**: `storage.go`
```sql
-- Add attestations table
CREATE TABLE attestations (...)
```

**File**: `api.go`
```go
// Update /reputation endpoint
attestations := storage.GetAttestations(systemID)
reputation := CalculateVerifiableReputation(attestations)
```

### Priority 2: Web UI for Name Setting

**Add to web UI:**
```html
<form action="/set-name" method="POST">
    <input name="system_name" placeholder="Enter system name">
    <button>SET NAME</button>
</form>
```

**Add to API:**
```go
api.router.HandleFunc("/set-name", api.setSystemName).Methods("POST")
```

### Priority 3: Key Persistence

**Save keys to file:**
```go
keysFile := "stellar-mesh-keys.json"
json.Marshal(system.Keys) -> save to file
chmod 0600 (owner only)
```

**Load on startup:**
```go
if fileExists(keysFile) {
    load keys
} else {
    generate new keys
}
```

## 📁 Files Overview

**Core System:**
- `system.go` - Star system, coordinates, stars, planets
- `planets.go` - Planetary generation
- `hardware.go` - Hardware ID for semi-deterministic UUIDs
- `attestation.go` - ✨ Cryptographic verification (NEW)

**Network:**
- `stellar-transport.go` - Peer discovery (needs attestation integration)
- `storage.go` - SQLite persistence (needs attestation table)
- `api.go` - HTTP API

**UI:**
- `web/index.html` - Minimal monitoring interface

**Documentation:**
- `DECENTRALIZED-REPUTATION-GUIDE.md` - ✨ How to fix reputation (NEW)
- `CHANGELOG-v2.md` - Update history
- `QUICK-REFERENCE.md` - Quick start guide

## 🎯 Summary

**You asked three critical questions:**

1. **"are credits and reputation decentralized?"**
   - NO - currently not trustworthy
   - SOLUTION: Cryptographic attestations (implemented in `attestation.go`, needs integration)

2. **"what's the most barebones HTTP interface?"**
   - ANSWER: `web/index.html` - plain HTML, terminal aesthetic, ~200 lines
   - Shows: name, reputation, peers, galaxy size, planets
   - Could be even simpler if needed

3. **"how are we handling reputation ticks?"**
   - CURRENT: Calculated on-demand (untrustworthy)
   - SHOULD BE: Attestations signed every 30s during heartbeat
   - Reputation = verified attestation count (trustworthy)

**Bottom line:**
- Multi-star systems ✅
- Planets ✅
- Web UI ✅
- Semi-deterministic UUIDs ✅
- Decentralized reputation ⚠️ (foundation built, needs integration)

The cryptographic foundation is there (`attestation.go`), it just needs to be wired into the transport protocol, database, and API.
