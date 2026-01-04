# Beacon System Design - v1.1.0

## Overview

Every 24 hours, a **deterministic, verifiable random selection** chooses one node as the **Beacon**. The beacon and all nodes on the shortest path from the previous beacon receive **Beacon Credits** as a reward. This system incentivizes network connectivity, rewards strategic positioning, and creates an economic layer for future features.

---

## Core Design Principles

1. **Separate Currency**: Beacon Credits are distinct from Reputation
2. **Deterministic Selection**: All nodes independently calculate the same beacon
3. **Percentage-Based Rewards**: Scale naturally with network growth
4. **Transparent & Verifiable**: Anyone can verify calculations
5. **7-Day Bootstrap**: No beacons until day 8 (credits seed from attestations)
6. **Path-Based Distribution**: Rewards network connectivity

---

## Beacon Credits vs Reputation

### Two Separate Metrics

```go
type NetworkContribution struct {
    // Trust/Security (permanent, proves reliability)
    ReputationPoints    int
    Rank               string
    
    // Participation Rewards (economic layer)
    AttestationCredits int    // From normal participation (days 1-7, then ongoing)
    BeaconCredits      int    // From being selected as beacon (day 8+)
    PathCredits        int    // From being on beacon paths (day 8+)
    BridgeCredits      int    // From being a critical bridge (future)
    
    TotalCredits       int    // Sum of all credits
}
```

### Why Separate?

| Metric | Purpose | Earned How | Properties |
|--------|---------|------------|------------|
| **Reputation** | Trust/Security | Attestations, uptime, diversity | Permanent, can't spend |
| **Credits** | Economy/Currency | Attestations, beacons, paths | Consumable, tradeable |

**Reputation** proves you're trustworthy
**Credits** enable you to do things (future features)

---

## Bootstrap Period (Days 1-7)

### No Beacons, Just Attestations

```
Day 1-7: Network launches, nodes join
├─ Nodes exchange attestations normally
├─ Reputation accumulates from attestations
├─ Credits = Reputation (1:1 initially)
└─ By day 7: Organic credit distribution exists

Example Day 7 State:
- 100 nodes active
- Average 500 attestations each
- Total network credits: ~50,000 (from attestations)
```

### Why 7 Days?

✅ **No artificial genesis grants** - Credits earned through real participation
✅ **Natural distribution** - Early adopters rewarded fairly
✅ **Sufficient baseline** - By day 7, enough credits exist for meaningful rewards
✅ **Network stabilization** - Gives network time to form healthy topology

---

## Beacon Selection (Day 8+)

### Deterministic Algorithm

**Every node independently calculates the same result**

```go
func SelectBeacon(date time.Time) uuid.UUID {
    // Use PREVIOUS day's finalized state (unchangeable)
    referenceTime := date.Add(-24 * time.Hour)
    
    // Get active nodes from reference time
    activeNodes := GetActiveNodesAt(referenceTime)
    
    // Sort for determinism
    sort.Slice(activeNodes, func(i, j int) bool {
        return activeNodes[i].String() < activeNodes[j].String()
    })
    
    // Create deterministic seed
    dayString := date.Format("2006-01-02")
    nodeList := SerializeNodeList(activeNodes)
    seed := sha256.Sum256([]byte(dayString + nodeList))
    
    // Convert to index
    seedInt := new(big.Int).SetBytes(seed[:])
    index := new(big.Int).Mod(seedInt, big.NewInt(int64(len(activeNodes))))
    
    return activeNodes[index.Int64()]
}
```

### Active Node Definition

Node is "active" if:
- Has sent attestation in last 48 hours
- Has reputation > 0
- Has at least 5 unique peers (prevents isolated sybil nodes)

### Timeline

```
00:00 UTC - New day starts
├─ Each node calculates beacon independently
├─ Uses previous day's finalized active node list
└─ All nodes should get same result

00:00 - 00:30 UTC - Gossip verification period (future v1.2.0)
├─ Nodes announce their calculation
├─ Verify peer calculations
├─ Reach consensus (95%+ agreement expected)
└─ Flag discrepancies

00:30 UTC - Beacon finalized
├─ Beacon calculation locked in
├─ Path calculation begins
└─ Rewards distributed
```

---

## Reward Calculation

### Percentage-Based (Self-Scaling)

