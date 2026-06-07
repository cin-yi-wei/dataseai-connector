package main

import (
	"testing"

	"github.com/cin-yi-wei/dataseai-connector/internal/control"
	"github.com/kardianos/service"
)

func TestMapServiceStatusNotInstalled(t *testing.T) {
	got := mapServiceStatus(service.StatusUnknown, service.ErrNotInstalled)
	if got != control.ServiceStatusNotInstalled {
		t.Fatalf("status = %q, want %q", got, control.ServiceStatusNotInstalled)
	}
}
