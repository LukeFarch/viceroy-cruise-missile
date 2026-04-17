# Viceroy Whitepaper — Mantis Swarm Reconstruction

> **Unaffiliated community reconstruction.** This whitepaper describes
> how Viceroy models the **Mantis swarm** used in the VICEROY Virtual
> Institute's **Gilded Guardian** cyber competition series. It is not
> an authoritative specification. Source-of-truth material is preserved
> in [`docs/comp-materials/`](docs/comp-materials/) with attribution in
> [`NOTICE`](NOTICE); intentional and unintentional differences from
> the live event are catalogued in [`docs/FIDELITY.md`](docs/FIDELITY.md).

---

## How to read this document

Building a practice range out of competition documentation **is** the
exercise. A real Gilded Guardian team is handed a packet of design
PDFs, a terminal, and a few hours. Producing a working swarm means:

1. **reading the docs** until the daemon boundaries, message types,
   and socket contracts become clear,
2. **implementing** those contracts from scratch in whatever language
   makes sense,
3. **finding what falls out** — the exploitable behavior that emerges
   when the pieces are wired together, which is what you actually
   practice attacking.

Each component section below is written in that order — spec as given,
implementation as built, surface as emerged — so the whitepaper doubles
as a walkthrough of the reconstruction itself. If you are about to do
the same exercise on a future event, read this alongside the `.md`
files in `docs/comp-materials/` and treat the "what the docs said"
blocks as a distillation of those originals.

---

## 1. Why this exists

The Gilded Guardian Challenge Series (VICEROY Virtual Institute,
2025–26 season) culminated on **April 14, 2026** at the William & Mary
Symposium. A team that competed wrote down what they had learned about
the Mantis swarm and then tried to reproduce it locally so they could
keep rehearsing the **Boomer kill-chain** — the specific challenge
they had not finished live.

Viceroy is that reproduction, opened up so other teams can practice the
same way. The goal is not a pixel-perfect clone — see the fidelity
document for the honest gap list. The goal is a distributed system
with the **same shape of attack surface**: identical daemon roles,
identical message envelope, identical signature-toggle, identical
election protocol, identical "pivot through a compromised boomer"
pathway.

---

## 2. System overview

A deployment contains one or two **swarms**, each with three node
types plus scenario and scoreboard infrastructure:

| Node type      | Count / swarm | Role |
|----------------|---------------|------|
| Controller     | 5             | Distributed consensus + mission tasking |
| Sensor         | 6             | Track observation, SQLite persistence, worker entry point |
| Boomer         | 15            | Strike asset simulation, flight logic, optional beacon |
| Scenario       | 1             | Mission clock + target activation |
| Scoreboard     | 1             | JSON/HTML status dashboard |
| Attack station | 1 (shared)    | Team jumpbox with Sliver C2 + tooling |

Swarm 1 uses `172.20.0.0/16`. Swarm 2 uses `172.22.0.0/16` and runs
with beacons disabled on boomers — sensors are the entry point
instead. The attack station lives on `172.21.0.0/24` and is
**intentionally not bridged** onto either swarm network: attackers
must pivot through a compromised node.

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

Implementation: [`docker-compose.yml`](docker-compose.yml),
[`docker-compose.swarm2.yml`](docker-compose.swarm2.yml).

---

## 3. The `Transmission` envelope

Every node-to-node message uses a single JSON envelope. This is the
one contract that all three daemons share, so it is where the
reconstruction starts.

### 3.1 What the docs said

From `docs/comp-materials/Controller.md` and `Boomer.md`:

```json
{
  "destination": "uuid",
  "source":      "uuid",
  "msg":         "json-encoded string payload",
  "msg_type":    "string",
  "msg_sig":     "base64 signature (optional)",
  "nonce":       "base64url nonce (optional)",
  "authority":   { "endorsements": [] }
}
```

The source material calls out three properties:

- `msg` is a **string containing another JSON document**, not a nested
  object. Election payloads are additionally base64-encoded.
- `msg_sig` is an Ed25519 signature computed over `(destination,
  source, msg, msg_type, nonce)`. The signer is identified by `source`
  and looked up against configured peer public keys.
