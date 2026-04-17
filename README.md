# Viceroy

A self-hostable practice range that reconstructs the **Mantis swarm**
scenario from the VICEROY Virtual Institute's **Gilded Guardian** cyber
competition series. Intended for defensive-cyber teams who want to keep
drilling the Boomer kill-chain between events.

> **Unaffiliated.** Viceroy is a community project. It is not produced,
> sponsored, or endorsed by the VICEROY Virtual Institute, the Department
> of Defense, Cyber Outcomes Group LLC, or any other organization
> associated with the official competitions. See [`DISCLAIMER.md`](DISCLAIMER.md).
>
> **Intentionally vulnerable.** Do not run on shared infrastructure. See
> [`SECURITY.md`](SECURITY.md).

---

## What you get

- A distributed **Mantis swarm** of 56 containers split across two
  isolated networks: 5 controllers + 6 sensors + 15 boomers per swarm.
- A locked-down **attack station** (Alpine + Sliver C2 + Python + nmap
  + tmux) that mirrors the real competition's "nothing in PATH, pivot
  through Sliver" experience.
- A **scenario engine** that activates targets on a mission clock and a
  **scoreboard** that tracks interceptions.
- A **WireGuard generator** so a remote team can share one host.

The Mantis architecture — `comms` / `election` / `control` / `hw`
daemons, Unix-socket IPC, Ed25519-signed `Transmission` envelopes, poll
mailbox model, Raft-like leader election — is reconstructed from
publicly available competition materials and the team's own writeup.

---

## Documentation — see the wiki

Long-form docs live in the
**[project wiki](https://github.com/LukeFarch/viceroy-cruise-missile/wiki)**.
The short map:

| Topic | Wiki page |
|---|---|
| Architecture deep-dive (controllers, sensors, boomers — spec vs implementation vs attack surface) | [Whitepaper](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Whitepaper) |
| Diffs from the live competition, with source citations | [Fidelity](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Fidelity) |
| Full stand-up + reset flows | [Deployment Guide](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Deployment-Guide) |
| WireGuard overlay (default) | [WireGuard Setup](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/WireGuard-Setup) |
| Tailscale overlay (alternative) | [Tailscale Setup](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Tailscale-Setup) |
| Team briefing, player guide, kill-chain walkthrough | [Team Briefing](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Team-Briefing) / [Team Guide](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Team-Guide) / [Player Guide](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Player-Guide) / [Boomer Walkthrough](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Boomer-Walkthrough) |

In-repo policy docs (`SECURITY.md`, `CONTRIBUTING.md`, `DISCLAIMER.md`,
`NOTICE`, `LICENSE`) stay next to the code so GitHub auto-surfaces
them; source material the reconstruction was built from lives in
[`docs/comp-materials/`](docs/comp-materials/).

---

## Quickstart

Requires Docker 24+, Docker Compose v2, Go 1.23, and `make`.

```bash
git clone https://github.com/<you>/viceroy.git
cd viceroy

cp .env.example .env        # edit BIND_ADDR / ports to taste
make configs                # generate UUIDs + Ed25519 keys
docker compose up -d        # bring up Swarm 1 (28 containers)

# both swarms
make configs-all
make up-all
```

Scoreboard: <http://127.0.0.1:8080> (Swarm 2: `:8081`)
SSH into the attack station:

```bash
ssh <username>@127.0.0.1 -p 2222
```

Change the default password before doing anything else —
`scripts/entrypoint-attack.sh` has it, `SECURITY.md` explains why.

Reset / redeploy:

```bash
make reset-full       # regenerate keys, rebuild, redeploy Swarm 1
make reset-full-all   # same for both swarms
```

---

## Remote team access

Viceroy ships a WireGuard setup as the default overlay and keeps
Tailscale as a documented alternative. The in-container flow is
identical either way. Full instructions:
[WireGuard Setup](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/WireGuard-Setup)
/
[Tailscale Setup](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Tailscale-Setup).

With `BIND_ADDR=10.8.0.1` in `.env`, the attack-station SSH port and
the scoreboard bind only to the tunnel interface — never the public
side of the host.

---

## Repository layout

```
cmd/                 Go binaries: controller, sensor, boomer,
                     configgen, scenario, scoreboard
internal/            Shared packages: protocol, config, signing, geo
configs/             Generated Swarm 1 node configs (gitignored keys)
configs-s2/          Generated Swarm 2 node configs
configgen/           Config generator entry point
docker/              Dockerfiles for each node type + attack station
docker-compose.yml   Swarm 1 (28 containers)
docker-compose.swarm2.yml  Swarm 2 overlay
scenario/            JSON scenarios (tutorial / standard / hard)
scoreboard/          Static assets for the dashboard
scripts/             Ops helpers (reset, add-user, wg-gen)
wireguard/           WireGuard config templates + generator output
docs/comp-materials/ Source materials and the team's post-event writeup
                     (see NOTICE for attribution)
                     — operator/player guides live in the project wiki
Makefile             Entrypoints for every common task
```

---

## Who this is for

- **VICEROY scholars** and cyber clubs who competed in the Gilded
  Guardian series and want to keep practicing offline.
- **Defensive-cyber instructors** looking for a ready-made, reset-on-
  demand training environment with a distributed-systems attack surface.
- **Anyone** curious about how a Raft-ish leader election plus a
  signature-toggle plus a Unix-socket IPC mesh produce exploitable
  behavior when combined.

---

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md). The headline asks:

1. **No real secrets in PRs.** `.gitignore` covers the obvious cases
   but please eyeball your diff.
2. **Regenerate configs** (`make configs-all`) before committing — the
   repo intentionally does not ship private keys.
3. **Keep the fidelity doc honest.** If you add a feature that is not
   in the real competition, update the
   [Fidelity](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Fidelity)
   wiki page.

---

## Acknowledgements

- The VICEROY Virtual Institute and the sponsors of the Gilded Guardian
  challenge series for running the competition this range trains
  against.
- The BishopFox team for Sliver C2.
- The Alpine and Go communities for containers small enough to run 56
  at a time on a laptop.

---

## License

GNU General Public License v3.0 or later. See [`LICENSE`](LICENSE) and
[`NOTICE`](NOTICE).
