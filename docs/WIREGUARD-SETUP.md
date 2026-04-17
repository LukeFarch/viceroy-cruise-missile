# WireGuard Setup — Viceroy Practice Range

WireGuard is the default way to share a running range with a remote team.
It replaces the Tailscale overlay the original team used while practicing
and has no runtime dependency on a third-party service. If you would
rather use Tailscale, see `docs/TAILSCALE-SETUP.md`.

This guide covers the two supported deployment modes:

1. **Host-level WireGuard (recommended).** Simple, small blast radius, no
   extra containers. Works on any Linux host with `wireguard-tools`.
2. **Sidecar container.** An opt-in compose overlay that runs WireGuard
   inside a container next to the range. Useful when the host kernel has
   no WireGuard support or you do not want to configure the host at all.

---

## 0. Prerequisites

On the range host:

```bash
# Debian/Ubuntu
sudo apt-get install -y wireguard wireguard-tools

# Arch
sudo pacman -S wireguard-tools

# Fedora
sudo dnf install -y wireguard-tools
```

Any modern Linux kernel (>= 5.6) includes WireGuard support. The `wg` and
`wg-quick` userspace tools are what you actually need.

Each remote team member needs a WireGuard client:

- macOS / iOS: the official **WireGuard** app (App Store).
- Windows: <https://www.wireguard.com/install/>.
- Linux: `wireguard-tools` from the distro package manager.
- Android: the official **WireGuard** app (Play Store / F-Droid).

---

## 1. Generate server + peer configs

From the repo root:

```bash
./scripts/wg-gen.sh \
    --peers 4 \
    --endpoint range.example.com:51820 \
    --names dash,laura,phoenix,alice
```

Outputs:

```
wireguard/server.conf                # server config — stays on the host
wireguard/peers/dash.conf            # one per team member — distribute
wireguard/peers/laura.conf
wireguard/peers/phoenix.conf
wireguard/peers/alice.conf
```

The peer configs are `chmod 600` and are ignored by git via `.gitignore`.
Send each `peers/<name>.conf` to the corresponding team member through a
trusted channel (Signal, 1Password, encrypted email, etc. — not a public
Slack).

Extra flags:

```
--port           WireGuard UDP port the server listens on.      default 51820
--ssh-port       Attack-station SSH port (passed into peer confs). default 2222
--scoreboard-port                                                  default 8080
```

---

## 2. Bring the server up

### Mode A — host-level (recommended)

```bash
sudo wg-quick up ./wireguard/server.conf
```

Verify:

```bash
sudo wg show
# should list one peer per config you handed out
```

Open UDP 51820 (or whatever `--port` you chose) on the range host's
firewall and any upstream NAT, and nothing else. The SSH port and
scoreboard port are now reachable only via the tunnel.

Stop / restart:

```bash
sudo wg-quick down ./wireguard/server.conf
```

### Mode B — sidecar container (optional)

If you prefer not to touch the host, copy
`docker-compose.wireguard.example.yml` → `docker-compose.wireguard.yml`,
adjust as needed, and run:

```bash
docker compose \
    -f docker-compose.yml \
    -f docker-compose.wireguard.yml \
    up -d
```

The sidecar bind-mounts `wireguard/server.conf` and exposes the UDP port
to the host. Host-level is still recommended — the sidecar is offered
only for environments where you cannot install kernel modules.

---

## 3. Publish the attack station on the tunnel

Edit `.env` (copy from `.env.example` if you have not already):

```
BIND_ADDR=10.8.0.1
SSH_HOST_PORT=2222
SCOREBOARD_PORT=8080
S2_SCOREBOARD_PORT=8081
```

`BIND_ADDR=10.8.0.1` tells docker-compose to publish the attack-station
SSH port and scoreboard **only** on the WireGuard interface. Anything on
the host's public interface cannot reach them.

Then (re)start the range:

```bash
docker compose up -d
# or, for both swarms:
docker compose -f docker-compose.yml -f docker-compose.swarm2.yml up -d
```

---

## 4. Team member connects

On each team member's device, import `<name>.conf` and bring the tunnel
up:

```bash
# Linux
sudo wg-quick up ./dash.conf
```

Then SSH into the attack station:

```bash
ssh <your-username>@10.8.0.1 -p 2222
# default password: see scripts/entrypoint-attack.sh
# (you MUST change it before a real deployment — see SECURITY.md)
```

Scoreboard: <http://10.8.0.1:8080>

From inside the attack station the rest of the kill-chain (Sliver C2,
pivot into the mantis-swarm network) is in `docs/PLAYER-GUIDE.md` and
`docs/BOOMER-WALKTHROUGH.md`.

---

## 5. Rotating team membership

Adding or removing a peer is a regenerate-everything operation — the
script is cheap and idempotent:

```bash
./scripts/wg-gen.sh --peers 5 --endpoint range.example.com:51820 \
    --names dash,laura,phoenix,alice,bob
sudo wg-quick down ./wireguard/server.conf
sudo wg-quick up   ./wireguard/server.conf
```

All existing peers get **new private keys**. Redistribute the new peer
configs. Rotating keys per session is a good habit.

---

## 6. Troubleshooting

- **Peer connects but cannot reach 10.8.0.1.** Check that `BIND_ADDR` is
  set to `10.8.0.1` in `.env` and that you recreated the containers
  after changing it (`docker compose up -d --force-recreate attack-station
  scoreboard`).
- **Peer cannot connect at all.** UDP 51820 (or your chosen port) must
  be open on the host firewall and any NAT between the peer and the host.
- **`wg-quick up` complains about `resolvconf`.** Either install
  `resolvconf` / `openresolv`, or comment out the `DNS =` line in your
  peer config.
- **You handed out a config to the wrong person.** Regenerate — private
  keys cannot be revoked individually; a rerun replaces every key.
