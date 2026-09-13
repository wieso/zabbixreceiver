package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectorValidateUsesResolvedZabbixConfig(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "otelcol-zabbix")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build custom Collector: %v\n%s", err, output)
	}

	tests := []struct {
		name         string
		receiverYAML string
		environment  map[string]string
		wantError    []string
	}{
		{
			name: "invalid host group format",
			receiverYAML: `
    mode: streaming
    metadata:
      host_groups_format: invalid`,
			wantError: []string{"metadata.host_groups_format"},
		},
		{
			name: "streaming enrichment with API credentials",
			receiverYAML: `
    mode: streaming
    metadata:
      enabled: true
      inherited_host_tags: true
      inventory_fields: [os, location]
      host_groups_format: both
    streaming:
      enrich_with_api: true`,
			environment: map[string]string{"ZABBIX_URL": "https://env.example/api_jsonrpc.php", "ZABBIX_TOKEN": "env-secret"},
		},
		{
			name: "streaming enrichment requires API credentials",
			receiverYAML: `
    mode: streaming
    streaming:
      enrich_with_api: true`,
			wantError: []string{"zabbix.url", "zabbix.token"},
		},
		{
			name: "streaming without API credentials",
			receiverYAML: `
    mode: streaming
    streaming:
      endpoint: 127.0.0.1:8081`,
			environment: map[string]string{
				"ZABBIX_URL":           "",
				"ZABBIX_TOKEN":         "",
				"ZABBIX_TIMEOUT":       "invalid",
				"MAX_METRICS_PER_HOST": "invalid",
			},
		},
		{
			name:         "environment-only URL and token",
			receiverYAML: "{}",
			environment: map[string]string{
				"ZABBIX_URL":   "https://env.example/api_jsonrpc.php",
				"ZABBIX_TOKEN": "env-secret",
			},
		},
		{
			name: "environment replaces invalid YAML request limit",
			receiverYAML: `
    zabbix:
      limits:
        items_per_request: 0`,
			environment: map[string]string{
				"ZABBIX_URL":               "https://env.example/api_jsonrpc.php",
				"ZABBIX_TOKEN":             "env-secret",
				"ZABBIX_ITEMS_PER_REQUEST": "250",
			},
		},
		{
			name: "environment replaces invalid YAML timeout",
			receiverYAML: `
    zabbix:
      timeout: 0s`,
			environment: map[string]string{
				"ZABBIX_URL":     "https://env.example/api_jsonrpc.php",
				"ZABBIX_TOKEN":   "env-secret",
				"ZABBIX_TIMEOUT": "7s",
			},
		},
		{
			name:         "missing URL and token",
			receiverYAML: "{}",
			wantError:    []string{"zabbix.url", "zabbix.token"},
		},
		{
			name:         "invalid request limit environment",
			receiverYAML: "{}",
			environment: map[string]string{
				"ZABBIX_URL":               "https://env.example/api_jsonrpc.php",
				"ZABBIX_TOKEN":             "env-secret",
				"ZABBIX_ITEMS_PER_REQUEST": "0",
			},
			wantError: []string{"zabbix.limits.items_per_request"},
		},
		{
			name:         "zero timeout environment",
			receiverYAML: "{}",
			environment: map[string]string{
				"ZABBIX_URL":     "https://env.example/api_jsonrpc.php",
				"ZABBIX_TOKEN":   "env-secret",
				"ZABBIX_TIMEOUT": "0s",
			},
			wantError: []string{"zabbix.timeout"},
		},
		{
			name: "negative YAML timeout",
			receiverYAML: `
    zabbix:
      timeout: -1s`,
			environment: map[string]string{
				"ZABBIX_URL":   "https://env.example/api_jsonrpc.php",
				"ZABBIX_TOKEN": "env-secret",
			},
			wantError: []string{"zabbix.timeout"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "otelcol.yaml")
			config := fmt.Sprintf(`receivers:
  zabbix: %s
exporters:
  prometheusremotewrite:
    endpoint: http://victoriametrics.example/api/v1/write
service:
  pipelines:
    metrics:
      receivers: [zabbix]
      exporters: [prometheusremotewrite]
`, tt.receiverYAML)
			if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}

			command := exec.Command(binary, "validate", "--config", configPath)
			command.Env = collectorValidateEnvironment(tt.environment)
			output, err := command.CombinedOutput()
			if len(tt.wantError) == 0 {
				if err != nil {
					t.Fatalf("Collector validate failed: %v\n%s", err, output)
				}
				return
			}
			if err == nil {
				t.Fatalf("Collector validate succeeded, want error containing %v", tt.wantError)
			}
			for _, want := range tt.wantError {
				if !strings.Contains(string(output), want) {
					t.Errorf("Collector validate output missing %q:\n%s", want, output)
				}
			}
		})
	}
}

func collectorValidateEnvironment(overrides map[string]string) []string {
	receiverVariables := map[string]struct{}{
		"ZABBIX_URL":               {},
		"ZABBIX_TOKEN":             {},
		"ZABBIX_TIMEOUT":           {},
		"MAX_METRICS_PER_HOST":     {},
		"ZABBIX_ITEMS_PER_REQUEST": {},
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, receiverVariable := receiverVariables[name]; !receiverVariable {
			environment = append(environment, entry)
		}
	}
	for name, value := range overrides {
		environment = append(environment, name+"="+value)
	}
	return environment
}
