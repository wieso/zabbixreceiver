package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

// Exercise the real Collector telemetry reader: SDK-only tests cannot catch
// component attribution, Prometheus name translation, or service configuration.
func TestCollectorPrometheusObservability(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "otelcol-zabbix")
	output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput()
	require.NoError(t, err, string(output))
	writes := make(chan prompb.WriteRequest, 8)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		decoded, err := snappy.Decode(nil, body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		var request prompb.WriteRequest
		if err := request.Unmarshal(decoded); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		select {
		case writes <- request:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer remote.Close()
	freePort := func() int {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		port := ln.Addr().(*net.TCPAddr).Port
		require.NoError(t, ln.Close())
		return port
	}
	metricsPort, streamPort := freePort(), freePort()
	config := fmt.Sprintf(`receivers:
  zabbix/stream:
    mode: streaming
    streaming:
      endpoint: 127.0.0.1:%d
      token: test-secret
exporters:
  prometheusremotewrite:
    endpoint: %s
service:
  telemetry:
    logs: {level: debug, encoding: json}
    metrics:
      readers:
        - pull:
            exporter:
              prometheus:
                host: 127.0.0.1
                port: %d
  pipelines:
    metrics:
      receivers: [zabbix/stream]
      exporters: [prometheusremotewrite]
`, streamPort, remote.URL, metricsPort)
	configPath := filepath.Join(t.TempDir(), "collector.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0600))
	logPath := filepath.Join(t.TempDir(), "collector.log")
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	cmd := exec.CommandContext(ctx, binary, "--config", configPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		defer cancel()
		_ = cmd.Process.Signal(os.Interrupt)
		waitErr := cmd.Wait()
		_ = logFile.Close()
		logs, err := os.ReadFile(logPath)
		require.NoError(t, err)
		require.NoError(t, waitErr, string(logs))
		require.Contains(t, string(logs), "Zabbix receiver started")
		require.Contains(t, string(logs), "Zabbix streaming request completed")
		require.Contains(t, string(logs), "Zabbix streaming request rejected")
		require.NotContains(t, string(logs), "test-secret")
	})
	client := &http.Client{Timeout: time.Second}
	scrape := func() string {
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", metricsPort))
		if err != nil {
			return ""
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}
	require.Eventually(t, func() bool { return strings.Contains(scrape(), "otelcol_receiver_zabbix_values_attempts") }, 10*time.Second, 50*time.Millisecond)
	send := func(token string) int {
		record := `{"host":{"host":"a","name":"Server A"},"name":"cpu","itemid":1,"clock":1700000000,"ns":1000000000,"value":1,"type":0,"groups":["Linux"],"item_tags":[{"tag":"app","value":"web"},{"tag":"app","value":"api"},{"tag":"empty","value":""},{"tag":"a.b","value":"dot"},{"tag":"a_b","value":"underscore"}]}`
		var compressed bytes.Buffer
		encoder := gzip.NewWriter(&compressed)
		_, err := encoder.Write([]byte(record + "\n{broken\n" + strings.Replace(record, `"ns":1000000000`, `"ns":2000000000`, 1)))
		require.NoError(t, err)
		require.NoError(t, encoder.Close())
		req, err := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/v1/history", streamPort), &compressed)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-ndjson")
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}
	require.Equal(t, 200, send("test-secret"))
	require.Equal(t, 401, send("wrong"))
	select {
	case write := <-writes:
		var labels map[string]string
		for _, series := range write.Timeseries {
			candidate := make(map[string]string)
			for _, label := range series.Labels {
				candidate[label.Name] = label.Value
			}
			if candidate["__name__"] == "zabbix_cpu" {
				labels = candidate
				require.Len(t, series.Samples, 2)
				require.EqualValues(t, 1700000001000, series.Samples[0].Timestamp)
				require.EqualValues(t, 1700000002000, series.Samples[1].Timestamp)
			}
		}
		require.NotNil(t, labels)
		require.Equal(t, "Server A", labels["host_name"])
		require.Equal(t, "Linux", labels["host_groups"])
		require.Equal(t, "true", labels["host_group_linux"])
		require.Equal(t, "api, web", labels["item_tag_app"])
		require.Equal(t, "", labels["item_tag_empty"])
		require.Equal(t, "dot, underscore", labels["item_tag_a_b"])
	case <-time.After(5 * time.Second):
		t.Fatal("no remote write received")
	}
	body := scrape()
	for _, name := range []string{"otelcol_receiver_accepted_metric_points", "otelcol_receiver_zabbix_values_errors", "otelcol_receiver_zabbix_values_duration_bucket", "otelcol_receiver_zabbix_streaming_requests", "otelcol_receiver_zabbix_values_last_success_timestamp"} {
		require.Contains(t, body, name)
	}
	require.Contains(t, body, `otelcol_receiver_accepted_metric_points{receiver="zabbix/stream",transport="http"} 2`)
	require.Contains(t, body, `otelcol_receiver_zabbix_values_attempts{mode="streaming",receiver="zabbix/stream"} 2`)
	require.Contains(t, body, `otelcol_receiver_zabbix_values_errors{mode="streaming",receiver="zabbix/stream"} 1`)
	require.Contains(t, body, `http_response_status_code="401"`)
	require.Contains(t, body, `otelcol_receiver_zabbix_invalid_values{mode="streaming",receiver="zabbix/stream"} 1`)
	require.NotContains(t, body, "test-secret")
}
