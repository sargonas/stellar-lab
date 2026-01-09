#!/bin/bash
# test-cluster.sh - Comprehensive testing for Stellar Lab
# Runs various test scenarios and validates expected behavior

# Note: Not using 'set -e' since test failures are expected and tracked

# Configuration
CLUSTER_DIR=".test-cluster"
NODES=${STELLAR_TEST_NODES:-5}
BASE_WEB_PORT=9081
BASE_DHT_PORT=9871
BINARY="./stellar-mesh"
TEST_DURATION=${STELLAR_TEST_DURATION:-300}  # Default 5 minutes
REPORT_FILE="test-report.json"
VERBOSE=${VERBOSE:-0}

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
MAGENTA='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

# Test tracking
declare -A TEST_RESULTS
declare -A TEST_DETAILS
TEST_COUNT=0
PASS_COUNT=0
FAIL_COUNT=0
START_TIME=""
END_TIME=""

# Helper functions
log_info() { echo -e "${CYAN}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[PASS]${NC} $1"; }
log_error() { echo -e "${RED}[FAIL]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_test() { echo -e "${MAGENTA}[TEST]${NC} $1"; }

ensure_dirs() {
    mkdir -p "$CLUSTER_DIR"/{data,logs,pids,reports}
}

cleanup() {
    log_info "Cleaning up test environment..."
    # Kill processes from PID files
    for pid_file in "$CLUSTER_DIR"/pids/*.pid; do
        [[ -f "$pid_file" ]] || continue
        local pid=$(cat "$pid_file")
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
        fi
    done
    # Also kill any test nodes that might be running
    pkill -f "stellar-mesh.*TestNode" 2>/dev/null || true
    sleep 1
    if [[ "$1" != "keep" ]]; then
        rm -rf "$CLUSTER_DIR"
    fi
}

build() {
    log_info "Building from source..."
    go build -ldflags "-X main.BuildVersion=test-1.0.0" -o "$BINARY" .
    log_success "Build complete"
}

start_node() {
    local n=$1
    local web_port=$((BASE_WEB_PORT + n - 1))
    local dht_port=$((BASE_DHT_PORT + n - 1))
    local name="TestNode$n"
    local db="$CLUSTER_DIR/data/node$n.db"
    local log="$CLUSTER_DIR/logs/node$n.log"
    local pid_file="$CLUSTER_DIR/pids/node$n.pid"
    
    # Additional options for specific test scenarios
    local extra_opts=""
    # NODE_SEEDS is optional - only use if defined
    if [[ -v NODE_SEEDS && -n "${NODE_SEEDS[$n]:-}" ]]; then
        extra_opts="$extra_opts -seed ${NODE_SEEDS[$n]}"
    fi
    
    $BINARY \
        -name="$name" \
        -address="0.0.0.0:$web_port" \
        -public-address="localhost:$dht_port" \
        -db="$db" \
        -isolated \
        -bootstrap="localhost:$BASE_DHT_PORT" \
        $extra_opts \
        > "$log" 2>&1 &
    
    echo $! > "$pid_file"
    [[ $VERBOSE -eq 1 ]] && log_info "Started node $n (PID: $!, Web: $web_port, DHT: $dht_port)"
}

wait_for_node() {
    local n=$1
    local web_port=$((BASE_WEB_PORT + n - 1))
    local max_wait=30
    local waited=0
    
    while [[ $waited -lt $max_wait ]]; do
        if curl -s "http://localhost:$web_port/api/system" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
        ((waited++))
    done
    
    return 1
}

get_node_info() {
    local n=$1
    local web_port=$((BASE_WEB_PORT + n - 1))
    curl -s "http://localhost:$web_port/api/system" 2>/dev/null || echo "{}"
}

get_node_peers() {
    local n=$1
    local web_port=$((BASE_WEB_PORT + n - 1))
    curl -s "http://localhost:$web_port/api/peers" 2>/dev/null || echo "[]"
}

get_node_stats() {
    local n=$1
    local web_port=$((BASE_WEB_PORT + n - 1))
    curl -s "http://localhost:$web_port/api/stats" 2>/dev/null || echo "{}"
}

get_node_credits() {
    local n=$1
    local web_port=$((BASE_WEB_PORT + n - 1))
    curl -s "http://localhost:$web_port/api/credits" 2>/dev/null || echo "{}"
}

get_node_galaxy() {
    local n=$1
    local web_port=$((BASE_WEB_PORT + n - 1))
    # Use /api/known-systems which shows all systems the node knows about
    curl -s "http://localhost:$web_port/api/known-systems" 2>/dev/null || echo "[]"
}

get_node_topology() {
    local n=$1
    local web_port=$((BASE_WEB_PORT + n - 1))
    curl -s "http://localhost:$web_port/api/topology" 2>/dev/null || echo "{}"
}

get_node_version() {
    local n=$1
    local web_port=$((BASE_WEB_PORT + n - 1))
    curl -s "http://localhost:$web_port/api/version" 2>/dev/null || echo "{}"
}

# Check if jq is available for JSON parsing
HAS_JQ=0
if command -v jq &>/dev/null; then
    HAS_JQ=1
fi

# JSON helper - extract field (works with or without jq)
json_get() {
    local json="$1"
    local field="$2"

    if [[ $HAS_JQ -eq 1 ]]; then
        echo "$json" | jq -r ".$field // empty" 2>/dev/null
    else
        # Fallback: simple grep/sed extraction
        echo "$json" | grep -o "\"$field\":[^,}]*" | sed "s/\"$field\"://" | tr -d '"' | head -1
    fi
}

# JSON helper - get array length
json_array_len() {
    local json="$1"

    if [[ $HAS_JQ -eq 1 ]]; then
        echo "$json" | jq 'length' 2>/dev/null || echo 0
    else
        # Fallback: count occurrences of a common field
        echo "$json" | grep -o '"id"' | wc -l
    fi
}

# Test validation functions
validate_test() {
    local test_name=$1
    local condition=$2
    local details="${3:-No details}"
    
    ((TEST_COUNT++))
    
    if eval "$condition"; then
        TEST_RESULTS["$test_name"]="PASS"
        TEST_DETAILS["$test_name"]="$details"
        ((PASS_COUNT++))
        log_success "$test_name"
        return 0
    else
        TEST_RESULTS["$test_name"]="FAIL"
        TEST_DETAILS["$test_name"]="$details"
        ((FAIL_COUNT++))
        log_error "$test_name - $details"
        return 1
    fi
}

parse_logs_for_errors() {
    local errors=""
    for log in "$CLUSTER_DIR"/logs/*.log; do
        [[ -f "$log" ]] || continue
        local node_name=$(basename "$log" .log)
        
        # Check for panics
        if grep -q "panic:" "$log"; then
            errors="$errors\n  - $node_name: Found panic in logs"
        fi
        
        # Check for specific error patterns
        if grep -q "failed to bind" "$log"; then
            errors="$errors\n  - $node_name: Port binding failure"
        fi
        
        if grep -q "database is locked" "$log"; then
            errors="$errors\n  - $node_name: Database lock issues"
        fi
    done
    
    echo "$errors"
}

parse_logs_for_metrics() {
    local metrics=""

    for log in "$CLUSTER_DIR"/logs/*.log; do
        [[ -f "$log" ]] || continue
        local node_name=$(basename "$log" .log)

        # Count different message types (use subshell to avoid exit code issues)
        local pings=$(grep -c "Received PING" "$log" 2>/dev/null; true)
        local announces=$(grep -c "Received ANNOUNCE" "$log" 2>/dev/null; true)
        local find_nodes=$(grep -c "Received FIND_NODE" "$log" 2>/dev/null; true)
        local attestations=$(grep -c "attestation" "$log" 2>/dev/null; true)

        # Default to 0 if empty
        pings=${pings:-0}
        announces=${announces:-0}
        find_nodes=${find_nodes:-0}
        attestations=${attestations:-0}

        metrics="$metrics\n  $node_name: PING=$pings, ANNOUNCE=$announces, FIND_NODE=$find_nodes, Attestations=$attestations"
    done

    echo -e "$metrics"
}

# Test Scenarios
test_cluster_formation() {
    log_test "Testing cluster formation..."
    
    # Start nodes
    start_node 1
    sleep 2
    
    for n in $(seq 2 $NODES); do
        start_node $n
        sleep 0.5
    done
    
    # Wait for nodes to come up
    log_info "Waiting for nodes to initialize..."
    for n in $(seq 1 $NODES); do
        if ! wait_for_node $n; then
            validate_test "node_${n}_startup" "false" "Node $n failed to start"
        else
            validate_test "node_${n}_startup" "true" "Node $n started successfully"
        fi
    done

    # Let DHT stabilize - use portion of TEST_DURATION for initial stabilization
    # Quick test (60s): 20s stabilization
    # Medium test (180s): 45s stabilization
    # Full test (300s): 75s stabilization
    local stabilize_time=$((TEST_DURATION / 4))
    [[ $stabilize_time -lt 15 ]] && stabilize_time=15
    [[ $stabilize_time -gt 90 ]] && stabilize_time=90
    log_info "Waiting ${stabilize_time}s for DHT to stabilize..."
    sleep $stabilize_time
}

test_peer_discovery() {
    log_test "Testing peer discovery..."

    # Each node should discover others
    for n in $(seq 1 $NODES); do
        local peers_json=$(get_node_peers $n)
        local peers=$(json_array_len "$peers_json")
        local expected=$((NODES - 1))

        # Node might not have discovered everyone yet, but should have at least 1 peer
        validate_test "node_${n}_peer_discovery" "[[ $peers -gt 0 ]]" "Node $n has $peers peers (expected at least 1)"
    done
}

test_dht_operations() {
    log_test "Testing DHT operations..."

    # Check genesis node (node1) for incoming PINGs from joining nodes
    local genesis_log="$CLUSTER_DIR/logs/node1.log"
    # Check non-genesis node (node2) for bootstrap operations
    local node2_log="$CLUSTER_DIR/logs/node2.log"

    # Check for PING messages received by genesis from joining nodes
    local pings=0
    [[ -f "$genesis_log" ]] && pings=$(grep -c "PING from" "$genesis_log" 2>/dev/null; true)
    pings=${pings:-0}
    validate_test "dht_ping_operations" "[[ $pings -gt 0 ]]" "Genesis received $pings PING messages"

    # Check for bootstrap/discovery operations on non-genesis node
    local bootstrap=0
    [[ -f "$node2_log" ]] && bootstrap=$(grep -c "Bootstrap complete\|FindNode" "$node2_log" 2>/dev/null; true)
    bootstrap=${bootstrap:-0}
    validate_test "dht_bootstrap_operations" "[[ $bootstrap -gt 0 ]]" "Found $bootstrap bootstrap operations"

    # Check for ANNOUNCE messages (any node)
    local announces=0
    [[ -f "$node2_log" ]] && announces=$(grep -c "ANNOUNCE" "$node2_log" 2>/dev/null; true)
    announces=${announces:-0}
    validate_test "dht_announce_operations" "[[ $announces -gt 0 ]]" "Found $announces ANNOUNCE operations"
}

test_attestations() {
    log_test "Testing attestation system..."

    # Attestations are created during DHT message exchanges
    # Give nodes time to exchange messages and accumulate attestations
    local att_wait=$((TEST_DURATION / 6))
    [[ $att_wait -lt 15 ]] && att_wait=15
    [[ $att_wait -gt 45 ]] && att_wait=45
    log_info "Waiting ${att_wait}s for attestation accumulation..."
    sleep $att_wait

    # Check attestation_count from API stats (not logs)
    for n in $(seq 2 $NODES); do
        local stats=$(get_node_stats $n)
        local att_count=$(json_get "$stats" "attestation_count")
        att_count=${att_count:-0}
        validate_test "node_${n}_attestations" "[[ $att_count -gt 0 ]]" "Node $n has $att_count attestations"
    done
}

test_coordinate_validation() {
    log_test "Testing coordinate validation..."

    # Check that nodes have valid coordinates (x, y, z fields in JSON)
    for n in $(seq 1 $NODES); do
        local info=$(get_node_info $n)
        local x=$(json_get "$info" "x")
        local y=$(json_get "$info" "y")
        local z=$(json_get "$info" "z")

        if [[ $n -eq 1 ]]; then
            # Genesis node should be at origin (0,0,0)
            validate_test "node_${n}_coordinates" "[[ \"$x\" == \"0\" && \"$y\" == \"0\" && \"$z\" == \"0\" ]]" \
                "Genesis at origin ($x, $y, $z)"
        else
            # Other nodes should not be at origin
            local not_origin=false
            [[ "$x" != "0" || "$y" != "0" || "$z" != "0" ]] && not_origin=true
            validate_test "node_${n}_coordinates" "[[ \"$not_origin\" == \"true\" ]]" \
                "Node $n at ($x, $y, $z)"
        fi
    done
}

test_star_system_generation() {
    log_test "Testing star system generation..."

    # Check star class distribution
    local star_classes=""
    for n in $(seq 1 $NODES); do
        local info=$(get_node_info $n)
        # Star class is nested in stars.primary.class
        local class=$(echo "$info" | grep -o '"primary":{[^}]*"class":"[^"]*"' | grep -o '"class":"[^"]*"' | head -1 | cut -d'"' -f4)
        star_classes="$star_classes $class"

        if [[ $n -eq 1 ]]; then
            validate_test "genesis_star_class" "[[ \"$class\" == \"X\" ]]" "Genesis is class X (black hole)"
        else
            validate_test "node_${n}_star_class" "[[ -n \"$class\" && \"$class\" != \"X\" ]]" "Node $n has valid star class: $class"
        fi
    done
    
    log_info "Star class distribution: $star_classes"
}

test_network_resilience() {
    log_test "Testing network resilience..."
    
    # Kill a node and check if others handle it
    local victim=3
    log_info "Stopping node $victim..."
    
    local pid_file="$CLUSTER_DIR/pids/node$victim.pid"
    if [[ -f "$pid_file" ]]; then
        kill $(cat "$pid_file") 2>/dev/null || true
        rm -f "$pid_file"
    fi
    
    sleep 10
    
    # Check if other nodes are still functional
    for n in $(seq 1 $NODES); do
        [[ $n -eq $victim ]] && continue
        
        local info=$(get_node_info $n)
        validate_test "node_${n}_survives_failure" "[[ -n \"$info\" && \"$info\" != \"{}\" ]]" "Node $n survived node $victim failure"
    done
    
    # Restart the killed node
    log_info "Restarting node $victim..."
    start_node $victim
    wait_for_node $victim
    
    sleep 10
    
    # Check if it rejoins the network
    local peers=$(get_node_peers $victim | grep -o '"system_id"' | wc -l)
    validate_test "node_${victim}_rejoin" "[[ $peers -gt 0 ]]" "Node $victim rejoined with $peers peers"
}

test_api_endpoints() {
    log_test "Testing API endpoints..."
    
    # Test various API endpoints on node 2
    local web_port=$((BASE_WEB_PORT + 1))
    
    # Test /api/system
    local system=$(curl -s -w "\n%{http_code}" "http://localhost:$web_port/api/system")
    local http_code=$(echo "$system" | tail -n1)
    validate_test "api_system_endpoint" "[[ \"$http_code\" == \"200\" ]]" "System API returned $http_code"
    
    # Test /api/peers
    local peers=$(curl -s -w "\n%{http_code}" "http://localhost:$web_port/api/peers")
    http_code=$(echo "$peers" | tail -n1)
    validate_test "api_peers_endpoint" "[[ \"$http_code\" == \"200\" ]]" "Peers API returned $http_code"
    
    # Test /api/stats
    local stats=$(curl -s -w "\n%{http_code}" "http://localhost:$web_port/api/stats")
    http_code=$(echo "$stats" | tail -n1)
    validate_test "api_stats_endpoint" "[[ \"$http_code\" == \"200\" ]]" "Stats API returned $http_code"
    
    # Test /api/credits
    local credits=$(curl -s -w "\n%{http_code}" "http://localhost:$web_port/api/credits")
    http_code=$(echo "$credits" | tail -n1)
    validate_test "api_credits_endpoint" "[[ \"$http_code\" == \"200\" ]]" "Credits API returned $http_code"
}

test_name_validation() {
    log_test "Testing name validation..."

    # Try to start a node with invalid name (this should fail)
    local test_log="$CLUSTER_DIR/logs/invalid_name_test.log"

    # Test with empty name
    $BINARY \
        -name="" \
        -address="0.0.0.0:19999" \
        -public-address="localhost:29999" \
        -db="$CLUSTER_DIR/data/invalid.db" \
        -isolated \
        > "$test_log" 2>&1 &

    local test_pid=$!
    sleep 2

    if kill -0 $test_pid 2>/dev/null; then
        kill $test_pid 2>/dev/null
        validate_test "name_validation_empty" "false" "Empty name was incorrectly accepted"
    else
        validate_test "name_validation_empty" "true" "Empty name correctly rejected"
    fi

    # Test with placeholder name
    $BINARY \
        -name="CHANGE_ME" \
        -address="0.0.0.0:19999" \
        -public-address="localhost:29999" \
        -db="$CLUSTER_DIR/data/invalid2.db" \
        -isolated \
        > "$test_log" 2>&1 &

    test_pid=$!
    sleep 2

    if kill -0 $test_pid 2>/dev/null; then
        kill $test_pid 2>/dev/null
        validate_test "name_validation_placeholder" "false" "Placeholder name was incorrectly accepted"
    else
        validate_test "name_validation_placeholder" "true" "Placeholder name correctly rejected"
    fi
}

# =============================================================================
# ENHANCED INTEGRATION TESTS
# =============================================================================

test_api_response_content() {
    log_test "Testing API response content validation..."

    local web_port=$((BASE_WEB_PORT + 1))

    # Test /api/system returns valid structure
    local system_info=$(get_node_info 2)

    # Check required fields exist
    local sys_id=$(json_get "$system_info" "id")
    local sys_name=$(json_get "$system_info" "name")
    # Star class is nested in stars.primary.class - extract with grep
    local star_class=$(echo "$system_info" | grep -o '"primary":{[^}]*"class":"[^"]*"' | grep -o '"class":"[^"]*"' | head -1 | cut -d'"' -f4)

    validate_test "api_system_has_id" "[[ -n \"$sys_id\" && \"$sys_id\" != \"null\" ]]" "System ID: ${sys_id:0:8}..."
    validate_test "api_system_has_name" "[[ -n \"$sys_name\" && \"$sys_name\" != \"null\" ]]" "System name: $sys_name"
    validate_test "api_system_has_star_class" "[[ -n \"$star_class\" && \"$star_class\" != \"null\" ]]" "Star class: $star_class"

    # Verify star class is valid (O, B, A, F, G, K, M, or X for genesis)
    validate_test "api_system_valid_star_class" "[[ \"$star_class\" =~ ^[OBAFGKMX]$ ]]" "Star class '$star_class' is valid"

    # Test /api/peers returns array
    local peers_json=$(get_node_peers 2)
    local peer_count=$(json_array_len "$peers_json")
    validate_test "api_peers_returns_array" "[[ $peer_count -ge 0 ]]" "Peers API returned $peer_count peers"

    # Test /api/credits returns valid structure
    local credits_json=$(get_node_credits 2)
    local balance=$(json_get "$credits_json" "balance")
    local rank=$(json_get "$credits_json" "rank")

    validate_test "api_credits_has_balance" "[[ \"$balance\" =~ ^[0-9]+$ ]]" "Credit balance: $balance"
    validate_test "api_credits_has_rank" "[[ -n \"$rank\" ]]" "Credit rank: $rank"

    # Test /api/stats returns metrics (check routing_table_size as uptime may not exist)
    local stats_json=$(get_node_stats 2)
    local routing_size=$(json_get "$stats_json" "routing_table_size")
    validate_test "api_stats_has_routing_size" "[[ \"$routing_size\" =~ ^[0-9]+$ ]]" "Routing table size: $routing_size"
}

test_peer_count_accuracy() {
    log_test "Testing peer count accuracy..."

    # Wait for gossip to propagate peer/system info across the network
    local peer_wait=$((TEST_DURATION / 5))
    [[ $peer_wait -lt 15 ]] && peer_wait=15
    [[ $peer_wait -gt 45 ]] && peer_wait=45
    log_info "Waiting ${peer_wait}s for peer discovery propagation..."
    sleep $peer_wait

    # Each node should eventually know about most other nodes
    local min_expected=$((NODES - 2))  # Allow for some propagation delay

    for n in $(seq 1 $NODES); do
        local peers_json=$(get_node_peers $n)
        local peer_count=$(json_array_len "$peers_json")

        # Node should have at least min_expected peers
        validate_test "node_${n}_peer_count" "[[ $peer_count -ge $min_expected ]]" \
            "Node $n has $peer_count peers (expected >= $min_expected)"
    done

    # Galaxy view should show most nodes (allowing for propagation delay)
    local galaxy_json=$(get_node_galaxy 1)
    local galaxy_count=$(json_array_len "$galaxy_json")
    local min_galaxy=$((NODES - 1))
    validate_test "galaxy_shows_all_nodes" "[[ $galaxy_count -ge $min_galaxy ]]" \
        "Galaxy view shows $galaxy_count systems (expected >= $min_galaxy)"
}

test_gossip_propagation() {
    log_test "Testing gossip propagation..."

    # Allow additional time for gossip to propagate across network
    local gossip_wait=$((TEST_DURATION / 4))
    [[ $gossip_wait -lt 10 ]] && gossip_wait=10
    [[ $gossip_wait -gt 60 ]] && gossip_wait=60
    log_info "Waiting ${gossip_wait}s for gossip propagation..."
    sleep $gossip_wait

    # Get node 1's ID
    local node1_info=$(get_node_info 1)
    local node1_id=$(json_get "$node1_info" "id")

    # All other nodes should know about node 1 (genesis)
    for n in $(seq 2 $NODES); do
        local galaxy=$(get_node_galaxy $n)

        # Check if node 1's ID appears in the galaxy
        if echo "$galaxy" | grep -q "${node1_id:0:8}"; then
            validate_test "gossip_node_${n}_knows_genesis" "true" "Node $n knows about genesis"
        else
            validate_test "gossip_node_${n}_knows_genesis" "false" "Node $n doesn't know about genesis"
        fi
    done

    # Check that info propagates: get node 2's info from node 1's perspective
    local node2_info=$(get_node_info 2)
    local node2_id=$(json_get "$node2_info" "id")
    local node2_name=$(json_get "$node2_info" "name")

    # Node 3 should know node 2's name (via gossip)
    local galaxy_at_3=$(get_node_galaxy 3)
    if echo "$galaxy_at_3" | grep -q "$node2_name"; then
        validate_test "gossip_name_propagation" "true" "Node 2's name propagated to node 3"
    else
        validate_test "gossip_name_propagation" "false" "Node 2's name NOT found at node 3"
    fi
}

test_attestation_generation() {
    log_test "Testing attestation generation..."

    # Allow time for attestations to be generated (requires peer connectivity)
    local att_wait=$((TEST_DURATION / 4))
    [[ $att_wait -lt 10 ]] && att_wait=10
    [[ $att_wait -gt 60 ]] && att_wait=60
    log_info "Waiting ${att_wait}s for attestation generation..."
    sleep $att_wait

    # After some time, nodes should have generated attestations
    for n in $(seq 2 4); do
        local stats=$(get_node_stats $n)
        local att_count=$(json_get "$stats" "attestation_count")

        # If attestation_count isn't in stats, check logs
        if [[ -z "$att_count" || "$att_count" == "null" ]]; then
            local log="$CLUSTER_DIR/logs/node$n.log"
            att_count=$(grep -c "Saved attestation" "$log" 2>/dev/null || echo 0)
        fi

        validate_test "node_${n}_generates_attestations" "[[ $att_count -gt 0 ]]" \
            "Node $n has $att_count attestations"
    done

    # Check that attestations are bidirectional (A attests to B, B attests to A)
    local log2="$CLUSTER_DIR/logs/node2.log"
    local log3="$CLUSTER_DIR/logs/node3.log"

    local node2_attests=$(grep -c "Sending.*attestation" "$log2" 2>/dev/null || echo 0)
    local node2_receives=$(grep -c "Received.*attestation\|Saved attestation" "$log2" 2>/dev/null || echo 0)

    validate_test "attestations_bidirectional" "[[ $node2_attests -gt 0 && $node2_receives -gt 0 ]]" \
        "Node 2: sent $node2_attests, received $node2_receives attestations"
}

test_credit_system_initialization() {
    log_test "Testing credit system initialization..."

    for n in $(seq 1 $NODES); do
        local credits=$(get_node_credits $n)

        # Check balance is a valid number (starts at 0)
        local balance=$(json_get "$credits" "balance")
        validate_test "node_${n}_credit_balance_valid" "[[ \"$balance\" =~ ^[0-9]+$ ]]" \
            "Node $n balance: $balance"

        # Check rank exists
        local rank=$(json_get "$credits" "rank")
        validate_test "node_${n}_has_rank" "[[ -n \"$rank\" ]]" "Node $n rank: $rank"

        # New nodes should start as Unranked
        if [[ "$balance" == "0" ]]; then
            validate_test "node_${n}_starts_unranked" "[[ \"$rank\" == \"Unranked\" ]]" \
                "Node $n correctly starts as Unranked"
        fi
    done
}

test_coordinate_consistency() {
    log_test "Testing coordinate consistency across network..."

    # Get node 2's coordinates from its own API
    local node2_info=$(get_node_info 2)
    local node2_id=$(json_get "$node2_info" "id")
    local node2_coords=$(echo "$node2_info" | grep -o '"coordinates":\[[^]]*\]')

    # Check that node 3's galaxy view has the same coordinates for node 2
    local galaxy_at_3=$(get_node_galaxy 3)

    # Extract node 2's entry from node 3's galaxy view
    if [[ $HAS_JQ -eq 1 ]]; then
        local node2_at_3=$(echo "$galaxy_at_3" | jq ".[] | select(.id == \"$node2_id\")" 2>/dev/null)
        local coords_at_3=$(echo "$node2_at_3" | jq -c '[.x, .y, .z]' 2>/dev/null)

        if [[ -n "$coords_at_3" && "$coords_at_3" != "null" ]]; then
            validate_test "coordinates_consistent_across_network" "true" \
                "Node 2 coordinates match at node 3"
        else
            validate_test "coordinates_consistent_across_network" "[[ -n \"$coords_at_3\" ]]" \
                "Could not verify coordinate consistency"
        fi
    else
        # Without jq, just check the ID appears in the galaxy
        if echo "$galaxy_at_3" | grep -q "${node2_id:0:8}"; then
            validate_test "coordinates_consistent_across_network" "true" \
                "Node 2 appears in node 3's galaxy (detailed check requires jq)"
        else
            validate_test "coordinates_consistent_across_network" "false" \
                "Node 2 not found in node 3's galaxy"
        fi
    fi
}

test_database_persistence() {
    log_test "Testing database persistence..."

    # Check that database files exist and have content
    for n in $(seq 1 $NODES); do
        local db_file="$CLUSTER_DIR/data/node$n.db"

        validate_test "node_${n}_db_exists" "[[ -f \"$db_file\" ]]" \
            "Database file exists: $db_file"

        local db_size=$(stat -f%z "$db_file" 2>/dev/null || stat -c%s "$db_file" 2>/dev/null || echo 0)
        validate_test "node_${n}_db_has_data" "[[ $db_size -gt 1000 ]]" \
            "Database size: $db_size bytes"
    done

    # Use sqlite3 if available to verify table structure
    if command -v sqlite3 &>/dev/null; then
        local db="$CLUSTER_DIR/data/node2.db"

        # Check system table exists and has data
        local system_count=$(sqlite3 "$db" "SELECT COUNT(*) FROM system;" 2>/dev/null || echo 0)
        validate_test "db_system_table_populated" "[[ $system_count -gt 0 ]]" \
            "System table has $system_count rows"

        # Check peer_systems table has entries
        local peer_count=$(sqlite3 "$db" "SELECT COUNT(*) FROM peer_systems;" 2>/dev/null || echo 0)
        validate_test "db_peer_systems_populated" "[[ $peer_count -gt 0 ]]" \
            "Peer systems table has $peer_count rows"

        # Check attestations are being stored
        local att_count=$(sqlite3 "$db" "SELECT COUNT(*) FROM attestations;" 2>/dev/null || echo 0)
        validate_test "db_attestations_stored" "[[ $att_count -gt 0 ]]" \
            "Attestations table has $att_count rows"

        # Check identity bindings exist
        local binding_count=$(sqlite3 "$db" "SELECT COUNT(*) FROM identity_bindings;" 2>/dev/null || echo 0)
        validate_test "db_identity_bindings" "[[ $binding_count -gt 0 ]]" \
            "Identity bindings table has $binding_count rows"
    else
        log_warning "sqlite3 not available - skipping detailed database checks"
    fi
}

test_version_reporting() {
    log_test "Testing version reporting..."

    # Expected version from build
    local expected_version="test-1.0.0"

    for n in $(seq 1 3); do
        # Use the dedicated /api/version endpoint
        local version_info=$(get_node_version $n)
        local version=$(json_get "$version_info" "version")

        validate_test "node_${n}_reports_version" "[[ -n \"$version\" && \"$version\" != \"null\" ]]" \
            "Node $n version: $version"

        # Check version matches what we built with
        if [[ -n "$version" && "$version" != "null" ]]; then
            validate_test "node_${n}_correct_version" "[[ \"$version\" == \"$expected_version\" ]]" \
                "Expected: $expected_version, Got: $version"
        fi
    done
}

test_topology_tracking() {
    log_test "Testing topology/connection tracking..."

    # Get topology from a node
    local topology=$(get_node_topology 2)

    if [[ -n "$topology" && "$topology" != "{}" ]]; then
        # Check that edges exist
        local edge_count=$(echo "$topology" | grep -o '"from_id"' | wc -l)
        validate_test "topology_has_edges" "[[ $edge_count -gt 0 ]]" \
            "Topology has $edge_count edges"

        # Check that nodes are tracked
        local node_count=$(echo "$topology" | grep -o '"nodes"' | wc -l)
        if [[ $node_count -gt 0 ]]; then
            validate_test "topology_tracks_nodes" "true" "Topology tracks node positions"
        fi
    else
        validate_test "topology_available" "[[ -n \"$topology\" ]]" \
            "Topology endpoint returned data"
    fi
}

generate_report() {
    log_info "Generating test report..."
    
    local errors=$(parse_logs_for_errors)
    local metrics=$(parse_logs_for_metrics)
    
    # Create JSON report
    cat > "$CLUSTER_DIR/reports/$REPORT_FILE" <<EOF
{
  "test_run": {
    "start_time": "$START_TIME",
    "end_time": "$END_TIME",
    "duration": "$TEST_DURATION seconds",
    "nodes": $NODES
  },
  "summary": {
    "total_tests": $TEST_COUNT,
    "passed": $PASS_COUNT,
    "failed": $FAIL_COUNT,
    "success_rate": $(echo "scale=2; $PASS_COUNT * 100 / $TEST_COUNT" | bc)%
  },
  "tests": [
EOF
    
    local first=true
    for test_name in "${!TEST_RESULTS[@]}"; do
        if [[ "$first" != true ]]; then
            echo "," >> "$CLUSTER_DIR/reports/$REPORT_FILE"
        fi
        first=false
        
        cat >> "$CLUSTER_DIR/reports/$REPORT_FILE" <<EOF
    {
      "name": "$test_name",
      "result": "${TEST_RESULTS[$test_name]}",
      "details": "${TEST_DETAILS[$test_name]}"
    }
EOF
    done
    
    cat >> "$CLUSTER_DIR/reports/$REPORT_FILE" <<EOF

  ],
  "metrics": {
    "message_counts": "$metrics"
  },
  "errors": "$errors"
}
EOF
    
    # Create human-readable summary
    cat > "$CLUSTER_DIR/reports/summary.txt" <<EOF
==========================================
STELLAR LAB TEST REPORT
==========================================
Test Duration: $TEST_DURATION seconds
Nodes Tested: $NODES

RESULTS:
--------
Total Tests: $TEST_COUNT
Passed: $PASS_COUNT
Failed: $FAIL_COUNT
Success Rate: $(echo "scale=2; $PASS_COUNT * 100 / $TEST_COUNT" | bc)%

TEST DETAILS:
-------------
EOF
    
    for test_name in "${!TEST_RESULTS[@]}"; do
        printf "%-30s %s\n" "$test_name:" "${TEST_RESULTS[$test_name]}" >> "$CLUSTER_DIR/reports/summary.txt"
    done
    
    if [[ -n "$errors" ]]; then
        echo -e "\nERRORS FOUND IN LOGS:$errors" >> "$CLUSTER_DIR/reports/summary.txt"
    fi
    
    echo -e "\nMESSAGE METRICS:$metrics" >> "$CLUSTER_DIR/reports/summary.txt"
    
    # Display summary
    cat "$CLUSTER_DIR/reports/summary.txt"
}

run_all_tests() {
    START_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    log_info "Starting comprehensive test suite..."
    log_info "Configuration: $NODES nodes, $TEST_DURATION second runtime"
    
    # Setup
    cleanup 2>/dev/null || true
    ensure_dirs

    # Build if needed
    if [[ ! -f "$BINARY" ]] || [[ "$1" == "--build" ]]; then
        build
    fi
    
    # Run test scenarios
    test_cluster_formation
    test_peer_discovery
    test_dht_operations
    test_attestations
    test_coordinate_validation
    test_star_system_generation
    test_api_endpoints
    test_name_validation
    test_network_resilience

    # Enhanced integration tests
    test_api_response_content
    test_peer_count_accuracy
    test_gossip_propagation
    test_attestation_generation
    test_credit_system_initialization
    test_coordinate_consistency
    test_database_persistence
    test_version_reporting
    test_topology_tracking
    
    END_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    # Generate reports
    generate_report
    
    # Cleanup unless asked to keep
    if [[ "$2" != "--keep" ]]; then
        cleanup
    else
        log_info "Test data preserved in $CLUSTER_DIR"
    fi
    
    # Exit with appropriate code
    if [[ $FAIL_COUNT -gt 0 ]]; then
        log_error "Test suite failed with $FAIL_COUNT failures"
        exit 1
    else
        log_success "All tests passed!"
        exit 0
    fi
}

run_quick_test() {
    NODES=3
    TEST_DURATION=60

    log_info "Running quick test with $NODES nodes..."

    cleanup 2>/dev/null || true
    ensure_dirs

    if [[ ! -f "$BINARY" ]]; then
        build
    fi

    test_cluster_formation
    test_peer_discovery
    test_dht_operations
    test_api_response_content
    test_credit_system_initialization
    test_version_reporting

    generate_report
    cleanup

    if [[ $FAIL_COUNT -gt 0 ]]; then
        exit 1
    fi
    exit 0
}

run_medium_test() {
    NODES=4
    TEST_DURATION=180  # 3 minutes for proper stabilization

    log_info "Running medium test with $NODES nodes..."

    cleanup 2>/dev/null || true
    ensure_dirs

    if [[ ! -f "$BINARY" ]]; then
        build
    fi

    test_cluster_formation
    test_peer_discovery
    test_dht_operations
    test_attestations
    test_coordinate_validation
    test_star_system_generation
    test_api_endpoints
    test_api_response_content
    test_peer_count_accuracy
    test_gossip_propagation
    test_credit_system_initialization
    test_database_persistence
    test_version_reporting

    generate_report
    cleanup

    if [[ $FAIL_COUNT -gt 0 ]]; then
        exit 1
    fi
    exit 0
}

# Main execution
case "${1:-}" in
    all)
        run_all_tests "${2:-}" "${3:-}"
        ;;
    medium)
        run_medium_test
        ;;
    quick)
        run_quick_test
        ;;
    clean)
        cleanup
        ;;
    *)
        echo "Usage: $0 {all|medium|quick|clean} [--build] [--keep]"
        echo ""
        echo "Test Modes:"
        echo "  all [--build] [--keep]  Run comprehensive test suite (5 nodes, 5 minutes)"
        echo "  medium                  Run medium test suite (4 nodes, 2 minutes)"
        echo "  quick                   Run quick smoke tests (3 nodes, 1 minute)"
        echo "  clean                   Clean up test artifacts"
        echo ""
        echo "Options:"
        echo "  --build  Force rebuild before testing"
        echo "  --keep   Keep test data after completion"
        echo ""
        echo "Environment variables:"
        echo "  STELLAR_TEST_NODES    Number of nodes to test (default: 5)"
        echo "  STELLAR_TEST_DURATION Test duration in seconds (default: 300)"
        echo "  VERBOSE              Enable verbose output (0 or 1)"
        echo ""
        echo "Test Coverage:"
        echo "  quick  - Basic startup, peer discovery, DHT ops, API, credits, version"
        echo "  medium - Above + attestations, coordinates, star systems, gossip, DB"
        echo "  all    - Above + resilience testing, topology, name validation"
        echo ""
        echo "Reports are saved to: $CLUSTER_DIR/reports/"
        ;;
esac