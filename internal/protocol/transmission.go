// SPDX-License-Identifier: GPL-3.0-or-later
package protocol

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
)

// Transmission is the wire-format envelope for all inter-node communication.
// Matches the documented Mantis protocol exactly.
type Transmission struct {
	Destination string    `json:"destination"`
	Source      string    `json:"source"`
	Msg         string    `json:"msg"` // JSON-encoded payload string (base64 for election, plain for worker)
	MsgType     string    `json:"msg_type"`
	MsgSig      string    `json:"msg_sig"`
	Nonce       string    `json:"nonce"`
	Authority   Authority `json:"authority"`
}

type Authority struct {
	Endorsements []Endorsement `json:"endorsements"`
}

// Endorsement matches the real Mantis COA endorsement structure.
type Endorsement struct {
	ValidAfter string `json:"valid_after"`
	Expiration string `json:"expiration"`
	Endorser   string `json:"endorser"`
	Endorsee   string `json:"endorsee"`
	Signature  string `json:"signature"`
}

// Message type constants — exact strings from the documentation.
const (
	MsgBoomerGetTasks      = "Boomer:Get Tasks"
	MsgBoomerEngageRequest = "Boomer:Engage Request"
	MsgBoomerEngageError   = "Boomer:Engage Error"

	MsgSensorGetTasks     = "Sensor:Get Tasks"
	MsgSensorTrackUpdate  = "Sensor:Track Update"
	MsgSensorTrackRequest = "Sensor:Track Request"
	MsgSensorTrackReply   = "Sensor:Track Response"

	MsgElectionVoteRequest        = "Election:Vote Request"
	MsgElectionVoteResponse       = "Election:Vote Response"
	MsgElectionEndorsementRequest = "Election:Endorsement Request"
	MsgElectionEndorsementReply   = "Election:Endorsement Response"

	MsgShutdown = "Shutdown"
)

// GenerateNonce returns a 16-byte base64url-encoded random nonce.
func GenerateNonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// NewTransmission creates a Transmission with the payload marshalled into msg.
// For non-election messages, msg is plain JSON. For election messages, msg is base64-encoded JSON.
func NewTransmission(dst, src, msgType string, payload any) (*Transmission, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	msg := string(data)
	// Election payloads are base64-encoded per the real protocol
	if len(msgType) > 9 && msgType[:9] == "Election:" {
		msg = base64.StdEncoding.EncodeToString(data)
	}
	return &Transmission{
		Destination: dst,
		Source:      src,
		Msg:         msg,
		MsgType:     msgType,
		Nonce:       GenerateNonce(),
		Authority:   Authority{Endorsements: []Endorsement{}},
	}, nil
}

// ParseMsg unmarshals the inner msg field into dst.
// Handles both plain JSON and base64-encoded JSON.
func (t *Transmission) ParseMsg(dst any) error {
	// Try plain JSON first
	if err := json.Unmarshal([]byte(t.Msg), dst); err == nil {
		return nil
	}
	// Try base64-decoded JSON
	decoded, err := base64.StdEncoding.DecodeString(t.Msg)
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, dst)
}

// --- Payload types ---

type BoomerGetTasksPayload struct {
	CurrentLat float64 `json:"current_lat"`
	CurrentLon float64 `json:"current_lon"`
	CurrentAlt float64 `json:"current_alt"`
}

type BoomerEngageRequestPayload struct {
	TrackID    string `json:"track_id"`
	SensorID   string `json:"sensor_id"`
	SensorHost string `json:"sensor_host"`
}

type BoomerEngageErrorPayload struct {
	TrackID  string `json:"track_id"`
	ErrorMsg string `json:"error_msg"`
}

type SensorGetTasksPayload struct {
	CurrentLat    float64 `json:"current_lat"`
	CurrentLon    float64 `json:"current_lon"`
	CurrentAlt    float64 `json:"current_alt"`
	ServerAddress string  `json:"server_address"`
}

type SensorTrackUpdatePayload struct {
	Tracks []Track `json:"tracks"`
}

type Track struct {
	TrackID   string  `json:"track_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type SensorTrackRequestPayload struct {
	TrackID string `json:"track_id"`
}

// Election payloads — field names match real Mantis protocol exactly

type VoteRequestPayload struct {
	Leader string `json:"leader"` // was "candidate_id" — real protocol uses "leader"
	Term   uint64 `json:"term"`
}

type VoteResponsePayload struct {
	Term        uint64 `json:"term"`
	VoteGranted bool   `json:"vote_granted"`
}

type EndorsementRequestPayload struct {
	Term uint64 `json:"term"` // no leader_id — real protocol only has term
}

type EndorsementResponsePayload struct {
	Term        uint64            `json:"term"`
	Endorsement EndorsementObject `json:"endorsement"`
}

// EndorsementObject is the inner endorsement returned in an endorsement response.
type EndorsementObject struct {
	ValidAfter string `json:"valid_after"`
	Expiration string `json:"expiration"`
	Endorser   string `json:"endorser"`
	Endorsee   string `json:"endorsee"`
	Signature  string `json:"signature"`
}
