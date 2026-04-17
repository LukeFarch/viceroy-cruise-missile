// SPDX-License-Identifier: GPL-3.0-or-later
package config

import (
	"encoding/json"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Peer struct {
	ID     string `json:"id" yaml:"id"`
	PubKey string `json:"pub_key" yaml:"pub_key"`
	IPAddr string `json:"ip_addr" yaml:"ip_addr"`
}

type MissionBox struct {
	North float64 `json:"north" yaml:"north"`
	South float64 `json:"south" yaml:"south"`
	East  float64 `json:"east" yaml:"east"`
	West  float64 `json:"west" yaml:"west"`
}

type Mission struct {
	BoundingBox MissionBox `json:"bounding_box" yaml:"bounding_box"`
	FriendlyIFF []string   `json:"friendly_iff" yaml:"friendly_iff"`
}

type ElectionConfig struct {
	HeartbeatMs   int `json:"heartbeat_ms" yaml:"heartbeat_ms"`
	ElectionMinMs int `json:"election_timeout_min_ms" yaml:"election_timeout_min_ms"`
	ElectionMaxMs int `json:"election_timeout_max_ms" yaml:"election_timeout_max_ms"`
}

type HuntConfig struct {
	PollIntervalMs int     `json:"poll_interval_ms" yaml:"poll_interval_ms"`
	ReachDistanceM float64 `json:"reach_distance_meters" yaml:"reach_distance_meters"`
}

type BeaconConfig struct {
	Enabled          bool    `json:"enabled" yaml:"enabled"`
	CallbackURL      string  `json:"callback_url" yaml:"callback_url"`
	AttackStationLat float64 `json:"attack_station_lat" yaml:"attack_station_lat"`
	AttackStationLon float64 `json:"attack_station_lon" yaml:"attack_station_lon"`
	RangeKm          float64 `json:"range_km" yaml:"range_km"`
	CheckIntervalSec int     `json:"check_interval_sec" yaml:"check_interval_sec"`
}

type NodeConfig struct {
	ID                 string `json:"id" yaml:"id"`
	IFF                uint64 `json:"iff" yaml:"iff"`
	NodeType           string `json:"node_type,omitempty" yaml:"node_type,omitempty"`
	ListenAddress      string `json:"listen_address" yaml:"listen_address"`
	ListenPort         int    `json:"listen_port" yaml:"listen_port"`
	CommsSocketPath    string `json:"comms_socket_path" yaml:"comms_socket_path"`
	ElectionSocketPath string `json:"election_socket_path" yaml:"election_socket_path"`
	HWSocketPath       string `json:"hw_socket_path" yaml:"hw_socket_path"`
	KeyPath            string `json:"key_path" yaml:"key_path"`
	VerifySignatures   bool   `json:"verify_signatures" yaml:"verify_signatures"`
	AllowBroadcast     bool   `json:"allow_broadcast" yaml:"allow_broadcast"`
	Verbosity          string `json:"verbosity" yaml:"verbosity"`
	DBPath             string `json:"db_path" yaml:"db_path"`

	Controllers []Peer `json:"controllers" yaml:"controllers"`
	Sensors     []Peer `json:"sensors" yaml:"sensors"`
	Boomers     []Peer `json:"boomers" yaml:"boomers"`

	Mission  Mission        `json:"mission" yaml:"mission"`
	Election ElectionConfig `json:"election" yaml:"election"`
	Hunt     HuntConfig     `json:"hunt" yaml:"hunt"`
	Beacon   BeaconConfig   `json:"beacon" yaml:"beacon"`

	// Runtime: initial position for sensors/boomers
	InitialLat float64 `json:"initial_lat" yaml:"initial_lat"`
	InitialLon float64 `json:"initial_lon" yaml:"initial_lon"`
	InitialAlt float64 `json:"initial_alt" yaml:"initial_alt"`
}

func LoadConfig(path string) (*NodeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg NodeConfig
	// Support both YAML and JSON based on file extension
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	} else {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	}
	// Defaults — match real NixOS paths
	if cfg.ListenPort == 0 {
		cfg.ListenPort = 10000
	}
	if cfg.ListenAddress == "" {
		cfg.ListenAddress = "0.0.0.0"
	}
	if cfg.CommsSocketPath == "" {
		cfg.CommsSocketPath = "/run/commsDaemon/comms.sock"
	}
	if cfg.ElectionSocketPath == "" {
		cfg.ElectionSocketPath = "/run/electionDaemon/election.sock"
	}
	if cfg.HWSocketPath == "" {
		cfg.HWSocketPath = "/run/hwDaemon/hw.sock"
	}
	if cfg.Election.HeartbeatMs == 0 {
		cfg.Election.HeartbeatMs = 500
	}
	if cfg.Election.ElectionMinMs == 0 {
		cfg.Election.ElectionMinMs = 1500
	}
	if cfg.Election.ElectionMaxMs == 0 {
		cfg.Election.ElectionMaxMs = 3000
	}
	if cfg.Hunt.PollIntervalMs == 0 {
		cfg.Hunt.PollIntervalMs = 1000
	}
	if cfg.Hunt.ReachDistanceM == 0 {
		cfg.Hunt.ReachDistanceM = 50.0
	}
	if cfg.Beacon.RangeKm == 0 {
		cfg.Beacon.RangeKm = 450.0
	}
	if cfg.Beacon.CheckIntervalSec == 0 {
		cfg.Beacon.CheckIntervalSec = 5
	}
	return &cfg, nil
}

func (c *NodeConfig) SaveYAML(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c *NodeConfig) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