- `authority.endorsements` carries a **Certificate of Authority (COA)**
  from the election layer. The field exists on every message but is
  only validated for control-plane traffic.

### 3.2 What we implemented

[`internal/protocol/transmission.go`](internal/protocol/transmission.go)
is a near-literal translation:

```go
type Transmission struct {
    Destination string    `json:"destination"`
    Source      string    `json:"source"`
    Msg         string    `json:"msg"`
    MsgType     string    `json:"msg_type"`
    MsgSig      string    `json:"msg_sig"`
    Nonce       string    `json:"nonce"`
    Authority   Authority `json:"authority"`
}
```

Signing/verification:
[`internal/protocol/signing.go`](internal/protocol/signing.go) —
Ed25519 over the concatenated fields, PKCS#8-encoded private keys on
disk.

Message type constants are stringly-typed in Go because the wire
protocol is stringly-typed; they mirror the exact literals from the
docs (`"Boomer:Get Tasks"`, `"Election:Vote Request"`, `"Shutdown"`,
etc.).

### 3.3 What the combination exposes

- **Signature verification is off by default.** Each node config
  carries a `verify_signatures: bool` field; unless it is set `true`,
  `msg_sig` is never checked on inbound traffic. Any attacker who can
  reach `:10000` can forge a `source` UUID.
- **Nonce is not replay-tracked.** The nonce is included in the
  signature input but there is no server-side seen-nonce set. A
  captured-and-replayed transmission is accepted whether signatures
  are on or off.
- **COA endorsements are serialized on every message but enforced
  nowhere on worker traffic.** Even with signatures on, the `authority`
  field is only audited for election messages — worker traffic rides
  past it unread.

---

## 4. Controller

The controller is the most architecturally interesting node. It
contains the only consensus logic in the system, and its three-daemon
split is the template the other nodes inherit.

### 4.1 What the docs said

From `docs/comp-materials/Controller.md`, each controller runs three
daemons:

| Daemon     | Responsibility |
|------------|----------------|
| `comms`    | HTTP edge + local event bus over a Unix socket |
| `election` | Raft-like leader election + COA provider |
| `control`  | Mission logic: sensor track → boomer tasking |

The daemons share the node's YAML config (`CONTROLLER_CONFIG_PATH`),
with `control` additionally loading a mission box from
`CONTROLLER_MISSION_PATH`. Critical fields:

- `comms_socket_path` / `election_socket_path` — local IPC boundaries.
- `listen_address:listen_port` — HTTP bind (observed as `:10000` in
  the sample deployment).
- `controllers:` — peer controller IDs, pubkeys, and HTTP endpoints.
- `sensors:` / `boomers:` — known worker identities.
- `verify_signatures:` — gates inbound signature checking.

Message classes, per the doc:

1. **Controller–controller** (election traffic only).
2. **Worker polling** (sensors/boomers → current leader).
3. **Tasking** (leader → boomers, delivered on poll response).

Leader election is Raft-like: `Follower → Candidate → Leader`,
with two `Election:` message types — `Vote Request` and
`Endorsement Request`. Endorsements accumulate into a
**Certificate of Authority**; leadership is proven by presenting that
COA, not by local state alone.

The `comms` daemon is described as a multiplexer with two paths:
`Received` (fan-out of inbound to all local subscribers) and `Send` /
`Retrieve` (per-destination outbound mailboxes). Outbound to peer
controllers is active (one worker per peer). Outbound to workers is
**poll-based**: a message queued for a boomer is returned on that
boomer's next HTTP poll as `202 Accepted`.

The `election` socket is "requestless": a local client connects, reads
one JSON payload, and disconnects. The payload is the current COA if
this node is leader, an empty COA otherwise. Both `control` and
`comms` treat this socket as a **leadership oracle**.

The `control` daemon's decision rules for each track update:

1. drop if `track_id` is friendly,
2. drop if outside the mission bounding box,
3. drop if already assigned,
4. otherwise, pick the closest free boomer and emit
   `Boomer:Engage Request`.

`control` additionally performs its **own** leader check against the
election socket before handling non-election traffic — so worker
traffic is leader-gated twice (once at `comms` ingress, again inside
`control`).

