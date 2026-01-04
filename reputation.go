package main

import (
	"math"
	"time"

	"github.com/google/uuid"
)

// NetworkContribution tracks a node's value to the network
type NetworkContribution struct {
	SystemID            uuid.UUID              `json:"system_id"`
	UptimeSeconds       int64                  `json:"uptime_seconds"`
	TotalPeersSeen      int                    `json:"total_peers_seen"`
	CurrentPeerCount    int                    `json:"current_peer_count"`
	BetweennesScore     float64                `json:"betweenness_score"`     // How many paths go through this node
	BridgeScore         float64                `json:"bridge_score"`          // Critical connector score
	ReputationPoints    float64                `json:"reputation_points"`     // Total accumulated reputation
	ContributionRank    string                 `json:"contribution_rank"`     // Bronze, Silver, Gold, Platinum, Diamond
	LastUpdated         time.Time              `json:"last_updated"`
}

// CalculateUptime returns current uptime in seconds
func (nc *NetworkContribution) CalculateUptime(systemCreatedAt time.Time) {
	nc.UptimeSeconds = int64(time.Since(systemCreatedAt).Seconds())
}

// CalculateBetweenness estimates betweenness centrality
// This is a simplified version - full betweenness requires shortest path calculations
// For now, we estimate based on peer count and connectivity patterns
func CalculateBetweenness(system *System, peers []*Peer, allSystemPeers map[uuid.UUID][]uuid.UUID) float64 {
	if len(peers) == 0 {
		return 0.0
	}

	// Simplified betweenness: if you have many peers, you're likely on many paths
	// Real implementation would need Brandes' algorithm
	
	// Score based on:
	// 1. How many peers you have (more = potentially more paths)
	// 2. How diverse your peers are (connecting different clusters = higher value)
	
	peerCount := float64(len(peers))
	
	// Bonus if you're connecting to peers with few other connections (you're their lifeline)
	criticalityBonus := 0.0
	for _, peer := range peers {
		peerPeers, exists := allSystemPeers[peer.SystemID]
		if exists && len(peerPeers) < 3 {
			// This peer has few connections - you're critical to them
			criticalityBonus += 10.0
		}
	}
	
	return peerCount*5.0 + criticalityBonus
}

// CalculateBridgeScore determines if this node is a critical bridge
// A bridge is a node whose removal would disconnect parts of the network
func CalculateBridgeScore(system *System, peers []*Peer, allSystemPeers map[uuid.UUID][]uuid.UUID) float64 {
	if len(peers) < 2 {
		return 0.0
	}

	// Check if removing this node would disconnect any peers from each other
	// For each pair of peers, see if they have alternate paths
	bridgeScore := 0.0
	
	for i := 0; i < len(peers); i++ {
		for j := i + 1; j < len(peers); j++ {
			peer1 := peers[i].SystemID
			peer2 := peers[j].SystemID
			
			// Check if peer1 and peer2 are directly connected to each other
			peer1Peers, exists1 := allSystemPeers[peer1]
			peer2Peers, exists2 := allSystemPeers[peer2]
			
			if !exists1 || !exists2 {
				continue
			}
			
			// Are they directly connected?
			directlyConnected := false
			for _, p := range peer1Peers {
				if p == peer2 {
					directlyConnected = true
					break
				}
			}
			
			if !directlyConnected {
				// They're not directly connected, so we might be their only link
				// Check if they share any other common peers
				commonPeers := 0
				for _, p1 := range peer1Peers {
					for _, p2 := range peer2Peers {
						if p1 == p2 && p1 != system.ID {
							commonPeers++
						}
					}
				}
				
				if commonPeers == 0 {
					// We're the ONLY connection between these two peers!
					bridgeScore += 50.0
				} else if commonPeers < 2 {
					// We're one of very few connections
					bridgeScore += 20.0
				}
			}
		}
	}
	
	return bridgeScore
}

// CalculateReputationPoints combines all factors into reputation score
func CalculateReputationPoints(contribution *NetworkContribution) float64 {
	// Base points from uptime (1 point per hour)
	uptimePoints := float64(contribution.UptimeSeconds) / 3600.0
	
	// Points from network position
	betweennessPoints := contribution.BetweennesScore
	bridgePoints := contribution.BridgeScore * 2.0 // Bridges are valuable
	
	// Points from peer count (but with diminishing returns)
	peerPoints := math.Log10(float64(contribution.CurrentPeerCount)+1) * 10.0
	
	return uptimePoints + betweennessPoints + bridgePoints + peerPoints
}

// DetermineRank assigns a rank based on reputation points
func DetermineRank(reputationPoints float64) string {
	switch {
	case reputationPoints >= 10000:
		return "Diamond"
	case reputationPoints >= 5000:
		return "Platinum"
	case reputationPoints >= 2000:
		return "Gold"
	case reputationPoints >= 500:
		return "Silver"
	case reputationPoints >= 100:
		return "Bronze"
	default:
		return "Unranked"
	}
}

// UpdateNetworkContribution recalculates all contribution metrics
func UpdateNetworkContribution(
	system *System,
	peers []*Peer,
	allSystemPeers map[uuid.UUID][]uuid.UUID,
	totalPeersSeen int,
) *NetworkContribution {
	
	contribution := &NetworkContribution{
		SystemID:         system.ID,
		TotalPeersSeen:   totalPeersSeen,
		CurrentPeerCount: len(peers),
		LastUpdated:      time.Now(),
	}
	
	contribution.CalculateUptime(system.CreatedAt)
	contribution.BetweennesScore = CalculateBetweenness(system, peers, allSystemPeers)
	contribution.BridgeScore = CalculateBridgeScore(system, peers, allSystemPeers)
	contribution.ReputationPoints = CalculateReputationPoints(contribution)
	contribution.ContributionRank = DetermineRank(contribution.ReputationPoints)
	
	return contribution
}

// GetContributionSummary returns a human-readable summary
func (nc *NetworkContribution) GetContributionSummary() map[string]interface{} {
	hours := float64(nc.UptimeSeconds) / 3600.0
	days := hours / 24.0
	
	return map[string]interface{}{
		"rank":              nc.ContributionRank,
		"reputation_points": int(nc.ReputationPoints),
		"uptime_hours":      int(hours),
		"uptime_days":       int(days),
		"is_critical_bridge": nc.BridgeScore > 50,
		"network_centrality": nc.BetweennesScore,
		"current_peers":     nc.CurrentPeerCount,
		"total_peers_seen":  nc.TotalPeersSeen,
	}
}
