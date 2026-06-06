package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kardianos/service"
	"gopkg.in/yaml.v3"
)

// Config is what the connector needs to do its job. Loaded from disk on
// service start, can be overridden by CLI flags during development.
type Config struct {
	Token    string `yaml:"token"`
	Server   string `yaml:"server"`
	Executor string `yaml:"executor"`
}

func defaultConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "dataseai-connector", "config.yaml")
	case "darwin":
		return "/Library/Application Support/dataseai-connector/config.yaml"
	default:
		return "/etc/dataseai-connector/config.yaml"
	}
}

func loadConfig(path string) (Config, error) {
	var cfg Config
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

func writeConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	// 0600: contains the token, only owner should read.
	return os.WriteFile(path, b, 0o600)
}

// program implements service.Interface so kardianos can drive us as a
// system service (systemd / launchd / Windows Service) and also when
// invoked from a tty.
type program struct {
	cfg  Config
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
	case "mock":
		return MockExecutor{}, nil
	default:
		return nil, fmt.Errorf("unknown executor %q; valid: mysql | mock", name)
	}
}

func newServiceConfig() *service.Config {
	return &service.Config{
		Name:        "dataseai-connector",
		DisplayName: "dataseai Connector",
		Description: "LAN agent that bridges local MySQL to the dataseai cloud over WebSocket.",
	}
}