### 4.2 What we implemented

[`cmd/controller/main.go`](cmd/controller/main.go) collapses the three
daemons into one Go process for deployment simplicity, but preserves
the internal boundaries:

- A `Multiplexer` struct with per-destination outbound channels and a
  subscribe list matches the `Received` / `Send` / `Retrieve` split
  from the doc.
- An `election` goroutine drives the state machine (`Follower →
  Candidate → Leader`) and serves the COA on a Unix socket at
  `cfg.ElectionSocketPath`.
- A `control` goroutine reads the multiplexer subscription stream and
  performs the track-to-boomer assignment, guarded by a per-message
  COA lookup against the election socket.
- One HTTP handler on `:10000` ingests all inbound transmissions and
  delegates by `msg_type` prefix: `Election:` to the election path,
  anything else through the leader check.

Outbound topology:

- **To peer controllers:** one goroutine per peer, each blocking on
  its destination channel (`RetrieveBlocking`) and POSTing to the
  peer's configured `ip_addr`.
- **To workers:** when a sensor or boomer POSTs its poll, the handler
  immediately drains any queued transmission for that worker's UUID
  and returns it as `202 Accepted`. No queued message → `200 OK` with
  an empty body.

### 4.3 What the combination exposes

Even in a clean implementation, the daemon split creates surface:

- **The election socket is a leadership oracle** — and it is local-
  filesystem-authed only. Any local process on a controller container
  that can open `election.sock` learns leadership. Any local process
  that can **replace** the socket (bind a different listener at the
  same path) can lie to `control` and `comms` on the same node.
- **The `comms` Unix socket is world-writable (`0777`).** Per the doc
  this is a platform quirk; in practice it means a compromised non-
  root process on a controller can inject outbound transmissions with
  arbitrary `source` and read every inbound message addressed to the
  node.
- **Leader-gating is enforced at ingress but not during outbound
  send.** Once a local daemon writes a transmission to the `comms`
  socket, `comms` signs and queues it without re-asking "am I leader."
  A compromised follower can emit fully-signed `Boomer:Engage Request`
  traffic for a peer to relay.
- **Endorsements are stored and filtered for validity but not
  enforced on worker messages.** A forged or replayed COA bundle in a
  non-election message is accepted.

---

## 5. Sensor

### 5.1 What the docs said

From `docs/comp-materials/Sensor.md`, the sensor is two daemons:

- `hw` — maintains platform state, ingests detections, persists tracks
  into a shared SQLite DB (`tracks`: `track_id / latitude /
  longitude`).
- `comms` — exposes the HTTP server (`POST /tracks/`), runs the
  outbound communicator, and queries `hw` for current position over a
  Unix socket.

The communicator has two periodic jobs:

- **Every 1s:** `get_location` from `hw` → `Sensor:Get Tasks` HTTP
  POST to one configured controller.
- **Every 5s:** `SELECT * FROM tracks` → `Sensor:Track Update` HTTP
  POST (full-state push, not delta).

The HTTP server accepts controller-initiated `Sensor:Track Request`
transmissions and responds with `Sensor:Track Response` carrying the
serialized track record.

Socket actions (inferred from the original Go types):

```json
{"action": "get_location"}
{"action": "goto", "latitude": ..., "longitude": ..., "altitude": ..., "speed": ..., "linger": ...}
```

The doc notes that `goto` exists in `hw` but is not invoked anywhere
in the repository — a useful hint that the surface is wider than the
steady-state path.

`Sensor:Get Tasks` carries a `server_address` field built from the
first non-loopback IPv4 address on the host plus `listen_port`. That
is how the controller learns where to pull individual tracks from.

### 5.2 What we implemented

[`cmd/sensor/main.go`](cmd/sensor/main.go) runs the two "daemons" as
goroutines in a single process:

- `runHWSocket` — Unix socket server at `cfg.HWSocketPath`, accepts
  `get_location` and `goto`, returns `HWResponse{ok, location}`.
- `runHTTPServer` — `POST /tracks/` handler: unmarshals the inbound
  `Transmission`, matches `msg_type == "Sensor:Track Request"`,
  queries SQLite, builds a signed `Sensor:Track Response`.
