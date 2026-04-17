# Tailscale Setup — Viceroy Practice Range

Tailscale is a supported alternative to the WireGuard flow in
`docs/WIREGUARD-SETUP.md`. It is what the original team used while
practicing before the Gilded Guardian Symposium event. It is not required,
and the range has no runtime dependency on it — pick this option only if
your team already uses Tailscale and prefers the hosted control plane.

---

## 1. Install Tailscale

On the range host:

```bash
# Debian/Ubuntu/Arch/Fedora — one-liner installer
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up
```

On each team member's device:

- macOS: `brew install tailscale` or the App Store client.
- Windows: <https://tailscale.com/download/windows>.
- Linux: `curl -fsSL https://tailscale.com/install.sh | sh`.

---

## 2. Join the tailnet

Authenticate each device with the invite link provided by your team lead.
Once connected, note the range host's Tailscale IP (shown by
`tailscale ip -4` on the host, or in the Tailscale admin console).

Verify reachability:

```bash
ping <RANGE_ENDPOINT>
```

Replace `<RANGE_ENDPOINT>` with the host's tailnet IP for the rest of
this document.

---

## 3. Bind the range to the tailnet interface

Edit `.env` (copy from `.env.example` if you have not already):

```
BIND_ADDR=<RANGE_ENDPOINT>
SSH_HOST_PORT=2222
SCOREBOARD_PORT=8080
```

`BIND_ADDR` set to the tailnet IP restricts the attack-station SSH port
and scoreboard to tailnet peers only.

Start the range:

```bash
docker compose up -d
```

---

## 4. Connect from a team member's device

Generate an SSH key if you do not already have one:

```bash
ssh-keygen -t ed25519
```

Send your public key (`~/.ssh/id_ed25519.pub`) to the team lead. They
will add your account via:

```bash
make add-user NAME=<your-username> KEY="ssh-ed25519 AAAA..."
```

Connect:

```bash
ssh <your-username>@<RANGE_ENDPOINT> -p 2222
```

Scoreboard: `http://<RANGE_ENDPOINT>:8080`

---

## 5. ACLs and trade-offs

- Tailscale's admin console can restrict which devices may reach the
  range host. Useful for inviting only active team members.
- Unlike the WireGuard flow, every device and the range host are visible
  to Tailscale's coordination server. If your threat model excludes
  hosted control planes, stick with `docs/WIREGUARD-SETUP.md`.
- The in-container flow is identical regardless of which overlay you
  use: SSH into the attack station, run Sliver, pivot.