```go
func CalculateBeaconReward(totalNetworkCredits int) BeaconRewards {
    // Beacon reward: 0.05% of total network credits
    beaconReward := int(float64(totalNetworkCredits) * 0.0005)
    
    // Path pool: 0.01% of total network credits
    pathPool := int(float64(totalNetworkCredits) * 0.0001)
    
    return BeaconRewards{
        Beacon:   beaconReward,
        PathPool: pathPool,
    }
}
```

### Why Percentages?

**Self-scaling with network growth:**

| Network Age | Total Credits | Beacon Reward | Path Pool |
|-------------|---------------|---------------|-----------|
| Day 8 | 50,000 | 25 | 5 |
| Week 2 | 60,000 | 30 | 6 |
| Month 1 | 200,000 | 100 | 20 |
| Month 6 | 1,000,000 | 500 | 100 |
| Year 1 | 5,000,000 | 2,500 | 500 |

✅ Always meaningful (scales with economy)
✅ Never too small (grows with network)
✅ Never too large (percentage controlled)
✅ Predictable inflation (~36.5% annual)

---

## Path Calculation & Distribution

### Shortest Path Algorithm

```go
func CalculateBeaconPath(prevBeacon, currentBeacon uuid.UUID) []uuid.UUID {
    // Use Dijkstra's algorithm on network topology
    // Topology built from attestations (proves connections)
    
    path := FindShortestPath(prevBeacon, currentBeacon, networkTopology)
    
    // Verify path is valid
    for i := 0; i < len(path)-1; i++ {
        if !HasRecentAttestation(path[i], path[i+1]) {
            return nil, errors.New("invalid path - no attestation proof")
        }
    }
    
    return path
}
```

### Path Reward Distribution

**Fixed pool split among path nodes:**

```go
func DistributePathRewards(path []uuid.UUID, pathPool int) {
    if len(path) == 0 {
        return
    }
    
    // Minimum per-node reward
    const minPerNode = 5
    
    // Calculate per-node share
    perNode := pathPool / len(path)
    
    // Apply minimum (for very long paths)
    if perNode < minPerNode {
        perNode = minPerNode
    }
    
    // Cap total distribution to prevent explosion
    maxTotal := int(float64(totalNetworkCredits) * 0.0005)  // 0.05% cap
    actualTotal := perNode * len(path)
    
    if actualTotal > maxTotal {
        // Scale down to fit cap
        perNode = maxTotal / len(path)
    }
    
    // Distribute to all path nodes (excluding previous beacon)
    for i := 1; i < len(path); i++ {  // Skip index 0 (previous beacon)
        GrantCredits(path[i], perNode)
    }
}
```

### Handling Large Paths

**Example with 500-node path:**

```
Network total: 1,000,000 credits
Path pool: 1,000 credits
Path length: 500 nodes

Naive calculation: 1,000 / 500 = 2 credits each
Apply minimum: max(2, 5) = 5 credits each
Total would be: 5 * 500 = 2,500 credits

Apply cap: 0.05% of 1M = 500 credits max
Actual per-node: 500 / 500 = 1 credit each

Result: Controlled distribution even with massive paths
```

---

## Example Scenarios

### Day 8 (First Beacon)

```
Network state:
- 100 active nodes
- Total credits: 50,000 (from week of attestations)

Beacon selection:
- Selected: "Sol System"
- Previous beacon: None (first event)

Rewards:
- Sol gets: 50,000 * 0.0005 = 25 credits
- No path rewards (no previous beacon)

New total: 50,025 credits
```

### Day 9 (First Path)

```
Network state:
- Total credits: 55,000

Beacon selection:
- Previous: Sol (abc123)
- Current: Proxima (def456)

Path calculation:
- Sol → Alpha Centauri → Sirius → Proxima
- Length: 3 intermediate nodes + beacon

Rewards:
- Proxima (beacon): 55,000 * 0.0005 = 27.5 → 27 credits
- Path pool: 55,000 * 0.0001 = 5.5 → 5 credits
- Alpha Centauri: 5/3 = 1 credit
- Sirius: 5/3 = 1 credit  
- Sol: 0 (previous beacon, no reward)

Total distributed: 29 credits
New total: 55,029 credits
```

### Month 1 (Mature Network)

