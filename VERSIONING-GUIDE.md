# Protocol Versioning & Backwards Compatibility

## Overview

Stellar Mesh uses **semantic versioning** with graceful degradation to ensure:
1. Old nodes can still participate in the network
2. New features don't break existing functionality
3. Updates can roll out gradually without network disruption
4. Brief downtimes (updates, restarts) don't heavily penalize reputation

## Version Format

**Protocol Version**: `MAJOR.MINOR.PATCH`
- **MAJOR**: Breaking changes (incompatible with previous versions)
- **MINOR**: New features, backwards compatible
- **PATCH**: Bug fixes, backwards compatible

**Current Version**: `1.0.0`

## Compatibility Rules

### Major Version Compatibility
```
v1.x.x ✅ Compatible with v1.x.x
v1.x.x ❌ NOT compatible with v2.x.x
v2.x.x ❌ NOT compatible with v1.x.x
```

Major version changes indicate **breaking changes**. Nodes with different major versions cannot communicate.

### Minor/Patch Version Compatibility
```
v1.0.0 ✅ Compatible with v1.1.0 (newer supports older)
v1.1.0 ✅ Compatible with v1.0.0 (newer supports older)
v1.0.1 ✅ Compatible with v1.0.0 (patch compatible)
```

**Newer nodes MUST support older nodes** within the same major version.

## Feature Negotiation

### How It Works

Every message includes version info:
```json
{
  "type": "heartbeat",
  "version": {
    "protocol_version": "1.0.0",
    "application_version": "1.0.0",
    "supported_features": ["attestations", "planets", "multi_star"]
  },
  "system": {...},
  "attestation": {...}  // Optional - only if both support it
}
```

### Feature Flags

Current features:
- `attestations` - Cryptographic reputation proofs (v1.0.0+)
- `planetary_systems` - Deterministic planets (v1.0.0+)
- `multi_star_systems` - Binary/trinary stars (v1.0.0+)
- `legacy_gossip` - Original protocol (always supported)

### Negotiation Process

1. **Node A** sends message with version info
2. **Node B** receives, stores peer version
3. **Node B** checks: `IsCompatibleWith(peerVersion)`
4. If compatible: determines shared features
5. Future messages only use shared features

Example:
```go
// Node A (v1.1.0) talking to Node B (v1.0.0)
negotiation := NegotiateFeatures(v1.1.0, v1.0.0)

if negotiation.UseAttestations() {
    // Both support attestations - use them
    msg.Attestation = SignAttestation(...)
} else {
    // Older node doesn't support - skip attestations
    msg.Attestation = nil
}
```

## Backwards Compatibility Examples

### Example 1: Attestations (v1.0.0 feature)

**Scenario**: v1.0.0 node talks to v0.9.0 node (hypothetical older version)

```go
// v1.0.0 behavior:
if peerSupportsAttestations() {
    msg.Attestation = SignAttestation(...)  // Include proof
} else {
    msg.Attestation = nil  // Skip for compatibility
}

// v0.9.0 behavior:
// Ignores 'attestation' field (doesn't understand it)
// Processes rest of message normally
```

**Result**: Both nodes communicate successfully!

### Example 2: New Feature in v1.1.0 (Future)

**Scenario**: v1.1.0 adds "resource trading"

```go
// v1.1.0 code:
func sendHeartbeat() {
    msg := TransportMessage{...}
    
    if peerVersion.SupportsFeature("resource_trading") {
        msg.Resources = calculateResources()  // New field
    }
    // else: skip resources, peer won't understand
}

// v1.0.0 code:
// Receives message
// Ignores unknown 'resources' field
// Processes normally
```

**Result**: v1.1.0 and v1.0.0 nodes coexist peacefully!

## Update Rollout Strategy

### Gradual Rollout

**Day 1**: Release v1.1.0
- 10% of nodes update
- 90% still on v1.0.0
- Network functions normally (backwards compat)

**Day 7**: 
- 50% on v1.1.0
- 50% on v1.0.0
- New features work between updated nodes
- Mixed conversations gracefully degrade

**Day 30**:
- 95% on v1.1.0
- 5% on v1.0.0
- Old nodes still function fine
- Full feature adoption near complete

### No "Flag Day" Required

Unlike traditional protocols, there's **no cutover date** where everyone must update simultaneously. The network evolves organically.

## Downtime Tolerance

### The Problem

Without tolerance:
```
Node restarts for update (10 minutes down)
Misses 20 heartbeats
Loses 20 attestations
Reputation unfairly penalized
```

### The Solution

**Uptime Calculation with Tolerance**:

```go
// Expected attestations: ~120 per hour (2 per minute)
expectedPerHour := 120.0
totalExpected := hours * 120.0

// Actual uptime ratio
actualRatio := attestations / totalExpected

// Minimal penalty for 90%+ uptime
if actualRatio >= 0.90 {
    // 90-100% uptime = nearly full points
    multiplier = 0.90 + (actualRatio - 0.90) * 1.0
}

points = hours * 0.5 * multiplier
```

**Examples**:

