package control

import (
	"runtime"
	"strings"
)

type ServiceStatus string

const (
	ServiceStatusRunning      ServiceStatus = "running"
	ServiceStatusStopped      ServiceStatus = "stopped"
	ServiceStatusNotInstalled ServiceStatus = "not_installed"
	ServiceStatusUnknown      ServiceStatus = "unknown"
)

type StatusReport struct {
	ServiceStatus ServiceStatus `json:"service_status"`
}

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
	LogPath       string        `json:"log_path"`
	PublicConfig  PublicConfig  `json:"config"`
	ServiceStatus ServiceStatus `json:"service_status"`
	AgentStatus   string        `json:"agent_status,omitempty"`
	LogLines      []string      `json:"log_lines,omitempty"`
	LogError      string        `json:"log_error,omitempty"`
}

type DiagnosticsInput struct {
	Version       string
	Commit        string
	Date          string
	ConfigPath    string
	LogPath       string
	Config        Config
	ServiceStatus ServiceStatus
	AgentStatus   string
	LogLines      []string
	LogError      string
}

func NewDiagnostics(input DiagnosticsInput) Diagnostics {
	tokenMasked := MaskToken(input.Config.Token)
	return Diagnostics{
		Version:    input.Version,
		Commit:     input.Commit,
		Date:       input.Date,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		ConfigPath: input.ConfigPath,
		LogPath:    input.LogPath,
		PublicConfig: PublicConfig{
			TokenMasked: tokenMasked,
			Server:      input.Config.Server,
			Executor:    input.Config.Executor,
		},
		ServiceStatus: input.ServiceStatus,
		AgentStatus:   input.AgentStatus,
		LogLines:      redactToken(input.LogLines, input.Config.Token, tokenMasked),
		LogError:      input.LogError,
	}
}

func redactToken(lines []string, token, replacement string) []string {
	if len(lines) == 0 || token == "" {
		return lines
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = strings.ReplaceAll(line, token, replacement)
	}
	return out
}
