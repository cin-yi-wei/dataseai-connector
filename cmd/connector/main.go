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
	"runtime"
	"sync"
	"time"

	"github.com/cin-yi-wei/dataseai-connector/pkg/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/kardianos/service"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `dataseai-connector — LAN agent for the dataseai cloud.

USAGE
  dataseai-connector [SUBCOMMAND] [FLAGS]

SUBCOMMANDS
  install     write config file and register as a system service
  uninstall   stop, unregister, leave config file in place
  start       start the installed service
  stop        stop the installed service
  restart     stop + start
  status      print service status
  run         run in the foreground (for development)
  version     print version and exit

If no subcommand is given, the binary runs in service mode (which is what
systemd / launchd / SCM invoke on boot).

FLAGS (for install / run)
  --token=ag_xxxx     agent token  (env: DATASEAI_TOKEN)
  --server=wss://…    broker URL   (default wss://dataseai.app/agent)
  --executor=mysql    mysql | mock (default mysql)
  --config=/path      override config path (default platform-specific)

CONFIG FILE
  Linux:   /etc/dataseai-connector/config.yaml
  macOS:   /Library/Application Support/dataseai-connector/config.yaml
  Windows: %ProgramData%\dataseai-connector\config.yaml
`

func main() {
	fs := flag.NewFlagSet("dataseai-connector", flag.ExitOnError)
	token := fs.String("token", "", "agent token")
	server := fs.String("server", "wss://dataseai.app/agent", "broker URL")
	execName := fs.String("executor", "mysql", "query executor")
	cfgPath := fs.String("config", defaultConfigPath(), "config file path")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	// First positional arg (if any) is the subcommand.
	var subcmd string
	if len(os.Args) >= 2 && len(os.Args[1]) > 0 && os.Args[1][0] != '-' {
		subcmd = os.Args[1]
		_ = fs.Parse(os.Args[2:])
	} else {
		_ = fs.Parse(os.Args[1:])
	}

	switch subcmd {
	case "version":
		fmt.Printf("dataseai-connector %s (commit=%s, date=%s)\n", version, commit, date)
		return
	case "install":
		runInstall(*cfgPath, *token, *server, *execName)
		return
	case "uninstall", "start", "stop", "restart":
		runControl(subcmd)
		return
	case "status":
		runStatus()
		return
	case "run":
		runForeground(*cfgPath, *token, *server, *execName)
		return
	case "":
		// service mode (default when invoked by systemd / launchd / SCM)
		runService(*cfgPath)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", subcmd)
		fs.Usage()
		os.Exit(2)
	}
}

func runInstall(cfgPath, token, server, execName string) {
	if token == "" {
		token = os.Getenv("DATASEAI_TOKEN")
	}
	if token == "" {
		log.Fatal("error: --token required for install (or DATASEAI_TOKEN env)")
	}
	if execName == "" {
		execName = "mysql"
	}
	cfg := Config{Token: token, Server: server, Executor: execName}
	if err := writeConfig(cfgPath, cfg); err != nil {
		log.Fatalf("write config %s: %v", cfgPath, err)
	}
	log.Printf("wrote config: %s", cfgPath)

	svc, err := service.New(&program{cfg: cfg}, newServiceConfig())
	if err != nil {
		log.Fatalf("service.New: %v", err)
	}
	if err := svc.Install(); err != nil {
		log.Fatalf("install service: %v (try running with sudo)", err)
	}
	log.Printf("installed service: dataseai-connector")
	if err := svc.Start(); err != nil {
		log.Printf("start service: %v (you can `dataseai-connector start` later)", err)
		return
	}
	log.Printf("started service")
}

func runControl(action string) {
	svc, err := service.New(&program{}, newServiceConfig())
	if err != nil {
		log.Fatalf("service.New: %v", err)
	}
	if err := service.Control(svc, action); err != nil {
		log.Fatalf("%s: %v (try with sudo)", action, err)
	}
	log.Printf("%s ok", action)
}

func runStatus() {
	svc, err := service.New(&program{}, newServiceConfig())
	if err != nil {
		log.Fatalf("service.New: %v", err)
	}
	st, err := svc.Status()
	if err != nil {
		log.Fatalf("status: %v", err)
	}
	switch st {
	case service.StatusRunning:
		fmt.Println("running")
	case service.StatusStopped:
		fmt.Println("stopped")
	default:
		fmt.Println("unknown")
	}
}

func runForeground(cfgPath, token, server, execName string) {
	cfg, err := loadConfig(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		log.Fatalf("load config: %v", err)
	}
	if token == "" {
		token = os.Getenv("DATASEAI_TOKEN")
	}
	if token != "" {
		cfg.Token = token
	}
	if server != "" && server != "wss://dataseai.app/agent" {
		cfg.Server = server
	}
	if cfg.Server == "" {
		cfg.Server = server
	}
	if execName != "" && execName != "mysql" {
		cfg.Executor = execName
	}
	if cfg.Executor == "" {
		cfg.Executor = execName
	}
	if cfg.Token == "" {
		log.Fatal("error: token required (--token, DATASEAI_TOKEN env, or in config file)")
	}

	svc, err := service.New(&program{cfg: cfg}, newServiceConfig())
	if err != nil {
		log.Fatalf("service.New: %v", err)
	}
	// service.Run handles both service and foreground modes; the framework
	// detects which it's in and dispatches accordingly.
	if err := svc.Run(); err != nil {
		log.Fatal(err)
	}
}

func runService(cfgPath string) {
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("load config %s: %v", cfgPath, err)
	}
	if cfg.Token == "" {
		log.Fatalf("config %s has no token", cfgPath)
	}
	if cfg.Server == "" {
		cfg.Server = "wss://dataseai.app/agent"
	}
	if cfg.Executor == "" {
		cfg.Executor = "mysql"
	}
	svc, err := service.New(&program{cfg: cfg}, newServiceConfig())
	if err != nil {
		log.Fatalf("service.New: %v", err)
	}
	if err := svc.Run(); err != nil {
		log.Fatal(err)
	}
}

// --- everything below is the original WebSocket / query plumbing ---

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
