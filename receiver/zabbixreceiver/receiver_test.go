package zabbixreceiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/discovery"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/receivertest"
	"go.opentelemetry.io/otel/metric/noop"
)

func TestReceiverHTTPIntegration(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		defer request.Body.Close()

		var rpcRequest struct {
			JSONRPC string                     `json:"jsonrpc"`
			Method  string                     `json:"method"`
			Params  map[string]json.RawMessage `json:"params"`
			ID      uint64                     `json:"id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&rpcRequest); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var result any
		switch rpcRequest.Method {
		case "host.get":
			result = []map[string]string{{"hostid": "101", "host": "prod-a"}}
		case "item.get":
			if _, discovering := rpcRequest.Params["hostids"]; discovering {
				result = []map[string]string{{
					"itemid": "202", "hostid": "101", "name": "CPU load", "key_": "system.cpu.load", "value_type": "0",
				}}
			} else if _, collecting := rpcRequest.Params["itemids"]; collecting {
				result = []map[string]string{{"itemid": "202", "lastvalue": "12.5", "lastclock": "1700000000"}}
			} else {
				http.Error(w, "unrecognized item.get form", http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": rpcRequest.ID})
	}))
	t.Cleanup(server.Close)

	factory := NewFactory()
	cfg := validConfig()
	cfg.Schedule.Jitter = 0
	cfg.Schedule.Jobs.Discover = JobConfig{Enabled: true, RunOnStart: true, Interval: 5 * time.Millisecond, Timeout: time.Second}
	cfg.Schedule.Jobs.Values = JobConfig{Enabled: true, RunOnStart: true, Interval: 5 * time.Millisecond, Timeout: time.Second}
	cfg.Prom.ConstLabels = map[string]string{"environment": "integration"}
	t.Setenv("ZABBIX_URL", server.URL)
	t.Setenv("ZABBIX_TOKEN", "integration-token")
	t.Setenv("ZABBIX_TIMEOUT", time.Second.String())
	t.Setenv("MAX_METRICS_PER_HOST", "10")
	t.Setenv("ZABBIX_ITEMS_PER_REQUEST", "10")

	consumer := newRecordingConsumer(t)
	created, err := factory.CreateMetrics(
		context.Background(),
		receivertest.NewNopSettings(factory.Type()),
		cfg,
		consumer.metrics,
	)
	require.NoError(t, err)
	require.NoError(t, created.Start(context.Background(), nil))

	require.Eventually(t, func() bool { return len(consumer.snapshot()) > 0 }, 2*time.Second, time.Millisecond)
	batches := consumer.snapshot()
	require.NotEmpty(t, batches)
	metrics := batches[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	require.Equal(t, 1, metrics.Len())
	metric := metrics.At(0)
	assert.Equal(t, "zabbix_system_cpu_load", metric.Name())
	assert.Equal(t, pmetric.MetricTypeGauge, metric.Type())
	points := metric.Gauge().DataPoints()
	require.Equal(t, 1, points.Len())
	point := points.At(0)
	assert.Equal(t, 12.5, point.DoubleValue())
	assert.Equal(t, pcommon.Timestamp(1700000000*1_000_000_000), point.Timestamp())
	assert.Equal(t, map[string]any{
		"environment": "integration",
		"host":        "prod-a",
		"hostid":      "101",
		"item_key":    "system.cpu.load",
		"itemid":      "202",
	}, point.Attributes().AsRaw())

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, created.Shutdown(shutdownCtx))
	requestsAfterShutdown := waitForStableRequestCount(t, &requests, 20*time.Millisecond, time.Second)
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, requestsAfterShutdown, requests.Load(), "requests continued after shutdown")
}

func waitForStableRequestCount(t *testing.T, requests *atomic.Int64, stableFor, timeout time.Duration) int64 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := requests.Load()
	stableSince := time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		current := requests.Load()
		if current != last {
			last = current
			stableSince = time.Now()
			continue
		}
		if time.Since(stableSince) >= stableFor {
			return current
		}
	}
	t.Fatalf("request count did not remain stable for %s", stableFor)
	return 0
}

func TestDiscoverReplacesSnapshotOnlyAfterSuccess(t *testing.T) {
	api := &fakeAPI{}
	receiver := newTestReceiver(t, validConfig(), api, newRecordingConsumer(t))
	api.hosts = func(context.Context) ([]zabbix.Host, error) {
		return []zabbix.Host{{ID: "1", Name: "server"}}, nil
	}
	api.items = func(_ context.Context, hostIDs []string) ([]zabbix.Item, error) {
		assert.Equal(t, []string{"1"}, hostIDs)
		assert.Nil(t, receiver.store.Load(), "snapshot was published before item discovery succeeded")
		return []zabbix.Item{{ID: "10", HostID: "1", Name: "CPU", Key: "system.cpu", ValueType: "0"}}, nil
	}

	require.NoError(t, receiver.discover(context.Background()))
	require.NotNil(t, receiver.store.Load())
	assert.Equal(t, []discovery.ItemMeta{{ID: "10", HostID: "1", Host: "server", Name: "CPU", Key: "system.cpu", ValueType: "0"}}, receiver.store.Load().Items())
}

func TestDiscoverRetainsSnapshotOnItemFailure(t *testing.T) {
	wantErr := errors.New("item failure")
	api := &fakeAPI{
		hosts: func(context.Context) ([]zabbix.Host, error) {
			return []zabbix.Host{{ID: "1", Name: "server"}}, nil
		},
		items: func(context.Context, []string) ([]zabbix.Item, error) {
			return nil, wantErr
		},
	}
	receiver := newTestReceiver(t, validConfig(), api, newRecordingConsumer(t))
	previous := discovery.NewSnapshot([]discovery.ItemMeta{{ID: "old"}})
	receiver.store.Replace(previous)

	err := receiver.discover(context.Background())
	require.ErrorIs(t, err, wantErr)
	assert.Same(t, previous, receiver.store.Load())
}

func TestValuesNoSnapshotIsNoOp(t *testing.T) {
	valuesCalls := 0
	api := &fakeAPI{values: func(context.Context, []string) ([]zabbix.Value, error) {
		valuesCalls++
		return nil, nil
	}}
	consumer := newRecordingConsumer(t)
	receiver := newTestReceiver(t, validConfig(), api, consumer)

	require.NoError(t, receiver.values(context.Background()))
	assert.Zero(t, valuesCalls)
	assert.Empty(t, consumer.snapshot())
}

func TestValuesChunksRequestsAndConsumesOneBatch(t *testing.T) {
	cfg := validConfig()
	cfg.Zabbix.Limits.ItemsPerRequest = 2
	var requests [][]string
	api := &fakeAPI{values: func(_ context.Context, itemIDs []string) ([]zabbix.Value, error) {
		requests = append(requests, append([]string(nil), itemIDs...))
		values := make([]zabbix.Value, 0, len(itemIDs))
		for _, id := range itemIDs {
			values = append(values, zabbix.Value{ItemID: id, LastValue: id, LastClock: "1"})
		}
		return values, nil
	}}
	consumer := newRecordingConsumer(t)
	receiver := newTestReceiver(t, cfg, api, consumer)
	receiver.store.Replace(discovery.NewSnapshot(testItems(5)))

	require.NoError(t, receiver.values(context.Background()))
	assert.Equal(t, [][]string{{"1", "2"}, {"3", "4"}, {"5"}}, requests)
	batches := consumer.snapshot()
	require.Len(t, batches, 1)
	assert.Equal(t, 5, batches[0].MetricCount())
	assert.Equal(t, 5, batches[0].DataPointCount())
}

func TestValuesAbortsBatchWhenAChunkFails(t *testing.T) {
	cfg := validConfig()
	cfg.Zabbix.Limits.ItemsPerRequest = 2
	wantErr := errors.New("chunk failure")
	var requests [][]string
	api := &fakeAPI{values: func(_ context.Context, itemIDs []string) ([]zabbix.Value, error) {
		requests = append(requests, append([]string(nil), itemIDs...))
		if len(requests) == 2 {
			return nil, wantErr
		}
		return []zabbix.Value{{ItemID: itemIDs[0], LastValue: "1", LastClock: "1"}}, nil
	}}
	consumer := newRecordingConsumer(t)
	receiver := newTestReceiver(t, cfg, api, consumer)
	receiver.store.Replace(discovery.NewSnapshot(testItems(5)))

	err := receiver.values(context.Background())
	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, [][]string{{"1", "2"}, {"3", "4"}}, requests)
	assert.Empty(t, consumer.snapshot())
}

func TestValuesReturnsConsumerError(t *testing.T) {
	wantErr := errors.New("consumer failure")
	api := &fakeAPI{values: func(_ context.Context, itemIDs []string) ([]zabbix.Value, error) {
		return []zabbix.Value{{ItemID: itemIDs[0], LastValue: "1", LastClock: "1"}}, nil
	}}
	consumer := newRecordingConsumer(t)
	consumer.err = wantErr
	receiver := newTestReceiver(t, validConfig(), api, consumer)
	receiver.store.Replace(discovery.NewSnapshot(testItems(1)))

	err := receiver.values(context.Background())
	require.ErrorIs(t, err, wantErr)
	assert.Len(t, consumer.snapshot(), 1)
}

func TestValuesSkipsConsumerWhenNoValidPoints(t *testing.T) {
	api := &fakeAPI{values: func(_ context.Context, itemIDs []string) ([]zabbix.Value, error) {
		return []zabbix.Value{{ItemID: itemIDs[0], LastValue: "not-a-number", LastClock: "1"}}, nil
	}}
	consumer := newRecordingConsumer(t)
	receiver := newTestReceiver(t, validConfig(), api, consumer)
	receiver.store.Replace(discovery.NewSnapshot(testItems(1)))

	require.NoError(t, receiver.values(context.Background()))
	assert.Empty(t, consumer.snapshot())
}

func TestStartAndShutdownStopAllJobs(t *testing.T) {
	cfg := validConfig()
	cfg.Schedule.Jitter = 0
	cfg.Schedule.Jobs.Discover = JobConfig{Enabled: true, RunOnStart: true, Interval: time.Hour, Timeout: time.Hour}
	cfg.Schedule.Jobs.Values = JobConfig{Enabled: true, RunOnStart: true, Interval: time.Hour, Timeout: time.Hour}

	discoverStarted := make(chan struct{}, 1)
	valuesStarted := make(chan struct{}, 1)
	api := &fakeAPI{
		hosts: func(ctx context.Context) ([]zabbix.Host, error) {
			discoverStarted <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		},
		values: func(ctx context.Context, _ []string) ([]zabbix.Value, error) {
			valuesStarted <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	receiver := newTestReceiver(t, cfg, api, newRecordingConsumer(t))
	receiver.store.Replace(discovery.NewSnapshot(testItems(1)))

	require.NoError(t, receiver.Start(context.Background(), nil))
	requireReceive(t, discoverStarted, "discover job did not start")
	requireReceive(t, valuesStarted, "values job did not start")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, receiver.Shutdown(shutdownCtx))
	assert.Zero(t, api.callCount())
}

func TestStartContextCancellationDoesNotStopJobs(t *testing.T) {
	cfg := validConfig()
	cfg.Schedule.Jitter = 0
	cfg.Schedule.Jobs.Discover.Enabled = false
	cfg.Schedule.Jobs.Values = JobConfig{Enabled: true, RunOnStart: true, Interval: time.Millisecond, Timeout: time.Hour}

	firstStarted := make(chan struct{}, 1)
	secondStarted := make(chan struct{}, 1)
	var calls atomic.Int32
	api := &fakeAPI{values: func(ctx context.Context, _ []string) ([]zabbix.Value, error) {
		switch calls.Add(1) {
		case 1:
			firstStarted <- struct{}{}
			return nil, nil
		case 2:
			secondStarted <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		default:
			return nil, errors.New("unexpected extra values run")
		}
	}}
	receiver := newTestReceiver(t, cfg, api, newRecordingConsumer(t))
	receiver.store.Replace(discovery.NewSnapshot(testItems(1)))

	startCtx, cancelStart := context.WithCancel(context.Background())
	require.NoError(t, receiver.Start(startCtx, nil))
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = receiver.Shutdown(shutdownCtx)
	})

	requireReceive(t, firstStarted, "first values run did not start")
	cancelStart()
	requireReceive(t, secondStarted, "startup context cancellation stopped the values job")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	require.NoError(t, receiver.Shutdown(shutdownCtx))
	assert.Equal(t, int32(2), calls.Load())
}

type fakeAPI struct {
	mu sync.Mutex

	hosts  func(context.Context) ([]zabbix.Host, error)
	items  func(context.Context, []string) ([]zabbix.Item, error)
	values func(context.Context, []string) ([]zabbix.Value, error)
	calls  int
}

func (f *fakeAPI) Hosts(ctx context.Context) ([]zabbix.Host, error) {
	f.beginCall()
	defer f.endCall()
	if f.hosts == nil {
		return nil, errors.New("unexpected Hosts call")
	}
	return f.hosts(ctx)
}

func (f *fakeAPI) Items(ctx context.Context, hostIDs []string) ([]zabbix.Item, error) {
	f.beginCall()
	defer f.endCall()
	if f.items == nil {
		return nil, errors.New("unexpected Items call")
	}
	return f.items(ctx, hostIDs)
}

func (f *fakeAPI) Values(ctx context.Context, itemIDs []string) ([]zabbix.Value, error) {
	f.beginCall()
	defer f.endCall()
	if f.values == nil {
		return nil, errors.New("unexpected Values call")
	}
	return f.values(ctx, itemIDs)
}

func (f *fakeAPI) beginCall() {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
}

func (f *fakeAPI) endCall() {
	f.mu.Lock()
	f.calls--
	f.mu.Unlock()
}

func (f *fakeAPI) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type recordingConsumer struct {
	mu      sync.Mutex
	batches []pmetric.Metrics
	err     error
	metrics consumer.Metrics
}

func newRecordingConsumer(t *testing.T) *recordingConsumer {
	t.Helper()
	recording := &recordingConsumer{}
	metrics, err := consumer.NewMetrics(func(_ context.Context, batch pmetric.Metrics) error {
		recording.mu.Lock()
		defer recording.mu.Unlock()
		copy := pmetric.NewMetrics()
		batch.CopyTo(copy)
		recording.batches = append(recording.batches, copy)
		return recording.err
	})
	require.NoError(t, err)
	recording.metrics = metrics
	return recording
}

func (r *recordingConsumer) snapshot() []pmetric.Metrics {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]pmetric.Metrics(nil), r.batches...)
}

func newTestReceiver(t *testing.T, cfg *Config, api zabbix.API, next *recordingConsumer) *zabbixReceiver {
	t.Helper()
	settings := receiver.Settings{ID: component.MustNewID("zabbix")}
	telemetry, err := newTelemetry(noop.NewMeterProvider().Meter("zabbixreceiver-test"))
	require.NoError(t, err)
	created, err := newReceiver(settings, cfg, next.metrics, api, telemetry)
	require.NoError(t, err)
	return created
}

func testItems(count int) []discovery.ItemMeta {
	items := make([]discovery.ItemMeta, 0, count)
	for i := 1; i <= count; i++ {
		id := fmt.Sprint(i)
		items = append(items, discovery.ItemMeta{
			ID: id, HostID: "host-" + id, Host: "server-" + id, Name: "Item " + id, Key: "test.item[" + id + "]", ValueType: "0",
		})
	}
	return items
}
