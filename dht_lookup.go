package main

import (
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// LookupResult contains the result of a DHT lookup
type LookupResult struct {
	Target       uuid.UUID
	ClosestNodes []*System
	Found        *System // Non-nil if exact target was found
	Hops         int
	Duration     time.Duration
}

// FindNode performs an iterative lookup for the K closest nodes to a target ID
// This is the core DHT lookup algorithm (Kademlia-style)
func (dht *DHT) FindNode(targetID uuid.UUID) *LookupResult {
	startTime := time.Now()
	result := &LookupResult{
		Target: targetID,
	}

	// Check if we have the target cached
	if cached := dht.routingTable.GetCachedSystem(targetID); cached != nil {
		result.Found = cached
		result.ClosestNodes = []*System{cached}
		result.Duration = time.Since(startTime)
		return result
	}

	// Get initial closest nodes from our routing table
	shortlist := dht.routingTable.GetClosest(targetID, Alpha)
	if len(shortlist) == 0 {
		log.Printf("FindNode: no nodes in routing table, cannot lookup %s", targetID.String()[:8])
		result.Duration = time.Since(startTime)
		return result
	}

	// Track which nodes we've queried
	queried := make(map[uuid.UUID]bool)

	// Track all nodes we've learned about, sorted by distance
	allNodes := make(map[uuid.UUID]*System)
	for _, sys := range shortlist {
		allNodes[sys.ID] = sys
	}

	hops := 0
	maxHops := 20 // Safety limit

	for hops < maxHops {
		hops++

		// Select Alpha closest unqueried nodes
		toQuery := selectUnqueried(shortlist, queried, Alpha)
		if len(toQuery) == 0 {
			// No more unqueried nodes in shortlist
			break
		}

		// Query nodes in parallel
		responses := dht.queryNodesParallel(toQuery, targetID)

		// Process responses
		newNodesFound := false
		for _, resp := range responses {
			if resp.err != nil {
				dht.routingTable.MarkFailed(resp.nodeID)
				continue
			}

			// Mark as queried
			queried[resp.nodeID] = true

			// Save learned peer connections (responder knows these nodes)
			if len(resp.nodes) > 0 {
				peerIDs := make([]uuid.UUID, 0, len(resp.nodes))
				for _, sys := range resp.nodes {
					if sys.ID != dht.localSystem.ID && sys.ID != resp.nodeID {
						peerIDs = append(peerIDs, sys.ID)
					}
				}
				if len(peerIDs) > 0 {
					dht.storage.SavePeerConnections(resp.nodeID, peerIDs)
				}
			}

			// Process returned nodes
			for _, sys := range resp.nodes {
				if sys.ID == dht.localSystem.ID {
					continue // Skip ourselves
				}

				// Check if this is the exact target
				if sys.ID == targetID {
					result.Found = sys
				}

				// Add to allNodes if not seen before
				if _, exists := allNodes[sys.ID]; !exists {
					allNodes[sys.ID] = sys
					newNodesFound = true

					// Update routing table and cache
					dht.routingTable.Update(sys)
				}
			}
		}

		// If we found the exact target, we can stop
		if result.Found != nil {
			break
		}

		// Update shortlist with all known nodes, sorted by distance
		shortlist = sortByDistance(allNodes, targetID)

		// Check termination condition: K closest nodes have all been queried
		if allClosestQueried(shortlist, queried, K) && !newNodesFound {
			break
		}
	}

	// Final result is K closest nodes
	result.ClosestNodes = truncateToK(shortlist, K)
	result.Hops = hops
	result.Duration = time.Since(startTime)

	log.Printf("FindNode(%s): found %d nodes in %d hops (%v)",
		targetID.String()[:8], len(result.ClosestNodes), hops, result.Duration)

	return result
}

// Lookup finds a specific system by ID
func (dht *DHT) Lookup(targetID uuid.UUID) (*System, error) {
	result := dht.FindNode(targetID)
	if result.Found != nil {
		return result.Found, nil
	}

	// Check if any of the closest nodes is the target
	for _, sys := range result.ClosestNodes {
		if sys.ID == targetID {
			return sys, nil
		}
	}

	return nil, &DHTError{Code: 404, Message: "system not found"}
}

// queryResponse holds the result of querying a single node
type queryResponse struct {
	nodeID uuid.UUID
	nodes  []*System
	err    error
}

// queryNodesParallel queries multiple nodes in parallel
func (dht *DHT) queryNodesParallel(nodes []*System, targetID uuid.UUID) []queryResponse {
	var wg sync.WaitGroup
	responses := make([]queryResponse, len(nodes))

	for i, node := range nodes {
		wg.Add(1)
		go func(idx int, sys *System) {
			defer wg.Done()

			responses[idx].nodeID = sys.ID

			if sys.PeerAddress == "" {
				responses[idx].err = &DHTError{Code: 400, Message: "no peer address"}
				return
			}

			closestNodes, err := dht.FindNodeDirect(sys.PeerAddress, targetID)
			if err != nil {
				responses[idx].err = err
				return
			}

			responses[idx].nodes = closestNodes
		}(i, node)
	}

	wg.Wait()
	return responses
}

// selectUnqueried returns up to count nodes from the list that haven't been queried
func selectUnqueried(nodes []*System, queried map[uuid.UUID]bool, count int) []*System {
	result := make([]*System, 0, count)
	for _, node := range nodes {
		if !queried[node.ID] {
			result = append(result, node)
			if len(result) >= count {
				break
			}
		}
	}
	return result
}

// sortByDistance returns all nodes sorted by XOR distance to target
func sortByDistance(nodes map[uuid.UUID]*System, target uuid.UUID) []*System {
	result := make([]*System, 0, len(nodes))
	for _, sys := range nodes {
		result = append(result, sys)
	}

	// Sort by XOR distance
	for i := 1; i < len(result); i++ {
		j := i
		for j > 0 && CompareXORDistance(target, result[j].ID, result[j-1].ID) < 0 {
			result[j], result[j-1] = result[j-1], result[j]
			j--
		}
	}

	return result
}

// allClosestQueried checks if the K closest nodes have all been queried
func allClosestQueried(nodes []*System, queried map[uuid.UUID]bool, k int) bool {
	count := 0
	for _, node := range nodes {
		if count >= k {
			break
		}
		if !queried[node.ID] {
			return false
		}
		count++
	}
	return true
}

// truncateToK returns at most K nodes from the list
func truncateToK(nodes []*System, k int) []*System {
	if len(nodes) <= k {
		return nodes
	}
	return nodes[:k]
}

// FindClosestNodes is a convenience wrapper for FindNode
func (dht *DHT) FindClosestNodes(targetID uuid.UUID, count int) []*System {
	result := dht.FindNode(targetID)
	if len(result.ClosestNodes) <= count {
		return result.ClosestNodes
	}
	return result.ClosestNodes[:count]
}