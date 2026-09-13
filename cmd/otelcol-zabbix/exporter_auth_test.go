package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test the real asynchronous exporter with batch processing at info log level.
// A receiver acknowledgement must not hide an expired remote-write credential.
func TestCollectorExporterAuthenticationAcrossModes(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "otelcol-zabbix")
	output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput()
	require.NoError(t, err, string(output))
	for _, mode := range []string{"api", "streaming", "enriched"} {
		t.Run(mode, func(t *testing.T) {
			var status atomic.Int64
			status.Store(401)
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer expired-vm-test-token" {
					t.Error("remote write bearer header missing")
				}
				_, _ = io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()
				code := int(status.Load())
				w.WriteHeader(code)
				if code != 204 {
					_, _ = io.WriteString(w, "credential expired")
				}
			}))
			defer remote.Close()
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Method string
					Params map[string]json.RawMessage
					ID     uint64
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				result := `[]`
				switch request.Method {
				case "host.get":
					result = `[{"hostid":"10","host":"srv","name":"Server","hostgroups":[]}]`
				case "item.get":
					if _, ok := request.Params["hostids"]; ok {
						result = `[{"itemid":"1","hostid":"10","name":"CPU Usage (%)","key_":"System.CPUUsage[%]","value_type":"0"}]`
					} else {
						result = `[{"itemid":"1","lastvalue":"2","lastclock":"1700000000"}]`
					}
				default:
					t.Errorf("unexpected API method: %s", request.Method)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": json.RawMessage(result)})
			}))
			defer api.Close()
			freePort := func() int {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				port := ln.Addr().(*net.TCPAddr).Port
				require.NoError(t, ln.Close())
				return port
			}
			metricsPort, streamPort := freePort(), freePort()
			receiverMode := mode
			if mode == "enriched" {
				receiverMode = "streaming"
			}
			config := fmt.Sprintf(`receivers:
  zabbix:
    mode: %s
    streaming:
      endpoint: 127.0.0.1:%d
      enrich_with_api: %t
    zabbix:
      url: %s
      token: api-test-token
    schedule:
      jitter: 0s
      jobs:
        discover: {enabled: true, run_on_start: true, interval: 100ms, timeout: 1s}
        values: {enabled: true, run_on_start: false, interval: 100ms, timeout: 1s}
processors:
  batch: {timeout: 50ms, send_batch_size: 1}
exporters:
  prometheusremotewrite:
    endpoint: %s
    headers:
      Authorization: Bearer expired-vm-test-token
service:
  telemetry:
    logs: {level: info, encoding: json}
    metrics:
      readers:
        - pull:
            exporter:
              prometheus:
                host: 127.0.0.1
                port: %d
  pipelines:
    metrics:
      receivers: [zabbix]
      processors: [batch]
      exporters: [prometheusremotewrite]
`, receiverMode, streamPort, mode == "enriched", api.URL, remote.URL, metricsPort)
			dir := t.TempDir()
			configPath := filepath.Join(dir, "collector.yaml")
			logPath := filepath.Join(dir, "collector.log")
			require.NoError(t, os.WriteFile(configPath, []byte(config), 0600))
			logFile, err := os.Create(logPath)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			cmd := exec.CommandContext(ctx, binary, "--config", configPath)
			cmd.Env = collectorValidateEnvironment(nil)
			cmd.Stdout = logFile
			cmd.Stderr = logFile
			require.NoError(t, cmd.Start())
			logs := func() string { data, _ := os.ReadFile(logPath); return string(data) }
			defer func() {
				_ = cmd.Process.Signal(os.Interrupt)
				waitErr := cmd.Wait()
				cancel()
				_ = logFile.Close()
				require.NoError(t, waitErr, logs())
				require.NotContains(t, logs(), "expired-vm-test-token")
				require.NotContains(t, logs(), "api-test-token")
			}()
			client := &http.Client{Timeout: time.Second}
			scrape := func() string {
				resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", metricsPort))
				if err != nil {
					return ""
				}
				defer resp.Body.Close()
				data, _ := io.ReadAll(resp.Body)
				return string(data)
			}
			require.Eventually(t, func() bool { return strings.Contains(scrape(), "otelcol_receiver_zabbix_values_attempts") }, 10*time.Second, 50*time.Millisecond, logs())
			send := func() {
				if mode == "api" {
					return
				}
				resp, err := client.Post(fmt.Sprintf("http://127.0.0.1:%d/v1/history", streamPort), "application/x-ndjson", strings.NewReader(`{"host":{"host":"srv"},"name":"CPU Usage (%)","itemid":1,"clock":1700000000,"value":2,"type":0}`))
				if err != nil {
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
			for _, code := range []int{401, 403} {
				status.Store(int64(code))
				require.Eventually(t, func() bool {
					send()
					return strings.Contains(logs(), fmt.Sprintf(`"status_code":%d`, code)) && strings.Contains(logs(), `"level":"error"`)
				}, 5*time.Second, 50*time.Millisecond)
			}
			hasPoints := func(body, name string) bool {
				return regexp.MustCompile(`(?m)^` + name + `\{[^\n]*exporter="prometheusremotewrite"[^\n]*\} [1-9][0-9]*(?:\.[0-9]+)?$`).MatchString(body)
			}
			require.Eventually(t, func() bool { return hasPoints(scrape(), "otelcol_exporter_send_failed_metric_points") }, 5*time.Second, 50*time.Millisecond)
			status.Store(204)
			require.Eventually(t, func() bool { send(); return hasPoints(scrape(), "otelcol_exporter_sent_metric_points") }, 5*time.Second, 50*time.Millisecond)
			require.Contains(t, scrape(), "otelcol_receiver_accepted_metric_points")
		})
	}
}