```
Network state:
- 500 active nodes
- Total credits: 200,000

Beacon selection:
- Previous: Node A
- Current: Node B
- Path length: 15 nodes

Rewards:
- Node B (beacon): 100 credits
- Path pool: 20 credits
- Each path node: 20/15 = 1 credit (apply min 5 → 5 each)
- Total path: 5 * 15 = 75 credits
- But cap at 100 credits max → 100/15 = 6 credits each

Total distributed: 100 + 90 = 190 credits
New total: 200,190 credits
```

---

## Anti-Gaming Measures

### 1. Minimum Peer Diversity

```go
func IsEligibleForBeacon(node *Node) bool {
    // Prevents isolated sybil nodes
    if node.UniquePeers < 5 {
        return false
    }
    
    // Must have meaningful reputation
    if node.Reputation < 100 {
        return false
    }
    
    // Must be recently active
    if time.Since(node.LastAttestation) > 48*time.Hour {
        return false
    }
    
    return true
}
```

### 2. Historical Reference Data

```go
// Use PREVIOUS day's state (can't be manipulated in real-time)
referenceTime := date.Add(-24 * time.Hour)
activeNodes := GetActiveNodesAt(referenceTime)
```

**Why this prevents gaming:**
- Attacker can't DDoS nodes to change today's beacon
- Would need to attack 24 hours in advance
- Don't know which nodes to attack (depends on tomorrow's calculation)

### 3. Path Verification

```go
// All path nodes must have recent attestations
func VerifyPath(path []uuid.UUID) bool {
    for i := 0; i < len(path)-1; i++ {
        att := GetAttestation(path[i], path[i+1])
        if att == nil || !att.Verify() {
            return false
        }
        if time.Since(att.Timestamp) > 24*time.Hour {
            return false
        }
    }
    return true
}
```

### 4. Reputation Diversity Multiplier

```go
// Low-diversity nodes get reputation penalty
func GetDiversityMultiplier(uniquePeers int) float64 {
    if uniquePeers < 3 {
        return 0.1  // 90% penalty - isolated nodes
    }
    if uniquePeers < 10 {
        return 0.5  // 50% penalty - poorly connected
    }
    return 1.0  // Full reputation
}
```

---

## Transparency & Predictability

### Users Can Calculate Future Beacons

```go
// Anyone can run this calculation
func ForecastBeacons(startDate time.Time, days int) []BeaconEvent {
    forecast := []BeaconEvent{}
    
    for i := 0; i < days; i++ {
        date := startDate.Add(time.Duration(i) * 24 * time.Hour)
        beacon := SelectBeacon(date)
        
        forecast = append(forecast, BeaconEvent{
            Date:     date,
            BeaconID: beacon,
        })
    }
    
    return forecast
}
```

### Why Transparency Is Good

✅ **Fair warning** - Operators ensure uptime when selected
✅ **Community engagement** - "Sol is beacon tomorrow!"
✅ **Strategic planning** - Users optimize network position
✅ **Trust** - No black boxes, fully verifiable
✅ **Infrastructure** - Prepare servers for beacon events

### What "Gaming" Actually Looks Like

```
User discovers: "I'm beacon in 5 days"

Actions they might take:
├─ Ensure server uptime ✅ Good for network
├─ Check bandwidth ✅ Good for network
├─ Connect to more peers ✅ Good for network
└─ Maintain attestations ✅ Good for network

Result: "Gaming" = Being a good participant
```

---

## Database Schema

