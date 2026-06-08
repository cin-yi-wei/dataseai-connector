package control

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := Config{
		Token:    "ag_secret_token",
		Server:   "wss://dataseai.conray.top/agent",
		Executor: "mysql",
	}

	if err := WriteConfig(path, cfg); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got != cfg {
		t.Fatalf("got %#v, want %#v", got, cfg)
	}
}

func TestMaskToken(t *testing.T) {
	token := "ag_1234567890abcdef"
	got := MaskToken(token)
	if got == token {
		t.Fatal("MaskToken leaked full token")
	}
	if !strings.HasPrefix(got, "ag_") {
		t.Fatalf("masked token should keep prefix, got %q", got)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("masked token should contain ellipsis, got %q", got)
	}
}

func TestWriteConfigPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not reliable on Windows")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := WriteConfig(path, Config{Token: "ag_secret"}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("mode = %o, want 0644", got)
	}
}
