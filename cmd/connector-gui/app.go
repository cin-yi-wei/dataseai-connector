package main

import (
	"context"
	"fmt"

	"github.com/cin-yi-wei/dataseai-connector-gui/internal/gui"
)

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

type GUIStatus struct {
	ServiceStatus string   `json:"service_status"`
	AgentStatus   string   `json:"agent_status"`
	ConfigPath    string   `json:"config_path"`
	Server        string   `json:"server"`
	Executor      string   `json:"executor"`
	TokenMasked   string   `json:"token_masked"`
	LogLines      []string `json:"log_lines"`
}

func (a *App) GetStatus() (GUIStatus, error) {
	cfgPath := gui.DefaultConfigPath()
	cfg, err := gui.LoadConfig(cfgPath)
	if err != nil {
		return GUIStatus{ConfigPath: cfgPath}, fmt.Errorf("load config: %w", err)
	}
	logs, _ := gui.TailLines(gui.DefaultLogPath(), 100)
	return GUIStatus{
		ServiceStatus: gui.ServiceStatus(),
		AgentStatus:   "",
		ConfigPath:    cfgPath,
		Server:        cfg.Server,
		Executor:      cfg.Executor,
		TokenMasked:   gui.MaskToken(cfg.Token),
		LogLines:      logs,
	}, nil
}

func (a *App) SaveConfig(token, server, executor string) error {
	return gui.SaveConfig(token, server, executor)
}

func (a *App) InstallAndStart(token, server, executor string) error {
	return gui.InstallAndStart(token, server, executor)
}

func (a *App) Start() error {
	return gui.Start()
}

func (a *App) Stop() error {
	return gui.Stop()
}

func (a *App) Restart() error {
	return gui.Restart()
}

func (a *App) CopyDiagnostics() (string, error) {
	return gui.Diagnostics()
}
