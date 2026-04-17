// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
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

// --- Multiplexer: per-destination outbound queues ---

type Multiplexer struct {
	mu     sync.Mutex
	queues map[string]chan *protocol.Transmission // destination UUID → outbound queue
	subs   []chan *protocol.Transmission          // local subscribers (control, election)
}

func NewMultiplexer() *Multiplexer {
	return &Multiplexer{queues: make(map[string]chan *protocol.Transmission)}
}

func (m *Multiplexer) Subscribe() chan *protocol.Transmission {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan *protocol.Transmission, 100)
	m.subs = append(m.subs, ch)
	return ch
}

func (m *Multiplexer) Broadcast(t *protocol.Transmission) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.subs {
		select {
		case ch <- t:
		default:
		}
	}
}

func (m *Multiplexer) Enqueue(dst string, t *protocol.Transmission) {
	m.mu.Lock()
	q, ok := m.queues[dst]
	if !ok {
		q = make(chan *protocol.Transmission, 50)
		m.queues[dst] = q
	}
	m.mu.Unlock()
	select {
	case q <- t:
	default:
	}
}

func (m *Multiplexer) Dequeue(dst string) *protocol.Transmission {
	m.mu.Lock()
	q, ok := m.queues[dst]
	m.mu.Unlock()
	if !ok {
		return nil
	}
	select {
	case t := <-q:
		return t
	default:
		return nil
	}
}

// --- Election state ---

type ElectionState int

const (
	Follower ElectionState = iota
	Candidate
	Leader
)

type Election struct {
	mu           sync.RWMutex
	state        ElectionState
	term         uint64
	votedFor     string
	leaderID     string
	votes        int
	peerCount    int
	nodeID       string
	endorsements []protocol.Endorsement
}

func (e *Election) IsLeader() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state == Leader
}

func (e *Election) GetTerm() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.term
}

func (e *Election) GetLeaderID() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.leaderID
}

// --- Control state ---

type ControlState struct {
	mu            sync.Mutex
	sensorLocs    map[string][3]float64 // id → [lat, lon, alt]
	sensorAddrs   map[string]string     // id → track server URL
	boomerLocs    map[string][3]float64
	trackToBoomer map[string]string // track_id → boomer_id
	boomerToTrack map[string]string // boomer_id → track_id
}

func NewControlState() *ControlState {
	return &ControlState{
		sensorLocs:    make(map[string][3]float64),
		sensorAddrs:   make(map[string]string),
		boomerLocs:    make(map[string][3]float64),
		trackToBoomer: make(map[string]string),
		boomerToTrack: make(map[string]string),
	}
}

func main() {
	cfgPath := os.Getenv("CONTROLLER_CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "/etc/controller/config.yaml"
	}
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	var privKey ed25519.PrivateKey
	if cfg.KeyPath != "" {
		privKey, err = protocol.LoadPrivateKey(cfg.KeyPath)
		if err != nil {
			log.Printf("WARN: could not load private key: %v (messages will be unsigned)", err)
		}
	}

	mux := NewMultiplexer()
	election := &Election{
		state:     Follower,
		term:      0,
		peerCount: len(cfg.Controllers),
		nodeID:    cfg.ID,
	}
	ctrl := NewControlState()

	// Build public key map for signature verification
	pubKeys := make(map[string]ed25519.PublicKey)
	for _, peers := range [][]config.Peer{cfg.Controllers, cfg.Sensors, cfg.Boomers} {
		for _, p := range peers {
			if p.PubKey != "" {
				if pk, err := protocol.LoadPublicKeyBase64(p.PubKey); err == nil {
					pubKeys[p.ID] = pk
				}
			}
		}
	}

	ctx := context.Background()

	// Start comms HTTP server
	go runCommsServer(ctx, cfg, mux, election, privKey, pubKeys)

	// Start comms Unix socket (world-writable — VULNERABILITY)
	go runCommsSocket(ctx, cfg, mux, election)

	// Start election daemon
	go runElection(ctx, cfg, mux, election, privKey)

	// Start election socket server
	go runElectionSocket(ctx, cfg, election)

	// Start control daemon
	go runControl(ctx, cfg, mux, election, ctrl, privKey)

	// Start peer communicator workers
	for _, peer := range cfg.Controllers {
		go runPeerCommunicator(ctx, peer, mux)
	}

	// Start fetcher/auto-puller (runs on ALL node types in real competition)
	if cfg.Beacon.Enabled {
		go runFetcher(ctx, cfg)
	}

	log.Printf("Controller %s started on :%d", cfg.ID, cfg.ListenPort)
	select {} // block forever
}

