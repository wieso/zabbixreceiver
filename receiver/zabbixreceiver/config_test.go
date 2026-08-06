package zabbixreceiver

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configopaque"
)

func TestCreateDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	require.Equal(t, 5*time.Second, cfg.Schedule.Jitter)
	require.Equal(t, JobConfig{Enabled: true, RunOnStart: true, Interval: 5 * time.Minute, Timeout: time.Minute}, cfg.Schedule.Jobs.Discover)
	require.Equal(t, JobConfig{Enabled: true, RunOnStart: false, Interval: 30 * time.Second, Timeout: 20 * time.Second}, cfg.Schedule.Jobs.Values)
	require.Equal(t, "zabbix_", cfg.Prom.Prefix)
	require.Equal(t, 30*time.Second, cfg.Zabbix.Timeout)
	require.Equal(t, LimitsConfig{MaxMetricsPerHost: 1000, ItemsPerRequest: 1000}, cfg.Zabbix.Limits)
}

func TestResolveEnvTakesPrecedence(t *testing.T) {
	cfg := validConfig()
	env := map[string]string{
		"ZABBIX_URL":               "https://env.example/api_jsonrpc.php",
		"ZABBIX_TOKEN":             "env-secret",
		"ZABBIX_TIMEOUT":           "7s",
		"MAX_METRICS_PER_HOST":     "12",
		"ZABBIX_ITEMS_PER_REQUEST": "34",
	}
	require.NoError(t, cfg.ResolveEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok }))
	assert.Equal(t, "https://env.example/api_jsonrpc.php", cfg.Zabbix.URL)
	assert.Equal(t, configopaque.String("env-secret"), cfg.Zabbix.Token)
	assert.Equal(t, 7*time.Second, cfg.Zabbix.Timeout)
	assert.Equal(t, 12, cfg.Zabbix.Limits.MaxMetricsPerHost)
	assert.Equal(t, 34, cfg.Zabbix.Limits.ItemsPerRequest)
}

func TestResolveEnvRejectsInvalidValues(t *testing.T) {
	for _, tt := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "duration", key: "ZABBIX_TIMEOUT", value: "soon"},
		{name: "per-host integer", key: "MAX_METRICS_PER_HOST", value: "many"},
		{name: "per-request integer", key: "ZABBIX_ITEMS_PER_REQUEST", value: "many"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			err := cfg.ResolveEnv(func(k string) (string, bool) { return tt.value, k == tt.key })
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.key)
		})
	}
}

func TestValidateResolvedConfig(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*Config)
		field  string
	}{
		{name: "missing URL", mutate: func(c *Config) { c.Zabbix.URL = "" }, field: "zabbix.url"},
		{name: "non HTTP URL", mutate: func(c *Config) { c.Zabbix.URL = "ftp://zabbix.example" }, field: "zabbix.url"},
		{name: "empty token", mutate: func(c *Config) { c.Zabbix.Token = "" }, field: "zabbix.token"},
		{name: "negative jitter", mutate: func(c *Config) { c.Schedule.Jitter = -time.Second }, field: "schedule.jitter"},
		{name: "discover non-positive interval", mutate: func(c *Config) { c.Schedule.Jobs.Discover.Interval = 0 }, field: "schedule.jobs.discover.interval"},
		{name: "values non-positive timeout", mutate: func(c *Config) { c.Schedule.Jobs.Values.Timeout = 0 }, field: "schedule.jobs.values.timeout"},
		{name: "zero Zabbix timeout", mutate: func(c *Config) { c.Zabbix.Timeout = 0 }, field: "zabbix.timeout"},
		{name: "negative Zabbix timeout", mutate: func(c *Config) { c.Zabbix.Timeout = -time.Second }, field: "zabbix.timeout"},
		{name: "invalid host include regex", mutate: func(c *Config) { c.Zabbix.Filters.HostIncludeRegex = "[" }, field: "zabbix.filters.host_include_regex"},
		{name: "invalid host exclude regex", mutate: func(c *Config) { c.Zabbix.Filters.HostExcludeRegex = "[" }, field: "zabbix.filters.host_exclude_regex"},
		{name: "invalid item include regex", mutate: func(c *Config) { c.Zabbix.Filters.ItemKeyIncludeRegex = "[" }, field: "zabbix.filters.item_key_include_regex"},
		{name: "invalid item exclude regex", mutate: func(c *Config) { c.Zabbix.Filters.ItemKeyExcludeRegex = "[" }, field: "zabbix.filters.item_key_exclude_regex"},
		{name: "non-positive per-host limit", mutate: func(c *Config) { c.Zabbix.Limits.MaxMetricsPerHost = 0 }, field: "zabbix.limits.max_metrics_per_host"},
		{name: "non-positive request limit", mutate: func(c *Config) { c.Zabbix.Limits.ItemsPerRequest = 0 }, field: "zabbix.limits.items_per_request"},
		{name: "invalid metric prefix", mutate: func(c *Config) { c.Prom.Prefix = "invalid-prefix" }, field: "prom.prefix"},
		{name: "both jobs disabled", mutate: func(c *Config) { c.Schedule.Jobs.Discover.Enabled = false; c.Schedule.Jobs.Values.Enabled = false }, field: "schedule.jobs"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(cfg)
			err := cfg.validateResolved()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.field)
		})
	}
}

