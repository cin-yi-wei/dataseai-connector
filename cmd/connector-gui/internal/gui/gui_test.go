package gui

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestInstallArgsDoNotExposeToken(t *testing.T) {
	token := "ag_secret_token_from_gui"
	args := installArgsFromSourceConfig("/tmp/gui-config.yaml", "wss://dataseai.conray.top/agent", "mysql")
	joined := strings.Join(args, " ")
	if strings.Contains(joined, token) {
		t.Fatalf("install args leaked token: %s", joined)
	}
	if !strings.Contains(joined, "--source-config=/tmp/gui-config.yaml") {
		t.Fatalf("install args missing source config: %v", args)
	}
}

func TestConfigureArgsDoNotExposeToken(t *testing.T) {
	token := "ag_secret_token_from_gui"
	args := configureArgsFromSourceConfig("/tmp/gui-config.yaml", "wss://dataseai.conray.top/agent", "mysql")
	joined := strings.Join(args, " ")
	if strings.Contains(joined, token) {
		t.Fatalf("configure args leaked token: %s", joined)
	}
	if !strings.Contains(joined, "--source-config=/tmp/gui-config.yaml") {
		t.Fatalf("configure args missing source config: %v", args)
	}
}

func TestLinuxElevatedCommandUsesPkexec(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("pkexec wrapping is Linux-specific")
	}
	name, args := elevatedConnectorCommand("dataseai-connector", []string{"install"})
	if name != "pkexec" {
		t.Fatalf("command = %q, want pkexec", name)
	}
	if len(args) != 2 || args[0] != "dataseai-connector" || args[1] != "install" {
		t.Fatalf("args = %#v", args)
	}
}

func TestConnectorBinaryCandidatesIncludeMacAppArchiveSibling(t *testing.T) {
	exe := filepath.Join(
		"/tmp/dataseai-connector-gui_0.3.6_darwin_arm64",
		"DataseAI Connector.app",
		"Contents",
		"MacOS",
		"dataseai-connector-gui",
	)
	candidates := connectorBinaryCandidates(exe, "dataseai-connector")
	want := filepath.Join("/tmp/dataseai-connector-gui_0.3.6_darwin_arm64", "dataseai-connector")
	if !containsString(candidates, want) {
		t.Fatalf("candidates missing archive sibling %q: %#v", want, candidates)
	}
}

func TestConnectorBinaryCandidatesPreferExecutableDirectory(t *testing.T) {
	exe := filepath.Join(
		"/Applications",
		"DataseAI Connector.app",
		"Contents",
		"MacOS",
		"dataseai-connector-gui",
	)
	candidates := connectorBinaryCandidates(exe, "dataseai-connector")
	want := filepath.Join("/Applications", "DataseAI Connector.app", "Contents", "MacOS", "dataseai-connector")
	if len(candidates) == 0 || candidates[0] != want {
		t.Fatalf("first candidate = %q, want %q; all=%#v", candidates[0], want, candidates)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