// ===========================================================================
// Fetcher / Auto-puller — downloads and executes implant from callback URL
// Runs on ALL node types in the real competition (not just boomers)
// ===========================================================================

func runFetcher(ctx context.Context, cfg *config.NodeConfig) {
	log.Printf("fetcher: polling %s every %ds", cfg.Beacon.CallbackURL, cfg.Beacon.CheckIntervalSec)

	for {
		time.Sleep(time.Duration(cfg.Beacon.CheckIntervalSec) * time.Second)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(cfg.Beacon.CallbackURL)
		if err != nil {
			continue // silently retry
		}

		payload, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 || len(payload) < 100 {
			continue
		}

		// Write to /run/fetcher/pulled_file (matches real NixOS path)
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
		return // fetcher exits after successful pull (one-shot per the real behavior)
	}
}

// --- Comms HTTP Server ---

func runCommsServer(ctx context.Context, cfg *config.NodeConfig, mux *Multiplexer, election *Election, privKey ed25519.PrivateKey, pubKeys map[string]ed25519.PublicKey) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var tx protocol.Transmission
		if err := json.Unmarshal(body, &tx); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		// Signature verification (only when enabled)
		if cfg.VerifySignatures {
			if pk, ok := pubKeys[tx.Source]; ok {
				if !protocol.Verify(&tx, pk) {
					log.Printf("Signature verification failed for %s from %s", tx.MsgType, tx.Source)
					http.Error(w, "signature verification failed", http.StatusForbidden)
					return
				}
			}
		}

		// Handle Shutdown — VULNERABILITY: causes immediate exit
		if tx.MsgType == protocol.MsgShutdown {
			log.Printf("Received Shutdown from %s — exiting", tx.Source)
			os.Exit(0)
		}

		// Non-controller traffic only accepted by leader
		isController := false
		for _, c := range cfg.Controllers {
			if c.ID == tx.Source {
				isController = true
				break
			}
		}
		if !isController && !election.IsLeader() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		// Broadcast to local subscribers
		mux.Broadcast(&tx)

		// Check for queued outbound response for this source
		if resp := mux.Dequeue(tx.Source); resp != nil {
			protocol.Sign(resp, privKey)
			data, _ := json.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			w.Write(data)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	addr := fmt.Sprintf("%s:%d", cfg.ListenAddress, cfg.ListenPort)
	server := &http.Server{Addr: addr, Handler: handler}
	log.Printf("Comms HTTP server listening on %s", addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

// --- Comms Unix Socket (world-writable — VULNERABILITY) ---

func runCommsSocket(ctx context.Context, cfg *config.NodeConfig, mux *Multiplexer, election *Election) {
	os.MkdirAll(filepath.Dir(cfg.CommsSocketPath), 0755)
	os.Remove(cfg.CommsSocketPath)
	ln, err := net.Listen("unix", cfg.CommsSocketPath)
	if err != nil {
		log.Fatalf("Failed to listen on comms socket: %v", err)
	}
	// VULNERABILITY: world-writable socket
	os.Chmod(cfg.CommsSocketPath, 0777)
	log.Printf("Comms Unix socket at %s (mode 0777)", cfg.CommsSocketPath)

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleCommsSocketConn(conn, mux, election)
	}
}

func handleCommsSocketConn(conn net.Conn, mux *Multiplexer, election *Election) {
	defer conn.Close()
	sub := mux.Subscribe()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	// Read outbound messages from local daemon, enqueue them
	go func() {
		for {
			var tx protocol.Transmission
			if err := dec.Decode(&tx); err != nil {
				return
			}
			if tx.Destination != "" {
				mux.Enqueue(tx.Destination, &tx)
			}
			// When a VoteRequest is injected via the socket, sync the
			// election state machine so it expects to receive votes.
			// This mirrors real Mantis where comms and election are
			// separate daemons sharing state through the socket bus.
			if tx.MsgType == protocol.MsgElectionVoteRequest {
				var payload protocol.VoteRequestPayload
				tx.ParseMsg(&payload)
				election.mu.Lock()
				if payload.Term > election.term || payload.Term == election.term {
					election.term = payload.Term
					election.state = Candidate
					election.votes = 1 // self-vote
					election.votedFor = payload.Leader
				}
				election.mu.Unlock()
			}
		}
	}()

	// Send inbound messages to local daemon
	for tx := range sub {
		if err := enc.Encode(tx); err != nil {
			return
		}
	}
}

// --- Election Socket (returns certificate of authority) ---

func runElectionSocket(ctx context.Context, cfg *config.NodeConfig, election *Election) {
	os.MkdirAll(filepath.Dir(cfg.ElectionSocketPath), 0755)
	os.Remove(cfg.ElectionSocketPath)
	ln, err := net.Listen("unix", cfg.ElectionSocketPath)
	if err != nil {
		log.Fatalf("Failed to listen on election socket: %v", err)
	}
	os.Chmod(cfg.ElectionSocketPath, 0777)

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			election.mu.RLock()
			resp := map[string]any{
				"endorsements": election.endorsements,
			}
			election.mu.RUnlock()
			json.NewEncoder(c).Encode(resp)
		}(conn)
	}
}

