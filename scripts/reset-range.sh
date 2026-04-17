#!/bin/bash
# Reset the Gilded Guardian practice range
# Runs from INSIDE the attack station container (mantis-attack-station)
# Talks to Docker API via mounted socket to restart swarm containers
#
# Usage: reset-range [1|2|all]
#   1   - reset swarm 1 only
#   2   - reset swarm 2 only
#   all - reset both swarms (default)

set -euo pipefail

RESET_LOCK="/tmp/.reset-in-progress"
DOCKER_SOCKET="/var/run/docker.sock"
SWARM_SELECTOR="${1:-all}"

# --- Build container lists per swarm ---
build_swarm1() {
    local containers=()
    for i in $(seq 1 5); do
        containers+=("mantis-controller-$i")
    done
    for i in $(seq 1 6); do
        containers+=("mantis-sensor-$i")
    done
    for i in $(seq 1 15); do
        containers+=("mantis-boomer-$i")
    done
    containers+=("mantis-scenario")
    echo "${containers[@]}"
}

build_swarm2() {
    local containers=()
    for i in $(seq 1 5); do
        containers+=("mantis-s2-controller-$i")
    done
    for i in $(seq 1 6); do
        containers+=("mantis-s2-sensor-$i")
    done
    for i in $(seq 1 15); do
        containers+=("mantis-s2-boomer-$i")
    done
    containers+=("mantis-s2-scenario")
    echo "${containers[@]}"
}

# --- Select containers based on argument ---
SWARM_CONTAINERS=()
case "$SWARM_SELECTOR" in
    1)
        read -ra SWARM_CONTAINERS <<< "$(build_swarm1)"
        SWARM_LABEL="SWARM 1"
        ;;
    2)
        read -ra SWARM_CONTAINERS <<< "$(build_swarm2)"
        SWARM_LABEL="SWARM 2"
        ;;
    all)
        read -ra S1 <<< "$(build_swarm1)"
        read -ra S2 <<< "$(build_swarm2)"
        SWARM_CONTAINERS=("${S1[@]}" "${S2[@]}")
        SWARM_LABEL="ALL SWARMS"
        ;;
    *)
        echo "Usage: reset-range [1|2|all]"
        exit 1
        ;;
esac

