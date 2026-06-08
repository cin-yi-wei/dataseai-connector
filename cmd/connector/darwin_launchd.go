package main

// macOS-specific launchd management using modern launchctl APIs.
// kardianos/service uses the deprecated `launchctl load/unload` which fails
// with "Load failed: 5: Input/output error" on macOS Ventura+.
// We bypass it entirely on darwin and use bootstrap/bootout/kickstart instead.

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cin-yi-wei/dataseai-connector/internal/control"
)

const darwinServiceLabel = "dataseai-connector"

func darwinPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", darwinServiceLabel+".plist")
}

func darwinDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func darwinWritePlist() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	logPath := control.DefaultLogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	plistPath := darwinPlistPath()
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>KeepAlive</key>
	<true/>
	<key>RunAtLoad</key>
	<false/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, darwinServiceLabel, exe, logPath, logPath)
	return os.WriteFile(plistPath, []byte(content), 0o644)
}

func darwinBootout() {
	exec.Command("launchctl", "bootout", darwinDomain()+"/"+darwinServiceLabel).Run()
}

func darwinInstall() {
	darwinBootout()
	if err := darwinWritePlist(); err != nil {
		log.Fatalf("write plist: %v", err)
	}
	out, err := exec.Command("launchctl", "bootstrap", darwinDomain(), darwinPlistPath()).CombinedOutput()
	if err != nil {
		log.Fatalf("launchctl bootstrap: %v\n%s", err, out)
	}
	out, err = exec.Command("launchctl", "kickstart", darwinDomain()+"/"+darwinServiceLabel).CombinedOutput()
	if err != nil {
		log.Printf("launchctl kickstart: %v\n%s (service will start on next login)", err, out)
		return
	}
	log.Printf("installed and started service")
}

func darwinControl(action string) {
	switch action {
	case "start":
		out, err := exec.Command("launchctl", "kickstart", darwinDomain()+"/"+darwinServiceLabel).CombinedOutput()
		if err != nil {
			if _, statErr := os.Stat(darwinPlistPath()); statErr == nil {
				exec.Command("launchctl", "bootstrap", darwinDomain(), darwinPlistPath()).Run()
				out, err = exec.Command("launchctl", "kickstart", darwinDomain()+"/"+darwinServiceLabel).CombinedOutput()
			}
			if err != nil {
				log.Fatalf("start: Failed to start dataseai Connector: %v\n%s (%s)", err, out, elevateHint())
			}
		}
	case "stop":
		exec.Command("launchctl", "kill", "SIGTERM", darwinDomain()+"/"+darwinServiceLabel).Run()
	case "restart":
		exec.Command("launchctl", "kill", "SIGTERM", darwinDomain()+"/"+darwinServiceLabel).Run()
	case "uninstall":
		darwinBootout()
		os.Remove(darwinPlistPath())
	default:
		log.Fatalf("unknown action: %s", action)
	}
	log.Printf("%s ok", action)
}

func darwinCurrentServiceStatus() control.ServiceStatus {
	out, err := exec.Command("launchctl", "print", darwinDomain()+"/"+darwinServiceLabel).Output()
	if err != nil {
		if _, statErr := os.Stat(darwinPlistPath()); os.IsNotExist(statErr) {
			return control.ServiceStatusNotInstalled
		}
		return control.ServiceStatusNotInstalled
	}
	if strings.Contains(string(out), "state = running") {
		return control.ServiceStatusRunning
	}
	return control.ServiceStatusStopped
}

// isDarwin is used in main.go to gate darwin-specific code paths without
// causing compilation errors on other platforms (all functions are defined
// on all platforms; only called when isDarwin is true).
var isDarwin = runtime.GOOS == "darwin"
