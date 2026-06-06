package main

import (
	"strings"
	"testing"
	"time"

	"github.com/cin-yi-wei/dataseai-connector/pkg/protocol"
)

func TestBuildMySQLConfig_UsesSSHDialer(t *testing.T) {
	target := protocol.MySQLTarget{
		Host:     "10.0.2.15",
		Port:     3306,
		User:     "app",
		Password: "dbpw",
		Database: "appdb",
		SSH: &protocol.SSHConfig{
			Host: "bastion.example.com",
			Port: 22,
			User: "ubuntu",
		},
	}

	cfg := buildMySQLConfig(target, time.Second, "ssh-test")

	if cfg.Net != "ssh-test" {
		t.Fatalf("Net = %q, want ssh-test", cfg.Net)
	}
	if cfg.Addr != "10.0.2.15:3306" {
		t.Fatalf("Addr = %q", cfg.Addr)
	}
	if dsn := cfg.FormatDSN(); !strings.Contains(dsn, "@ssh-test(10.0.2.15:3306)/appdb") {
		t.Fatalf("dsn = %s", dsn)
	}
}
