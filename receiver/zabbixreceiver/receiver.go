package zabbixreceiver

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/discovery"
	otelmetrics "github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/metrics"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"
)

var _ receiver.Metrics = (*zabbixReceiver)(nil)

type zabbixReceiver struct {
	settings  receiver.Settings
	config    *Config
	api       zabbix.API
	next      consumer.Metrics
	filters   discovery.Filters
	store     discovery.Store
	logger    *zap.Logger
	telemetry *receiverTelemetry

	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	started     bool
	shutdown    bool
	done        chan struct{}
	wg          sync.WaitGroup
}

func newReceiver(
	settings receiver.Settings,
	config *Config,
	next consumer.Metrics,
	api zabbix.API,
	telemetry *receiverTelemetry,
) (*zabbixReceiver, error) {
	if config == nil {
		return nil, errors.New("config must not be nil")
	}
	if next == nil {
		return nil, errors.New("metrics consumer must not be nil")
	}
	if api == nil {
		return nil, errors.New("Zabbix API must not be nil")
	}
	if telemetry == nil {
		return nil, errors.New("telemetry must not be nil")
	}

	filters, err := compileFilters(config.Zabbix.Filters)
	if err != nil {
		return nil, err
	}
	logger := settings.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &zabbixReceiver{
		settings:  settings,
		config:    config.Clone(),
		api:       api,
		next:      next,
		filters:   filters,
		logger:    logger,
		telemetry: telemetry,
		done:      make(chan struct{}),
	}, nil
}

func (r *zabbixReceiver) Start(_ context.Context, _ component.Host) error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.started {
		return nil
	}
	if r.shutdown {
		return errors.New("receiver has already shut down")
	}

	lifetimeCtx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.started = true

	jobs := []struct {
		name   string
		config JobConfig
		run    func(context.Context) error
	}{
		{name: "discover", config: r.config.Schedule.Jobs.Discover, run: r.discover},
		{name: "values", config: r.config.Schedule.Jobs.Values, run: r.values},
	}

	startedJobs := 0
	for _, job := range jobs {
		if !job.config.Enabled {
			continue
		}
		startedJobs++
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			runJob(lifetimeCtx, job.config, r.config.Schedule.Jitter, waitTimer, randomJitter, job.run, func(err error) {
				r.logger.Error("Zabbix receiver job failed", zap.String("job", job.name), zap.Error(err))
			})
		}()
	}

	if startedJobs == 0 {
		close(r.done)
	} else {
		go func() {
			r.wg.Wait()
			close(r.done)
		}()
	}
	return nil
}

func (r *zabbixReceiver) Shutdown(ctx context.Context) error {
	r.lifecycleMu.Lock()
	if !r.started {
		r.shutdown = true
		r.lifecycleMu.Unlock()
		return nil
	}
	if !r.shutdown {
		r.shutdown = true
		r.cancel()
	}
	done := r.done
	r.lifecycleMu.Unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *zabbixReceiver) discover(ctx context.Context) (err error) {
	started := time.Now()
	r.telemetry.discoverAttempts.Add(ctx, 1)
	defer func() {
		r.telemetry.discoverDuration.Record(ctx, time.Since(started).Seconds())
		if err != nil {
			r.telemetry.discoverErrors.Add(ctx, 1)
		}
	}()

	hosts, err := r.api.Hosts(ctx)
	if err != nil {
		return fmt.Errorf("get Zabbix hosts: %w", err)
	}
	hostIDs := make([]string, 0, len(hosts))
	for _, host := range hosts {
		hostIDs = append(hostIDs, host.ID)
	}

	items, err := r.api.Items(ctx, hostIDs)
	if err != nil {
		return fmt.Errorf("get Zabbix items: %w", err)
	}
	selected, filtered, limited := discovery.Select(hosts, items, r.filters, r.config.Zabbix.Limits.MaxMetricsPerHost)
	r.telemetry.filteredItems.Add(ctx, int64(filtered))
	r.telemetry.limitedItems.Add(ctx, int64(limited))
	r.store.Replace(discovery.NewSnapshot(selected))
	return nil
}

func (r *zabbixReceiver) values(ctx context.Context) (err error) {
	started := time.Now()
	r.telemetry.valuesAttempts.Add(ctx, 1)
	defer func() {
		r.telemetry.valuesDuration.Record(ctx, time.Since(started).Seconds())
		if err != nil {
			r.telemetry.valuesErrors.Add(ctx, 1)
		}
	}()

	snapshot := r.store.Load()
	if snapshot == nil {
		return nil
	}
	items := snapshot.Items()
	if len(items) == 0 {
		return nil
	}

	itemIDs := make([]string, 0, len(items))
	for _, item := range items {
		itemIDs = append(itemIDs, item.ID)
	}

	requestSize := r.config.Zabbix.Limits.ItemsPerRequest
	values := make([]zabbix.Value, 0, len(items))
	for start := 0; start < len(itemIDs); start += requestSize {
		end := min(start+requestSize, len(itemIDs))
		chunk, err := r.api.Values(ctx, itemIDs[start:end])
		if err != nil {
			return fmt.Errorf("get Zabbix values: %w", err)
		}
		values = append(values, chunk...)
	}

	batch, stats := otelmetrics.Build(items, values, otelmetrics.Config{
		Prefix:      r.config.Prom.Prefix,
		ConstLabels: r.config.Prom.ConstLabels,
		ScopeName:   r.settings.ID.String(),
	})
	r.telemetry.emittedPoints.Add(ctx, stats.Emitted)
	r.telemetry.invalidValues.Add(ctx, stats.Invalid)
	if stats.Emitted == 0 {
		return nil
	}
	if err := r.next.ConsumeMetrics(ctx, batch); err != nil {
		return fmt.Errorf("consume Zabbix metrics: %w", err)
	}
	return nil
}

func compileFilters(config FiltersConfig) (discovery.Filters, error) {
	hostInclude, err := compileOptionalRegex("host_include_regex", config.HostIncludeRegex)
	if err != nil {
		return discovery.Filters{}, err
	}
	hostExclude, err := compileOptionalRegex("host_exclude_regex", config.HostExcludeRegex)
	if err != nil {
		return discovery.Filters{}, err
	}
	itemInclude, err := compileOptionalRegex("item_key_include_regex", config.ItemKeyIncludeRegex)
	if err != nil {
		return discovery.Filters{}, err
	}
	itemExclude, err := compileOptionalRegex("item_key_exclude_regex", config.ItemKeyExcludeRegex)
	if err != nil {
		return discovery.Filters{}, err
	}
	return discovery.Filters{
		HostInclude: hostInclude, HostExclude: hostExclude,
		ItemKeyInclude: itemInclude, ItemKeyExclude: itemExclude,
	}, nil
}

func compileOptionalRegex(name, pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("compile Zabbix filter %s: %w", name, err)
	}
	return compiled, nil
}