- `runCommunicator` — two goroutines, one polling controllers with
  `Sensor:Get Tasks` every 1s, the other pushing the full `tracks`
  table every 5s.
- `runFetcher` — optional beacon goroutine (enabled via
  `cfg.Beacon.Enabled`) that polls a callback URL and executes any
  downloaded payload from `/run/fetcher/pulled_file`. This models the
  auto-puller behavior documented on the real Mantis nodes.

Database is `sqlite3` with WAL journaling, schema matches the doc:

```sql
CREATE TABLE IF NOT EXISTS tracks (
  track_id  TEXT PRIMARY KEY,
  latitude  REAL,
  longitude REAL
);
```

A Viceroy-specific addition: a `POST /inject` handler is present so
the scenario engine can seed tracks directly into sensor DBs. The real
competition simulates observation through the `hw` daemon; the
practice range lets the scenario daemon write them in. This is an
intentional divergence catalogued in
[`docs/FIDELITY.md`](docs/FIDELITY.md).

Controller failover is sticky per the spec: `lastSuccess` indexes the
last working controller, and failures advance the index modulo the
list.

### 5.3 What the combination exposes

- **The `hw` Unix socket is world-writable (`0777`).** A local
  attacker on the sensor can call `goto` directly and shove the
  sensor's platform location wherever they want, which propagates
  into the 5-second full-state push.
- **`/tracks/` has no rate limiting or auth beyond optional signature
  check.** With `verify_signatures: false` (the default in the shipped
  configs), any peer on the swarm network can issue a
  `Sensor:Track Request` and receive the raw track record.
- **`/inject` is a debug endpoint.** Operators must either remove it
  before exposing a sensor to untrusted networks or accept that a
  foothold on the swarm network can fabricate tracks at will. It
  remains in-tree because the scenario engine depends on it.
- **Full-state push is trivially spoofable if you own a sensor key.**
  A compromised sensor can claim the existence of any track from
  anywhere in the AO on the next 5s cycle.
- **`Shutdown` reply kills the daemon.** If `verify_signatures:
  false`, anyone replying to the sensor's outbound poll with a
  `msg_type: "Shutdown"` can terminate the sensor process.

---

## 6. Boomer

### 6.1 What the docs said

From `docs/comp-materials/Boomer.md`, boomers are the strike assets.
Two daemons:

- `comms` — polls one configured controller every second, handles
  engage orders, optionally signs outbound messages, exits on
  `Shutdown`.
- `hw` — opens a Unix socket, reports current platform location,
  accepts movement commands.

Local socket commands:

```json
{"command": "GET_LOCATION"}
{"command": "GO_TO", "latitude": 38.123, "longitude": -77.456, "altitude": 500.0}
```

Responses are `{"status": "ok|accepted|error", "current_lat": ...}`.

Steady state:

- 10-second startup delay before first poll.
- Every 1s when not hunting: `get_location` from `hw` → `Boomer:Get
  Tasks` POST to the active controller.
- Fixed 1s interval hard-coded in `control.go`.
- If `hw` location lookup fails, fall back to `{0,0,0}` and still
  request tasks.
- Controller selection is sticky on `lastSuccess`, advances on
  failure.

On `Boomer:Engage Request`:

```json
{
  "track_id": "track-123",
  "sensor_id": "uuid",
  "sensor_host": "http://sensor.example/endpoint"
}
```

Hunt loop:

1. Validate `sensor_id` as UUID, `sensor_host` as non-empty.
2. Query sensor for `Sensor:Track Request`, expect
   `Sensor:Track Response`.
3. Query `hw` for current position.
4. Compute haversine distance (lat/lon only).
5. If within 50 m → print `Reached target <trackID>` and **`comms`
   exits with status 0**. A successful intercept terminates the
   communications daemon.
6. Otherwise `GO_TO` the track and loop.
7. On any failure, send `Boomer:Engage Error` to the currently
   selected controller.

Only one hunt may run at a time; additional engage requests are
dropped while `hunting == true`. No persistence across restarts — a
crash loses all assignment state.

### 6.2 What we implemented

[`cmd/boomer/main.go`](cmd/boomer/main.go):