TOTAL=${#SWARM_CONTAINERS[@]}

# --- Preflight checks ---

if [ ! -S "$DOCKER_SOCKET" ]; then
    echo ""
    echo "  ERROR: Cannot reset from here -- Docker socket not available."
    echo "  Ask your team lead to run: make reset-hard"
    echo ""
    exit 1
fi

# Clean stale lock (older than 5 minutes)
if [ -f "$RESET_LOCK" ]; then
    find "$RESET_LOCK" -mmin +5 -delete 2>/dev/null || true
fi

# Check if someone else is resetting
if [ -f "$RESET_LOCK" ]; then
    lock_user=$(cat "$RESET_LOCK" 2>/dev/null)
    echo ""
    echo "  RESET ALREADY IN PROGRESS by ${lock_user}"
    echo "  Please wait..."
    echo ""
    exit 1
fi

# Check who else is logged in
ACTIVE_USERS=$(ps aux | grep "sshd:.*@" | grep -v "$(whoami)" | grep -v grep | awk '{print $1}' | sort -u) || true

if [ -n "$ACTIVE_USERS" ]; then
    echo ""
    echo "  WARNING: Other users are currently connected:"
    for u in $ACTIVE_USERS; do
        echo "    - $u"
    done
    echo ""
    read -p "  They will lose their Sliver sessions. Continue? (y/N): " confirm
    if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
        echo "  Reset cancelled."
        exit 0
    fi
fi

# --- Acquire lock ---
echo "$(whoami)" > "$RESET_LOCK"
trap 'rm -f "$RESET_LOCK"' EXIT

echo ""
echo "  ============================================"
echo "  GILDED GUARDIAN -- RESETTING ${SWARM_LABEL}"
echo "  ============================================"
echo ""
echo "  This will:"
echo "    - Restart $TOTAL containers"
echo "    - Reset scenario timer(s) to 0"
echo "    - Clear all strikes and targets"
echo "    - Restart beacon callbacks"
echo "    - Kill any running Sliver/listener processes"
echo ""
echo "  Estimated time: ~45 seconds"
echo ""

# Kill any existing Sliver and listener processes so they don't conflict
echo "  [1/4] Cleaning up old processes..."
killall sliver-server 2>/dev/null || true
killall python3 2>/dev/null || true
sleep 1

# --- Stop all swarm containers ---
echo "  [2/4] Stopping $TOTAL containers..."
COUNT=0
STOP_FAILURES=0
for container in "${SWARM_CONTAINERS[@]}"; do
    if ! curl -sf --unix-socket "$DOCKER_SOCKET" -X POST "http://localhost/containers/$container/stop?t=2" > /dev/null 2>&1; then
        STOP_FAILURES=$((STOP_FAILURES + 1))
    fi
    COUNT=$((COUNT + 1))
    printf "\r  Stopped %d/%d" $COUNT $TOTAL
done
echo ""
if [ $STOP_FAILURES -gt 0 ]; then
    echo "  (${STOP_FAILURES} containers were already stopped or not found)"
fi

# --- Start all swarm containers ---
echo "  [3/4] Starting $TOTAL containers..."
COUNT=0
START_FAILURES=0
FAILED_NAMES=()
for container in "${SWARM_CONTAINERS[@]}"; do
    if ! curl -sf --unix-socket "$DOCKER_SOCKET" -X POST "http://localhost/containers/$container/start" > /dev/null 2>&1; then
        STATE=$(curl -s --unix-socket "$DOCKER_SOCKET" "http://localhost/containers/$container/json" 2>/dev/null | \
            python3 -c "import json,sys; print(json.load(sys.stdin)['State']['Running'])" 2>/dev/null || echo "Unknown")
        if [ "$STATE" != "True" ]; then
            START_FAILURES=$((START_FAILURES + 1))
            FAILED_NAMES+=("$container")
        fi
    fi
    COUNT=$((COUNT + 1))
    printf "\r  Started %d/%d" $COUNT $TOTAL
done
echo ""

if [ $START_FAILURES -gt 0 ]; then
    echo ""
    echo "  WARNING: ${START_FAILURES} container(s) failed to start:"
    for name in "${FAILED_NAMES[@]}"; do
        echo "    - $name"
    done
    echo "  Run 'docker ps -a' on the host to investigate."
fi

# Wait for grace period
echo "  [4/4] Waiting for nodes to initialize (30s)..."
sleep 30

# --- Verify results ---
ALIVE=0
DEAD_NAMES=()
for container in "${SWARM_CONTAINERS[@]}"; do
    STATE=$(curl -s --unix-socket "$DOCKER_SOCKET" "http://localhost/containers/$container/json" 2>/dev/null | \
        python3 -c "import json,sys; print(json.load(sys.stdin)['State']['Running'])" 2>/dev/null || echo "Unknown")
    if [ "$STATE" = "True" ]; then
        ALIVE=$((ALIVE + 1))
    else
        DEAD_NAMES+=("$container")
    fi
done

echo ""
echo "  ============================================"
if [ $ALIVE -eq $TOTAL ]; then
    echo "  RESET COMPLETE -- ALL NODES ONLINE"
else
    echo "  RESET COMPLETE -- WARNING: SOME NODES DOWN"
fi
echo "  ============================================"
echo "  Containers online: ${ALIVE}/${TOTAL} (${SWARM_LABEL})"
if [ ${#DEAD_NAMES[@]} -gt 0 ]; then
    echo "  OFFLINE:"
    for name in "${DEAD_NAMES[@]}"; do
        echo "    - $name"
    done
fi
echo "  Strikes: 0/3"
echo "  Timer: starting now"
echo ""
echo "  Scoreboards:"
echo "    Swarm 1: http://${RANGE_ENDPOINT:-127.0.0.1}:8080"
echo "    Swarm 2: http://${RANGE_ENDPOINT:-127.0.0.1}:8081"
echo ""
echo "  Next steps:"
echo "    1. Start sliver-server"
echo "    2. Generate implant(s) and start listener"
echo "    3. Serve implant files:"
echo "       Swarm 1: golden.exe on port 8443"
echo "       Swarm 2: payload.bin on port 9443"
echo "    4. Catch sessions and complete the mission"
echo "  ============================================"
echo ""
