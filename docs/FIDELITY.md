# Fidelity: Viceroy vs. the Real Gilded Guardian Competition

This document exists to keep claims about Viceroy honest. Viceroy is a
community reconstruction. If you are using it to train for the real
event, know exactly where it matches and where it does not.

The table below is the source of truth. Every row cites where the
claim comes from so you can verify.

Legend:
- **Match** — the practice range behaves like the live event.
- **Close** — same *shape* of behavior, simplified details.
- **Differs** — the practice range intentionally diverges.
- **Unknown** — the real-event behavior is not public.

| Aspect | Real Gilded Guardian | Viceroy | Status | Source |
|---|---|---|---|---|
| Target CPU architecture | `linux/arm64` | `linux/amd64` | **Differs** | Team notes; PLAYER-GUIDE.md (pre-edit) |
| C2 tool on attack station | Sliver implants with restricted `execute` / `download` / `ls` | Sliver available **and** a raw Python reverse shell option ("Quick Start") | **Close** | `docs/PLAYER-GUIDE.md` lines 46–82 |
| Node PATH | Locked down, `curl` / `python3` require full paths like `/usr/bin/curl` | Same — Alpine minimal image, tools under `/opt/tools/` | **Match** | `docker/Dockerfile.*` + project memory |
| Network recon | Players enumerate IPs live | Peer IPs are pre-known from generated configs (172.20.x.x / 172.22.x.x are deterministic) | **Differs** | Team-generated configs; original DASH-KILL-CHAIN.md:202 |
| Swarm size | Exact live-event scale is not published | 5 controllers + 6 sensors + 15 boomers per swarm, two swarms | **Unknown** → stated openly | Exploration of `configs/`, `configs-s2/` |
| Consensus protocol | Raft-like leader election with endorsement-based Certificate of Authority | Raft-like leader election with endorsement-based COA | **Match** | `docs/comp-materials/Controller.md` §Election Flow |
| Message envelope | JSON `Transmission` with `destination/source/msg/msg_type/msg_sig/nonce/authority` | Same envelope, same signature material `(destination, source, msg, msg_type, nonce)` | **Match** | `docs/comp-materials/Boomer.md` §1, `docs/comp-materials/Sensor.md` §Transmission Envelope |
| `verify_signatures` dial | Exists, can be turned off to widen attack surface | Exists, same semantics, configurable per-node via YAML | **Match** | `internal/config/config.go`, team notes |
| `Shutdown` reply kills daemon | Yes, `os.Exit(0)` on receipt | Same | **Match** | `docs/comp-materials/Boomer.md` §Shutdown Flow |
| Leader selection is endorsement-based (COA) | Yes | Yes — followers/leaders filter endorsements before exposing COA | **Match** | `docs/comp-materials/Controller.md` §Election Flow |
| Election socket as leadership oracle | Yes, world-writable Unix socket | Yes, same | **Match** | `docs/comp-materials/Controller.md` §election Socket Semantics |
| Sensor track persistence | SQLite with `(track_id, latitude, longitude)` | Same schema | **Match** | `docs/comp-materials/Sensor.md` §Shared State |
| Boomer fetcher / beacon | Live event had beacon+fetcher on strike assets | Swarm 1 boomers beacon and fetch; Swarm 2 boomers do not — sensors are entry instead | **Differs** (by design, for practice variety) | Original DASH-GUIDE.md:438,532; `configgen` output |
| Scoring | Official competition scoring rubric (not public) | Simple "targets protected vs struck, max 3 strikes before fail" | **Differs** | `cmd/scoreboard/main.go`, team notes |
| Mission timing | 60-minute clock, live adversary dynamics | 60-minute static schedule in `scenario/scenarios/*.json` — no live human adversary | **Close** | `scenario/scenarios/standard.json`, `docs/TEAM-BRIEFING.md` §3 |
| Rules of Engagement | Free-fire within mission box | Same mission box enforced by `control` filtering | **Match** | `docs/TEAM-BRIEFING.md` §6 |
| Team structure | VICEROY scholar + up to 3 teammates | Any number (operator-defined); `make add-user` adds SSH accounts | **Differs** | `Makefile` `add-user` target |
| Attack station tooling | nmap, curl, python3, tmux, tcpdump, vim, git, go, Sliver | Same set | **Match** | `docker/Dockerfile.attack` |
| Isolation model | Attack host not directly on swarm network | Same — `attack-net` is a separate Docker network, pivot required | **Match** | `docker-compose.yml` networks |
| Node restart on failure | systemd under NixOS | Docker restart policy `unless-stopped` | **Close** | `docker-compose.yml` |
| COA endorsement verification | Enforced on control-plane traffic to some extent | Exists as a type; verification depth is limited in the current implementation | **Close** | `docs/comp-materials/Sensor.md` §Signature and Trust Model |

---

## Notable things the practice range does **not** attempt

- Live adversary pressure. There is no human red team moving targets
  in real time.
- Network segmentation between competing teams. Viceroy is a single-
  tenant range.
- Official scoreboard integration. The local scoreboard is a training
  aid, not a submission target.
- Exact competition wire-format compatibility. The envelope and
  semantics match; byte-for-byte identity is not guaranteed.

---

## Notable things the practice range adds

- Two parallel swarms (1 and 2) so a team can run different scenarios
  simultaneously or compare strategies.
- A WireGuard generator and documented multi-operator access so an
  online team can share one host.
- A reset pipeline (`make reset-full-all`) that rotates every key on
  every deployment.
- A hints-graded `docs/PLAYER-GUIDE.md` for teams that want a ladder
  into the first kill-chain. The live event has no hints.

---

## How to update this document

When any PR changes behavior that appears in this table:

1. Update the relevant row.
2. If the status moves away from **Match**, say so explicitly in the
   PR description.
3. If a row becomes **Unknown → resolved**, update the source citation
   with the new evidence.

Claims without citations will be removed. Overstated claims ("this is
the same as the real event") are the specific thing this file exists
to prevent.
