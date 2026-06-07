# DataseAI Connector GUI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a Windows-first desktop GUI for `dataseai-connector` that installs/controls the connector service, writes config, shows status/logs, and copies diagnostics.

**Architecture:** Keep the existing CLI/service connector as the core. Extract config/service/status helpers into reusable internal packages, add machine-readable CLI commands, then build a Wails React GUI that calls those helpers. The GUI is Windows-first but the package boundaries should keep macOS/Linux possible later.

**Tech Stack:** Go 1.25, kardianos/service, YAML config, Wails v2, React/Vite/TypeScript, Vitest-style frontend tests where practical, Go unit tests for backend helpers.

---

## Context

Current repo: `/home/conray/project/dataseai-connector`

Current core:

- `cmd/connector/main.go` contains CLI parsing, service commands, foreground run, WebSocket loop.
- `cmd/connector/service.go` contains `Config`, config path/read/write, service program loop, executor selection, service metadata.
- `cmd/connector/mysql.go` contains MySQL and SSH tunnel executor.
- `pkg/protocol/types.go` contains broker protocol types.
- Existing Makefile has `build`, `ci`, `snapshot`, `release`.

Design doc:

- `docs/plans/2026-06-07-connector-gui-design.md`

Implementation rules:

- Use TDD for behavior changes.
- Keep commits small.
- Do not break existing CLI commands.
- Do not leak full agent tokens in diagnostics or logs.
- First release can be unsigned Windows preview.

---

### Task 1: Extract Config Helpers

**Files:**

- Create: `internal/control/config.go`
- Create: `internal/control/config_test.go`
- Modify: `cmd/connector/service.go`
- Modify: `cmd/connector/main.go`

**Step 1: Write failing config tests**

Create `internal/control/config_test.go`:

```go
package control

import (
	"os"
	"path/filepath"
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
	got := MaskToken("ag_1234567890abcdef")
	if got == "ag_1234567890abcdef" {
		t.Fatal("MaskToken leaked full token")
	}
	if !strings.HasPrefix(got, "ag_") {
		t.Fatalf("masked token should keep prefix, got %q", got)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("masked token should contain ellipsis, got %q", got)
	}
}

func TestWriteConfigUsesOwnerOnlyPermissions(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
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
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 0600", got)
	}
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/control -run 'TestConfig|TestMaskToken|TestWriteConfig' -count=1
```

Expected: FAIL because `internal/control` does not exist.

**Step 3: Implement config helper**

Create `internal/control/config.go`:

```go
package control

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Token    string `yaml:"token" json:"-"`
	Server   string `yaml:"server" json:"server"`
	Executor string `yaml:"executor" json:"executor"`
}

func DefaultConfigPath() string {
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

func LoadConfig(path string) (Config, error) {
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

func WriteConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func MaskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 10 {
		return token[:min(len(token), 3)] + "..."
	}
	return token[:6] + "..." + token[len(token)-4:]
}
```

If Go 1.25 does not allow built-in `min`, replace with a tiny helper.

Update `cmd/connector/service.go` to remove duplicated `Config`, `defaultConfigPath`, `loadConfig`, and `writeConfig`. Import `github.com/cin-yi-wei/dataseai-connector/internal/control` and use `control.Config`.

Update `cmd/connector/main.go` to use `control.DefaultConfigPath`, `control.LoadConfig`, and `control.WriteConfig`.

**Step 4: Run tests**

Run:

```bash
go test ./internal/control ./cmd/connector -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/control/config.go internal/control/config_test.go cmd/connector/main.go cmd/connector/service.go
git commit -m "refactor: extract connector config helpers"
```

---

### Task 2: Add Machine-Readable Status And Diagnostics

**Files:**

- Create: `internal/control/status.go`
- Create: `internal/control/status_test.go`
- Modify: `cmd/connector/main.go`
- Modify: `cmd/connector/service.go`

**Step 1: Write failing status tests**

Create `internal/control/status_test.go`:

