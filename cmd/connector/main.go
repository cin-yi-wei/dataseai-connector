package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/cin-yi-wei/dataseai-connector/pkg/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	token := flag.String("token", "", "agent token (env: DATASEAI_TOKEN)")
	server := flag.String("server", "ws://localhost:8080/agent", "broker URL")
	execName := flag.String("executor", "mysql", "query executor: mysql | mock")
	flag.Parse()

	if *token == "" {
		*token = os.Getenv("DATASEAI_TOKEN")
	}
	if *token == "" {
		log.Fatal("error: --token required (or DATASEAI_TOKEN env)")
	}

	log.Printf("dataseai-connector %s (commit=%s, date=%s)", version, commit, date)
	log.Printf("system: %s/%s, go=%s", runtime.GOOS, runtime.GOARCH, runtime.Version())
	log.Printf("server: %s", *server)
	log.Printf("executor: %s", *execName)

	var executor Executor
	switch *execName {
	case "mock":
		executor = MockExecutor{}
	case "mysql":
		executor = MySQLExecutor{}
	default:
		log.Fatalf("unknown executor %q; valid: mysql | mock", *execName)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	backoff := time.Second
	for ctx.Err() == nil {
		err := runOnce(ctx, *server, *token, executor)
		if ctx.Err() != nil {
			break
		}
		log.Printf("session ended: %v", err)
		log.Printf("reconnecting in %s", backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
	log.Println("connector exiting")
}

func runOnce(ctx context.Context, server, token string, executor Executor) error {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, server, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.CloseNow()
	log.Printf("connected to broker")

	s := &session{conn: conn, executor: executor}

	hostname, _ := os.Hostname()
	hello := protocol.Envelope{
		Type: protocol.TypeHello,
		Payload: protocol.Hello{
			Token:        token,
			AgentVersion: version,
			OS:           runtime.GOOS,
			Arch:         runtime.GOARCH,
			Hostname:     hostname,
		},
	}
	if err := s.send(ctx, hello); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}

	var ack protocol.Envelope
	if err := wsjson.Read(ctx, conn, &ack); err != nil {
		return fmt.Errorf("read hello_ack: %w", err)
	}
	switch ack.Type {
	case protocol.TypeHelloAck:
		var info protocol.HelloAck
		_ = remarshal(ack.Payload, &info)
		log.Printf("authenticated: agent_id=%s session=%s heartbeat=%ds",
			info.AgentID, info.SessionID, info.HeartbeatSeconds)
		hb := time.Duration(info.HeartbeatSeconds) * time.Second
		if hb <= 0 {
			hb = 30 * time.Second
		}
		return s.run(ctx, hb)
	case protocol.TypeHelloFail:
		var fail protocol.HelloFail
		_ = remarshal(ack.Payload, &fail)
		return fmt.Errorf("broker rejected hello: %s", fail.Reason)
	default:
		return fmt.Errorf("unexpected first message: %s", ack.Type)
	}
}

// session bundles a single WebSocket conn with everything that may write to
// it. All writers go through send() so writes are serialized.
type session struct {
	conn     *websocket.Conn
	mu       sync.Mutex
	executor Executor
}

func (s *session) send(ctx context.Context, env protocol.Envelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return wsjson.Write(ctx, s.conn, env)
}

func (s *session) run(ctx context.Context, hbInterval time.Duration) error {
	hbCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		t := time.NewTicker(hbInterval)
		defer t.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-t.C:
				msg := protocol.Envelope{
					Type:    protocol.TypePing,
					Payload: protocol.Ping{Ts: time.Now().UnixMilli()},
				}
				if err := s.send(hbCtx, msg); err != nil {
					log.Printf("heartbeat write failed: %v", err)
					return
				}
			}
		}
	}()

	for {
		var msg protocol.Envelope
		if err := wsjson.Read(ctx, s.conn, &msg); err != nil {
			if errors.Is(err, context.Canceled) || isClose(err) {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}
		switch msg.Type {
		case protocol.TypePong:
			// quiet
		case protocol.TypePing:
			pong := protocol.Envelope{
				Type:    protocol.TypePong,
				Payload: protocol.Pong{Ts: time.Now().UnixMilli()},
			}
			if err := s.send(ctx, pong); err != nil {
				return fmt.Errorf("pong write: %w", err)
			}
		case protocol.TypeQueryRequest:
			var req protocol.QueryRequest
			if err := remarshal(msg.Payload, &req); err != nil {
				log.Printf("bad query_request: %v", err)
				continue
			}
			go s.handleQuery(ctx, req)
		default:
			log.Printf("unhandled message type: %s", msg.Type)
		}
	}
}

func (s *session) handleQuery(ctx context.Context, req protocol.QueryRequest) {
	log.Printf("[%s] query: %s (target=%s:%d/%s)",
		req.RequestID, ellipsis(req.SQL, 80),
		req.Target.Host, req.Target.Port, req.Target.Database)

	sink := &wsSink{s: s, ctx: ctx, requestID: req.RequestID}
	start := time.Now()
	err := s.executor.Run(ctx, req, sink)
	dur := time.Since(start)

	if err != nil {
		log.Printf("[%s] query error: %v", req.RequestID, err)
		_ = s.send(ctx, protocol.Envelope{
			Type: protocol.TypeQueryError,
			Payload: protocol.QueryError{
				RequestID: req.RequestID,
				Error:     err.Error(),
			},
		})
		return
	}

	log.Printf("[%s] query done: %d rows in %s", req.RequestID, sink.rowCount, dur)
	_ = s.send(ctx, protocol.Envelope{
		Type: protocol.TypeQueryDone,
		Payload: protocol.QueryDone{
			RequestID:  req.RequestID,
			RowCount:   sink.rowCount,
			DurationMs: dur.Milliseconds(),
		},
	})
}

// wsSink adapts an Executor's stream callbacks to WebSocket writes.
type wsSink struct {
	s         *session
	ctx       context.Context
	requestID string
	rowCount  int
}

func (w *wsSink) Meta(cols []protocol.ColInfo) error {
	return w.s.send(w.ctx, protocol.Envelope{
		Type: protocol.TypeQueryMeta,
		Payload: protocol.QueryMeta{
			RequestID: w.requestID,
			Columns:   cols,
		},
	})
}

func (w *wsSink) Rows(batch [][]any) error {
	w.rowCount += len(batch)
	return w.s.send(w.ctx, protocol.Envelope{
		Type: protocol.TypeQueryRows,
		Payload: protocol.QueryRows{
			RequestID: w.requestID,
			Rows:      batch,
		},
	})
}

func remarshal(payload any, dest any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

func ellipsis(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func isClose(err error) bool {
	var ce websocket.CloseError
	if errors.As(err, &ce) {
		return true
	}
	var nerr net.Error
	return errors.As(err, &nerr) && nerr.Timeout()
}
