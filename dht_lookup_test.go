package main

import (
	"testing"

	"github.com/google/uuid"
)

// =============================================================================
// SELECT UNQUERIED TESTS
// =============================================================================

func TestSelectUnqueried_AllUnqueried(t *testing.T) {
	nodes := []*System{
		{ID: uuid.New()},
		{ID: uuid.New()},
		{ID: uuid.New()},
	}
	queried := make(map[uuid.UUID]bool)

	result := selectUnqueried(nodes, queried, 2)

	if len(result) != 2 {
		t.Errorf("selectUnqueried() returned %d nodes, want 2", len(result))
	}
}

func TestSelectUnqueried_SomeQueried(t *testing.T) {
	node1 := &System{ID: uuid.New()}
	node2 := &System{ID: uuid.New()}
	node3 := &System{ID: uuid.New()}
	nodes := []*System{node1, node2, node3}

	queried := map[uuid.UUID]bool{
		node1.ID: true, // First node already queried
	}

	result := selectUnqueried(nodes, queried, 3)

	if len(result) != 2 {
		t.Errorf("selectUnqueried() returned %d nodes, want 2", len(result))
	}

	// First result should be node2 (node1 was queried)
	if result[0].ID != node2.ID {
		t.Error("First result should be node2 (first unqueried)")
	}
}

func TestSelectUnqueried_AllQueried(t *testing.T) {
	node1 := &System{ID: uuid.New()}
	node2 := &System{ID: uuid.New()}
	nodes := []*System{node1, node2}

	queried := map[uuid.UUID]bool{
		node1.ID: true,
		node2.ID: true,
	}

	result := selectUnqueried(nodes, queried, 3)

	if len(result) != 0 {
		t.Errorf("selectUnqueried() returned %d nodes, want 0", len(result))
	}
}

func TestSelectUnqueried_EmptyList(t *testing.T) {
	nodes := []*System{}
	queried := make(map[uuid.UUID]bool)

	result := selectUnqueried(nodes, queried, 3)

	if len(result) != 0 {
		t.Errorf("selectUnqueried() returned %d nodes, want 0", len(result))
	}
}

func TestSelectUnqueried_LimitRespected(t *testing.T) {
	nodes := make([]*System, 10)
	for i := range nodes {
		nodes[i] = &System{ID: uuid.New()}
	}
	queried := make(map[uuid.UUID]bool)

	result := selectUnqueried(nodes, queried, 3)

	if len(result) != 3 {
		t.Errorf("selectUnqueried() returned %d nodes, want 3 (limit)", len(result))
	}
}

// =============================================================================
// SORT BY DISTANCE TESTS
// =============================================================================

func TestSortByDistance(t *testing.T) {
	// Create a target and nodes at known XOR distances
	target := uuid.MustParse("00000000-0000-0000-0000-000000000000")

	// Create nodes with known distances
	// Closer nodes have more leading zero bits in XOR distance
	close := &System{ID: uuid.MustParse("00000000-0000-0000-0000-000000000001")}  // 127 leading zeros
	medium := &System{ID: uuid.MustParse("00000000-0000-0000-0000-000000000100")} // 119 leading zeros
	far := &System{ID: uuid.MustParse("00000001-0000-0000-0000-000000000000")}    // 31 leading zeros

	nodes := map[uuid.UUID]*System{
		far.ID:    far,
		close.ID:  close,
		medium.ID: medium,
	}

	result := sortByDistance(nodes, target)

	if len(result) != 3 {
		t.Fatalf("Expected 3 nodes, got %d", len(result))
	}

	// Should be sorted: close, medium, far
	if result[0].ID != close.ID {
		t.Error("First node should be closest (00...001)")
	}
	if result[1].ID != medium.ID {
		t.Error("Second node should be medium distance (00...0100)")
	}
	if result[2].ID != far.ID {
		t.Error("Third node should be farthest (01...000)")
	}
}

func TestSortByDistance_Empty(t *testing.T) {
	target := uuid.New()
	nodes := map[uuid.UUID]*System{}

	result := sortByDistance(nodes, target)

	if len(result) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(result))
	}
}

func TestSortByDistance_SingleNode(t *testing.T) {
	target := uuid.New()
	node := &System{ID: uuid.New()}
	nodes := map[uuid.UUID]*System{node.ID: node}

	result := sortByDistance(nodes, target)

	if len(result) != 1 {
		t.Errorf("Expected 1 node, got %d", len(result))
	}
	if result[0].ID != node.ID {
		t.Error("Single node should be returned")
	}
}

// =============================================================================
// ALL CLOSEST QUERIED TESTS
// =============================================================================

func TestAllClosestQueried_AllQueried(t *testing.T) {
	node1 := &System{ID: uuid.New()}
	node2 := &System{ID: uuid.New()}
	node3 := &System{ID: uuid.New()}
	nodes := []*System{node1, node2, node3}

	queried := map[uuid.UUID]bool{
		node1.ID: true,
		node2.ID: true,
		node3.ID: true,
	}

	if !allClosestQueried(nodes, queried, 3) {
		t.Error("All 3 closest are queried, should return true")
	}
}

