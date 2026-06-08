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
	name := "dataseai-connector"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe, err := os.Executable()
	if err == nil {
		for _, candidate := range connectorBinaryCandidates(exe, name) {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	return name
}

func connectorBinaryCandidates(exePath, name string) []string {
	exeDir := filepath.Dir(exePath)
	candidates := []string{filepath.Join(exeDir, name)}
	if filepath.Base(exeDir) == "MacOS" &&
		filepath.Base(filepath.Dir(exeDir)) == "Contents" &&
		strings.HasSuffix(filepath.Base(filepath.Dir(filepath.Dir(exeDir))), ".app") {
		candidates = append(candidates, filepath.Clean(filepath.Join(exeDir, "..", "..", "..", name)))
	}
	return candidates
}

func runConnector(args ...string) (string, error) {
	bin := connectorBinary()
	name, cmdArgs := connectorCommand(bin, args)
	return runCommand(name, cmdArgs)
}

func runElevatedConnector(args ...string) (string, error) {
	bin := connectorBinary()
	name, cmdArgs := elevatedConnectorCommand(bin, args)
	return runCommand(name, cmdArgs)
}

func connectorCommand(bin string, args []string) (string, []string) {
	return bin, args
}

func elevatedConnectorCommand(bin string, args []string) (string, []string) {
	if runtime.GOOS != "linux" {
		return connectorCommand(bin, args)
	}
	return "pkexec", append([]string{bin}, args...)
}

func runCommand(name string, args []string) (string, error) {
	cmd := exec.Command(name, args...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w\n%s", strings.Join(append([]string{name}, args...), " "), err, out)
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
	sourceConfig, err := writeTempConnectorConfig(token, server, executor)
	if err != nil {
		return err
	}
	defer os.Remove(sourceConfig)

	installArgs := installArgsFromSourceConfig(sourceConfig, server, executor)
	runner := serviceControlRunner()
	if _, installErr := runner(installArgs...); installErr != nil {
		// Service may be registered at an old path (e.g. a previous download folder).
		// Stop + uninstall the stale entry, then reinstall from the current location.
		_, _ = runner("stop")
		_, _ = runner("uninstall")
		if _, reinstallErr := runner(installArgs...); reinstallErr != nil {
			return fmt.Errorf("install: %v", reinstallErr)
		}
	}
	// connector install already starts the service.
	return nil
}

func SaveConfig(token, server, executor string) error {
	if token == "" {
		return fmt.Errorf("token is required")
	}
	sourceConfig, err := writeTempConnectorConfig(token, server, executor)
	if err != nil {
		return err
	}
	defer os.Remove(sourceConfig)

	_, err = serviceControlRunner()(configureArgsFromSourceConfig(sourceConfig, server, executor)...)
	return err
}

func writeTempConnectorConfig(token, server, executor string) (string, error) {
	cfg := Config{
		Token:    token,
		Server:   server,
		Executor: executor,
	}
	f, err := os.CreateTemp("", "dataseai-connector-gui-*.yaml")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := WriteConfig(path, cfg); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func installArgsFromSourceConfig(sourceConfig, server, executor string) []string {
	if server == "" {
		server = DefaultServer
	}
	if executor == "" {
		executor = DefaultExecutor
	}
	return []string{
		"install",
		"--source-config=" + sourceConfig,
		"--server=" + server,
		"--executor=" + executor,
	}
}

func configureArgsFromSourceConfig(sourceConfig, server, executor string) []string {
	if server == "" {
		server = DefaultServer
	}
	if executor == "" {
		executor = DefaultExecutor
	}
	return []string{
		"configure",
		"--source-config=" + sourceConfig,
		"--server=" + server,
		"--executor=" + executor,
	}
}

func serviceControlRunner() func(args ...string) (string, error) {
	if runtime.GOOS == "linux" {
		return runElevatedConnector
	}
	return runConnector
}

func Start() error {
	_, err := serviceControlRunner()("start")
	return wrapServiceError("start", err)
}

func Stop() error {
	_, err := serviceControlRunner()("stop")
	return wrapServiceError("stop", err)
}

func Restart() error {
	_, err := serviceControlRunner()("restart")
	return wrapServiceError("restart", err)
}

// wrapServiceError converts low-level service errors into user-friendly messages.
func wrapServiceError(action string, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "already running") ||
		strings.Contains(msg, "already started") {
		return nil // already running is success
	}
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
