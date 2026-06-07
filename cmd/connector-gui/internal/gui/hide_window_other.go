//go:build !windows

package gui

import "os/exec"

func hideWindow(_ *exec.Cmd) {}
