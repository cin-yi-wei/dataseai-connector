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
	return wrapServiceError("start", err)
}

func Stop() error {
	_, err := runConnector("stop")
	return wrapServiceError("stop", err)
}

func Restart() error {
	_, err := runConnector("restart")
	return wrapServiceError("restart", err)
}

// wrapServiceError converts low-level service errors into user-friendly messages.
func wrapServiceError(action string, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "does not exist as an installed service") ||
		strings.Contains(msg, "not installed") {
		return fmt.Errorf("service is not installed — use \"Install & Start\" first")
	}
	if strings.Contains(msg, "Access is denied") || strings.Contains(msg, "permission denied") {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("%s failed: %w (run the GUI as Administrator)", action, err)
		}
		return fmt.Errorf("%s failed: %w (try running with sudo)", action, err)
	}
	return err
}

func Diagnostics() (string, error) {
	out, err := runConnector("diagnostics", "--json")
	if err != nil {
		return "", err
	}
	return out, nil
}
