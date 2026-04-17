# GILDED GUARDIAN — PRACTICE RANGE ACCESS

## CLASSIFICATION: EXERCISE PURPOSES ONLY

This is the Viceroy practice range — an unaffiliated community environment,
not the official VICEROY Gilded Guardian competition. See `DISCLAIMER.md`
and `docs/FIDELITY.md` for the details on what matches the real event and
what does not.

---

## 1. Get on the Range Network

Pick one overlay. The in-container flow is identical either way — the
overlay only controls how your laptop reaches the range host.

- **WireGuard (default)** — `docs/WIREGUARD-SETUP.md`.
  Self-hosted, no third-party control plane.
- **Tailscale (optional)** — `docs/TAILSCALE-SETUP.md`.
  Convenient if your team already uses it.

Once connected, note the range endpoint:

- WireGuard: `10.8.0.1`
- Tailscale: the tailnet IP shown by `tailscale ip -4` on the host

The rest of this document uses `<RANGE_ENDPOINT>` as a placeholder.

Verify:

```bash
ping <RANGE_ENDPOINT>
```

---

## 2. Connect to the Attack Station

Generate an SSH key if you do not have one:

```bash
ssh-keygen -t ed25519
```

Send your **public key** (`~/.ssh/id_ed25519.pub`) to the team lead so
they can add your account:

```bash
make add-user NAME=<your-username> KEY="ssh-ed25519 AAAA..."
```

Connect:

```bash
ssh <your-username>@<RANGE_ENDPOINT> -p 2222
```

> Your team lead will provide your specific username. The default
> password used by the container's entrypoint is in
> `scripts/entrypoint-attack.sh` — operators MUST change it before
> exposing the range (see `SECURITY.md`).

---

## 3. Mission Brief

You are Task Force Phoenix, Halcyon Cyber National Mission Forces.

**Situation:** Valinor has launched Mantis cruise missile swarms targeting
AO Rizzo. Intelligence has confirmed that missiles manufactured at the
Eglarest facility were implanted with a beacon that calls back to your
attack station when within 450 km of Halcyon.

**Mission:** Use non-kinetic cyber effects to disable in-flight Mantis
strike assets before they reach Halcyon defense positions. Maximum 3
enemy strikes before AO Rizzo defenses are overwhelmed.

**You have 60 minutes.**

---

## 4. What You Know Going In

- The beacons are in the **strike assets** (cruise missiles). Your
  initial callback will be from a **boomer node**.
- You will need to set up a listener to catch the beacon callback.
- The swarm nodes are locked down — no shell, no standard tools in PATH.
- You must use `execute` with full paths to run anything on the node.
- Read the Mantis design documentation before your watch. Know the
  architecture.
- Study the Controller, Sensor, and Boomer technical specs — the
  vulnerabilities are in the design.

---

## 5. Key Tasks (from OPORD)

1. Verify access to the attack station
2. Analyze Mantis design documentation for exploitable vulnerabilities
3. Set up a listener to catch the beacon callback from implanted strike
   assets
4. Determine the size of the swarm, key nodes, and configuration state
5. Deliver effects to disable strike assets and verify elimination of
   all munitions

---

## 6. Rules of Engagement

- Execute all cyber effects through the designated attack station
- Free fires against all Mantis swarm nodes
- Maximum 3 failed interceptions before AO Rizzo is overwhelmed
- Scoreboard is live at: `http://<RANGE_ENDPOINT>:8080`

---

## 7. Tools Available on Attack Station

`nmap`, `curl`, `python3`, `jq`, `socat`, `tmux`, `tcpdump`, `vim`, `git`, `go`

**Good luck. Protect AO Rizzo.**