func TestAllClosestQueried_SomeUnqueried(t *testing.T) {
	node1 := &System{ID: uuid.New()}
	node2 := &System{ID: uuid.New()}
	node3 := &System{ID: uuid.New()}
	nodes := []*System{node1, node2, node3}

	queried := map[uuid.UUID]bool{
		node1.ID: true,
		// node2 not queried
		node3.ID: true,
	}

	if allClosestQueried(nodes, queried, 3) {
		t.Error("node2 is not queried, should return false")
	}
}

func TestAllClosestQueried_PartialK(t *testing.T) {
	node1 := &System{ID: uuid.New()}
	node2 := &System{ID: uuid.New()}
	node3 := &System{ID: uuid.New()}
	nodes := []*System{node1, node2, node3}

	queried := map[uuid.UUID]bool{
		node1.ID: true,
		node2.ID: true,
		// node3 not queried
	}

	// Only checking K=2, so node3 doesn't matter
	if !allClosestQueried(nodes, queried, 2) {
		t.Error("First 2 are queried, should return true for K=2")
	}
}

func TestAllClosestQueried_KLargerThanList(t *testing.T) {
	node1 := &System{ID: uuid.New()}
	node2 := &System{ID: uuid.New()}
	nodes := []*System{node1, node2}

	queried := map[uuid.UUID]bool{
		node1.ID: true,
		node2.ID: true,
	}

	// K=10 but only 2 nodes, all are queried
	if !allClosestQueried(nodes, queried, 10) {
		t.Error("All nodes queried, should return true even if K > len(nodes)")
	}
}

func TestAllClosestQueried_EmptyList(t *testing.T) {
	nodes := []*System{}
	queried := make(map[uuid.UUID]bool)

	if !allClosestQueried(nodes, queried, 3) {
		t.Error("Empty list should return true (vacuously true)")
	}
}

// =============================================================================
// TRUNCATE TO K TESTS
// =============================================================================

func TestTruncateToK_LessThanK(t *testing.T) {
	nodes := []*System{
		{ID: uuid.New()},
		{ID: uuid.New()},
	}

	result := truncateToK(nodes, 5)

	if len(result) != 2 {
		t.Errorf("Expected 2 nodes (all of them), got %d", len(result))
	}
}

func TestTruncateToK_ExactlyK(t *testing.T) {
	nodes := []*System{
		{ID: uuid.New()},
		{ID: uuid.New()},
		{ID: uuid.New()},
	}

	result := truncateToK(nodes, 3)

	if len(result) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(result))
	}
}

func TestTruncateToK_MoreThanK(t *testing.T) {
	nodes := make([]*System, 10)
	for i := range nodes {
		nodes[i] = &System{ID: uuid.New()}
	}

	result := truncateToK(nodes, 5)

	if len(result) != 5 {
		t.Errorf("Expected 5 nodes (truncated), got %d", len(result))
	}

	// Verify we got the first 5
	for i := 0; i < 5; i++ {
		if result[i].ID != nodes[i].ID {
			t.Errorf("Node %d should be preserved", i)
		}
	}
}

func TestTruncateToK_Empty(t *testing.T) {
	nodes := []*System{}

	result := truncateToK(nodes, 5)

	if len(result) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(result))
	}
}

func TestTruncateToK_ZeroK(t *testing.T) {
	nodes := []*System{
		{ID: uuid.New()},
	}

	result := truncateToK(nodes, 0)

	if len(result) != 0 {
		t.Errorf("Expected 0 nodes for K=0, got %d", len(result))
	}
}

// =============================================================================
// LOOKUP RESULT TESTS
// =============================================================================

func TestLookupResult_Structure(t *testing.T) {
	targetID := uuid.New()
	result := &LookupResult{
		Target:       targetID,
		ClosestNodes: []*System{{ID: uuid.New()}},
		Found:        nil,
		Hops:         5,
	}

	if result.Target != targetID {
		t.Error("Target should be set")
	}
	if len(result.ClosestNodes) != 1 {
		t.Error("ClosestNodes should have 1 entry")
	}
	if result.Hops != 5 {
		t.Errorf("Hops = %d, want 5", result.Hops)
	}
	if result.Found != nil {
		t.Error("Found should be nil when target not found")
	}
}

// =============================================================================
// QUERY RESPONSE TESTS
// =============================================================================

func TestQueryResponse_Structure(t *testing.T) {
	nodeID := uuid.New()
	resp := queryResponse{
		nodeID: nodeID,
		nodes:  []*System{{ID: uuid.New()}},
		err:    nil,
	}

	if resp.nodeID != nodeID {
		t.Error("nodeID should be set")
	}
	if len(resp.nodes) != 1 {
		t.Error("nodes should have 1 entry")
	}
	if resp.err != nil {
		t.Error("err should be nil for successful response")
	}
}

func TestQueryResponse_WithError(t *testing.T) {
	resp := queryResponse{
		nodeID: uuid.New(),
		nodes:  nil,
		err:    &DHTError{Code: 500, Message: "test error"},
	}

	if resp.err == nil {
		t.Error("err should be set for failed response")
	}
	if resp.nodes != nil {
		t.Error("nodes should be nil for failed response")
	}
}
