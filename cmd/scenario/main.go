// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"viceroy/internal/protocol"
)

type ScenarioConfig struct {
	Name             string   `json:"name"`
	DurationMinutes  int      `json:"duration_minutes"`
	AOCenter         LatLon   `json:"ao_center"`
	AORadiusKm       float64  `json:"ao_radius_km"`
	Targets          []Target `json:"targets"`
	MaxStrikes       int      `json:"max_strikes"`
	SensorEndpoints  []string `json:"sensor_endpoints"`
	BoomerContainers []string `json:"boomer_containers"`
}

type LatLon struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Target represents a Halcyon defensive position that sensors will detect.
// These are STATIC — the targets don't move. The boomers fly toward them.
type Target struct {
	ID          string  `json:"id"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	DelaySec    int     `json:"delay_seconds"`
	Description string  `json:"description"`
}

type ScoreState struct {
	mu              sync.RWMutex
	Scenario        string         `json:"scenario"`
	StartTime       time.Time      `json:"start_time"`
	ElapsedSec      int            `json:"elapsed_sec"`
	DurationMin     int            `json:"duration_minutes"`
	TargetsInjected int            `json:"targets_injected"`
	TotalTargets    int            `json:"total_targets"`
	Strikes         int            `json:"strikes"`
	MaxStrikes      int            `json:"max_strikes"`
	GameOver        bool           `json:"game_over"`
	Result          string         `json:"result"`
	Targets         []TargetStatus `json:"targets"`
	Boomers         []BoomerStatus `json:"boomers"`
}

type TargetStatus struct {
	ID          string  `json:"id"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Active      bool    `json:"active"`
	Description string  `json:"description"`
	Injected    bool    `json:"injected"`
}

type BoomerStatus struct {
	Container string `json:"container"`
	Alive     bool   `json:"alive"`
	KilledBy  string `json:"killed_by"` // "engage_success", "defender", "unknown"
}

var score ScoreState

