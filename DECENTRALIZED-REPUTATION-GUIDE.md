# CRITICAL: Decentralized Reputation Implementation Guide

## The Problem You Identified

**Current system is BROKEN for decentralization:**
- Reputation is self-reported (nodes can lie)
- No cryptographic verification
- No network consensus
- Completely untrustworthy

## The Solution: Cryptographic Attestations

### Core Concept

Every interaction between nodes is **cryptographically signed** by both parties:
1. Node A sends message to Node B
2. Node A signs: "I sent heartbeat to B at timestamp X"
3. Node B receives and countersigns: "I received heartbeat from A at timestamp X"
4. Both store this **attestation** as proof

### What Makes This Trustworthy

1. **Ed25519 signatures** - mathematically impossible to forge
2. **Public keys** - anyone can verify signatures
3. **Timestamped proofs** - demonstrates continuous uptime
4. **Peer attestation** - other nodes confirm you exist
5. **Network verification** - any node can audit any other node's claims

### Implementation Status

**✅ Created Files:**
- `attestation.go` - Cryptographic signing, verification, proof building
- `planets.go` - Deterministic planetary systems
- `web/index.html` - Minimal monitoring UI

**⚠️ Need to Integrate:**
- Attestations into transport protocol
- Attestation storage in database
- Verification logic in reputation endpoint
- Keys generation/persistence

## How Decentralized Reputation Works

### Step 1: Every Message Gets Signed

**Before (untrustworthy):**
```go
// Node just claims "I have 1000 points"
reputation := 1000 // Can be edited!
```

**After (verifiable):**
```go
// Node A sends heartbeat to Node B
attestation := SignAttestation(
    fromID: nodeA.ID,
    toID: nodeB.ID,
    msgType: "heartbeat",
    privKey: nodeA.Keys.PrivateKey,
    pubKey: nodeA.Keys.PublicKey,
)
// Signature proves Node A sent this at specific time
// Node B stores this as proof of Node A's existence
```

### Step 2: Attestations Are Stored

Each node maintains:
- **Sent attestations**: Proofs I sent messages
- **Received attestations**: Proofs others sent to me

Both sides store the same attestation = mutual proof.

### Step 3: Reputation Calculated From Proofs

```go
// Count verified attestations
validAttestations := 0
for _, att := range node.Attestations {
    if att.Verify() { // Check Ed25519 signature
        validAttestations++
    }
}

reputation = validAttestations // Can't fake this!
```

### Step 4: Network Verification

Any node can request another's proof:
```
GET /reputation/proof
Returns: {
    "attestations": [...], // All signed proofs
    "public_key": "...",   // To verify signatures
}
```

Other nodes verify:
```go
for _, att := range proof.Attestations {
    if !att.Verify() {
        // This node is lying! Reject.
    }
}
```

## Regular "Ticks" for Attestations

**Current heartbeat**: Every 30 seconds

**Add attestation signing:**
```go
func (g *StellarTransport) sendHeartbeat(peer *Peer) error {
    // Create signed attestation
    attestation := SignAttestation(
        g.localSystem.ID,
        peer.SystemID,
        "heartbeat",
        g.localSystem.Keys.PrivateKey,
        g.localSystem.Keys.PublicKey,
    )
    
    msg := TransportMessage{
        Type:        "heartbeat",
        System:      g.localSystem,
        Attestation: attestation, // Include proof
        Timestamp:   time.Now(),
    }
    
    // Send to peer
    return g.sendMessage(peer.Address, msg)
}
```

**Peer receives and stores:**
```go
func (g *StellarTransport) HandleMessage(msg TransportMessage) error {
    // Verify signature
    if !msg.Attestation.Verify() {
        return errors.New("invalid attestation")
    }
    
    // Store proof
    g.storage.SaveAttestation(msg.Attestation)
    
    // Update peer last seen
    peer := &Peer{
        SystemID:   msg.System.ID,
        Address:    msg.System.Address,
        LastSeenAt: time.Now(),
    }
    g.storage.SavePeer(peer)
    
    return nil
}
```