```go
package control

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticsRedactsToken(t *testing.T) {
	d := Diagnostics{
		Version:       "v0.2.0",
		Commit:        "abc123",
		Date:          "2026-06-07",
		OS:            "windows",
		Arch:          "amd64",
		ConfigPath:    `C:\ProgramData\dataseai-connector\config.yaml`,
		Config:        Config{Token: "ag_1234567890abcdef", Server: "wss://dataseai.conray.top/agent", Executor: "mysql"},
		ServiceStatus: "running",
		LogLines:      []string{"connected to broker"},
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out := string(b)
	if strings.Contains(out, "ag_1234567890abcdef") {
		t.Fatalf("diagnostics leaked token: %s", out)
	}
	if !strings.Contains(out, "token_masked") {
		t.Fatalf("diagnostics missing masked token: %s", out)
	}
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/control -run TestDiagnosticsRedactsToken -count=1
```

Expected: FAIL because `Diagnostics` is undefined.

**Step 3: Implement status and diagnostics types**

Create `internal/control/status.go`:

```go
package control

import "runtime"

type ServiceStatus string

const (
	ServiceStatusRunning      ServiceStatus = "running"
	ServiceStatusStopped      ServiceStatus = "stopped"
	ServiceStatusNotInstalled ServiceStatus = "not_installed"
	ServiceStatusUnknown      ServiceStatus = "unknown"
)

type PublicConfig struct {
	TokenMasked string `json:"token_masked"`
	Server      string `json:"server"`
	Executor    string `json:"executor"`
}

type Diagnostics struct {
	Version       string        `json:"version"`
	Commit        string        `json:"commit"`
	Date          string        `json:"date"`
	OS            string        `json:"os"`
	Arch          string        `json:"arch"`
	ConfigPath    string        `json:"config_path"`
	Config        Config        `json:"-"`
	PublicConfig  PublicConfig  `json:"config"`
	ServiceStatus ServiceStatus `json:"service_status"`
	AgentStatus   string        `json:"agent_status,omitempty"`
	LogLines      []string      `json:"log_lines,omitempty"`
}

func NewDiagnostics(version, commit, date, configPath string, cfg Config, svc ServiceStatus, logs []string) Diagnostics {
	return Diagnostics{
		Version:    version,
		Commit:     commit,
		Date:       date,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		ConfigPath: configPath,
		Config:     cfg,
		PublicConfig: PublicConfig{
			TokenMasked: MaskToken(cfg.Token),
			Server:      cfg.Server,
			Executor:    cfg.Executor,
		},
		ServiceStatus: svc,
		LogLines:      logs,
	}
}
```

Update test to call `NewDiagnostics(...)` instead of constructing the struct with `Config` directly if needed.

**Step 4: Add CLI JSON commands**

Modify `cmd/connector/main.go`:

- Extend usage with:
  - `status --json`
  - `diagnostics --json`
- Add `jsonOut := fs.Bool("json", false, "print JSON output")`.
- Change `runStatus()` to accept `jsonOut bool`.
- Add `runDiagnostics(cfgPath string)`.

Expected behavior:

```bash
dataseai-connector status
# running

dataseai-connector status --json
# {"service_status":"running"}

dataseai-connector diagnostics --json
# redacted JSON diagnostic payload
```

Use `encoding/json` with indentation for human copy/paste.

**Step 5: Run tests**

Run:

```bash
go test ./... -count=1
go build ./cmd/connector
```

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/control/status.go internal/control/status_test.go cmd/connector/main.go cmd/connector/service.go
git commit -m "feat: add connector diagnostics output"
```

---

### Task 3: Add Stable Log Location

**Files:**

- Create: `internal/control/logs.go`
- Create: `internal/control/logs_test.go`
- Modify: `cmd/connector/service.go`
- Modify: `cmd/connector/main.go`

**Step 1: Write failing log helper tests**

Create `internal/control/logs_test.go`:

```go
package control

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTailLinesReturnsLastLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connector.log")
	content := "one\ntwo\nthree\nfour\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
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
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/control -run TestTailLinesReturnsLastLines -count=1
```

Expected: FAIL because `TailLines` is undefined.

**Step 3: Implement log helpers**

Create `internal/control/logs.go`:

```go
package control

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
)

func DefaultLogPath() string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "dataseai-connector", "connector.log")
	case "darwin":
		return "/Library/Logs/dataseai-connector/connector.log"
	default:
		return "/var/log/dataseai-connector.log"
	}
}

