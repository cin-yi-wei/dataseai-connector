package main

import (
	"testing"

	"github.com/cin-yi-wei/dataseai-connector/internal/control"
	"github.com/kardianos/service"
)

func TestMapServiceStatusNotInstalled(t *testing.T) {
	got := mapServiceStatus(service.StatusUnknown, service.ErrNotInstalled)
	if got != control.ServiceStatusNotInstalled {
		t.Fatalf("status = %q, want %q", got, control.ServiceStatusNotInstalled)
	}
}

func TestBuildInstallConfigUsesSourceConfigToken(t *testing.T) {
	sourcePath := t.TempDir() + "/gui-config.yaml"
	if err := control.WriteConfig(sourcePath, control.Config{
		Token:    "ag_from_gui_only",
		Server:   "wss://wrong.example/agent",
		Executor: "mock",
	}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	cfg, err := buildInstallConfig(installConfigInput{
		SourceConfigPath: sourcePath,
		Server:           "wss://dataseai.conray.top/agent",
		Executor:         "mysql",
	})
	if err != nil {
		t.Fatalf("buildInstallConfig: %v", err)
	}
	if cfg.Token != "ag_from_gui_only" {
		t.Fatalf("token = %q, want source config token", cfg.Token)
	}
	if cfg.Server != "wss://dataseai.conray.top/agent" {
		t.Fatalf("server = %q, want explicit server", cfg.Server)
	}
	if cfg.Executor != "mysql" {
		t.Fatalf("executor = %q, want explicit executor", cfg.Executor)
	}
}

func TestBuildInstallConfigRequiresToken(t *testing.T) {
	_, err := buildInstallConfig(installConfigInput{
		Server:   "wss://dataseai.conray.top/agent",
		Executor: "mysql",
	})
	if err == nil {
		t.Fatal("expected token error, got nil")
	}
}
