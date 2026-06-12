package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/cin-yi-wei/dataseai-connector/internal/control"
	"github.com/kardianos/service"
)

// program implements service.Interface so kardianos can drive us as a
// system service (systemd / launchd / Windows Service) and also when
// invoked from a tty.
type program struct {
	cfg  control.Config
	exit chan struct{}
}

func (p *program) Start(s service.Service) error {
	// Must return quickly; do the work in a goroutine.
	p.exit = make(chan struct{})
	go p.runLoop()
	return nil
}

func (p *program) Stop(s service.Service) error {
	close(p.exit)
	return nil
}

func (p *program) runLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-p.exit
		cancel()
	}()

	executor, err := buildExecutor(p.cfg.Executor)
	if err != nil {
		log.Printf("fatal: %v", err)
		return
	}

	log.Printf("dataseai-connector %s (commit=%s, date=%s)", version, commit, date)
	log.Printf("system: %s/%s, go=%s", runtime.GOOS, runtime.GOARCH, runtime.Version())
	log.Printf("server: %s, executor: %s", p.cfg.Server, p.cfg.Executor)

	backoff := time.Second
	for ctx.Err() == nil {
		err := runOnce(ctx, p.cfg.Server, p.cfg.Token, executor)
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

func buildExecutor(name string) (Executor, error) {
	switch name {
	case "", "mysql":
		return MySQLExecutor{}, nil
	case "postgres", "postgresql":
		return PostgreSQLExecutor{}, nil
	case "bytehouse":
		return ByteHouseExecutor{}, nil
	case "sqlite":
		return SQLiteExecutor{}, nil
	case "mssql", "sqlserver":
		return SQLServerExecutor{}, nil
	case "auto":
		return dialectRouter{
			mysql:     MySQLExecutor{},
			postgres:  PostgreSQLExecutor{},
			bytehouse: ByteHouseExecutor{},
			sqlite:    SQLiteExecutor{},
			mssql:     SQLServerExecutor{},
		}, nil
	case "mock":
		return MockExecutor{}, nil
	default:
		return nil, fmt.Errorf("unknown executor %q; valid: mysql | postgres | bytehouse | sqlite | mssql | auto | mock", name)
	}
}

func currentServiceStatus() control.ServiceStatus {
	if isDarwin {
		return darwinCurrentServiceStatus()
	}
	svc, err := service.New(&program{}, newServiceConfig())
	if err != nil {
		return control.ServiceStatusUnknown
	}
	st, err := svc.Status()
	return mapServiceStatus(st, err)
}

func mapServiceStatus(st service.Status, err error) control.ServiceStatus {
	if errors.Is(err, service.ErrNotInstalled) {
		return control.ServiceStatusNotInstalled
	}
	if err != nil {
		return control.ServiceStatusUnknown
	}
	switch st {
	case service.StatusRunning:
		return control.ServiceStatusRunning
	case service.StatusStopped:
		return control.ServiceStatusStopped
	default:
		return control.ServiceStatusUnknown
	}
}

func newServiceConfig() *service.Config {
	cfg := &service.Config{
		Name:        "dataseai-connector",
		DisplayName: "dataseai Connector",
		Description: "LAN agent that bridges local MySQL to the dataseai cloud over WebSocket.",
	}
	if runtime.GOOS == "darwin" {
		// LaunchAgent (user-level): no root needed for install/start/stop/status.
		cfg.Option = service.KeyValue{"UserService": true}
	}
	return cfg
}

func setupServiceLogOutput(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	log.SetOutput(f)
	return nil
}