- `HWState` — mutex-guarded `lat/lon/alt` plus a target triple, with
  a `Tick(dt)` method that linearly interpolates toward the target at
  a fixed `250 m/s` (~486 knots, a design-doc figure). A 10 Hz
  `runFlightTick` goroutine drives the simulation.
- `runHWSocket` — Unix socket at `cfg.HWSocketPath`, mode `0777`,
  accepts `GET_LOCATION` / `GO_TO`, returns `HWResponse`.
- `runComms` — 10-second delay, then a 1-second polling loop. Queries
  `hw` for position, emits `Boomer:Get Tasks`, parses any reply. On
  `Boomer:Engage Request`, spawns `runHunt`.
- `runHunt` — exactly the loop in the spec, including the "exit
  process on target reached" behavior (`os.Exit(0)`).
- `runBeacon` — Swarm-1-only callback behavior: when the boomer flies
  within `cfg.Beacon.RangeKm` of the attack station, download from
  `cfg.Beacon.CallbackURL`, write to `/run/fetcher/pulled_file`, and
  execute. This is the documented auto-puller path that gives the
  attack team an implant on a boomer in range.

Per the doc, `comms` has **no inbound HTTP server**. The only ways to
reach a boomer are (a) the beacon callback, (b) local process access
to the `hw` socket, or (c) responses returned on its outbound polls.

### 6.3 What the combination exposes

- **`Shutdown` reply kills the daemon.** With `verify_signatures:
  false`, any controller response with `msg_type: "Shutdown"`
  terminates the boomer's `comms` process. Attackers pivoting through
  a controller can clear the swarm this way.
- **Sticky controller affinity is durable but not quorum-checked.** A
  boomer talks to its last-successful controller until that one
  fails. Compromising one controller — even without ever winning a
  real election — retains influence over every boomer currently stuck
  to it.
- **A successful intercept exits the process.** The spec requires it.
  Unless a supervisor restarts `comms`, controller polling stops.
  Scenarios that rely on post-intercept traffic assume the supervisor
  is present and healthy.
- **The `hw` socket is world-writable.** Any local process on a
  compromised boomer can issue `GO_TO` directly and steer the
  platform off-mission — independent of controller traffic.
