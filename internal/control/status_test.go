package control

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticsRedactsToken(t *testing.T) {
	token := "ag_super_secret_token_1234567890"
	diag := NewDiagnostics(DiagnosticsInput{
		ConfigPath:    "/etc/dataseai-connector/config.yaml",
		Config:        Config{Token: token, Server: "wss://dataseai.example/agent", Executor: "mysql"},
		ServiceStatus: ServiceStatusRunning,
		LogLines:      []string{"connected with token " + token},
	})

	b, err := json.Marshal(diag)
	if err != nil {
		t.Fatalf("marshal diagnostics: %v", err)
	}
	got := string(b)
	if strings.Contains(got, token) {
		t.Fatalf("diagnostics leaked full token: %s", got)
	}
	if !strings.Contains(got, `"token_masked"`) {
		t.Fatalf("diagnostics missing token_masked: %s", got)
	}
	if !strings.Contains(got, `"server":"wss://dataseai.example/agent"`) {
		t.Fatalf("diagnostics missing server: %s", got)
	}
	if !strings.Contains(got, `"executor":"mysql"`) {
		t.Fatalf("diagnostics missing executor: %s", got)
	}
}

func TestStatusReportJSON(t *testing.T) {
	report := StatusReport{ServiceStatus: ServiceStatusStopped}

	b, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal status report: %v", err)
	}
	if got, want := string(b), `{"service_status":"stopped"}`; got != want {
		t.Fatalf("status JSON = %s, want %s", got, want)
	}
}