```sql
-- Beacon events
CREATE TABLE beacon_events (
    id TEXT PRIMARY KEY,
    date TEXT NOT NULL UNIQUE,
    beacon_system_id TEXT NOT NULL,
    prev_beacon_id TEXT,
    
    -- Selection metadata
    active_nodes_count INTEGER NOT NULL,
    calculation_seed TEXT NOT NULL,
    
    -- Path and rewards
    path_json TEXT,  -- JSON array of node IDs
    path_length INTEGER,
    beacon_reward INTEGER,
    path_pool_reward INTEGER,
    total_credits_distributed INTEGER,
    
    -- Status
    finalized INTEGER DEFAULT 0,
    created_at INTEGER NOT NULL,
    finalized_at INTEGER,
    
    FOREIGN KEY (beacon_system_id) REFERENCES system(id),
    FOREIGN KEY (prev_beacon_id) REFERENCES system(id)
);

-- Credit tracking (separate from reputation)
CREATE TABLE beacon_credits (
    system_id TEXT PRIMARY KEY,
    
    -- Credit sources
    attestation_credits INTEGER DEFAULT 0,  -- From participation (ongoing)
    beacon_credits INTEGER DEFAULT 0,       -- From being beacon
    path_credits INTEGER DEFAULT 0,         -- From path appearances
    bridge_credits INTEGER DEFAULT 0,       -- From bridge bonuses (future)
    
    -- Totals
    total_credits INTEGER DEFAULT 0,
    
    -- Metadata
    times_beacon INTEGER DEFAULT 0,
    times_on_path INTEGER DEFAULT 0,
    last_beacon_date TEXT,
    last_path_date TEXT,
    last_updated INTEGER NOT NULL,
    
    FOREIGN KEY (system_id) REFERENCES system(id)
);

-- Gossip verification (future v1.2.0)
CREATE TABLE beacon_votes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date TEXT NOT NULL,
    beacon_id TEXT NOT NULL,
    voter_id TEXT NOT NULL,
    voter_signature TEXT NOT NULL,
    active_nodes_hash TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    
    UNIQUE(date, voter_id)
);

-- Indexes
CREATE INDEX idx_beacon_events_date ON beacon_events(date);
CREATE INDEX idx_beacon_credits_total ON beacon_credits(total_credits DESC);
```

---

## API Endpoints

### GET /api/beacon/current

Returns today's beacon event

```json
{
  "date": "2026-01-09",
  "beacon_id": "def456...",
  "beacon_name": "Proxima",
  "prev_beacon_id": "abc123...",
  "prev_beacon_name": "Sol",
  "path": [
    {"id": "abc123", "name": "Sol", "reward": 0},
    {"id": "ghi789", "name": "Alpha Centauri", "reward": 6},
    {"id": "jkl012", "name": "Sirius", "reward": 6},
    {"id": "def456", "name": "Proxima", "reward": 27}
  ],
  "path_length": 3,
  "beacon_reward": 27,
  "path_pool": 20,
  "total_distributed": 47,
  "finalized": true
}
```

### GET /api/beacon/forecast?days=30

Returns predicted beacons for next N days

```json
{
  "forecast": [
    {
      "date": "2026-01-10",
      "beacon_id": "...",
      "beacon_name": "Betelgeuse",
      "is_me": false,
      "days_until": 1
    },
    {
      "date": "2026-01-15",
      "beacon_id": "...",
      "beacon_name": "Sol",
      "is_me": true,
      "days_until": 6,
      "estimated_reward": 150
    }
  ]
}
```

### GET /api/beacon/history?limit=30

Returns past beacon events

```json
{
  "events": [
    {
      "date": "2026-01-09",
      "beacon_name": "Proxima",
      "path_length": 3,
      "total_distributed": 47
    },
    {
      "date": "2026-01-08",
      "beacon_name": "Sol",
      "path_length": 0,
      "total_distributed": 25
    }
  ],
  "total_events": 2,
  "network_age_days": 9
}
```

### GET /api/credits

Returns user's credit breakdown

```json
{
  "system_id": "abc123...",
  "system_name": "Sol",
  "attestation_credits": 50000,
  "beacon_credits": 2500,
  "path_credits": 3400,
  "bridge_credits": 0,
  "total_credits": 55900,
  "breakdown": {
    "times_beacon": 2,
    "times_on_path": 34,
    "times_bridge": 0,
    "last_beacon": "2026-01-08",
    "last_path": "2026-01-09"
  },
  "rank": {
    "total_credits_rank": 15,
    "beacon_count_rank": 8
  }
}
```

---

## Implementation Phases

### v1.1.0 - Basic Beacon System

**Core Features:**
- ✅ Deterministic beacon selection
- ✅ Percentage-based rewards
- ✅ Path calculation and distribution
- ✅ Credit tracking (separate from reputation)
- ✅ 7-day bootstrap period
- ✅ Anti-sybil measures (peer diversity)
- ✅ Forecast API

**NOT Included Yet:**
- ❌ Gossip verification (assumed network synchronized)
- ❌ Credit spending features
- ❌ Bridge detection/bonuses
- ❌ Byzantine fault tolerance

### v1.2.0 - Consensus & Verification

**Add:**
- Gossip-based beacon verification
- Vote counting
- Dispute resolution
- Network partition handling
- Improved path verification