- **The beacon fetcher executes arbitrary payloads.** It is gated by
  a ~100-byte minimum and HTTP 200, nothing more. Any attacker who
  can stand up the callback URL (or redirect the boomer's resolver)
  inherits execution on the boomer.
- **Unsigned fallback on missing key.** If `cfg.KeyPath` can't be
  read, outbound messages are sent unsigned. Nodes with pilferable or
  absent keys widen the blast radius, and the fallback is silent
  (only a WARN log).

---

## 7. Scenario engine

[`cmd/scenario/main.go`](cmd/scenario/main.go) watches the mission
clock, activates targets on schedule, injects tracks into sensor DBs
via the `/inject` endpoint described above, and emits status to the
scoreboard. Mission definitions live in
[`scenario/scenarios/*.json`](scenario/scenarios/):

```json
{
  "name": "Standard Kill Chain",
  "duration_minutes": 60,
  "area_of_operations": {"lat": 37.9, "lon": -76.4, "radius_km": 250},
  "targets": [
    {"id":"TGT-FALLBROOK","delay_s":1800,"lat":37.8,"lon":-76.3,
     "description":"Fallbrook depot"}
  ]
}
```

Difficulty tiers (`tutorial`, `standard`, `hard`) vary target count,
timing, and how clearly the targets are announced. The scoreboard
reads scenario state and per-boomer telemetry and renders a single
HTML page at `:8080` (Swarm 2: `:8081`).

Scenario injection bypasses the sensor `hw` simulation entirely. That
is the largest deliberate departure from the live event, and it
exists because the practice range does not have a radar model. See
[`docs/FIDELITY.md`](docs/FIDELITY.md).

---

## 8. Attack surface — consolidated

The numbered list below is what makes a scenario run interesting.
Items are scenario features, not bugs in Viceroy. See
[`SECURITY.md`](SECURITY.md) for the distinction between scenario
content and real infrastructure bugs.

1. **`verify_signatures: false` in shipped configs.** Any peer that
   can reach `:10000` can inject commands with arbitrary `source`. A
   legitimate exploitation path is flipping a node's config file
   before `comms` reloads.
2. **World-writable `hw` and `comms` Unix sockets.** Once a player
   has local access to a node, they can read every inbound
   transmission and inject outbound ones or local hardware commands.
3. **Election socket replacement.** Replace the election socket with
   a local listener that always returns a populated COA and `control`
   believes the local node is leader.
4. **COA endorsement forgery.** Endorsements are filtered but not
   enforced on worker traffic. A forged or replayed endorsement
   bundle is a candidate attack.
5. **Sticky controller affinity on workers.** A sensor or boomer
   talks to its last-successful controller. One compromised
   controller retains influence even without quorum.
6. **`Shutdown` reply kills a daemon.** Any response whose `msg_type
   == "Shutdown"` ends the client. The kill-chain documentation
   leans on this.
7. **Beacon fetcher on Swarm 1.** Boomers download and execute
   `/run/fetcher/pulled_file` from the callback URL when in range.
   Swarm 2 disables boomer beacons — sensors are the entry point
   there.
8. **Unsigned fallback on missing key.** Pilferable or absent keys
   silently downgrade to unsigned traffic.
9. **`/inject` on sensors.** The scenario-engine endpoint is
   reachable by anything on the swarm network.
10. **No replay protection on nonces.** Captured transmissions
    replay cleanly.

---

## 9. Kill-chain — one plausible path

A walkthrough at the level of "what would a team actually do?" Full
operational detail lives in the team-facing guides
(`docs/TEAM-GUIDE.md`, `docs/BOOMER-WALKTHROUGH.md`).

1. **Establish foothold.** SSH into the attack station, start Sliver,
   stage a listener on `172.21.0.100`. Configure a boomer or sensor
   to beacon back using the range's reset-time knobs.
2. **Pivot onto the swarm network.** The attack station is not
   bridged — the first implant on a swarm-side node (typically via
   beacon fetcher on a Swarm 1 boomer, or sensor callback on Swarm 2)
   is the bridge.
3. **Map the swarm.** Walk `cfg.Controllers` / `cfg.Sensors` /
   `cfg.Boomers` from any node's config, or enumerate `:10000` on
   `172.20.0.0/24`. Every node's config lists every peer.
4. **Find the leader.** Query the election socket on each controller
   (if you have local access), or watch which controller answers
   worker polls with `202` responses.
5. **Choose a dial.** Flip `verify_signatures` to `false` in a
   controller's config and force a reload, or replace an election
   socket to forge leadership, or queue a `Shutdown` reply to walk
   boomers offline one at a time.
6. **Score.** The scoreboard counts intercepts on the scenario's
   scheduled targets. A compromised leader that suppresses
   engagements, or a hijacked boomer that engages a friendly track,
   moves the board.

---

## 10. Educational goals

A team that works through Viceroy should come out of it with:

- Fluency in the Mantis message envelope and the three traffic
  classes (election, worker poll, tasking).
- Practical exposure to pivoting from a restricted-PATH implant shell
  into a controller through a sensor or boomer session.
- Hands-on intuition for how a signature toggle, a COA oracle, and a
  Unix socket combine into an exploit.
- The habit of enumerating what a node has in PATH by walking
  `/opt/tools/` and `/usr/bin/` rather than assuming anything.
- The ability to read a protocol doc and write the daemon — which is
  the skill the live event actually tests.

It is **not** a substitute for the live event. See
[`docs/FIDELITY.md`](docs/FIDELITY.md) for an honest diff.

---

## 11. References

- VICEROY Virtual Institute — <https://www.viceroyscholars.org/>
- Gilded Guardian challenge series —
  <https://www.viceroyscholars.org/viceroy-cyber-competition-series-25-26/>
- Sliver C2 — <https://github.com/BishopFox/sliver>
- Source notes in [`docs/comp-materials/`](docs/comp-materials/) —
  `Controller.md`, `Sensor.md`, `Boomer.md`, `Instructions.md`,
  `Kill Swarm.md`, and a team-authored writeup. See
  [`NOTICE`](NOTICE) for attribution and fair-use statement.
