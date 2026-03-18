#!/bin/bash
# dev-cluster.sh - Spin up a local N-node cluster from source
# Uses -isolated flag to create a completely separate test network

set -e

CLUSTER_DIR=".dev-cluster"
NODES=${STELLAR_DEV_NODES:-5}
BASE_WEB_PORT=8081
BASE_DHT_PORT=7871
BINARY="./stellar-lab"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

ensure_dirs() {
    mkdir -p "$CLUSTER_DIR"/{data,logs,pids}
}

build() {
    echo -e "${YELLOW}Building from source...${NC}"
    go build -o "$BINARY" .
    echo -e "${GREEN}Build complete${NC}"
}

start_node() {
    local n=$1
    local web_port=$((BASE_WEB_PORT + n - 1))
    local dht_port=$((BASE_DHT_PORT + n - 1))
    local name="DevNode$n"
    local db="$CLUSTER_DIR/data/node$n.db"
    local log="$CLUSTER_DIR/logs/node$n.log"
    local pid_file="$CLUSTER_DIR/pids/node$n.pid"
    
    # Skip if already running
    if [[ -f "$pid_file" ]] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
        echo -e "${YELLOW}Node $n already running (PID $(cat "$pid_file"))${NC}"
        return
    fi
    
    # All nodes use -isolated flag to prevent any contact with production network
    # All nodes bootstrap to node 1's DHT port
    # Node 1 bootstrapping to itself triggers genesis mode (becomes Class X black hole)
    # Nodes 2+ successfully connect to node 1 and discover each other via DHT
    $BINARY \
        -name="$name" \
        -address="0.0.0.0:$web_port" \
        -public-address="localhost:$dht_port" \
        -db="$db" \
        -isolated \
        -bootstrap="localhost:$BASE_DHT_PORT" \
        > "$log" 2>&1 &
    
    echo $! > "$pid_file"
    echo -e "${GREEN}Started node $n${NC} - Web: $web_port, DHT: $dht_port, PID: $!"
}

start() {
    ensure_dirs
    
    if [[ ! -f "$BINARY" ]] || [[ "$1" == "--build" ]]; then
        build
    fi
    
    echo -e "${YELLOW}Starting $NODES nodes in isolated mode...${NC}"
    
    # Start node 1 first, give it a moment to bind
    start_node 1
    sleep 2
    
    # Start remaining nodes
    for n in $(seq 2 $NODES); do
        start_node $n
        sleep 0.5
    done
    
    echo ""
    echo -e "${GREEN}Cluster started.${NC} Web UIs at:"
    for n in $(seq 1 $NODES); do
        echo "  Node $n: http://localhost:$((BASE_WEB_PORT + n - 1))"
    done
    echo ""
    echo -e "${YELLOW}Note:${NC} Node 1 is the genesis black hole (Class X) at origin"
}

stop() {
    echo -e "${YELLOW}Stopping cluster...${NC}"
    for pid_file in "$CLUSTER_DIR"/pids/*.pid; do
        [[ -f "$pid_file" ]] || continue
        local pid=$(cat "$pid_file")
        local node=$(basename "$pid_file" .pid)
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid"
            echo -e "Stopped $node (PID $pid)"
        fi
        rm -f "$pid_file"
    done
    echo -e "${GREEN}Cluster stopped${NC}"
}

status() {
    echo -e "${YELLOW}Cluster Status:${NC}"
    echo ""
    
    for n in $(seq 1 $NODES); do
        local pid_file="$CLUSTER_DIR/pids/node$n.pid"
        local web_port=$((BASE_WEB_PORT + n - 1))
        
        if [[ -f "$pid_file" ]] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
            # Try to get system info
            local info=$(curl -s --max-time 2 "http://localhost:$web_port/api/system" 2>/dev/null)
            if [[ -n "$info" ]]; then
                local name=$(echo "$info" | grep -o '"name":"[^"]*"' | cut -d'"' -f4)
                local class=$(echo "$info" | grep -o '"star_class":"[^"]*"' | cut -d'"' -f4)
                local peers=$(curl -s --max-time 2 "http://localhost:$web_port/api/peers" 2>/dev/null | grep -o '"system_id"' | wc -l)
                echo -e "  ${GREEN}●${NC} Node $n: $name ($class) - $peers peers - :$web_port"
            else
                echo -e "  ${YELLOW}●${NC} Node $n: Running but not responding - :$web_port"
            fi
        else
            echo -e "  ${RED}●${NC} Node $n: Stopped"
        fi
    done
}

logs() {
    local node=${1:-1}
    local log="$CLUSTER_DIR/logs/node$node.log"
    
    if [[ -f "$log" ]]; then
        tail -f "$log"
    else
        echo "No log file for node $node"
    fi
}

clean() {
    stop 2>/dev/null || true
    echo -e "${YELLOW}Cleaning cluster data...${NC}"
    rm -rf "$CLUSTER_DIR"
    echo -e "${GREEN}Clean complete${NC}"
}

restart() {
    stop
    sleep 1
    start "$@"
}

case "${1:-}" in
    build)   build ;;
    start)   start "${2:-}" ;;
    stop)    stop ;;
    restart) restart "${2:-}" ;;
    status)  status ;;
    logs)    logs "${2:-}" ;;
    clean)   clean ;;
    *)
        echo "Usage: $0 {build|start|stop|restart|status|logs [n]|clean}"
        echo ""
        echo "  build          Build binary from source"
        echo "  start          Start cluster (builds if needed)"
        echo "  start --build  Force rebuild before starting"
        echo "  stop           Stop all nodes"
        echo "  restart        Stop then start"
        echo "  status         Show node status and peer counts"
        echo "  logs [n]       Tail logs for node n (default: 1)"
        echo "  clean          Stop and wipe all data"
        echo ""
        echo "Set STELLAR_DEV_NODES=N to change cluster size (default: 5)"
        echo ""
        echo "This creates a fully isolated test network that never contacts"
        echo "production seed nodes. Node 1 becomes the genesis black hole."
        ;;
esac
