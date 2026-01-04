#!/bin/bash

# Example script to test the stellar-mesh network locally

echo "Building stellar-mesh..."
go build -o stellar-mesh

echo ""
echo "Starting three nodes to demonstrate clustering..."
echo "Node 1 (Sol System) on :8080 - Bootstrap node"
echo "Node 2 (Alpha Centauri) on :8081 - Will cluster near Node 1"
echo "Node 3 (Proxima) on :8082 - Will cluster near Node 1"
echo ""

# Clean up old databases
rm -f node1.db node2.db node3.db

# Start node 1 (bootstrap node)
echo "Starting Node 1 (Bootstrap)..."
./stellar-mesh -name "Sol System" -address "localhost:8080" -db "node1.db" &
NODE1_PID=$!
sleep 3

# Get Node 1's coordinates
echo ""
echo "=== Bootstrap Node (Sol System) ==="
NODE1_INFO=$(curl -s http://localhost:8080/system)
echo "$NODE1_INFO" | jq '{name, star_type: .star_type.class, description: .star_type.description, coordinates: {x, y, z}}'
echo ""

# Start node 2 (connects to node 1)
echo "Starting Node 2 (will cluster near Node 1)..."
./stellar-mesh -name "Alpha Centauri" -address "localhost:8081" -db "node2.db" -bootstrap "localhost:8080" &
NODE2_PID=$!
sleep 3

echo ""
echo "=== Node 2 (Alpha Centauri) ==="
NODE2_INFO=$(curl -s http://localhost:8081/system)
echo "$NODE2_INFO" | jq '{name, star_type: .star_type.class, description: .star_type.description, coordinates: {x, y, z}}'
echo ""

# Start node 3 (connects to node 1)
echo "Starting Node 3 (will cluster near Node 1)..."
./stellar-mesh -name "Proxima" -address "localhost:8082" -db "node3.db" -bootstrap "localhost:8080" &
NODE3_PID=$!
sleep 3

echo ""
echo "=== Node 3 (Proxima) ==="
NODE3_INFO=$(curl -s http://localhost:8082/system)
echo "$NODE3_INFO" | jq '{name, star_type: .star_type.class, description: .star_type.description, coordinates: {x, y, z}}'
echo ""

# Calculate distances (simple demonstration)
echo "=== Spatial Analysis ==="
echo "Nodes 2 and 3 should be within 100-500 units of Node 1"
echo ""

# Wait for gossip to happen
echo "Waiting 15 seconds for peer discovery..."
sleep 15

# Check peers on each node
echo ""
echo "=== Node 1 Peer List ==="
curl -s http://localhost:8080/peers | jq '.count, .peers[].system_id'
echo ""

echo "=== Node 2 Peer List ==="
curl -s http://localhost:8081/peers | jq '.count, .peers[].system_id'
echo ""

echo "=== Node 3 Peer List ==="
curl -s http://localhost:8082/peers | jq '.count, .peers[].system_id'
echo ""

# Show stats
echo "=== Network Stats (Node 1) ==="
curl -s http://localhost:8080/stats | jq '.'
echo ""

echo ""
echo "All nodes running! Try these commands:"
echo "  curl http://localhost:8080/system | jq ."
echo "  curl http://localhost:8081/peers | jq ."
echo "  curl http://localhost:8082/stats | jq ."
echo ""
echo "Press Ctrl+C to stop all nodes..."

# Cleanup handler
cleanup() {
    echo ""
    echo "Shutting down nodes..."
    kill $NODE1_PID $NODE2_PID $NODE3_PID 2>/dev/null
    exit 0
}

trap cleanup INT TERM

wait $NODE1_PID $NODE2_PID $NODE3_PID
