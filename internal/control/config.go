package control

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// Config is what the connector needs to do its job.
type Config struct {
	Token    string `yaml:"token"`
	Server   string `yaml:"server"`
	Executor string `yaml:"executor"`
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
	return os.WriteFile(path, b, 0o644)
}

func MaskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 3 {
		return "***"
	}
	if len(token) <= 10 {
		return token[:3] + "..."
	}
	return token[:6] + "..." + token[len(token)-4:]
}