// --- Election Daemon (Simplified RAFT) ---

func runElection(ctx context.Context, cfg *config.NodeConfig, mux *Multiplexer, election *Election, privKey ed25519.PrivateKey) {
	inbox := mux.Subscribe()

	electionTimeout := func() time.Duration {
		min := cfg.Election.ElectionMinMs
		max := cfg.Election.ElectionMaxMs
		return time.Duration(min+rand.Intn(max-min)) * time.Millisecond
	}
	heartbeatInterval := time.Duration(cfg.Election.HeartbeatMs) * time.Millisecond

	timer := time.NewTimer(electionTimeout())

	for {
		select {
		case <-timer.C:
			election.mu.Lock()
			if election.state != Leader {
				// Start election
				election.state = Candidate
				election.term++
				election.votedFor = cfg.ID
				election.votes = 1 // vote for self
				term := election.term
				election.mu.Unlock()

				log.Printf("Starting election for term %d", term)

				for _, peer := range cfg.Controllers {
					tx, _ := protocol.NewTransmission(peer.ID, cfg.ID, protocol.MsgElectionVoteRequest,
						protocol.VoteRequestPayload{Term: term, Leader: cfg.ID})
					protocol.Sign(tx, privKey)
					mux.Enqueue(peer.ID, tx)
				}
				timer.Reset(electionTimeout())
			} else {
				// Leader: send endorsement requests (heartbeat)
				term := election.term
				election.mu.Unlock()

				for _, peer := range cfg.Controllers {
					tx, _ := protocol.NewTransmission(peer.ID, cfg.ID, protocol.MsgElectionEndorsementRequest,
						protocol.EndorsementRequestPayload{Term: term})
					protocol.Sign(tx, privKey)
					mux.Enqueue(peer.ID, tx)
				}
				timer.Reset(heartbeatInterval)
			}

		case tx := <-inbox:
			if !strings.HasPrefix(tx.MsgType, "Election:") {
				continue
			}

			switch tx.MsgType {
			case protocol.MsgElectionVoteRequest:
				var payload protocol.VoteRequestPayload
				tx.ParseMsg(&payload)

				election.mu.Lock()
				granted := false
				// VULNERABILITY: higher term always wins — can be exploited with spoofed term
				if payload.Term > election.term {
					election.term = payload.Term
					election.state = Follower
					election.votedFor = payload.Leader
					election.leaderID = ""
					granted = true
				} else if payload.Term == election.term && (election.votedFor == "" || election.votedFor == payload.Leader) {
					election.votedFor = payload.Leader
					granted = true
				}
				election.mu.Unlock()

				resp, _ := protocol.NewTransmission(tx.Source, cfg.ID, protocol.MsgElectionVoteResponse,
					protocol.VoteResponsePayload{Term: payload.Term, VoteGranted: granted})
				protocol.Sign(resp, privKey)
				mux.Enqueue(tx.Source, resp)

				timer.Reset(time.Duration(cfg.Election.ElectionMinMs+rand.Intn(cfg.Election.ElectionMaxMs-cfg.Election.ElectionMinMs)) * time.Millisecond)

			case protocol.MsgElectionVoteResponse:
				var payload protocol.VoteResponsePayload
				tx.ParseMsg(&payload)

				election.mu.Lock()
				if payload.VoteGranted && payload.Term == election.term && election.state == Candidate {
					election.votes++
					quorum := (election.peerCount+1)/2 + 1
					if election.votes >= quorum {
						election.state = Leader
						election.leaderID = cfg.ID
						log.Printf("Became leader for term %d", election.term)
					}
				}
				if payload.Term > election.term {
					election.term = payload.Term
					election.state = Follower
					election.votedFor = ""
					election.leaderID = ""
				}
				election.mu.Unlock()

			case protocol.MsgElectionEndorsementRequest:
				var payload protocol.EndorsementRequestPayload
				tx.ParseMsg(&payload)

				election.mu.Lock()
				if payload.Term >= election.term {
					election.term = payload.Term
					election.state = Follower
					election.leaderID = tx.Source
					election.votedFor = ""

					resp, _ := protocol.NewTransmission(tx.Source, cfg.ID, protocol.MsgElectionEndorsementReply,
						protocol.EndorsementResponsePayload{
							Term: payload.Term,
							Endorsement: protocol.EndorsementObject{
								ValidAfter: time.Now().UTC().Format(time.RFC3339),
								Expiration: time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339),
								Endorser:   cfg.ID,
								Endorsee:   tx.Source,
								Signature:  "", // filled by Sign()
							},
						})
					protocol.Sign(resp, privKey)
					mux.Enqueue(tx.Source, resp)
				}
				election.mu.Unlock()

				timer.Reset(time.Duration(cfg.Election.ElectionMinMs+rand.Intn(cfg.Election.ElectionMaxMs-cfg.Election.ElectionMinMs)) * time.Millisecond)

			case protocol.MsgElectionEndorsementReply:
				var payload protocol.EndorsementResponsePayload
				tx.ParseMsg(&payload)

				election.mu.Lock()
				election.endorsements = append(election.endorsements, protocol.Endorsement{
					ValidAfter: payload.Endorsement.ValidAfter,
					Expiration: payload.Endorsement.Expiration,
					Endorser:   payload.Endorsement.Endorser,
					Endorsee:   payload.Endorsement.Endorsee,
					Signature:  payload.Endorsement.Signature,
				})
				election.mu.Unlock()
			}
		}
	}
}

