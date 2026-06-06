// Package protocol defines the JSON message types exchanged over the
// WebSocket between a connector and the broker.
package protocol

const (
	TypeHello     = "hello"
	TypeHelloAck  = "hello_ack"
	TypeHelloFail = "hello_fail"
	TypePing      = "ping"
	TypePong      = "pong"
)

// Envelope is the outer wrapper of every message. Type tells the decoder
// what concrete struct to unmarshal Payload into.
type Envelope struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

type Hello struct {
	Token        string `json:"token"`
	AgentVersion string `json:"agent_version"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Hostname     string `json:"hostname,omitempty"`
}

type HelloAck struct {
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
	HeartbeatSeconds int `json:"heartbeat_seconds"`
}

type HelloFail struct {
	Reason string `json:"reason"`
}

type Ping struct {
	Ts int64 `json:"ts"`
}

type Pong struct {
	Ts int64 `json:"ts"`
}
