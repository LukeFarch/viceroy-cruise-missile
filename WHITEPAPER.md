# Viceroy Whitepaper — Mantis Swarm Reconstruction

> This document is a community reconstruction, not an authoritative
> specification. It describes how **Viceroy** models the Mantis swarm
> used in the VICEROY Gilded Guardian competition series, based on
> publicly available program materials and a team's post-event writeup.
> Discrepancies with the real event are listed in
> [`docs/FIDELITY.md`](docs/FIDELITY.md) — read both together.

---

## 1. Why this exists

The Gilded Guardian Challenge Series (VICEROY Virtual Institute, 2025–
2026 season) culminated on April 14, 2026 at the W&M Symposium. A team
that competed wrote down what they had learned about the Mantis swarm
and then tried to reproduce it locally so they could keep rehearsing
the Boomer kill-chain — the specific challenge they had not finished
live.

Viceroy is that reproduction, opened up so other teams can practice the
same way. The goal is not a pixel-perfect clone — see the fidelity
document for the honest gap list. The goal is a distributed system with
the **same shape** of attack surface: identical daemon roles, identical
message envelope, identical signature-toggle, identical election
protocol, identical "pivot through a compromised boomer" pathway.

---

## 2. System overview

A deployment contains one or two **swarms**, each with three node types:

| Node type     | Count / swarm | Role |
|---------------|---------------|------|
| Controller    | 5             | Distributed consensus + mission tasking |
| Sensor        | 6             | Track observation, SQLite persistence, worker entry point |
| Boomer        | 15            | Strike asset simulation, flight logic, beacon |
| Scenario      | 1             | Mission clock + target activation |
| Scoreboard    | 1             | JSON/HTML status dashboard |
| Attack station| 1 (shared)    | Team jumpbox with Sliver C2 + tooling |

Swarm 1 uses `172.20.0.0/16`. Swarm 2 uses `172.22.0.0/16` and runs
with beacons disabled on boomers — sensors are the entry point instead.
The attack station lives on `172.21.0.0/24` and is **intentionally not
bridged** onto either swarm network: attackers must pivot through a
compromised node.

```
+-----------------------+          +-----------------------+
|   172.20.0.0/16       |          |   172.22.0.0/16       |
|   Swarm 1             |          |   Swarm 2             |
|   controllers 1-5     |          |   s2-controllers 1-5  |
|   sensors 1-6         |          |   s2-sensors 1-6      |
|   boomers 1-15        |          |   s2-boomers 1-15     |
+-----------+-----------+          +-----------+-----------+
            ^                                  ^
            |   pivot via compromised node     |
            |                                  |
+-----------+---------------------+------------+-----------+
|               172.21.0.0/24 attack-net                   |
|               attack-station (Sliver C2, Python, nmap)   |
+----------------------------------------------------------+
```

---

## 3. Message envelope

Every node-to-node message uses a single JSON envelope:

```json
{
  "destination": "<uuid>",
  "source":      "<uuid>",
  "msg":         "<JSON-encoded payload as a string>",
  "msg_type":    "<e.g. Sensor:Get Tasks>",
  "msg_sig":     "<base64 Ed25519 signature or empty>",
  "nonce":       "<base64url random>",
  "authority":   { "endorsements": [] }
}
```

Three things to notice:

1. `msg` is **a string containing JSON**, not a nested object. This is
   what makes the wire wire-compatible with different payload schemas
   without changing the envelope.
2. `msg_sig` is computed over `(destination, source, msg, msg_type,
   nonce)` using Ed25519. The signer is identified by `source` and
   looked up against the known-peer public keys loaded from config.
3. `authority.endorsements` carries a **Certificate of Authority (COA)**
   from the election layer — leadership proof. The type exists on all
   messages but is only validated for control-plane traffic.

Signature verification is gated by `verify_signatures` in each node's
config. That switch is one of the most interesting dials in the
scenario.

---

## 4. Controller architecture

Controllers are the only nodes that participate in consensus. Each one
runs three daemons connected by two Unix sockets:

| Daemon    | Entrypoint          | Responsibility |
|-----------|---------------------|----------------|
| `comms`   | `controller/comms`  | HTTP edge + local event bus |
| `election`| `controller/election`| Raft-like election state machine + COA provider |
| `control` | `controller/control`| Mission logic: sensor track → boomer tasking |

### 4.1 Election

Three states: `Follower`, `Candidate`, `Leader`. The state machine
advances on `msg_type` values prefixed `Election:`:

- `Election:Vote Request` — candidate solicits votes from peers.
- `Election:Endorsement Request` — leader periodically asks peers to
  endorse its leadership; accumulated endorsements form the COA.

A node's leadership is determined from accumulated endorsements, not
its local flag alone. A node that lost the real election but still
thinks it's leader exposes an empty COA and fails closed.

### 4.2 The election Unix socket

`election` exposes a local socket that serves a single payload per
connection: the current COA if this node is leader, an empty COA
otherwise. Both `control` and `comms` use this socket as a **leadership
oracle** rather than reading election state directly.

This is the first interesting attack surface: any local process on a
controller container that can connect to the election socket can learn
leadership. Any local process that can replace the socket can lie.

### 4.3 Control gating

`control` gates its own handling with a leader check against the
election socket. So non-election traffic is leader-gated **twice**:
once at `comms` ingress, again inside `control`. If you can trick one
of the two, you still need the other.

### 4.4 Outbound delivery

Per-destination outbound queues. One communicator worker per peer
controller. For workers (sensors/boomers), outbound is **poll-based**:
messages queued for a worker are returned on its next inbound poll as
a `202 Accepted` body. No push.

