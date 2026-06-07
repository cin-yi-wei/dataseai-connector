package gui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// connectorBinary returns the path to the dataseai-connector binary.
// Looks next to the GUI executable first, then falls back to PATH.
func connectorBinary() string {
	exe, err := os.Executable()
	if err == nil {
		name := "dataseai-connector"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		candidate := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if runtime.GOOS == "windows" {
		return "dataseai-connector.exe"
	}
	return "dataseai-connector"
}

func runConnector(args ...string) (string, error) {
	bin := connectorBinary()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w\n%s", strings.Join(append([]string{bin}, args...), " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func ServiceStatus() string {
	type statusJSON struct {
		ServiceStatus string `json:"service_status"`
	}
	out, err := runConnector("status", "--json")
	if err != nil {
		return "unknown"
	}
	var s statusJSON
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		return "unknown"
	}
	return s.ServiceStatus
}

func InstallAndStart(token, server, executor string) error {
	if token == "" {
		return fmt.Errorf("token is required")
	}
	cfgPath := DefaultConfigPath()
	cfg := Config{
		Token:    token,
		Server:   server,
		Executor: executor,
	}
	if err := WriteConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if _, err := runConnector("install"); err != nil {
		_, _ = runConnector("start")
		return nil
	}
	_, err := runConnector("start")
	return err
}

func Start() error {
	_, err := runConnector("start")
	return err
}

func Stop() error {
	_, err := runConnector("stop")
	return err
}

func Restart() error {
	_, err := runConnector("restart")
	return err
}

func Diagnostics() (string, error) {
	out, err := runConnector("diagnostics", "--json")
	if err != nil {
		return "", err
	}
	return out, nil
}
