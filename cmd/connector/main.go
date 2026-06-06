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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	backoff := time.Second
	for ctx.Err() == nil {
		err := runOnce(ctx, *server, *token)
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

func runOnce(ctx context.Context, server, token string) error {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, server, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.CloseNow()
	log.Printf("connected to broker")

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
	if err := wsjson.Write(ctx, conn, hello); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}

	var ack protocol.Envelope
	if err := wsjson.Read(ctx, conn, &ack); err != nil {
		return fmt.Errorf("read hello_ack: %w", err)
	}
	switch ack.Type {
	case protocol.TypeHelloAck:
		var info protocol.HelloAck
		b, _ := json.Marshal(ack.Payload)
		_ = json.Unmarshal(b, &info)
		log.Printf("authenticated: agent_id=%s session=%s heartbeat=%ds",
			info.AgentID, info.SessionID, info.HeartbeatSeconds)
		hb := time.Duration(info.HeartbeatSeconds) * time.Second
		if hb <= 0 {
			hb = 30 * time.Second
		}
		return runSession(ctx, conn, hb)
	case protocol.TypeHelloFail:
		var fail protocol.HelloFail
		b, _ := json.Marshal(ack.Payload)
		_ = json.Unmarshal(b, &fail)
		return fmt.Errorf("broker rejected hello: %s", fail.Reason)
	default:
		return fmt.Errorf("unexpected first message: %s", ack.Type)
	}
}

func runSession(ctx context.Context, conn *websocket.Conn, hbInterval time.Duration) error {
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
				if err := wsjson.Write(hbCtx, conn, msg); err != nil {
					log.Printf("heartbeat write failed: %v", err)
					return
				}
			}
		}
	}()

	for {
		var msg protocol.Envelope
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			if errors.Is(err, context.Canceled) || isClose(err) {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}
		switch msg.Type {
		case protocol.TypePong:
			log.Printf("pong")
		case protocol.TypePing:
			pong := protocol.Envelope{
				Type:    protocol.TypePong,
				Payload: protocol.Pong{Ts: time.Now().UnixMilli()},
			}
			if err := wsjson.Write(ctx, conn, pong); err != nil {
				return fmt.Errorf("pong write: %w", err)
			}
		default:
			log.Printf("unhandled message type: %s", msg.Type)
		}
	}
}

func isClose(err error) bool {
	var ce websocket.CloseError
	if errors.As(err, &ce) {
		return true
	}
	var nerr net.Error
	return errors.As(err, &nerr) && nerr.Timeout()
}