// --- Control Daemon (Mission Logic) ---

func runControl(ctx context.Context, cfg *config.NodeConfig, mux *Multiplexer, election *Election, ctrl *ControlState, privKey ed25519.PrivateKey) {
	inbox := mux.Subscribe()

	for tx := range inbox {
		if strings.HasPrefix(tx.MsgType, "Election:") {
			continue
		}
		if !election.IsLeader() {
			continue
		}

		switch tx.MsgType {
		case protocol.MsgSensorGetTasks:
			var payload protocol.SensorGetTasksPayload
			tx.ParseMsg(&payload)

			ctrl.mu.Lock()
			ctrl.sensorLocs[tx.Source] = [3]float64{payload.CurrentLat, payload.CurrentLon, payload.CurrentAlt}
			if payload.ServerAddress != "" {
				addr := payload.ServerAddress
				if !strings.HasSuffix(addr, "/tracks/") {
					addr = strings.TrimSuffix(addr, "/") + "/tracks/"
				}
				ctrl.sensorAddrs[tx.Source] = addr
			}
			ctrl.mu.Unlock()

		case protocol.MsgSensorTrackUpdate:
			var payload protocol.SensorTrackUpdatePayload
			tx.ParseMsg(&payload)

			ctrl.mu.Lock()
			// Only process if sensor has registered
			if _, ok := ctrl.sensorAddrs[tx.Source]; !ok {
				ctrl.mu.Unlock()
				continue
			}

			for _, track := range payload.Tracks {
				// Skip friendly IFF
				friendly := false
				for _, iff := range cfg.Mission.FriendlyIFF {
					if track.TrackID == iff {
						friendly = true
						break
					}
				}
				if friendly {
					continue
				}

				// Skip if outside bounding box
				bb := cfg.Mission.BoundingBox
				if !protocol.InsideBoundingBox(track.Latitude, track.Longitude, bb.North, bb.South, bb.East, bb.West) {
					continue
				}

				// Skip if already assigned
				if _, ok := ctrl.trackToBoomer[track.TrackID]; ok {
					continue
				}

				// Find closest available boomer
				bestDist := math.MaxFloat64
				bestBoomer := ""
				for _, bp := range cfg.Boomers {
					if _, busy := ctrl.boomerToTrack[bp.ID]; busy {
						continue
					}
					if loc, ok := ctrl.boomerLocs[bp.ID]; ok {
						d := protocol.HaversineMeters(loc[0], loc[1], track.Latitude, track.Longitude)
						if d < bestDist {
							bestDist = d
							bestBoomer = bp.ID
						}
					}
				}

				if bestBoomer != "" {
					ctrl.trackToBoomer[track.TrackID] = bestBoomer
					ctrl.boomerToTrack[bestBoomer] = track.TrackID

					// Find sensor host for this track
					sensorHost := ctrl.sensorAddrs[tx.Source]

					engage, _ := protocol.NewTransmission(bestBoomer, cfg.ID, protocol.MsgBoomerEngageRequest,
						protocol.BoomerEngageRequestPayload{
							TrackID:    track.TrackID,
							SensorID:   tx.Source,
							SensorHost: sensorHost,
						})
					protocol.Sign(engage, privKey)
					mux.Enqueue(bestBoomer, engage)

					log.Printf("Assigned boomer %s to track %s (dist=%.0fm)", bestBoomer[:8], track.TrackID, bestDist)
				}
			}
			ctrl.mu.Unlock()

		case protocol.MsgBoomerGetTasks:
			var payload protocol.BoomerGetTasksPayload
			tx.ParseMsg(&payload)

			ctrl.mu.Lock()
			ctrl.boomerLocs[tx.Source] = [3]float64{payload.CurrentLat, payload.CurrentLon, payload.CurrentAlt}
			ctrl.mu.Unlock()
			// Queued engage requests are returned via the HTTP handler's Dequeue

		case protocol.MsgBoomerEngageError:
			var payload protocol.BoomerEngageErrorPayload
			tx.ParseMsg(&payload)

			ctrl.mu.Lock()
			if boomerID, ok := ctrl.trackToBoomer[payload.TrackID]; ok {
				delete(ctrl.boomerToTrack, boomerID)
				delete(ctrl.trackToBoomer, payload.TrackID)
				log.Printf("Boomer %s reported engage error for track %s: %s", tx.Source[:8], payload.TrackID, payload.ErrorMsg)
			}
			ctrl.mu.Unlock()
		}
	}
}

// --- Peer Communicator Worker ---

func runPeerCommunicator(ctx context.Context, peer config.Peer, mux *Multiplexer) {
	mux.mu.Lock()
	q, ok := mux.queues[peer.ID]
	if !ok {
		q = make(chan *protocol.Transmission, 50)
		mux.queues[peer.ID] = q
	}
	mux.mu.Unlock()

	client := &http.Client{Timeout: 5 * time.Second}

	for tx := range q {
		data, _ := json.Marshal(tx)
		resp, err := client.Post(peer.IPAddr, "application/json", strings.NewReader(string(data)))
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if len(body) > 2 { // non-empty response
			var reply protocol.Transmission
			if json.Unmarshal(body, &reply) == nil && reply.MsgType != "" {
				mux.Broadcast(&reply)
			}
		}
	}
}
