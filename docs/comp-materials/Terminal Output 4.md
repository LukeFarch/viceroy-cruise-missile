---
tags:
lecture:
date:
related:
aliases:
created: 2026-03-28T19:52
modified: 2026-03-28T19:52
---

# Terminal Output 4

```bash
❯ scp ~/Downloads/check_leader.py operator1@sliver-69c7ea82bb324af474135496.vpn.cog:/tmp/payloads/
Received disconnect from 100.84.8.29 port 22:2: Too many authentication failures
Disconnected from 100.84.8.29 port 22
scp: Connection closed
❯ scp ~/Downloads/check_leader.py operator1@sliver-69c7ea82bb324af474135496.vpn.cog:/tmp/payloads/ -o PreferredAuthentications=password -o PubkeyAuthentication=no
PubkeyAuthentication=no: No such file or directory
❯ scp ~/Downloads/check_leader.py operator1@sliver-69c7ea82bb324af474135496.vpn.cog:/tmp/payloads/ -o PreferredAuthentications=password -o PubkeyAuthentication=no
❯
        scp -o PreferredAuthentications=password -o PubkeyAuthentication=no ~/Downloads/check_leader.py operator1@sliver-69c7ea82bb324af474135496.vpn.cog:/tmp/payloads/
operator1@sliver-69c7ea82bb324af474135496.vpn.cog's password:
scp: dest open "/tmp/payloads/": Failure
scp: failed to upload file /Users/Laura/Downloads/check_leader.py to /tmp/payloads/
❯
        scp -o PreferredAuthentications=password -o PubkeyAuthentication=no ~/Downloads/check_leader.py operator1@sliver-69c7ea82bb324af474135496.vpn.cog:/tmp/payloads/
operator1@sliver-69c7ea82bb324af474135496.vpn.cog's password:
check_leader.py                                                                                                                                100%  845    13.0KB/s   00:00
❯
        scp -o PreferredAuthentications=password -o PubkeyAuthentication=no ~/Downloads/sniff_comms.py operator1@sliver-69c7ea82bb324af474135496.vpn.cog:/tmp/payloads/
operator1@sliver-69c7ea82bb324af474135496.vpn.cog's password:
Permission denied, please try again.
operator1@sliver-69c7ea82bb324af474135496.vpn.cog's password:
Permission denied, please try again.
operator1@sliver-69c7ea82bb324af474135496.vpn.cog's password:
sniff_comms.py                                                                                                                                 100% 1770    28.1KB/s   00:00
❯
        scp -o PreferredAuthentications=password -o PubkeyAuthentication=no ~/Downloads/win_election.py operator1@sliver-69c7ea82bb324af474135496.vpn.cog:/tmp/payloads/
operator1@sliver-69c7ea82bb324af474135496.vpn.cog's password:
win_election.py                                                                                                                                100% 7853   110.2KB/s   00:00
❯
        scp -o PreferredAuthentications=password -o PubkeyAuthentication=no ~/Downloads/replace_election_socket.py operator1@sliver-69c7ea82bb324af474135496.vpn.cog:/tmp/payloads/
operator1@sliver-69c7ea82bb324af474135496.vpn.cog's password:
replace_election_socket.py                                                                                                                     100% 1635    25.0KB/s   00:00
❯
        scp -o PreferredAuthentications=password -o PubkeyAuthentication=no ~/Downloads/kill_swarm.py operator1@sliver-69c7ea82bb324af474135496.vpn.cog:/tmp/payloads/
operator1@sliver-69c7ea82bb324af474135496.vpn.cog's password:
Permission denied, please try again.
operator1@sliver-69c7ea82bb324af474135496.vpn.cog's password:
kill_swarm.py                                                                                                                                  100% 2489    31.7KB/s   00:00

    ~                                                                                                                                                             ✔  8s 
```