func main() {
	scenarioPath := os.Getenv("SCENARIO_PATH")
	if scenarioPath == "" {
		scenarioPath = "/etc/mantis/scenario.json"
	}

	data, err := os.ReadFile(scenarioPath)
	if err != nil {
		log.Fatalf("Failed to read scenario: %v", err)
	}

	var cfg ScenarioConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Failed to parse scenario: %v", err)
	}

	if cfg.MaxStrikes == 0 {
		cfg.MaxStrikes = 3
	}
	if cfg.DurationMinutes == 0 {
		cfg.DurationMinutes = 240 // 4 hours per EXORD
	}

	// Initialize target statuses
	var targetStatuses []TargetStatus
	for _, t := range cfg.Targets {
		targetStatuses = append(targetStatuses, TargetStatus{
			ID: t.ID, Lat: t.Lat, Lon: t.Lon,
			Active: false, Description: t.Description, Injected: false,
		})
	}

	// Initialize boomer statuses
	var boomerStatuses []BoomerStatus
	for _, c := range cfg.BoomerContainers {
		boomerStatuses = append(boomerStatuses, BoomerStatus{
			Container: c, Alive: true,
		})
	}

	score = ScoreState{
		Scenario:     cfg.Name,
		StartTime:    time.Now(),
		DurationMin:  cfg.DurationMinutes,
		TotalTargets: len(cfg.Targets),
		MaxStrikes:   cfg.MaxStrikes,
		Targets:      targetStatuses,
		Boomers:      boomerStatuses,
	}

	go runScoreAPI()

	// Grace period: wait for all containers to start before tracking deaths
	log.Printf("Waiting 30s for all containers to start...")
	time.Sleep(30 * time.Second)

	// Initialize boomer state from actual Docker state after grace period
	initState := getDockerState(cfg.BoomerContainers)
	score.mu.Lock()
	for i, b := range score.Boomers {
		score.Boomers[i].Alive = initState[b.Container]
	}
	score.StartTime = time.Now() // reset timer to after grace period
	score.mu.Unlock()

	log.Printf("Scenario '%s': %d targets, max %d strikes, %d min duration",
		cfg.Name, len(cfg.Targets), cfg.MaxStrikes, cfg.DurationMinutes)

	client := &http.Client{Timeout: 5 * time.Second}

	// Docker API client via Unix socket
	dockerClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", "/var/run/docker.sock")
			},
		},
		Timeout: 3 * time.Second,
	}

	// Track which targets have been injected
	injected := make(map[int]bool)

	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		elapsed := int(time.Since(score.StartTime).Seconds())

		score.mu.Lock()
		score.ElapsedSec = elapsed

		// Check game over conditions
		if !score.GameOver {
			if elapsed >= cfg.DurationMinutes*60 {
				score.GameOver = true
				score.Result = "MISSION SUCCESS — engagement window expired, AO Rizzo defended"
				log.Printf("SCENARIO END: %s", score.Result)
			}
		}
		gameOver := score.GameOver
		score.mu.Unlock()

		if gameOver {
			continue
		}

		// Inject targets on schedule
		for i, t := range cfg.Targets {
			if !injected[i] && elapsed >= t.DelaySec {
				injected[i] = true
				tracks := []protocol.Track{{
					TrackID:   t.ID,
					Latitude:  t.Lat,
					Longitude: t.Lon,
				}}
				trackData, _ := json.Marshal(tracks)

				for _, ep := range cfg.SensorEndpoints {
					go func(endpoint string) {
						client.Post(endpoint+"/inject", "application/json", bytes.NewReader(trackData))
					}(ep)
				}

				score.mu.Lock()
				score.TargetsInjected++
				for j := range score.Targets {
					if score.Targets[j].ID == t.ID {
						score.Targets[j].Injected = true
						score.Targets[j].Active = true
					}
				}
				score.mu.Unlock()

				log.Printf("Target %s injected: %s (%.4f, %.4f)", t.ID, t.Description, t.Lat, t.Lon)
			}
		}

		// Monitor boomer container state via Docker API
		dresp, derr := dockerClient.Get("http://localhost/containers/json?all=true")
		if derr == nil {
			body, _ := io.ReadAll(dresp.Body)
			dresp.Body.Close()

			var containers []struct {
				Names []string `json:"Names"`
				State string   `json:"State"`
			}
			if json.Unmarshal(body, &containers) == nil {
				containerState := make(map[string]bool)
				for _, c := range containers {
					for _, name := range c.Names {
						name = strings.TrimPrefix(name, "/")
						containerState[name] = (c.State == "running")
					}
				}

				score.mu.Lock()
				for i, b := range score.Boomers {
					wasAlive := b.Alive
					nowAlive := containerState[b.Container]
					score.Boomers[i].Alive = nowAlive

					if wasAlive && !nowAlive {
						// Boomer just died — count as a strike
						// (In real CTF, os.Exit(0) on successful engage kills the container)
						score.Strikes++
						score.Boomers[i].KilledBy = "engage_success"
						log.Printf("STRIKE #%d/%d — %s destroyed (successful engagement)",
							score.Strikes, score.MaxStrikes, b.Container)

						if score.Strikes >= score.MaxStrikes {
							score.GameOver = true
							score.Result = "MISSION FAILURE — AO Rizzo defenses overwhelmed"
							log.Printf("SCENARIO END: %s", score.Result)
						}
					}
				}
				score.mu.Unlock()
			}
		}
	}
}

func getDockerState(containers []string) map[string]bool {
	dockerClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", "/var/run/docker.sock")
			},
		},
		Timeout: 3 * time.Second,
	}
	state := make(map[string]bool)
	dresp, err := dockerClient.Get("http://localhost/containers/json?all=true")
	if err != nil {
		return state
	}
	body, _ := io.ReadAll(dresp.Body)
	dresp.Body.Close()
	var ctrs []struct {
		Names []string `json:"Names"`
		State string   `json:"State"`
	}
	if json.Unmarshal(body, &ctrs) == nil {
		for _, c := range ctrs {
			for _, name := range c.Names {
				name = strings.TrimPrefix(name, "/")
				state[name] = (c.State == "running")
			}
		}
	}
	return state
}

func runScoreAPI() {
	http.HandleFunc("/score", func(w http.ResponseWriter, r *http.Request) {
		score.mu.RLock()
		data, _ := json.MarshalIndent(score, "", "  ")
		score.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(data)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	log.Println("Scenario score API on :9090")
	http.ListenAndServe(":9090", nil)
}
