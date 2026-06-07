package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConfigRequiresToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	err := WriteConfig(path, Config{Server: "wss://example.com"})
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
}

func TestMaskTokenRedactsFull(t *testing.T) {
	token := "ag_1234567890abcdef"
	masked := MaskToken(token)
	if masked == token {
		t.Fatalf("MaskToken did not redact: %q", masked)
	}
	if strings.Contains(masked, "1234567890abcdef") {
		t.Fatalf("MaskToken leaked token body: %q", masked)
	}
	if !strings.Contains(masked, "...") {
		t.Fatalf("MaskToken missing ellipsis: %q", masked)
	}
}

func TestLoadConfigMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if cfg.Server != DefaultServer {
		t.Fatalf("expected default server, got %q", cfg.Server)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	want := Config{
		Token:    "ag_testtoken123456",
		Server:   "wss://dataseai.conray.top/agent",
		Executor: "mysql",
	}
	if err := WriteConfig(path, want); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestWriteConfigOwnerOnlyPerms(t *testing.T) {
	if os.Getenv("GOOS") == "windows" || os.Getenv("OS") == "Windows_NT" {
		t.Skip("file mode bits not reliable on Windows")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := WriteConfig(path, Config{Token: "ag_secret123456"}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 0600", got)
	}
}

func TestTailLinesReturnsLast(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := TailLines(path, 2)
	if err != nil {
		t.Fatalf("TailLines: %v", err)
	}
	if len(got) != 2 || got[0] != "three" || got[1] != "four" {
		t.Fatalf("got %#v", got)
	}
}

func TestTailLinesMissingFileReturnsNil(t *testing.T) {
	got, err := TailLines(filepath.Join(t.TempDir(), "no.log"), 10)
	if err != nil {
		t.Fatalf("expected nil error for missing log, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil lines, got %v", got)
	}
}
