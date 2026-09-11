package packaging

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGrafanaVerificationRequiresProvisionedDashboard(t *testing.T) {
	for _, dependency := range []string{"curl", "jq"} {
		if _, err := exec.LookPath(dependency); err != nil {
			t.Skip(dependency + " is required")
		}
	}
	script, err := filepath.Abs("../../demo/verify-grafana.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body string
		status     int
		success    bool
	}{
		{"ready", `{"dashboard":{"uid":"zabbix-receiver","panels":[{}]}}`, 200, true},
		{"dashboard missing", `{"message":"not found"}`, 404, false},
		{"wrong dashboard", `{"dashboard":{"uid":"other","panels":[{}]}}`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/health":
					w.Write([]byte(`{"database":"ok"}`))
				case "/api/dashboards/uid/zabbix-receiver":
					w.WriteHeader(tc.status)
					w.Write([]byte(tc.body))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			cmd := exec.Command("/bin/sh", script)
			cmd.Env = append(os.Environ(), "GRAFANA_URL="+server.URL, "VERIFY_TIMEOUT_SECONDS=2")
			output, err := cmd.CombinedOutput()
			if (err == nil) != tc.success {
				t.Fatalf("success=%v err=%v output=%s", tc.success, err, output)
			}
		})
	}
}
