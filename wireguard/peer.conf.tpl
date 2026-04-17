# WireGuard peer config for a Viceroy practice-range team member.
# Import this file into your WireGuard client, then run `wg-quick up <name>`.
#
# After the tunnel is up, connect to the range:
#   ssh <username>@10.8.0.1 -p {{SSH_HOST_PORT}}
# Scoreboard (read-only):
#   http://10.8.0.1:{{SCOREBOARD_PORT}}

[Interface]
PrivateKey = {{PEER_PRIVKEY}}
Address = {{PEER_ADDRESS}}/32
DNS = 1.1.1.1

[Peer]
PublicKey = {{SERVER_PUBKEY}}
Endpoint = {{SERVER_ENDPOINT}}
# Only route range traffic through the tunnel. Change to 0.0.0.0/0 if you
# want everything to egress via the range host.
AllowedIPs = 10.8.0.0/24
PersistentKeepalive = 25
