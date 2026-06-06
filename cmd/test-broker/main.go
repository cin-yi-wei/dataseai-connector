// test-broker is a throwaway local broker for developing the connector.
// Not for production. Accepts any non-empty token.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
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
		w.Write([]byte("dataseai test-broker; agents connect to /agent\n"))
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
	b, _ := json.Marshal(first.Payload)
	_ = json.Unmarshal(b, &hello)
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

	if err := wsjson.Write(ctx, conn, protocol.Envelope{
		Type: protocol.TypeHelloAck,
		Payload: protocol.HelloAck{
			AgentID:          agentID,
			SessionID:        sessionID,
			HeartbeatSeconds: 5, // short for dev so we see pings fast
		},
	}); err != nil {
		log.Printf("send hello_ack: %v", err)
		return
	}

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
			pb, _ := json.Marshal(msg.Payload)
			_ = json.Unmarshal(pb, &ping)
			lag := time.Now().UnixMilli() - ping.Ts
			log.Printf("[%s] ping (one-way lag %dms)", agentID, lag)
			pong := protocol.Envelope{
				Type:    protocol.TypePong,
				Payload: protocol.Pong{Ts: time.Now().UnixMilli()},
			}
			if err := wsjson.Write(ctx, conn, pong); err != nil {
				log.Printf("[%s] write pong: %v", agentID, err)
				return
			}
		default:
			log.Printf("[%s] unhandled type: %s", agentID, msg.Type)
		}
	}
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
