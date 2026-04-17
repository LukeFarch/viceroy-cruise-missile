// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"viceroy/internal/config"

	"github.com/google/uuid"
)

type swarmParams struct {
	num          int
	prefix       string // "" for swarm 1, "s2-" for swarm 2
	outDir       string // "configs" or "configs-s2"
	internalNet  string // "172.20" or "172.22"
	beaconURL    string
	boomerBeacon bool // whether boomers get the fetcher
	sensorBeacon bool // whether sensors get the fetcher
}

func getSwarmParams(n int) swarmParams {
	switch n {
	case 2:
		return swarmParams{
			num:          2,
			prefix:       "s2-",
			outDir:       "configs-s2",
			internalNet:  "172.22",
			beaconURL:    "http://172.21.0.100:9443/payload.bin",
			boomerBeacon: false, // swarm 2: NO fetcher on boomers
			sensorBeacon: true,  // swarm 2: sensors ARE entry points
		}
	default:
		return swarmParams{
			num:          1,
			prefix:       "",
			outDir:       "configs",
			internalNet:  "172.20",
			beaconURL:    "http://172.21.0.100:8443/golden.exe",
			boomerBeacon: true,
			sensorBeacon: true,
		}
	}
}

func main() {
	swarmNum := flag.Int("swarm", 1, "swarm number (1 or 2)")
	flag.Parse()

	p := getSwarmParams(*swarmNum)
	outDir := p.outDir
	keyDir := filepath.Join(outDir, "keys")
	os.MkdirAll(keyDir, 0755)

	fmt.Printf("Generating swarm %d configs (subnet %s.x.x, output %s/)\n", p.num, p.internalNet, outDir)

	// Generate nodes
	type nodeInfo struct {
		id      string
		pubKey  string
		privPEM string
		ntype   string
		addr    string
		lat     float64
		lon     float64
	}

	var controllers, sensors, boomers []nodeInfo

	genNode := func(name, ntype, ip string, lat, lon float64) nodeInfo {
		pub, priv, _ := ed25519.GenerateKey(nil)
		pkix, _ := x509.MarshalPKIXPublicKey(pub)
		pkcs8, _ := x509.MarshalPKCS8PrivateKey(priv)
		b64pub := base64.StdEncoding.EncodeToString(pkix)
		privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})

		id := uuid.New().String()
		os.WriteFile(filepath.Join(keyDir, name+".pem"), privPEM, 0600)

		return nodeInfo{
			id: id, pubKey: b64pub, privPEM: string(privPEM),
			ntype: ntype, addr: fmt.Sprintf("http://%s:10000", ip),
			lat: lat, lon: lon,
		}
	}

	// AO Rizzo center: 34.0, -118.0 (fictional)
	// 5 Controllers per EXORD "standard package"
	controllers = append(controllers, genNode(p.prefix+"controller-1", "controller", p.internalNet+".1.1", 34.0, -118.0))
	controllers = append(controllers, genNode(p.prefix+"controller-2", "controller", p.internalNet+".1.2", 34.01, -117.99))
	controllers = append(controllers, genNode(p.prefix+"controller-3", "controller", p.internalNet+".1.3", 33.99, -118.01))
	controllers = append(controllers, genNode(p.prefix+"controller-4", "controller", p.internalNet+".1.4", 34.02, -118.02))
	controllers = append(controllers, genNode(p.prefix+"controller-5", "controller", p.internalNet+".1.5", 33.98, -117.98))

	// 6 Sensors per EXORD
	sensors = append(sensors, genNode(p.prefix+"sensor-1", "sensor", p.internalNet+".2.1", 34.05, -117.95))
	sensors = append(sensors, genNode(p.prefix+"sensor-2", "sensor", p.internalNet+".2.2", 33.95, -118.05))
	sensors = append(sensors, genNode(p.prefix+"sensor-3", "sensor", p.internalNet+".2.3", 34.06, -118.06))
	sensors = append(sensors, genNode(p.prefix+"sensor-4", "sensor", p.internalNet+".2.4", 33.94, -117.94))
	sensors = append(sensors, genNode(p.prefix+"sensor-5", "sensor", p.internalNet+".2.5", 34.03, -118.08))
	sensors = append(sensors, genNode(p.prefix+"sensor-6", "sensor", p.internalNet+".2.6", 33.97, -117.92))

	// 15 Strike Assets (boomers) per EXORD
	boomers = append(boomers, genNode(p.prefix+"boomer-1", "boomer", p.internalNet+".3.1", 34.02, -117.98))
	boomers = append(boomers, genNode(p.prefix+"boomer-2", "boomer", p.internalNet+".3.2", 33.98, -118.02))
	boomers = append(boomers, genNode(p.prefix+"boomer-3", "boomer", p.internalNet+".3.3", 34.0, -117.96))
	boomers = append(boomers, genNode(p.prefix+"boomer-4", "boomer", p.internalNet+".3.4", 34.01, -118.04))
	boomers = append(boomers, genNode(p.prefix+"boomer-5", "boomer", p.internalNet+".3.5", 33.97, -117.97))
	boomers = append(boomers, genNode(p.prefix+"boomer-6", "boomer", p.internalNet+".3.6", 34.03, -117.94))
	boomers = append(boomers, genNode(p.prefix+"boomer-7", "boomer", p.internalNet+".3.7", 33.96, -118.06))
	boomers = append(boomers, genNode(p.prefix+"boomer-8", "boomer", p.internalNet+".3.8", 34.04, -118.01))
	boomers = append(boomers, genNode(p.prefix+"boomer-9", "boomer", p.internalNet+".3.9", 33.99, -117.93))
	boomers = append(boomers, genNode(p.prefix+"boomer-10", "boomer", p.internalNet+".3.10", 34.05, -118.03))
	boomers = append(boomers, genNode(p.prefix+"boomer-11", "boomer", p.internalNet+".3.11", 33.95, -117.99))
	boomers = append(boomers, genNode(p.prefix+"boomer-12", "boomer", p.internalNet+".3.12", 34.02, -118.07))
	boomers = append(boomers, genNode(p.prefix+"boomer-13", "boomer", p.internalNet+".3.13", 33.93, -117.96))
	boomers = append(boomers, genNode(p.prefix+"boomer-14", "boomer", p.internalNet+".3.14", 34.06, -117.97))
	boomers = append(boomers, genNode(p.prefix+"boomer-15", "boomer", p.internalNet+".3.15", 33.94, -118.04))

	// Build peer lists
	ctrlPeers := make([]config.Peer, len(controllers))
	for i, c := range controllers {
		ctrlPeers[i] = config.Peer{ID: c.id, PubKey: c.pubKey, IPAddr: c.addr}
	}
	sensorPeers := make([]config.Peer, len(sensors))
	for i, s := range sensors {
		sensorPeers[i] = config.Peer{ID: s.id, PubKey: s.pubKey, IPAddr: s.addr}
	}
	boomerPeers := make([]config.Peer, len(boomers))
	for i, b := range boomers {
		boomerPeers[i] = config.Peer{ID: b.id, PubKey: b.pubKey, IPAddr: b.addr}
	}

	// Build friendly IFF list: all sensor and boomer entity IDs
	var friendlyIFF []string
	for i := range sensors {
		friendlyIFF = append(friendlyIFF, fmt.Sprintf("%d", 2000+i))
	}
	for i := range boomers {
		friendlyIFF = append(friendlyIFF, fmt.Sprintf("%d", 3000+i))
	}

	mission := config.Mission{
		BoundingBox: config.MissionBox{
			North: 34.1, South: 33.9, East: -117.9, West: -118.1,
		},
		FriendlyIFF: friendlyIFF,
	}

	// Beacons: controllers always get the fetcher
	ctrlBeacon := config.BeaconConfig{
		Enabled:          true,
		CallbackURL:      p.beaconURL,
		AttackStationLat: 34.3,
		AttackStationLon: -117.7,
		RangeKm:          450.0,
		CheckIntervalSec: 5,
	}

	sensorBeacon := config.BeaconConfig{
		Enabled:          p.sensorBeacon,
		CallbackURL:      p.beaconURL,
		AttackStationLat: 34.3,
		AttackStationLon: -117.7,
		RangeKm:          450.0,
		CheckIntervalSec: 5,
	}

	boomerBeacon := config.BeaconConfig{
		Enabled:          p.boomerBeacon,
		CallbackURL:      p.beaconURL,
		AttackStationLat: 34.3,
		AttackStationLon: -117.7,
		RangeKm:          450.0,
		CheckIntervalSec: 5,
	}

	// Generate controller configs
	for i, c := range controllers {
		name := fmt.Sprintf("%scontroller-%d", p.prefix, i+1)
		// Peer list excludes self
		var peers []config.Peer
		for j, pc := range ctrlPeers {
			if j != i {
				peers = append(peers, pc)
			}
		}
		cfg := &config.NodeConfig{
			ID:                 c.id,
			IFF:                uint64(1000 + i),
			ListenAddress:      "0.0.0.0",
			ListenPort:         10000,
			CommsSocketPath:    "/run/commsDaemon/comms.sock",
			ElectionSocketPath: "/run/electionDaemon/election.sock",
			HWSocketPath:       "/run/hwDaemon/hw.sock",
			KeyPath:            "/etc/controller/key.pem",
			VerifySignatures:   false, // VULNERABILITY: default false
			AllowBroadcast:     true,  // VULNERABILITY: accept broadcast messages
			Verbosity:          "info",
			Controllers:        peers,
			Sensors:            sensorPeers,
			Boomers:            boomerPeers,
			Mission:            mission,
			Beacon:             ctrlBeacon,
			InitialLat:         c.lat,
			InitialLon:         c.lon,
		}
		os.MkdirAll(filepath.Join(outDir, name), 0755)
		cfg.SaveYAML(filepath.Join(outDir, name, "config.yaml"))
		keyData, _ := os.ReadFile(filepath.Join(keyDir, name+".pem"))
		os.WriteFile(filepath.Join(outDir, name, "key.pem"), keyData, 0600)
		fmt.Printf("Generated %s (id=%s)\n", name, c.id)
	}

	// Generate sensor configs
	for i, s := range sensors {
		name := fmt.Sprintf("%ssensor-%d", p.prefix, i+1)
		cfg := &config.NodeConfig{
			ID:               s.id,
			IFF:              uint64(2000 + i),
			ListenAddress:    "0.0.0.0",
			ListenPort:       10000,
			CommsSocketPath:  "/run/commsDaemon/comms.sock",
			HWSocketPath:     "/run/hwDaemon/hw.sock",
			KeyPath:          "/etc/sensor/key.pem",
			VerifySignatures: false,
			AllowBroadcast:   true,
			Verbosity:        "info",
			DBPath:           "/var/lib/mantis/tracks.db",
			Controllers:      ctrlPeers,
			Sensors:          sensorPeers,
			Boomers:          boomerPeers,
			Beacon:           sensorBeacon,
			InitialLat:       s.lat,
			InitialLon:       s.lon,
			InitialAlt:       100.0,
		}
		os.MkdirAll(filepath.Join(outDir, name), 0755)
		cfg.SaveYAML(filepath.Join(outDir, name, "config.yaml"))
		keyData, _ := os.ReadFile(filepath.Join(keyDir, name+".pem"))
		os.WriteFile(filepath.Join(outDir, name, "key.pem"), keyData, 0600)
		fmt.Printf("Generated %s (id=%s)\n", name, s.id)
	}

	// Generate boomer configs
	for i, b := range boomers {
		name := fmt.Sprintf("%sboomer-%d", p.prefix, i+1)
		cfg := &config.NodeConfig{
			ID:               b.id,
			IFF:              uint64(3000 + i),
			ListenAddress:    "0.0.0.0",
			ListenPort:       10000,
			HWSocketPath:     "/run/hwDaemon/hw.sock",
			KeyPath:          "/etc/boomer/key.pem",
			VerifySignatures: false,
			AllowBroadcast:   true,
			Verbosity:        "info",
			Controllers:      ctrlPeers,
			Sensors:          sensorPeers,
			Boomers:          boomerPeers,
			Beacon:           boomerBeacon,
			InitialLat:       b.lat,
			InitialLon:       b.lon,
			InitialAlt:       500.0,
		}
		os.MkdirAll(filepath.Join(outDir, name), 0755)
		cfg.SaveYAML(filepath.Join(outDir, name, "config.yaml"))
		keyData, _ := os.ReadFile(filepath.Join(keyDir, name+".pem"))
		os.WriteFile(filepath.Join(outDir, name, "key.pem"), keyData, 0600)
		fmt.Printf("Generated %s (id=%s)\n", name, b.id)
	}

	fmt.Printf("\nSwarm %d configs generated in %s/\n", p.num, outDir)
	fmt.Printf("Keys generated in %s/\n", keyDir)
}
