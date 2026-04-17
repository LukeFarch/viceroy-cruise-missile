#!/bin/bash
# Add a team member to the attack station
set -e

if [ $# -lt 2 ]; then
  echo "Usage: $0 <username> <ssh-public-key>"
  echo "Example: $0 alice 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... alice@laptop'"
  exit 1
fi

USERNAME="$1"
PUBKEY="$2"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

# Save key to keys directory
mkdir -p keys
echo "$PUBKEY" > "keys/${USERNAME}.pub"

# Add user inside attack station container
docker compose exec attack-station sh -c "
  adduser -D -G team -s /bin/bash '${USERNAME}' 2>/dev/null || true
  mkdir -p '/home/${USERNAME}/.ssh'
  echo '${PUBKEY}' > '/home/${USERNAME}/.ssh/authorized_keys'
  chmod 700 '/home/${USERNAME}/.ssh'
  chmod 600 '/home/${USERNAME}/.ssh/authorized_keys'
  chown -R '${USERNAME}:team' '/home/${USERNAME}'
"

echo "User '${USERNAME}' added to attack station"
echo "Connect with: ssh ${USERNAME}@<host-ip> -p 2222"
