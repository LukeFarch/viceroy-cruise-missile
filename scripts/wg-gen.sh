#!/usr/bin/env bash
# wg-gen.sh — generate a WireGuard server config plus N peer configs for a
# Viceroy practice-range team.
#
# Usage:
#   ./scripts/wg-gen.sh --peers <N> --endpoint <host:port> [--port <port>]
#                       [--ssh-port <port>] [--scoreboard-port <port>]
#                       [--names name1,name2,...]
#
# Output:
#   wireguard/server.conf
#   wireguard/peers/<name>.conf   (one per peer)
#
# The server private key and each peer private key are written only to
# local files. .gitignore excludes wireguard/server.conf, wireguard/peers/,
# and *.key so they never get committed.

set -euo pipefail

PEERS=""
ENDPOINT=""
SERVER_PORT="51820"
SSH_HOST_PORT="${SSH_HOST_PORT:-2222}"
SCOREBOARD_PORT="${SCOREBOARD_PORT:-8080}"
NAMES=""

usage() {
    sed -n '2,15p' "$0"
    exit 1
}

while [ $# -gt 0 ]; do
    case "$1" in
        --peers)             PEERS="$2"; shift 2 ;;
        --endpoint)          ENDPOINT="$2"; shift 2 ;;
        --port)              SERVER_PORT="$2"; shift 2 ;;
        --ssh-port)          SSH_HOST_PORT="$2"; shift 2 ;;
        --scoreboard-port)   SCOREBOARD_PORT="$2"; shift 2 ;;
        --names)             NAMES="$2"; shift 2 ;;
        -h|--help)           usage ;;
        *) echo "unknown flag: $1" >&2; usage ;;
    esac
done

if [ -z "$PEERS" ] || [ -z "$ENDPOINT" ]; then
    usage
fi

if ! command -v wg >/dev/null 2>&1; then
    echo "error: 'wg' binary not found. Install wireguard-tools first." >&2
    exit 2
fi

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WG_DIR="$REPO_ROOT/wireguard"
PEERS_DIR="$WG_DIR/peers"
SERVER_TPL="$WG_DIR/server.conf.tpl"
PEER_TPL="$WG_DIR/peer.conf.tpl"

mkdir -p "$PEERS_DIR"
chmod 700 "$WG_DIR" "$PEERS_DIR"

if [ ! -f "$SERVER_TPL" ] || [ ! -f "$PEER_TPL" ]; then
    echo "error: templates missing in $WG_DIR" >&2
    exit 3
fi

# --- Generate server keypair ---
SERVER_PRIV="$(wg genkey)"
SERVER_PUB="$(echo "$SERVER_PRIV" | wg pubkey)"

# --- Resolve peer name list ---
if [ -n "$NAMES" ]; then
    IFS=',' read -r -a NAME_ARR <<< "$NAMES"
    if [ "${#NAME_ARR[@]}" -ne "$PEERS" ]; then
        echo "error: --names count (${#NAME_ARR[@]}) does not match --peers ($PEERS)" >&2
        exit 4
    fi
else
    NAME_ARR=()
    for i in $(seq 1 "$PEERS"); do
        NAME_ARR+=("peer$i")
    done
fi

PEER_BLOCKS=""

for i in $(seq 1 "$PEERS"); do
    NAME="${NAME_ARR[$((i-1))]}"
    PEER_PRIV="$(wg genkey)"
    PEER_PUB="$(echo "$PEER_PRIV" | wg pubkey)"
    PEER_ADDR="10.8.0.$((i+1))"

    # server-side [Peer] block
    PEER_BLOCKS+="
[Peer]
# $NAME
PublicKey = $PEER_PUB
AllowedIPs = $PEER_ADDR/32
"

    # peer config file
    PEER_CONF="$PEERS_DIR/$NAME.conf"
    sed \
        -e "s|{{PEER_PRIVKEY}}|$PEER_PRIV|" \
        -e "s|{{PEER_ADDRESS}}|$PEER_ADDR|" \
        -e "s|{{SERVER_PUBKEY}}|$SERVER_PUB|" \
        -e "s|{{SERVER_ENDPOINT}}|$ENDPOINT|" \
        -e "s|{{SSH_HOST_PORT}}|$SSH_HOST_PORT|" \
        -e "s|{{SCOREBOARD_PORT}}|$SCOREBOARD_PORT|" \
        "$PEER_TPL" > "$PEER_CONF"
    chmod 600 "$PEER_CONF"
    echo "wrote $PEER_CONF"
done

# --- Write server config ---
SERVER_CONF="$WG_DIR/server.conf"
awk -v priv="$SERVER_PRIV" -v port="$SERVER_PORT" -v blocks="$PEER_BLOCKS" '
    { gsub(/\{\{SERVER_PRIVKEY\}\}/, priv);
      gsub(/\{\{SERVER_PORT\}\}/, port);
      gsub(/\{\{PEER_BLOCKS\}\}/, blocks);
      print }
' "$SERVER_TPL" > "$SERVER_CONF"
chmod 600 "$SERVER_CONF"
echo "wrote $SERVER_CONF"

cat <<EOF

Next steps:
  1. Bring the server up on the range host:
       sudo wg-quick up $SERVER_CONF
  2. Hand wireguard/peers/<name>.conf to each team member (encrypted channel).
  3. Make sure docker-compose publishes the attack-station on the wg interface:
       SSH_HOST_PORT=$SSH_HOST_PORT SCOREBOARD_PORT=$SCOREBOARD_PORT \\
       BIND_ADDR=10.8.0.1 docker compose up -d
EOF