func TailLines(path string, n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return lines, scanner.Err()
}
```

**Step 4: Wire logging**

Modify `cmd/connector/service.go` so service mode writes logs to the stable log path where possible.

Keep foreground `run` useful for developers. If foreground logging changes are risky, only add `dataseai-connector logs` command first and leave service logging as a follow-up.

Modify `cmd/connector/main.go`:

- Add `logs` subcommand.
- Add `--tail=100` option.
- `logs` prints last N lines from `control.DefaultLogPath()`.

**Step 5: Run tests**

Run:

```bash
go test ./... -count=1
go build ./cmd/connector
```

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/control/logs.go internal/control/logs_test.go cmd/connector/main.go cmd/connector/service.go
git commit -m "feat: expose connector logs"
```

---

### Task 4: Scaffold Wails GUI

**Files:**

- Create: `cmd/connector-gui/`
- Modify: `Makefile`
- Modify: `.gitignore` if generated build artifacts need ignoring.

**Step 1: Check Wails availability**

Run:

```bash
wails version
```

If missing, install Wails using the official current command from Wails docs. Do not vendor Wails into this repo manually.

**Step 2: Scaffold in a temporary directory**

Run:

```bash
tmp="$(mktemp -d)"
cd "$tmp"
wails init -n connector-gui -t react-ts
```

Copy the generated app into:

```text
/home/conray/project/dataseai-connector/cmd/connector-gui/
```

Avoid overwriting repo root `go.mod`. The GUI should be a separate Go module only if Wails requires it. Prefer a single repo module if Wails supports building from a subdirectory cleanly.

**Step 3: Build immediately**

Run:

```bash
cd /home/conray/project/dataseai-connector/cmd/connector-gui
wails build
```

Expected: PASS with generated default app.

**Step 4: Add Makefile target**

Modify root `Makefile`:

```make
gui-build:
	cd cmd/connector-gui && wails build
```

Add `gui-build` to `.PHONY`.

Do not add it to `make ci` yet unless CI has Wails dependencies.

**Step 5: Commit**

```bash
git add cmd/connector-gui Makefile .gitignore
git commit -m "feat: scaffold connector GUI"
```

---

### Task 5: Add GUI Backend API

**Files:**

- Modify: `cmd/connector-gui/app.go` or Wails-generated backend file.
- Create/modify: `cmd/connector-gui/frontend/src/App.tsx`
- Create/modify: `cmd/connector-gui/frontend/src/App.css`

**Step 1: Define backend methods**

Expose these Wails methods:

```go
type App struct {
	ctx context.Context
}

type GUIStatus struct {
	ServiceStatus string `json:"service_status"`
	AgentStatus   string `json:"agent_status"`
	ConfigPath    string `json:"config_path"`
	Server        string `json:"server"`
	Executor      string `json:"executor"`
	TokenMasked   string `json:"token_masked"`
	LogLines      []string `json:"log_lines"`
}

func (a *App) GetStatus() (GUIStatus, error)
func (a *App) SaveConfig(token string, server string, executor string) error
func (a *App) InstallAndStart(token string, server string, executor string) error
func (a *App) Start() error
func (a *App) Stop() error
func (a *App) Restart() error
func (a *App) CopyDiagnostics() (string, error)
```

**Step 2: Write backend tests where possible**

If generated Wails app makes direct backend tests awkward, put service/config
logic in a plain package under:

```text
cmd/connector-gui/internal/gui/
```

Test that:

- Empty token returns validation error.
- Diagnostics redact token.
- Status handles missing config file.

**Step 3: Implement using `internal/control`**

Use `control.LoadConfig`, `control.WriteConfig`, `control.MaskToken`,
`control.TailLines`, and diagnostics helpers.

For service start/stop/restart, first version may shell out to the sibling
`dataseai-connector.exe` if direct kardianos service control from the GUI is
too slow to cleanly wire. Prefer shared Go helpers if straightforward.

**Step 4: Run**

Run:

