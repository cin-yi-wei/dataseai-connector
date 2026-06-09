// Package protocol defines the JSON message types exchanged over the
// WebSocket between a connector and the broker.
package protocol

const (
	TypeHello     = "hello"
	TypeHelloAck  = "hello_ack"
	TypeHelloFail = "hello_fail"
	TypePing      = "ping"
	TypePong      = "pong"

	// Query relay
	TypeQueryRequest = "query_request"
	TypeQueryMeta    = "query_meta"
	TypeQueryRows    = "query_rows"
	TypeQueryDone    = "query_done"
	TypeQueryError   = "query_error"
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
	AgentID          string `json:"agent_id"`
	SessionID        string `json:"session_id"`
	HeartbeatSeconds int    `json:"heartbeat_seconds"`
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

// MySQLTarget identifies the database the broker wants the connector to
// run the query against.
type MySQLTarget struct {
	Host     string     `json:"host"`
	Port     int        `json:"port"`
	User     string     `json:"user"`
	Password string     `json:"password,omitempty"`
	Database string     `json:"database,omitempty"`
	SSH      *SSHConfig `json:"ssh,omitempty"`
}

type SSHConfig struct {
	Host          string `json:"host"`
	Port          int    `json:"port,omitempty"`
	User          string `json:"user"`
	Password      string `json:"password,omitempty"`
	PrivateKey    string `json:"private_key,omitempty"`
	KeyPassphrase string `json:"key_passphrase,omitempty"`
}

func (c *SSHConfig) IsZero() bool {
	return c == nil || c.Host == "" || c.User == ""
}

// QueryRequest is sent by the broker to ask the connector to execute SQL.
type QueryRequest struct {
	RequestID string      `json:"request_id"`
	Target    MySQLTarget `json:"target"`
	SQL       string      `json:"sql"`
	// Dialect selects the database driver: "mysql" (default/empty) or "postgres".
	Dialect string `json:"dialect,omitempty"`
	// MaxRows caps how many rows the connector will stream back. 0 = no cap.
	MaxRows int `json:"max_rows,omitempty"`
	// BatchSize is the row batch size for streaming. 0 → connector picks.
	BatchSize int `json:"batch_size,omitempty"`
}

// QueryMeta is the first response after a request. Sent before any rows.
type QueryMeta struct {
	RequestID string    `json:"request_id"`
	Columns   []ColInfo `json:"columns"`
}

type ColInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// QueryRows is a streamed batch of row data. Values are heterogeneous
// (string / number / null) so use []any per row.
type QueryRows struct {
	RequestID string  `json:"request_id"`
	Rows      [][]any `json:"rows"`
}

// QueryDone marks the end of a successful query stream.
type QueryDone struct {
	RequestID  string `json:"request_id"`
	RowCount   int    `json:"row_count"`
	DurationMs int64  `json:"duration_ms"`
}

// QueryError marks the end of a failed query stream.
type QueryError struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error"`
}
