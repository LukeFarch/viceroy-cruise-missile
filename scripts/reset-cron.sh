#!/bin/bash
# Hard reset the practice range. Intended to be invoked from cron.
#
# Honors environment overrides:
#   VICEROY_ROOT   repo root (defaults to the directory containing this script's parent)
#   TEAM_PASS      password to set on the restored team accounts (MUST be set for any
#                  shared deployment — see SECURITY.md).
set -eu

VICEROY_ROOT="${VICEROY_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
cd "$VICEROY_ROOT"

TEAM_PASS="${TEAM_PASS:-phoenix123}"

docker compose down -v 2>&1
sleep 3
docker compose up -d 2>&1
sleep 5

# Re-add team accounts (volumes were wiped)
docker compose exec -T -e TEAM_PASS="$TEAM_PASS" attack-station sh -c '
  sed -i "s/PasswordAuthentication no/PasswordAuthentication yes/" /etc/ssh/sshd_config
  chmod 1777 /var/www
  adduser -D -G team -s /bin/bash dash 2>/dev/null || true
  adduser -D -G team -s /bin/bash laura 2>/dev/null || true
  echo "dash:${TEAM_PASS}"  | chpasswd
  echo "laura:${TEAM_PASS}" | chpasswd
  DOCKER_GID=$(stat -c "%g" /var/run/docker.sock)
  addgroup -g $DOCKER_GID docker 2>/dev/null || true
  addgroup laura docker 2>/dev/null
  addgroup dash docker 2>/dev/null
  kill -HUP 1
' 2>&1

echo "$(date): Practice range reset complete — users re-added" >> /tmp/viceroy-resets.log
