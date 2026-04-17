#!/bin/bash
# Reset the Gilded Guardian practice range
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

case "${1:-soft}" in
  soft)
    echo "Soft reset: restarting all swarm nodes..."
    docker compose restart controller-1 controller-2 controller-3 \
      sensor-1 sensor-2 \
      boomer-1 boomer-2 boomer-3 boomer-4 boomer-5 \
      scenario
    echo "Done. All nodes restarted with fresh in-memory state."
    ;;
  hard)
    echo "Hard reset: destroying and recreating all containers..."
    docker compose down -v
    docker compose up -d
    echo "Done. Fresh environment."
    ;;
  full)
    echo "Full reset: regenerating configs, rebuilding images..."
    go run ./cmd/configgen/
    docker compose down -v
    docker compose build --no-cache
    docker compose up -d
    echo "Done. New UUIDs, new keys, fresh everything."
    ;;
  *)
    echo "Usage: $0 [soft|hard|full]"
    echo "  soft  - restart containers (default)"
    echo "  hard  - destroy and recreate"
    echo "  full  - new configs, rebuild, redeploy"
    exit 1
    ;;
esac