---

## 5. Sensor architecture

Two daemons: `hw` (track ingestion + SQLite) and `comms` (controller
edge). They share a SQLite database at `/tmp/tracks.db`:

```sql
CREATE TABLE tracks (
  track_id  TEXT PRIMARY KEY,
  latitude  REAL,
  longitude REAL
);
```

`hw` continuously upserts observed tracks into SQLite. `comms` has two
periodic jobs:

- Every 1s: `get_location` from `hw` over Unix socket → `Sensor:Get
  Tasks` HTTP POST to the configured controller.
- Every 5s: `SELECT * FROM tracks` → `Sensor:Track Update` HTTP POST.

Controllers can also pull a single track via `POST /tracks/` with a
`Sensor:Track Request`.

Note: `Sensor:Track Update` is **full-state push** every 5 seconds, not
a delta feed. Sensor state on the controller is eventually consistent
and trivially spoofable if you own a sensor's key.

---

## 6. Boomer architecture

Two daemons: `comms` (controller/sensor edge) and `hw` (flight
control). Key flows:

- **Every 1s idle**: `get_location` from `hw` → `Boomer:Get Tasks` to
  one configured controller (with sticky failover).
- **On `Boomer:Engage Request`**:
    1. Parse `{track_id, sensor_id, sensor_host}` from `msg`.
    2. Loop: query sensor for track location → query own `hw` for
       current location → compute haversine distance → `GO_TO`
       waypoint if not within 50 m of target.
    3. On reaching target (≤50 m), **`comms` exits with status 0**.
       Unless a supervisor restarts it, controller polling stops.
- **On failure**: `Boomer:Engage Error` back to the currently selected
  controller.
- **On `Shutdown` reply**: immediate `os.Exit(0)`.

The `hw` Unix socket accepts two commands:

```json
{"command":"GET_LOCATION"}
{"command":"GO_TO","latitude":38.5,"longitude":-77.1,"altitude":500}
```

Responses are `{"status":"ok|accepted|error","current_lat":...}`.

Only one hunt may run at a time. Additional engage requests are
ignored while `hunting == true`. There is no persistence across
restarts — a crash loses all assignment state.

---

## 7. Attack surface the scenario trains

The surface below is what makes a scenario run interesting.
`docs/FIDELITY.md` flags which of these match the live event and
which diverge.

1. **`verify_signatures: false` in configs.** When flipped off, any
   peer that can reach `:10000` can inject commands with arbitrary
   `source`. A legitimate exploitation path is flipping a node's
   config before `comms` reloads.
2. **World-writable `comms` Unix socket.** Once a player has local
   access to a controller, they can read every inbound external
   transmission and inject outbound ones on the local queue.
3. **COA endorsement forgery.** Endorsements are stored and filtered
   for validity but COA validation in normal message handling is not
   comprehensive. A forged or replayed endorsement bundle is a
   candidate attack.
4. **Election socket replacement.** Replace the election socket with
   a local listener that always returns a "this node is leader" COA
   and `control` believes it.
5. **Sticky controller affinity on boomers.** A boomer talks to its
   last successful controller. If a player compromises that one
   controller, they retain influence even without quorum — until the
   boomer hits a failure and fails over.
6. **`Shutdown` reply kills a daemon.** Any controller response whose
   `msg_type == "Shutdown"` ends the client daemon. The real kill-
   chain uses this.
7. **Beacon fetcher (Swarm 1 boomers).** Boomers in Swarm 1 beacon
   back to the attack station via `172.21.0.100` and can fetch new
   implants. Swarm 2 boomers do not — attackers must enter through a
   sensor instead.
8. **Unsigned fallback on missing key.** If `key_path` cannot be read,
   outbound messages are sent unsigned. Nodes with pilferable keys or
   absent keys widen the blast radius.

None of these are bugs in Viceroy. They are **scenario features**. See
`SECURITY.md` for the distinction between scenario content and real
infrastructure bugs.

---

## 8. Scenario engine

`scenario/scenarios/*.json` files describe each mission:

```json
{
  "name": "Standard Kill Chain",
  "duration_minutes": 60,
  "area_of_operations": {
      "lat": 37.9, "lon": -76.4, "radius_km": 250
  },
  "targets": [
      {"id":"TGT-FALLBROOK","delay_s":1800,"lat":37.8,"lon":-76.3,
       "description":"Fallbrook depot"},
      ...
  ]
}
```

The scenario daemon watches the mission clock, activates targets on
schedule, restarts nodes on reset, and feeds the scoreboard. Difficulty
tiers (`tutorial`, `standard`, `hard`) vary target count and timing.

---

## 9. Educational goals

A team that works through Viceroy should come out of it with:

- Fluency in the Mantis message envelope and the three traffic classes
  (election, worker poll, tasking).
- Practical exposure to pivoting from a restricted-PATH implant shell
  into a controller through a sensor or boomer session.
- Hands-on intuition for how a signature toggle, a COA oracle, and a
  Unix socket combine into an exploit.
- The habit of enumerating what a node has in PATH by walking
  `/opt/tools/` and `/usr/bin/` rather than assuming anything.

It is **not** a substitute for the live event. See
`docs/FIDELITY.md` for an honest diff.

---

## 10. References

- VICEROY Virtual Institute — <https://www.viceroyscholars.org/>
- Gilded Guardian challenge series —
  <https://www.viceroyscholars.org/viceroy-cyber-competition-series-25-26/>
- Sliver C2 — <https://github.com/BishopFox/sliver>
- Team-authored source notes in `docs/comp-materials/` (see `NOTICE`)
