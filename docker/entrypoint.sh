#!/bin/bash
# Create NixOS-compatible paths in /run (tmpfs, needs setup each boot)
mkdir -p /run/current-system/sw/bin
ln -sf /usr/bin/python3 /run/current-system/sw/bin/python3
ln -sf /usr/bin/python3 /run/current-system/sw/bin/python3.11
ln -sf /bin/bash /run/current-system/sw/bin/bash
ln -sf /bin/cat /run/current-system/sw/bin/cat
ln -sf /bin/ls /run/current-system/sw/bin/ls
ln -sf /usr/bin/env /run/current-system/sw/bin/env

# Create daemon socket directories
mkdir -p /run/commsDaemon /run/electionDaemon /run/hwDaemon /run/fetcher
chmod 777 /run/commsDaemon /run/electionDaemon /run/hwDaemon

# Create /tmp/payloads for team script uploads
mkdir -p /tmp/payloads
chmod 777 /tmp/payloads

exec /usr/bin/mantis-node "$@"