| Uptime % | Hours | Expected Att. | Actual Att. | Penalty | Points |
|----------|-------|---------------|-------------|---------|--------|
| 100% | 24 | 2,880 | 2,880 | 0% | 12.0 |
| 95% | 24 | 2,880 | 2,736 | Minimal | 11.4 |
| 90% | 24 | 2,880 | 2,592 | Small | 10.8 |
| 80% | 24 | 2,880 | 2,304 | Moderate | 9.6 |

**Result**: Brief downtimes (90%+ uptime) barely affect reputation!

### Update Downtime Example

```
Node has 1000 hours uptime, 99.5% uptime ratio
  ├─ Shuts down for update
  ├─ 10 minutes offline
  ├─ Restarts
  └─ New uptime ratio: 99.48% (barely changed)

Reputation impact: ~0.02% loss
```

**Conclusion**: Updates don't significantly harm reputation.

## API Endpoints

### GET /version

Check node's version and peer compatibility:

```bash
curl http://localhost:8080/version
```

Response:
```json
{
  "protocol_version": "1.0.0",
  "application_version": "1.0.0",
  "supported_features": [
    "attestations",
    "planetary_systems",
    "multi_star_systems"
  ],
  "peer_compatibility": {
    "compatible_peers": 45,
    "incompatible_peers": 0,
    "unknown_peers": 5,
    "total_peers": 50
  }
}
```

### Checking Peer Versions

```bash
# Get your peers
curl http://localhost:8080/peers

# Each peer connection tracks version
# (stored internally, not exposed in basic peer list)
```

## Migration Guide

### For Node Operators

**Updating your node**:

```bash
# 1. Stop current node
docker stop stellar-mesh

# 2. Pull new version
docker pull stellar-mesh:1.1.0

# 3. Start with same config
docker start stellar-mesh

# Network continues to function!
# Attestations missed during update: minimal impact
# Old peers still communicate fine
```

**Rollback if needed**:
```bash
# Works! v1.0.0 still compatible with v1.1.0 network
docker run stellar-mesh:1.0.0 -name "Sol" ...
```

### For Developers

**Adding a new feature in v1.1.0**:

```go
// 1. Add feature flag
func (v ProtocolVersion) SupportsFeature(feature string) bool {
    switch feature {
    case "new_feature":
        return v.Minor >= 1  // v1.1.0+
    ...
    }
}

// 2. Use conditionally
if peerVersion.SupportsFeature("new_feature") {
    // Use new feature
} else {
    // Graceful fallback
}

// 3. Bump minor version
CurrentProtocolVersion = ProtocolVersion{
    Major: 1,
    Minor: 1,  // ← Incremented
    Patch: 0,
}
```

**Making a breaking change (v2.0.0)**:

```go
// 1. Only do this for MAJOR changes
// 2. Bump major version
CurrentProtocolVersion = ProtocolVersion{
    Major: 2,  // ← Breaking change
    Minor: 0,
    Patch: 0,
}

// 3. v1.x.x nodes will see incompatibility
// 4. Network splits temporarily during migration
// 5. Eventually all nodes upgrade to v2.0.0
```

**Avoid breaking changes when possible!**

## Testing Compatibility

### Test Mixed Versions

```bash
# Start v1.0.0 node
docker run stellar-mesh:1.0.0 -name "Old" -address ":8080"

# Start v1.1.0 node (hypothetical)
docker run stellar-mesh:1.1.0 -name "New" -address ":8081" -bootstrap "old:8080"

# Check compatibility
curl http://localhost:8081/version
# Should show: compatible_peers: 1

# Both nodes should communicate normally
```

### Verify Feature Negotiation

```bash
# Check logs
docker logs stellar-mesh-new

# Should see:
# "Peer version: 1.0.0, compatible: true"
# "Using shared features: [attestations, planets]"
# "Skipping feature 'new_feature' (peer doesn't support)"
```

## Best Practices

### DO ✅

- **Bump MINOR** for new features
- **Bump PATCH** for bug fixes
- **Make new features optional** (check peer support)
- **Test mixed versions** before releasing
- **Document compatibility** in release notes

### DON'T ❌

- **Don't bump MAJOR** unless absolutely necessary
- **Don't require new features** from all peers
- **Don't break old message formats**
- **Don't remove backwards compatibility** without major version bump

## Future-Proofing

### Planned Features (v1.x.x)

These will be backwards compatible:
- Bridge detection (v1.1.0)
- Resource trading (v1.2.0)
- Faction support (v1.3.0)
- Economic layer (v1.4.0)

**All will support v1.0.0 nodes!**

### Potential Breaking Changes (v2.0.0)

Only if necessary:
- New transport protocol
- Different cryptography
- Fundamental architecture change

**Avoid unless critical!**

## Summary

**Protocol versioning ensures**:
- ✅ Old nodes keep working
- ✅ New features roll out gradually
- ✅ No "flag day" required
- ✅ Network stays healthy during updates
- ✅ Brief downtimes don't penalize reputation
- ✅ Backwards compatibility is mandatory
- ✅ Forward compatibility is encouraged

**Bottom line**: Update whenever you want. The network adapts.
