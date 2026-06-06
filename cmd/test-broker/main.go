// test-broker is a throwaway local broker for developing the connector.
// Not for production. Accepts any non-empty token. After the handshake it
// fires a single demo query at the connector and logs the streamed result.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/cin-yi-wei/dataseai-connector/pkg/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/agent", handleAgent)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("dataseai test-broker; agents connect to /agent\n"))
	})

	log.Printf("test-broker listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func handleAgent(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // dev only
	})
	if err != nil {
		log.Printf("accept: %v", err)
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var first protocol.Envelope
	if err := wsjson.Read(ctx, conn, &first); err != nil {
		log.Printf("read hello: %v", err)
		return
	}
	if first.Type != protocol.TypeHello {
		log.Printf("expected hello, got %q", first.Type)
		_ = wsjson.Write(ctx, conn, protocol.Envelope{
			Type:    protocol.TypeHelloFail,
			Payload: protocol.HelloFail{Reason: "expected hello as first message"},
		})
		return
	}
	var hello protocol.Hello
	_ = remarshal(first.Payload, &hello)
	if hello.Token == "" {
		log.Printf("rejecting connection: empty token")
		_ = wsjson.Write(ctx, conn, protocol.Envelope{
			Type:    protocol.TypeHelloFail,
			Payload: protocol.HelloFail{Reason: "empty token"},
		})
		return
	}

	agentID := randID(8)
	sessionID := randID(12)
	log.Printf("hello: agent=%s host=%s os=%s/%s version=%s token=%s...",
		agentID, hello.Hostname, hello.OS, hello.Arch, hello.AgentVersion, shortTok(hello.Token))

	s := &serverSession{conn: conn, agentID: agentID}
	if err := s.send(ctx, protocol.Envelope{
		Type: protocol.TypeHelloAck,
		Payload: protocol.HelloAck{
			AgentID:          agentID,
			SessionID:        sessionID,
			HeartbeatSeconds: 5,
		},
	}); err != nil {
		log.Printf("send hello_ack: %v", err)
		return
	}

	// Demo: send a query 2 seconds after handshake.
	go func() {
		time.Sleep(2 * time.Second)
		req := protocol.QueryRequest{
			RequestID: "req_" + randID(4),
			Target: protocol.MySQLTarget{
				Host:     "10.0.0.50",
				Port:     3306,
				User:     "admin",
				Database: "demo",
			},
			SQL: "SELECT id, name, email FROM users LIMIT 50",
		}
		log.Printf("[%s] sending demo query %s", agentID, req.RequestID)
		if err := s.send(ctx, protocol.Envelope{
			Type:    protocol.TypeQueryRequest,
			Payload: req,
		}); err != nil {
			log.Printf("[%s] send query_request: %v", agentID, err)
		}
	}()

	for {
		var msg protocol.Envelope
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("[%s] read: %v", agentID, err)
			return
		}
		switch msg.Type {
		case protocol.TypePing:
			var ping protocol.Ping
			_ = remarshal(msg.Payload, &ping)
			log.Printf("[%s] ping ts=%d", agentID, ping.Ts)
			if err := s.send(ctx, protocol.Envelope{
				Type:    protocol.TypePong,
				Payload: protocol.Pong{Ts: time.Now().UnixMilli()},
			}); err != nil {
				log.Printf("[%s] write pong: %v", agentID, err)
				return
			}
		case protocol.TypeQueryMeta:
			var meta protocol.QueryMeta
			_ = remarshal(msg.Payload, &meta)
			cols := make([]string, len(meta.Columns))
			for i, c := range meta.Columns {
				cols[i] = fmt.Sprintf("%s %s", c.Name, c.Type)
			}
			log.Printf("[%s][%s] meta: %v", agentID, meta.RequestID, cols)
		case protocol.TypeQueryRows:
			var rows protocol.QueryRows
			_ = remarshal(msg.Payload, &rows)
			log.Printf("[%s][%s] %d rows (first: %v, last: %v)",
				agentID, rows.RequestID, len(rows.Rows), rows.Rows[0], rows.Rows[len(rows.Rows)-1])
		case protocol.TypeQueryDone:
			var done protocol.QueryDone
			_ = remarshal(msg.Payload, &done)
			log.Printf("[%s][%s] DONE %d rows in %dms",
				agentID, done.RequestID, done.RowCount, done.DurationMs)
		case protocol.TypeQueryError:
			var qe protocol.QueryError
			_ = remarshal(msg.Payload, &qe)
			log.Printf("[%s][%s] ERROR %s", agentID, qe.RequestID, qe.Error)
		default:
			log.Printf("[%s] unhandled type: %s", agentID, msg.Type)
		}
	}
}

type serverSession struct {
	conn    *websocket.Conn
	agentID string
	mu      sync.Mutex
}

func (s *serverSession) send(ctx context.Context, env protocol.Envelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return wsjson.Write(ctx, s.conn, env)
}

func remarshal(payload any, dest any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

func randID(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func shortTok(t string) string {
	if len(t) <= 8 {
		return t
	}
	return t[:6]
}