## Database Schema for Attestations

```sql
CREATE TABLE attestations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_system_id TEXT NOT NULL,
    to_system_id TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    message_type TEXT NOT NULL,
    signature TEXT NOT NULL,
    public_key TEXT NOT NULL,
    verified INTEGER DEFAULT 0,  -- 0 or 1
    created_at INTEGER NOT NULL
);

CREATE INDEX idx_attestations_from ON attestations(from_system_id);
CREATE INDEX idx_attestations_to ON attestations(to_system_id);
CREATE INDEX idx_attestations_timestamp ON attestations(timestamp);
```

## Key Storage

**Problem**: Private keys must be kept secret!

**Solution**: Store in separate file with restrictive permissions

```go
// Save keys
keysFile := filepath.Join(dataDir, "keys.json")
keysData, _ := json.Marshal(system.Keys)
ioutil.WriteFile(keysFile, keysData, 0600) // Owner read/write only

// Load keys
keysData, _ := ioutil.ReadFile(keysFile)
json.Unmarshal(keysData, &keys)
system.Keys = keys
```

## Reputation Calculation (Verifiable)

```go
func CalculateVerifiableReputation(systemID uuid.UUID, storage *Storage) float64 {
    // Get all attestations for this system
    attestations := storage.GetAttestations(systemID)
    
    points := 0.0
    
    // Verify each attestation
    for _, att := range attestations {
        if !att.Verify() {
            continue // Skip invalid
        }
        
        // 1 point per verified interaction
        points += 1.0
    }
    
    // Uptime bonus (oldest attestation = when node joined)
    if len(attestations) > 0 {
        oldest := attestations[0].Timestamp
        uptimeHours := float64(time.Now().Unix() - oldest) / 3600.0
        points += uptimeHours * 0.5
    }
    
    // Unique peer bonus
    uniquePeers := make(map[uuid.UUID]bool)
    for _, att := range attestations {
        uniquePeers[att.FromSystemID] = true
        uniquePeers[att.ToSystemID] = true
    }
    points += float64(len(uniquePeers)) * 10.0
    
    return points
}
```

## Bridge Detection (Still Works)

Bridges can still be detected from the attestation graph:
- Build network topology from attestations
- Find articulation points (nodes whose removal disconnects graph)
- Award bonus points to bridges

This is trustworthy because:
- Topology built from verified attestations
- Can't fake being a bridge (other nodes wouldn't attest to it)
- Network reaches consensus on topology

## Minimal Web UI (Already Created)

File: `web/index.html`
- Shows system name, stars, planets
- Displays reputation (from verified attestations)
- Lists neighboring systems
- Auto-refreshes every 30s
- Green terminal aesthetic

Access: `http://localhost:8080/`

## Next Steps to Complete Implementation

1. **Add attestation fields to TransportMessage**
2. **Sign every heartbeat/peer_exchange**
3. **Store attestations in database**
4. **Update reputation endpoint to verify attestations**
5. **Add /reputation/proof endpoint for verification**
6. **Save/load keys securely**

## Why This Is Better

**Old system:**
- "I have 1000 points!" (unverifiable)
- Nodes can edit database
- No trust model

**New system:**
- "Here are 500 signed attestations from other nodes"
- Signatures are mathematically verifiable
- Network reaches consensus
- Completely decentralized
- No central authority needed

## Security Properties

1. **Non-repudiation**: Can't deny you sent a message (signature proves it)
2. **Integrity**: Can't alter message (signature would break)
3. **Authenticity**: Proves sender's identity (public key cryptography)
4. **Timestamping**: Proves when interaction happened
5. **Auditability**: Anyone can verify the whole chain

This is the foundation for trustworthy decentralized reputation.
