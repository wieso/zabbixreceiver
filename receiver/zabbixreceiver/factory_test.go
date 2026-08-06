package zabbixreceiver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory()

	assert.Equal(t, "zabbix", factory.Type().String())
	assert.Equal(t, component.StabilityLevelDevelopment, factory.MetricsStability())
	assert.Equal(t, createDefaultConfig(), factory.CreateDefaultConfig())
}

func TestNewFactoryCreatesReceiverFromClonedResolvedConfig(t *testing.T) {
	factory := NewFactory()
	cfg := validConfig()
	original := cfg.Clone()
	t.Setenv("ZABBIX_URL", "https://env.example/api_jsonrpc.php")
	t.Setenv("ZABBIX_TOKEN", "env-token")
	t.Setenv("ZABBIX_TIMEOUT", "7s")
	t.Setenv("MAX_METRICS_PER_HOST", "12")
	t.Setenv("ZABBIX_ITEMS_PER_REQUEST", "34")

	created, err := factory.CreateMetrics(
		context.Background(),
		receivertest.NewNopSettings(factory.Type()),
		cfg,
		newRecordingConsumer(t).metrics,
	)
	require.NoError(t, err)

	receiver, ok := created.(*zabbixReceiver)
	require.True(t, ok)
	assert.Equal(t, original, cfg, "factory mutated the caller-owned config")
	assert.Equal(t, "https://env.example/api_jsonrpc.php", receiver.config.Zabbix.URL)
	assert.Equal(t, 34, receiver.config.Zabbix.Limits.ItemsPerRequest)
}

func TestNewFactoryValidatesResolvedConfigBeforeConstructingReceiver(t *testing.T) {
	factory := NewFactory()
	cfg := validConfig()
	t.Setenv("ZABBIX_URL", cfg.Zabbix.URL)
	t.Setenv("ZABBIX_TOKEN", string(cfg.Zabbix.Token))
	t.Setenv("ZABBIX_TIMEOUT", cfg.Zabbix.Timeout.String())
	t.Setenv("MAX_METRICS_PER_HOST", "1")
	t.Setenv("ZABBIX_ITEMS_PER_REQUEST", "0")

	created, err := factory.CreateMetrics(
		context.Background(),
		receivertest.NewNopSettings(factory.Type()),
		cfg,
		newRecordingConsumer(t).metrics,
	)

	require.Error(t, err)
	assert.Nil(t, created)
	assert.Contains(t, err.Error(), "zabbix.limits.items_per_request")
	assert.Equal(t, 1000, cfg.Zabbix.Limits.ItemsPerRequest, "environment resolution mutated the caller-owned config")
}

func TestNewFactoryRejectsNonPositiveResolvedZabbixTimeout(t *testing.T) {
	for _, timeout := range []string{"0s", "-1s"} {
		t.Run(timeout, func(t *testing.T) {
			factory := NewFactory()
			cfg := validConfig()
			original := cfg.Clone()
			setValidReceiverEnvironment(t)
			t.Setenv("ZABBIX_TIMEOUT", timeout)

			created, err := factory.CreateMetrics(
				context.Background(),
				receivertest.NewNopSettings(factory.Type()),
				cfg,
				newRecordingConsumer(t).metrics,
			)

			require.Error(t, err)
			assert.Nil(t, created)
			assert.Contains(t, err.Error(), "zabbix.timeout")
			assert.Equal(t, original, cfg, "factory mutated the caller-owned config")
		})
	}
}

func TestNewFactoryAcceptsValidZabbixTimeoutEnvironmentReplacement(t *testing.T) {
	factory := NewFactory()
	cfg := validConfig()
	cfg.Zabbix.Timeout = 0
	original := cfg.Clone()
	setValidReceiverEnvironment(t)
	t.Setenv("ZABBIX_TIMEOUT", "7s")

	created, err := factory.CreateMetrics(
		context.Background(),
		receivertest.NewNopSettings(factory.Type()),
		cfg,
		newRecordingConsumer(t).metrics,
	)
	require.NoError(t, err)

	receiver, ok := created.(*zabbixReceiver)
	require.True(t, ok)
	assert.Equal(t, 7*time.Second, receiver.config.Zabbix.Timeout)
	assert.Equal(t, original, cfg, "factory mutated the caller-owned config")
}

func TestNewFactoryRejectsUnexpectedConfigType(t *testing.T) {
	factory := NewFactory()

	created, err := factory.CreateMetrics(
		context.Background(),
		receivertest.NewNopSettings(factory.Type()),
		struct{}{},
		newRecordingConsumer(t).metrics,
	)

	require.Error(t, err)
	assert.Nil(t, created)
	assert.Contains(t, err.Error(), "*zabbixreceiver.Config")
}