### v1.3.0 - Economic Layer

**Add:**
- Credit spending features
- Trading between nodes
- Governance voting
- Bridge detection and bonuses
- Faction support

---

## Network Growth Projections

### Credits Over Time

```
Day 8:
- Total: 50,000 credits
- Daily distribution: ~30 credits
- Growth rate: ~0.06%/day

Month 1:
- Total: 200,000 credits
- Daily distribution: ~120 credits
- Growth rate: ~0.06%/day

Month 6:
- Total: 1,000,000 credits
- Daily distribution: ~600 credits
- Growth rate: ~0.06%/day

Year 1:
- Total: 5,000,000 credits
- Daily distribution: ~3,000 credits
- Growth rate: ~0.06%/day

Annual inflation: ~0.06% * 365 = ~22% (from beacons)
Plus ~15% from ongoing attestations
Total: ~37% annual inflation
```

**Controlled, predictable growth**

### Node Distribution

```
Week 1: 100 nodes
├─ Each has ~500 credits
├─ Top nodes: ~1,000 credits
└─ Beacon chances: 1%

Month 1: 500 nodes
├─ Average: ~400 credits
├─ Top nodes: ~2,000 credits
└─ Beacon chances: 0.2%

Year 1: 5,000 nodes
├─ Average: ~1,000 credits
├─ Top nodes: ~10,000 credits
└─ Beacon chances: 0.02%
```

---

## Future Credit Uses (v1.3.0+)

### Cosmetic
- Name a planet: 100 credits
- Custom system description: 50 credits
- Galaxy map highlight: 500 credits

### Functional
- Create faction: 5,000 credits
- Boost priority: 1,000 credits
- Initiate trade route: 200 credits

### Governance
- Submit proposal: 500 credits
- Vote weight multiplier: Credits as voting power

### Economic
- Trade credits peer-to-peer
- Stake for services
- Pay for computational tasks

---

## Security Considerations

### What We Prevent

✅ **Sybil attacks** - Require 5+ peers, meaningful reputation
✅ **Path manipulation** - Verify attestations on path
✅ **Active list gaming** - Use historical data (24h old)
✅ **Isolation farming** - Diversity multiplier penalties
✅ **Fake connections** - Cryptographic attestation proofs

### What We Accept

✅ **Predictability** - Users can forecast beacons
✅ **Strategic positioning** - Users optimize network location
✅ **Coordinated groups** - Friends connecting strategically

**Why?** Because these behaviors improve network topology!

### Attack Economics

```
Attack: Run 100 isolated node pairs (200 nodes total)
├─ Each pair farms attestations
├─ 200 nodes / 10,000 network = 2% beacon chance
├─ Diversity penalty: 90% reputation reduction
├─ Never on paths (isolated)
└─ Cost > benefit

Real participation: Run 1 well-connected node
├─ Connect to 20 diverse peers
├─ 1 node / 10,000 network = 0.01% beacon chance
├─ On paths frequently = regular rewards
├─ Bridge bonuses if strategic
└─ Much better ROI
```

---

## Success Metrics

### Network Health
- Average path length trending down (better connectivity)
- Unique peer count increasing
- Bridge node count stable (redundant paths exist)

### Economic Activity
- Credit distribution spreading (not concentrated)
- Top 10% nodes have <30% of credits
- New nodes earning credits within first week

### Engagement
- Beacon forecast usage
- Uptime during beacon events >99%
- Community activity around beacon selections

---

## Summary

**Beacon System v1.1.0 provides:**

✅ **Economic incentive layer** - Credits reward participation
✅ **Self-scaling rewards** - Percentage-based, grows with network
✅ **Natural bootstrap** - 7-day attestation seeding
✅ **Deterministic selection** - Verifiable, transparent, fair
✅ **Path rewards** - Incentivizes connectivity
✅ **Anti-gaming** - Diversity requirements, historical data
✅ **Future-proof** - Foundation for economic features

**Key Innovation:**
Separating Credits (economic) from Reputation (trust) creates a dual-metric system where both matter but serve different purposes. This enables future economic features without compromising security.

**Philosophy:**
Transparency is a feature, not a bug. Users knowing future beacons encourages good behavior (maintaining uptime, building connections) which strengthens the network. "Gaming" the system requires real participation, which is the goal.