func TestValidateResolvesExplicitEnvironmentOnClone(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Zabbix.Limits.ItemsPerRequest = 0
	original := cfg.Clone()
	setValidReceiverEnvironment(t)
	t.Setenv("ZABBIX_ITEMS_PER_REQUEST", "250")

	require.NoError(t, cfg.Validate())
	assert.Equal(t, original, cfg, "validation mutated the caller-owned config")
}

func TestValidateZabbixTimeoutEnvironmentPrecedence(t *testing.T) {
	for _, tt := range []struct {
		name        string
		yamlTimeout time.Duration
		envTimeout  string
		wantError   bool
	}{
		{name: "zero override is rejected", yamlTimeout: 30 * time.Second, envTimeout: "0s", wantError: true},
		{name: "negative override is rejected", yamlTimeout: 30 * time.Second, envTimeout: "-1s", wantError: true},
		{name: "valid override replaces YAML zero", yamlTimeout: 0, envTimeout: "7s"},
		{name: "valid override replaces YAML negative", yamlTimeout: -time.Second, envTimeout: "7s"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Zabbix.Timeout = tt.yamlTimeout
			original := cfg.Clone()
			setValidReceiverEnvironment(t)
			t.Setenv("ZABBIX_TIMEOUT", tt.envTimeout)

			err := cfg.Validate()
			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "zabbix.timeout")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, original, cfg, "validation mutated the caller-owned config")
		})
	}
}

func TestValidateAggregatesIndependentErrors(t *testing.T) {
	cfg := validConfig()
	cfg.Zabbix.URL = ""
	cfg.Zabbix.Token = ""
	err := cfg.validateResolved()
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "zabbix.url") && strings.Contains(err.Error(), "zabbix.token"))
}

func TestCloneDoesNotAliasConstLabels(t *testing.T) {
	cfg := validConfig()
	cfg.Prom.ConstLabels = map[string]string{"environment": "test"}

	clone := cfg.Clone()
	clone.Prom.ConstLabels["environment"] = "production"

	assert.Equal(t, "test", cfg.Prom.ConstLabels["environment"])
}

func validConfig() *Config {
	cfg := createDefaultConfig().(*Config)
	cfg.Zabbix.URL = "https://zabbix.example/api_jsonrpc.php"
	cfg.Zabbix.Token = configopaque.String("test-secret")
	return cfg
}

func setValidReceiverEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("ZABBIX_URL", "https://env.example/api_jsonrpc.php")
	t.Setenv("ZABBIX_TOKEN", "env-secret")
	t.Setenv("ZABBIX_TIMEOUT", "7s")
	t.Setenv("MAX_METRICS_PER_HOST", "12")
	t.Setenv("ZABBIX_ITEMS_PER_REQUEST", "34")
}