```bash
go test ./... -count=1
cd cmd/connector-gui && wails build
```

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/connector-gui internal/control
git commit -m "feat: add connector GUI backend"
```

---

### Task 6: Build The GUI Screen

**Files:**

- Modify: `cmd/connector-gui/frontend/src/App.tsx`
- Modify: `cmd/connector-gui/frontend/src/App.css`
- Modify generated Wails frontend bindings if needed.

**Step 1: Replace generated demo UI**

Build one operational screen:

- Header: `DataseAI Connector`
- Service status badge.
- Agent status badge.
- Token input.
- Server URL input in an advanced/details section.
- Buttons:
  - `Install & Start`
  - `Restart`
  - `Stop`
  - `Copy Diagnostics`
- Logs panel.
- Error banner.

**Step 2: UX behavior**

- On load, call `GetStatus()`.
- Poll status every 5 seconds while the window is open.
- Mask token after save.
- Disable action buttons while a command is running.
- Show raw error details in a collapsible area.

**Step 3: Build**

Run:

```bash
cd cmd/connector-gui && wails build
```

Expected: PASS.

**Step 4: Manual smoke test**

Run:

```bash
cd cmd/connector-gui && wails dev
```

Verify:

- Window loads.
- Empty token validation works.
- Server URL defaults correctly.
- Logs panel handles missing log file.
- Copy diagnostics does not include full token.

**Step 5: Commit**

```bash
git add cmd/connector-gui/frontend
git commit -m "feat: build connector GUI screen"
```

---

### Task 7: Package Windows Preview

**Files:**

- Modify: `Makefile`
- Create: `docs/windows-gui-preview.md`
- Modify: `.github/workflows/release.yml` if present and safe.

**Step 1: Add local packaging target**

Modify `Makefile`:

```make
gui-package-windows:
	GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/dataseai-connector.exe ./cmd/connector
	cd cmd/connector-gui && wails build -platform windows/amd64
```

Adjust for Wails' actual platform syntax if needed.

**Step 2: Write preview docs**

Create `docs/windows-gui-preview.md`:

```markdown
# Windows GUI Preview

This preview is unsigned. Windows SmartScreen may warn on first run.

## Install

1. Download `dataseai-connector-gui.exe`.
2. Run as Administrator.
3. Paste the agent token from DataseAI.
4. Click `Install & Start`.

## Troubleshooting

- If install fails, run as Administrator.
- If service is running but agent is offline, check server URL and network.
- Use `Copy Diagnostics` before reporting bugs.
```

**Step 3: Build package locally**

Run:

```bash
make build
make gui-build
```

If on non-Windows, document that final Windows service smoke test must happen
on Windows.

**Step 4: Commit**

```bash
git add Makefile docs/windows-gui-preview.md .github/workflows
git commit -m "build: add Windows GUI preview packaging"
```

---

### Task 8: End-To-End Verification

**Files:**

- No code changes unless a bug is found.

**Step 1: Run full connector checks**

Run:

```bash
make ci
```

Expected:

- `go vet ./...` passes.
- `go test ./... -count=1` passes.
- `go build ./...` passes.
- `goreleaser check` passes.

**Step 2: Run GUI build**

Run:

```bash
make gui-build
```

Expected: PASS.

**Step 3: Windows manual test**

On Windows:

1. Launch GUI as Administrator.
2. Paste a fresh DataseAI agent token.
3. Click `Install & Start`.
4. Confirm Windows service is running.
5. Confirm DataseAI Local Agents page shows the agent online.
6. Click `Restart`; confirm agent comes back online.
7. Click `Copy Diagnostics`; confirm token is redacted.

**Step 4: Final commit if fixes were needed**

If no changes were needed, do not create an empty commit.

If fixes were needed:

```bash
git add <changed files>
git commit -m "fix: stabilize connector GUI preview"
```

---

## Open Questions For Implementation

- Whether Wails can live cleanly under `cmd/connector-gui` inside the existing Go module, or needs its own nested module.
- Whether GUI should control service via shared Go helpers or shell out to `dataseai-connector.exe` for first release.
- Where Windows service logs should reliably live when installed through kardianos/service.
- Whether DataseAI needs a small agent status API optimized for this GUI, or if current Local Agents API is enough.

## Definition Of Done

- Existing CLI still supports `install`, `run`, `start`, `stop`, `restart`, `status`, and `version`.
- GUI can write config and start/stop/restart connector service on Windows.
- GUI never displays or copies the full token after save.
- GUI can copy useful diagnostics.
- Full Go tests pass.
- GUI build passes.
- Windows manual smoke test passes with a real DataseAI agent token.
