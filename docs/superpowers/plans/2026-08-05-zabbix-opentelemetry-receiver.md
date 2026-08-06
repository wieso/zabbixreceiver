# Zabbix OpenTelemetry Receiver Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and deploy a native Go OpenTelemetry Collector receiver that polls Zabbix numeric items and sends them through a Collector metrics pipeline to VictoriaMetrics.

**Architecture:** A typed Zabbix JSON-RPC client feeds separate discovery and values jobs. Discovery publishes immutable filtered metadata snapshots; values collection turns `lastvalue` and `lastclock` into OpenTelemetry gauges and passes them to `consumer.Metrics`. A custom Collector distribution embeds the receiver and standard Prometheus remote-write exporter.

**Tech Stack:** Go 1.25+, OpenTelemetry Collector v0.153.0 with stable modules v1.59.0, OpenTelemetry Collector Contrib v0.153.0, Zabbix 7.4.12 demo images, VictoriaMetrics v1.148.0 community image, Docker Compose v2.24+, Kubernetes manifests, systemd.

## Global Constraints

- The deliverable is a native OpenTelemetry Collector metrics receiver, not a standalone Prometheus exporter.
- The public receiver type is `zabbix` and the Go module is `github.com/aleksandr/zabbix-otel`.
- Preserve the public receiver `schedule`, `prom.prefix`, `prom.const_labels`, and `zabbix` configuration keys and documented Zabbix environment overrides.
- Omit `base.address`, standalone exporter HTTP endpoints, standalone CLI flags, and agent-specific configuration.
- Keep the receiver independent of VictoriaMetrics; use `prometheusremotewrite` in deployment configuration.
- Poll only Zabbix numeric item types float (`0`) and unsigned integer (`3`).
- Emit OpenTelemetry gauge data points with Zabbix `lastclock` timestamps and the four reserved attributes `host`, `hostid`, `item_key`, and `itemid`.
- Never log the configured Zabbix token or full authenticated JSON-RPC request bodies.
- The runtime container and Kubernetes workload run as non-root.
- The Compose demonstration must use a real Zabbix server/API and prove that the seeded metric is queryable from VictoriaMetrics.
- Every implementation change follows red-green-refactor and ends with focused tests plus a commit.

## File Map

### Module and project automation

- `go.mod`, `go.sum`: root Go module and pinned Collector dependencies.
- `Makefile`: formatting, unit, race, vet, build, container, Compose, and verification targets.
- `.gitignore`, `.dockerignore`: generated binaries, local environment files, coverage, and build context exclusions.
- `.github/workflows/ci.yml`: repeatable Go and packaging checks.

### Receiver component

- `receiver/zabbixreceiver/config.go`: Public receiver configuration types, defaults, environment resolution, and validation.
- `receiver/zabbixreceiver/config_test.go`: defaults, validation, and environment precedence.
- `receiver/zabbixreceiver/factory.go`: Collector `NewFactory` and production construction.
- `receiver/zabbixreceiver/factory_test.go`: component type, stability, default config, and creation checks.
- `receiver/zabbixreceiver/receiver.go`: discovery/value operations, lifecycle, snapshot use, and downstream delivery.
- `receiver/zabbixreceiver/receiver_test.go`: operation, chunking, recovery, and integration-fixture tests.
- `receiver/zabbixreceiver/scheduler.go`: independent non-overlapping job loops with interval, timeout, run-on-start, and jitter behavior.
- `receiver/zabbixreceiver/scheduler_test.go`: deterministic delay calculation, cancellation, and non-overlap tests.
- `receiver/zabbixreceiver/telemetry.go`: Collector-native receiver instruments.
- `receiver/zabbixreceiver/telemetry_test.go`: instrument creation and counter recording smoke tests.

### Internal packages

- `internal/zabbix/types.go`: `Host`, `Item`, `Value`, API interface, and JSON-RPC errors.
- `internal/zabbix/client.go`: authenticated JSON-RPC transport and typed API methods.
- `internal/zabbix/client_test.go`: request contracts, modern/legacy auth, decoding, timeouts, and redaction.
- `internal/discovery/select.go`: regex filtering and deterministic per-host limits.
- `internal/discovery/select_test.go`: include/exclude precedence and limit tests.
- `internal/discovery/snapshot.go`: immutable atomic snapshot store.
- `internal/discovery/snapshot_test.go`: copy and replacement behavior.
- `internal/metrics/name.go`: Prometheus-compatible metric-name construction.
- `internal/metrics/name_test.go`: sanitization and validation table tests.
- `internal/metrics/builder.go`: Zabbix-to-`pmetric.Metrics` conversion.
- `internal/metrics/builder_test.go`: gauges, values, timestamps, attributes, descriptions, and skipped values.

### Distribution and deployment

- `cmd/otelcol-zabbix/main.go`: Collector command entry point and build information.
- `cmd/otelcol-zabbix/components.go`: receiver, processors, exporter, and extension factory maps.
- `cmd/otelcol-zabbix/components_test.go`: exact embedded component inventory.
- `configs/otelcol.yaml`: documented VictoriaMetrics pipeline example.
- `Dockerfile`: multi-stage non-root distribution image.
- `deployments/systemd/otelcol-zabbix.service`: hardened service unit.
- `deployments/systemd/otelcol-zabbix.yaml`: VM Collector configuration.
- `deployments/systemd/otelcol-zabbix.env.example`: credential and endpoint environment template.
- `deployments/kubernetes/*.yaml`: namespace, Secret example, ConfigMap, Deployment, Service, and Kustomize inventory.
- `compose.yaml`: complete Zabbix-to-VictoriaMetrics demo.
- `demo/Dockerfile.tools`: pinned curl/jq utility image for bootstrap and verification jobs.
- `demo/bootstrap.sh`: idempotent Zabbix object/token setup and generated Collector config.
- `demo/producer.sh`: changing trapper values sent to Zabbix.
- `demo/collector.yaml.tmpl`: short-interval receiver pipeline used only in the demo.
- `demo/verify.sh`: VictoriaMetrics query and label assertions.
- `internal/packaging/assets_test.go`: static assertions over service, Kubernetes, Docker, and Compose assets.
- `README.md`, `docs/configuration.md`, `docs/deployment.md`: usage and operational documentation.

---

### Task 1: Go Module and Receiver Configuration Contract

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `receiver/zabbixreceiver/config.go`
- Test: `receiver/zabbixreceiver/config_test.go`

**Interfaces:**
- Produces: `type Config`, `createDefaultConfig() component.Config`, `func (c *Config) Clone() *Config`, `func (c *Config) ResolveEnv(getenv func(string) (string, bool)) error`, and `func (c *Config) Validate() error`. Public `Validate` resolves explicit receiver overrides on a clone and invokes a pure resolved-config validator without mutating its caller.
- Produces exact nested types: `ScheduleConfig`, `JobsConfig`, `JobConfig`, `PromConfig`, `ZabbixConfig`, `LimitsConfig`, and `FiltersConfig`.
- Uses `configopaque.String` for `ZabbixConfig.Token` so Collector config rendering redacts the credential.

- [ ] **Step 1: Initialize the module with pinned Collector APIs**

Run:

```bash
go mod init github.com/aleksandr/zabbix-otel
go get go.opentelemetry.io/collector/component@v1.59.0
go get go.opentelemetry.io/collector/config/configopaque@v1.59.0
go get go.opentelemetry.io/collector/receiver@v1.59.0
go get github.com/stretchr/testify@v1.11.1
```

Expected: `go.mod` declares Go 1.25 or newer and contains the requested module versions.

- [ ] **Step 2: Write failing defaults, environment, and validation tests**

Create table-driven tests with these exact assertions:

```go
func TestCreateDefaultConfig(t *testing.T) {
    cfg := createDefaultConfig().(*Config)
    require.Equal(t, 5*time.Second, cfg.Schedule.Jitter)
    require.Equal(t, JobConfig{Enabled: true, RunOnStart: true, Interval: 5*time.Minute, Timeout: time.Minute}, cfg.Schedule.Jobs.Discover)
    require.Equal(t, JobConfig{Enabled: true, RunOnStart: false, Interval: 30*time.Second, Timeout: 20*time.Second}, cfg.Schedule.Jobs.Values)
    require.Equal(t, "zabbix_", cfg.Prom.Prefix)
    require.Equal(t, 30*time.Second, cfg.Zabbix.Timeout)
    require.Equal(t, LimitsConfig{MaxMetricsPerHost: 1000, ItemsPerRequest: 1000}, cfg.Zabbix.Limits)
}

func TestResolveEnvTakesPrecedence(t *testing.T) {
    cfg := validConfig()
    env := map[string]string{
        "ZABBIX_URL": "https://env.example/api_jsonrpc.php",
        "ZABBIX_TOKEN": "env-secret",
        "ZABBIX_TIMEOUT": "7s",
        "MAX_METRICS_PER_HOST": "12",
        "ZABBIX_ITEMS_PER_REQUEST": "34",
    }
    require.NoError(t, cfg.ResolveEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok }))
    assert.Equal(t, "https://env.example/api_jsonrpc.php", cfg.Zabbix.URL)
    assert.Equal(t, configopaque.String("env-secret"), cfg.Zabbix.Token)
    assert.Equal(t, 7*time.Second, cfg.Zabbix.Timeout)
    assert.Equal(t, 12, cfg.Zabbix.Limits.MaxMetricsPerHost)
    assert.Equal(t, 34, cfg.Zabbix.Limits.ItemsPerRequest)
}

func validConfig() *Config {
    cfg := createDefaultConfig().(*Config)
    cfg.Zabbix.URL = "https://zabbix.example/api_jsonrpc.php"
    cfg.Zabbix.Token = configopaque.String("test-secret")
    return cfg
}
```

Validation cases must assert field-specific errors for missing URL, non-HTTP URL, empty token, negative jitter, enabled job with non-positive interval or timeout, invalid regex, non-positive limits, invalid metric prefix, and both jobs disabled. Environment parsing cases must cover invalid duration and integer strings.

Add `TestCloneDoesNotAliasConstLabels`: mutate the clone's `Prom.ConstLabels` and assert the original map is unchanged.

- [ ] **Step 3: Run the tests and confirm the red state**

Run: `go test ./receiver/zabbixreceiver -run 'Test(CreateDefaultConfig|ResolveEnv|Validate)' -count=1`

Expected: FAIL because the configuration types and functions do not exist.

- [ ] **Step 4: Implement the minimal configuration contract**

Use `mapstructure` tags matching the approved YAML exactly:

```go
type Config struct {
    Schedule ScheduleConfig `mapstructure:"schedule"`
    Prom     PromConfig     `mapstructure:"prom"`
    Zabbix   ZabbixConfig   `mapstructure:"zabbix"`
}

type JobConfig struct {
    Enabled    bool          `mapstructure:"enabled"`
    RunOnStart bool          `mapstructure:"run_on_start"`
    Interval   time.Duration `mapstructure:"interval"`
    Timeout    time.Duration `mapstructure:"timeout"`
}

type PromConfig struct {
    Prefix      string            `mapstructure:"prefix"`
    ConstLabels map[string]string `mapstructure:"const_labels"`
}

type ZabbixConfig struct {
    URL     string              `mapstructure:"url"`
    Token   configopaque.String `mapstructure:"token"`
    Timeout time.Duration       `mapstructure:"timeout"`
    Limits  LimitsConfig        `mapstructure:"limits"`
    Filters FiltersConfig       `mapstructure:"filters"`
}
```

`Clone` copies the struct and deep-copies `Prom.ConstLabels`. `ResolveEnv` must parse with `time.ParseDuration` and `strconv.Atoi`, leave fields unchanged when a variable is unset, and return messages naming the invalid environment variable. Public `Validate` clones, resolves with `os.LookupEnv`, and invokes a pure validator for the resolved clone. Resolved validation must require a positive `zabbix.timeout`, compile all four regex fields, require a final metric name compatible with `[a-zA-Z_:][a-zA-Z0-9_:]*`, and aggregate independent field errors with `errors.Join`.

- [ ] **Step 5: Run focused and package tests**

Run:

```bash
go test ./receiver/zabbixreceiver -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the configuration contract**

```bash
git add go.mod go.sum .gitignore receiver/zabbixreceiver/config.go receiver/zabbixreceiver/config_test.go
git commit -m "feat: define Zabbix receiver configuration"
```

---

### Task 2: Typed Zabbix JSON-RPC Client

**Files:**
- Create: `internal/zabbix/types.go`
- Create: `internal/zabbix/client.go`
- Test: `internal/zabbix/client_test.go`

**Interfaces:**
- Produces:

```go
type API interface {
    Hosts(context.Context) ([]Host, error)
    Items(context.Context, []string) ([]Item, error)
    Values(context.Context, []string) ([]Value, error)
}

type Host struct { ID, Name string }
type Item struct { ID, HostID, Name, Key, ValueType string }
type Value struct { ItemID, LastValue, LastClock string }
type ClientConfig struct { URL, Token string; Timeout time.Duration }
func NewClient(ClientConfig, *http.Client) (*Client, error)
```

- `Items` consumes host IDs; `Values` consumes item IDs. Empty ID slices return empty results without network calls.
- Authentication begins with `Authorization: Bearer`; on a legacy Zabbix authentication error it retries once with the JSON-RPC `auth` property and caches the successful mode. This preserves Zabbix 5.x session-token compatibility without using deprecated auth on modern servers.

- [ ] **Step 1: Write failing request-contract tests**

Use `httptest.Server` to capture requests. Assert:

```go
func TestHostsUsesBearerAndExactFields(t *testing.T) {
    // Server asserts Content-Type application/json-rpc and Authorization Bearer secret.
    // Decode body and assert method=host.get, output=[hostid,host], id is non-zero,
    // and no auth property is present. Return two host objects.
}

func TestItemsRequestsNumericItems(t *testing.T) {
    // Assert item.get params contain output itemid,hostid,name,key_,value_type;
    // hostids are the supplied IDs; filter.value_type is ["0","3"].
}

func TestValuesRequestsLastFields(t *testing.T) {
    // Assert item.get params contain output itemid,lastvalue,lastclock and exact itemids.
}
```

Add cases for empty slices, HTTP 503, malformed JSON, mismatched response ID, JSON-RPC error fields, context cancellation, client timeout, legacy auth retry/cache, and an invalid URL. Error strings must contain operation and safe endpoint but never the token.

- [ ] **Step 2: Run the client tests and confirm failure**

Run: `go test ./internal/zabbix -count=1`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement typed transport and methods**

Define an internal envelope with `json.RawMessage` result and a typed `RPCError`:

```go
type RPCError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    string `json:"data"`
}

func (e *RPCError) Error() string {
    return fmt.Sprintf("zabbix API error %d: %s: %s", e.Code, e.Message, e.Data)
}
```

The request body uses JSON-RPC `2.0`, an atomic numeric ID, the method, params, and only includes `auth` in legacy mode. Limit response bodies to 8 MiB, require HTTP 2xx, close every response body, verify response IDs, and wrap errors as `zabbix <method> at <scheme://host/path>: ...`.

Use these exact API params:

```go
host.get: {"output": ["hostid", "host"], "sortfield": "hostid"}
item.get discovery: {
  "output": ["itemid", "hostid", "name", "key_", "value_type"],
  "hostids": hostIDs,
  "filter": {"value_type": ["0", "3"]},
  "sortfield": ["hostid", "itemid"]
}
item.get values: {
  "output": ["itemid", "lastvalue", "lastclock"],
  "itemids": itemIDs,
  "sortfield": "itemid"
}
```

- [ ] **Step 4: Run tests, race tests, and vet for the client**

Run:

```bash
go test ./internal/zabbix -count=1
go test -race ./internal/zabbix -count=1
go vet ./internal/zabbix
```

Expected: PASS and no token appears in captured error text.

- [ ] **Step 5: Commit the Zabbix client**

```bash
git add internal/zabbix
git commit -m "feat: add typed Zabbix API client"
```

---

### Task 3: Discovery Filters, Limits, and Atomic Snapshots

**Files:**
- Create: `internal/discovery/select.go`
- Create: `internal/discovery/snapshot.go`
- Test: `internal/discovery/select_test.go`
- Test: `internal/discovery/snapshot_test.go`

**Interfaces:**
- Consumes: `zabbix.Host` and `zabbix.Item` from Task 2.
- Produces:

```go
type Filters struct {
    HostInclude, HostExclude, ItemKeyInclude, ItemKeyExclude *regexp.Regexp
}
type ItemMeta struct { ID, HostID, Host, Name, Key, ValueType string }
func Select([]zabbix.Host, []zabbix.Item, Filters, int) (selected []ItemMeta, filtered, limited int)
type Snapshot struct { /* immutable */ }
func NewSnapshot([]ItemMeta) *Snapshot
func (s *Snapshot) Items() []ItemMeta
type Store struct { /* atomic pointer */ }
func (s *Store) Load() *Snapshot
func (s *Store) Replace(*Snapshot)
```

- [ ] **Step 1: Write failing selection tests**

Create table cases that prove:

```go
// Hosts: prod-a, prod-a-backup, dev-a.
// Include prod-.*, then exclude .*-backup => only prod-a.
// Items: system.cpu.util, vm.memory.size, system.log.
// Include system\..*|vm\..*, then exclude .*\.log => CPU and memory.
// max=1 keeps the first Zabbix-sorted item per host and increments limited.
// Items with unknown host IDs are filtered.
```

Snapshot tests must mutate the input slice after `NewSnapshot` and the slice returned from `Items`; neither mutation may affect stored content. A concurrent test runs repeated `Replace` and `Load` calls under the race detector.

- [ ] **Step 2: Run discovery tests and confirm failure**

Run: `go test ./internal/discovery -count=1`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement selection and immutable storage**

Build a host-ID lookup, evaluate non-nil include expressions before excludes, count all removals in `filtered`, and count over-limit items in `limited`. Preserve the input item order. Implement `Store` with `atomic.Pointer[Snapshot]`; clone slices at snapshot construction and access boundaries.

- [ ] **Step 4: Run normal and race tests**

Run:

```bash
go test ./internal/discovery -count=1
go test -race ./internal/discovery -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit discovery behavior**

```bash
git add internal/discovery
git commit -m "feat: add Zabbix discovery snapshots"
```

---

### Task 4: OpenTelemetry Metric Conversion

**Files:**
- Create: `internal/metrics/name.go`
- Create: `internal/metrics/builder.go`
- Test: `internal/metrics/name_test.go`
- Test: `internal/metrics/builder_test.go`

**Interfaces:**
- Consumes: `discovery.ItemMeta` and `zabbix.Value`.
- Produces:

```go
func Name(prefix, itemKey string) (string, error)
type Config struct { Prefix string; ConstLabels map[string]string; ScopeName string }
type Stats struct { Emitted, Invalid, Missing int64 }
func Build([]discovery.ItemMeta, []zabbix.Value, Config) (pmetric.Metrics, Stats)
```

- [ ] **Step 1: Write failing name and conversion tests**

Name table:

```go
{"zabbix_", "system.cpu.util", "zabbix_system_cpu_util"}
{"zabbix_", `vfs.fs.size[/,free]`, "zabbix_vfs_fs_size___free"}
{"", "9bad", "_9bad"}
{"zabbix_", "ends...", "zabbix_ends"}
```

Builder test input contains one float, one unsigned integer, an invalid numeric value, a missing metadata value, and constant labels including a conflicting `host`. Assert that:

- Output has one resource-metrics and one scope-metrics entry.
- The valid items are gauges with double values.
- `lastclock=1700000000` becomes `pcommon.Timestamp(1700000000 * 1_000_000_000)`.
- Description equals the Zabbix item display name.
- `env=production` is present.
- Reserved `host` comes from discovery, not constant labels.
- `Stats` reports valid, invalid, and missing values precisely.

- [ ] **Step 2: Run conversion tests and confirm failure**

Run: `go test ./internal/metrics -count=1`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement name construction and pdata building**

Sanitize the item key rune-by-rune, replacing characters outside `[a-zA-Z0-9_:]` with `_`, trimming trailing underscores, and ensuring the combined first character matches `[a-zA-Z_:]`. Return an error if no valid name remains.

Build one metric per matched item value. Set scope name to the supplied value, metric name/description, gauge data point value and timestamp, then add constant attributes before overwriting the four reserved attributes. Parse values with `strconv.ParseFloat` and timestamps with `strconv.ParseInt`.

- [ ] **Step 4: Run conversion tests and all internal tests**

Run:

```bash
go test ./internal/metrics -count=1
go test ./internal/... -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit conversion**

```bash
git add internal/metrics go.mod go.sum
git commit -m "feat: convert Zabbix values to OTel metrics"
```

---

### Task 5: Dual-Job Scheduler and Receiver Lifecycle

**Files:**
- Create: `receiver/zabbixreceiver/scheduler.go`
- Create: `receiver/zabbixreceiver/receiver.go`
- Test: `receiver/zabbixreceiver/scheduler_test.go`
- Test: `receiver/zabbixreceiver/receiver_test.go`

**Interfaces:**
- Consumes: Task 1 config, `zabbix.API`, `discovery.Store`, `discovery.Select`, `metrics.Build`, and downstream `consumer.Metrics`.
- Produces an unexported receiver implementing `receiver.Metrics`:

```go
type zabbixReceiver struct { /* config, API, next consumer, store, cancellation, wait group */ }
func newReceiver(receiver.Settings, *Config, consumer.Metrics, zabbix.API) (*zabbixReceiver, error)
func (r *zabbixReceiver) Start(context.Context, component.Host) error
func (r *zabbixReceiver) Shutdown(context.Context) error
func (r *zabbixReceiver) discover(context.Context) error
func (r *zabbixReceiver) values(context.Context) error
```

- [ ] **Step 1: Write failing pure scheduler tests**

Extract injectable functions:

```go
type timerFunc func(context.Context, time.Duration) error
type jitterFunc func(time.Duration) time.Duration
func runJob(context.Context, JobConfig, time.Duration, timerFunc, jitterFunc, func(context.Context) error, func(error))
```

Tests must prove the delay sequence:

- `run_on_start=true`: jitter, run, interval, jitter, run.
- `run_on_start=false`: interval, jitter, run.
- Every run receives its own timeout context.
- Cancellation stops before another run.
- A blocked run cannot overlap itself.
- Errors reach the reporting callback and do not stop later cycles.

- [ ] **Step 2: Write failing receiver-operation tests**

Use a fake `zabbix.API` and recording `consumer.Metrics`. Cover:

```go
func TestDiscoverReplacesSnapshotOnlyAfterSuccess(t *testing.T)
func TestDiscoverRetainsSnapshotOnItemFailure(t *testing.T)
func TestValuesNoSnapshotIsNoOp(t *testing.T)
func TestValuesChunksRequestsAndConsumesOneBatch(t *testing.T)
func TestValuesAbortsBatchWhenAChunkFails(t *testing.T)
func TestValuesReturnsConsumerError(t *testing.T)
func TestStartAndShutdownStopAllJobs(t *testing.T)
```

For chunking, configure `items_per_request=2`, seed five items, and assert request sizes `2,2,1` plus one downstream batch containing five points.

- [ ] **Step 3: Run receiver tests and confirm failure**

Run: `go test ./receiver/zabbixreceiver -run 'Test(Discover|Values|Start|RunJob)' -count=1`

Expected: FAIL because lifecycle and scheduler functions do not exist.

- [ ] **Step 4: Implement scheduler and receiver operations**

`Start` derives one cancellable lifetime context, starts only enabled loops, and records two wait-group entries. `Shutdown` cancels once and returns either when the wait group finishes or when the caller context expires.

`discover` calls `Hosts`, collects their IDs, calls `Items`, compiles configured filters once during construction, selects metadata, and replaces the store only after both calls succeed.

`values` clones snapshot items, loops over exact chunks, accumulates all returned values, aborts on any request error, builds metrics, skips downstream delivery when no valid points exist, and otherwise calls `ConsumeMetrics` once.

- [ ] **Step 5: Run receiver, race, and repository tests**

Run:

```bash
go test ./receiver/zabbixreceiver -count=1
go test -race ./receiver/zabbixreceiver -count=1
go test ./... -count=1
```

Expected: PASS with no goroutine leak or race report.

- [ ] **Step 6: Commit receiver runtime**

```bash
git add receiver/zabbixreceiver
git commit -m "feat: run Zabbix discovery and values jobs"
```

---

### Task 6: Collector Factory, Self-Telemetry, and HTTP-Fixture Integration

**Files:**
- Create: `receiver/zabbixreceiver/factory.go`
- Create: `receiver/zabbixreceiver/telemetry.go`
- Test: `receiver/zabbixreceiver/factory_test.go`
- Test: `receiver/zabbixreceiver/telemetry_test.go`
- Modify: `receiver/zabbixreceiver/receiver.go`
- Modify: `receiver/zabbixreceiver/receiver_test.go`

**Interfaces:**
- Produces: `func NewFactory() receiver.Factory` for component type `zabbix` with development metrics stability.
- Produces unexported telemetry instruments named:
  - `otelcol_receiver_zabbix_discover_attempts`
  - `otelcol_receiver_zabbix_discover_errors`
  - `otelcol_receiver_zabbix_discover_duration`
  - `otelcol_receiver_zabbix_values_attempts`
  - `otelcol_receiver_zabbix_values_errors`
  - `otelcol_receiver_zabbix_values_duration`
  - `otelcol_receiver_zabbix_emitted_points`
  - `otelcol_receiver_zabbix_invalid_values`
  - `otelcol_receiver_zabbix_filtered_items`
  - `otelcol_receiver_zabbix_limited_items`

This exact ten-instrument list is the authoritative telemetry acceptance surface. Attempts and durations cover every respective cycle; errors cover cycles returning errors; emitted points count valid built gauges before the consumer returns; invalid values count malformed numbers, invalid timestamps, and unusable metric names; filtered and limited counts follow discovery selection. Do not infer separate success, host-count, item-count, requested-item, or downstream-failure instruments.

- [ ] **Step 1: Write failing factory and telemetry tests**

Assert `NewFactory().Type().String() == "zabbix"`, default config equality, and successful creation with `receivertest.NewNopSettings`. Use the SDK manual reader to assert one discover attempt, one emitted-point count, and one duration sample after recording.

- [ ] **Step 2: Write the in-process integration test**

Start one `httptest.Server` that responds to `host.get` and the two forms of `item.get`. Construct the receiver through the factory with 5 ms job intervals, use a recording consumer, start it, and require an emitted metric within two seconds. Assert exact name, value, timestamp, and attributes, then shut down and assert the request count stops changing.

- [ ] **Step 3: Run integration tests and confirm failure**

Run: `go test ./receiver/zabbixreceiver -run 'Test(NewFactory|Telemetry|ReceiverHTTPIntegration)' -count=1`

Expected: FAIL because the factory and telemetry do not exist.

- [ ] **Step 4: Implement factory and telemetry wiring**

Factory construction must type-check `*Config`, clone before environment resolution, call `ResolveEnv(os.LookupEnv)`, invoke only the pure resolved-config validator, build the HTTP client, initialize telemetry, and call `newReceiver`. Receiver operations record attempts and durations once per cycle, error counters on returned errors, discovery selection counters, and conversion counters.

- [ ] **Step 5: Run all Go quality checks**

Run:

```bash
gofmt -w receiver internal
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
```

Expected: all commands pass.

- [ ] **Step 6: Commit the public Collector component**

```bash
git add receiver/zabbixreceiver go.mod go.sum
git commit -m "feat: expose Zabbix Collector receiver"
```

---

### Task 7: Custom Collector Distribution and Sample Pipeline

**Files:**
- Create: `cmd/otelcol-zabbix/main.go`
- Create: `cmd/otelcol-zabbix/components.go`
- Test: `cmd/otelcol-zabbix/components_test.go`
- Create: `configs/otelcol.yaml`
- Create: `Makefile`

**Interfaces:**
- Consumes: `zabbixreceiver.NewFactory()`.
- Embeds the standard `batch`, `memory_limiter`, `prometheusremotewrite`, and `health_check` factories at v0.153.0.
- Produces binary `bin/otelcol-zabbix`.

- [ ] **Step 1: Add distribution dependencies and failing inventory test**

Run:

```bash
go get go.opentelemetry.io/collector/otelcol@v0.153.0
go get go.opentelemetry.io/collector/processor/batchprocessor@v0.153.0
go get go.opentelemetry.io/collector/processor/memorylimiterprocessor@v0.153.0
go get github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter@v0.153.0
go get github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension@v0.153.0
```

Write `TestComponents` to call `components()` and assert exact map keys: receivers `zabbix`; processors `batch,memory_limiter`; exporters `prometheusremotewrite`; extensions `health_check`.

- [ ] **Step 2: Run the inventory test and confirm failure**

Run: `go test ./cmd/otelcol-zabbix -count=1`

Expected: FAIL because `components()` does not exist.

- [ ] **Step 3: Implement the Collector command**

Build factory maps with the Collector `MakeFactoryMap` helpers. `main.go` calls `otelcol.NewCommand` with build info command `otelcol-zabbix`, description `OpenTelemetry Collector with Zabbix receiver`, version injected from `-ldflags`, and `components` as the factory callback. Exit non-zero on command failure.

- [ ] **Step 4: Add the production sample configuration**

`configs/otelcol.yaml` must configure:

```yaml
extensions:
  health_check:
    endpoint: 0.0.0.0:13133
receivers:
  zabbix:
    schedule:
      jitter: 5s
      jobs:
        discover: {enabled: true, run_on_start: true, interval: 5m, timeout: 60s}
        values: {enabled: true, run_on_start: false, interval: 30s, timeout: 20s}
    prom:
      prefix: zabbix_
      const_labels: {env: production, instance: zabbix-01}
    zabbix:
      url: ${env:ZABBIX_URL}
      token: ${env:ZABBIX_TOKEN}
      timeout: 30s
      limits: {max_metrics_per_host: 1000, items_per_request: 1000}
      filters: {host_include_regex: "", host_exclude_regex: "", item_key_include_regex: "", item_key_exclude_regex: ""}
processors:
  memory_limiter: {check_interval: 1s, limit_mib: 256}
  batch: {}
exporters:
  prometheusremotewrite:
    endpoint: ${env:VICTORIAMETRICS_REMOTE_WRITE_URL}
service:
  extensions: [health_check]
  pipelines:
    metrics:
      receivers: [zabbix]
      processors: [memory_limiter, batch]
      exporters: [prometheusremotewrite]
```

- [ ] **Step 5: Add repeatable Make targets**

Create targets `fmt`, `test`, `test-race`, `vet`, `build`, `validate-config`, `docker-build`, `compose-config`, `demo-up`, `demo-verify`, and `demo-down`. `build` writes `bin/otelcol-zabbix`; `validate-config` supplies non-secret dummy environment values and invokes `bin/otelcol-zabbix validate --config configs/otelcol.yaml`.

- [ ] **Step 6: Build and validate the distribution**

Run:

```bash
make fmt
make test
make vet
make build
make validate-config
bin/otelcol-zabbix components
```

Expected: build and validation pass; component output contains `zabbix` and `prometheusremotewrite`.

- [ ] **Step 7: Commit the distribution**

```bash
git add cmd configs Makefile go.mod go.sum
git commit -m "feat: build custom Zabbix Collector distribution"
```

---

### Task 8: Container, systemd, and Kubernetes Packaging

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`
- Create: `deployments/systemd/otelcol-zabbix.service`
- Create: `deployments/systemd/otelcol-zabbix.yaml`
- Create: `deployments/systemd/otelcol-zabbix.env.example`
- Create: `deployments/kubernetes/namespace.yaml`
- Create: `deployments/kubernetes/secret.example.yaml`
- Create: `deployments/kubernetes/configmap.yaml`
- Create: `deployments/kubernetes/deployment.yaml`
- Create: `deployments/kubernetes/service.yaml`
- Create: `deployments/kubernetes/kustomization.yaml`
- Test: `internal/packaging/assets_test.go`

**Interfaces:**
- Produces OCI image `zabbix-otel-collector:local` with entrypoint `/otelcol-zabbix` and default config `/etc/otelcol-zabbix/config.yaml`.
- Produces systemd service user/group `otelcol-zabbix` and Kubernetes workload `otelcol-zabbix` in namespace `observability`.

- [ ] **Step 1: Write failing static asset tests**

Tests read repository-root files and assert:

- Dockerfile has a Go build stage, numeric non-root `USER 10001:10001`, and no token value.
- systemd unit has `User=otelcol-zabbix`, `EnvironmentFile=/etc/otelcol-zabbix/otelcol-zabbix.env`, `Restart=on-failure`, `NoNewPrivileges=true`, `ProtectSystem=strict`, and exact `ExecStart`.
- Kubernetes Deployment has `runAsNonRoot: true`, `readOnlyRootFilesystem: true`, dropped capabilities, health probes on 13133, resource requests/limits, Secret references, and no RBAC objects.
- Kustomization lists namespace, Secret example, ConfigMap, Deployment, and Service.

- [ ] **Step 2: Run packaging tests and confirm failure**

Run: `go test ./internal/packaging -count=1`

Expected: FAIL because assets do not exist.

- [ ] **Step 3: Implement the non-root container**

Use `golang:1.25-alpine` as builder and `gcr.io/distroless/static-debian13:nonroot` as runtime. Build with `CGO_ENABLED=0`, `-trimpath`, and stripped linker flags. Copy the binary plus `configs/otelcol.yaml`; run as numeric UID/GID 10001 and expose 13133 and 8888.

- [ ] **Step 4: Implement VM deployment assets**

The unit reads `/etc/otelcol-zabbix/otelcol-zabbix.env`, executes `/usr/local/bin/otelcol-zabbix --config=/etc/otelcol-zabbix/config.yaml`, uses a dedicated state directory, and applies the hardening asserted by tests. The environment template defines `ZABBIX_URL`, `ZABBIX_TOKEN`, and `VICTORIAMETRICS_REMOTE_WRITE_URL` with safe example values.

- [ ] **Step 5: Implement Kubernetes assets**

The Deployment has one replica, rolling updates, the custom image, environment values from Secret/ConfigMap, config volume, port 13133, HTTP `/` probes, 100m/128Mi requests, 500m/512Mi limits, and 30-second termination grace. The Service is `ClusterIP` and exposes only health and internal telemetry. No ClusterRole, Role, or service-account token mount is needed.

- [ ] **Step 6: Validate packaging**

Run:

```bash
go test ./internal/packaging -count=1
docker build -t zabbix-otel-collector:local .
kubectl kustomize deployments/kubernetes >/tmp/zabbix-otel-rendered.yaml
systemd-analyze verify deployments/systemd/otelcol-zabbix.service
```

Expected: tests and available validators pass; Docker image reports user `10001:10001`. If `systemd-analyze` is unavailable on the development OS, run it inside a current Debian systemd container and record that command in verification notes.

- [ ] **Step 7: Commit deployment packaging**

```bash
git add Dockerfile .dockerignore deployments internal/packaging
git commit -m "feat: package Collector for VM and Kubernetes"
```

---

### Task 9: Real Zabbix-to-VictoriaMetrics Docker Compose Demo

**Files:**
- Create: `compose.yaml`
- Create: `demo/Dockerfile.tools`
- Create: `demo/bootstrap.sh`
- Create: `demo/producer.sh`
- Create: `demo/collector.yaml.tmpl`
- Create: `demo/verify.sh`
- Modify: `internal/packaging/assets_test.go`

**Interfaces:**
- Uses Zabbix 7.4.12 PostgreSQL server/web images, PostgreSQL 17 Alpine, and VictoriaMetrics v1.148.0.
- Produces metric `zabbix_demo_counter` with `host="otel-demo-host"`, `item_key="demo.counter"`, `env="compose"`, and changing numeric values.

- [ ] **Step 1: Extend failing packaging tests for Compose topology**

Parse `compose.yaml` as YAML and assert services `postgres`, `zabbix-server`, `zabbix-web`, `bootstrap`, `producer`, `otelcol-zabbix`, and `victoriametrics`; exact pinned Zabbix and VictoriaMetrics tags; health/dependency conditions; a private network; and a shared `demo-config` volume used only by bootstrap and Collector.

- [ ] **Step 2: Write the bootstrap script with idempotent API helpers**

Implement POSIX shell functions `rpc_unauthenticated`, `rpc_session`, and `rpc_bearer` using `curl --fail-with-body` and `jq -e`. The script must:

1. Poll `apiinfo.version` until ready.
2. Log in as `Admin` with the Compose-only password.
3. Query or create host group `OpenTelemetry Demo`.
4. Query or create host `otel-demo-host`.
5. Query or create trapper item `demo.counter` with float value type.
6. Delete any prior token named `otel-demo-receiver`, then create and generate a token for the current user.
7. Render `demo/collector.yaml.tmpl` to `/generated/otelcol.yaml` with the generated token embedded only in that runtime volume file. Run bootstrap as root solely to set ownership to `10001:10001` and mode `0400`, allowing the non-root Collector to read it without exposing it through Compose interpolation or tracked files.
8. Write `/generated/ready` last.

Every API response must be checked for `.error`; token output must never be printed.

- [ ] **Step 3: Write producer and verification scripts**

`producer.sh` waits for Zabbix server port 10051 and sends one increasing value every five seconds:

```sh
value=1
while :; do
  zabbix_sender -z zabbix-server -s otel-demo-host -k demo.counter -o "$value"
  value=$((value + 1))
  sleep 5
done
```

`verify.sh` polls for up to 180 seconds:

```text
GET http://victoriametrics:8428/api/v1/query?query=zabbix_demo_counter{host="otel-demo-host",env="compose"}
```

It succeeds only when `.status == "success"`, exactly one result exists, `item_key == "demo.counter"`, `hostid` and `itemid` are non-empty, the sample value parses as a positive number, and the sample timestamp is at or after the verifier start. It ignores matching stale samples and prints only the accepted fresh metric JSON, never credentials.

- [ ] **Step 4: Implement the Compose topology and demo receiver config**

Use a short demo schedule: zero jitter, discovery on start every 15 seconds, values on start every 5 seconds, and request limits 100. Configure the Collector health extension and `prometheusremotewrite` endpoint `http://victoriametrics:8428/api/v1/write`.

Build `demo/Dockerfile.tools` from `alpine:3.24.1` with only `curl`, `jq`, and CA certificates. Use this local tools image for bootstrap and verification; use the official Zabbix agent image for `zabbix_sender` in the producer.

Pin images to:

```yaml
postgres:17-alpine
zabbix/zabbix-server-pgsql:alpine-7.4.12
zabbix/zabbix-web-nginx-pgsql:alpine-7.4.12
zabbix/zabbix-agent2:alpine-7.4.12
victoriametrics/victoria-metrics:v1.148.0
```

Build the Collector service from the local Dockerfile. The Collector depends on successful bootstrap, producer depends on successful bootstrap plus healthy Zabbix server, and verifier runs under the `verify` Compose profile.

- [ ] **Step 5: Validate syntax and static contract**

Run:

```bash
go test ./internal/packaging -count=1
docker compose config --quiet
shellcheck demo/*.sh
```

Expected: all checks pass and expanded Compose output contains no receiver token.

- [ ] **Step 6: Run the real end-to-end demo**

Run:

```bash
docker compose up -d --build postgres zabbix-server zabbix-web bootstrap producer victoriametrics otelcol-zabbix
docker compose --profile verify run --rm verify
docker compose logs --no-color otelcol-zabbix
```

Expected: verification prints one `zabbix_demo_counter` series with all required labels; Collector logs show successful discovery and values cycles without authentication errors.

- [ ] **Step 7: Tear down the demo and commit it**

Run: `docker compose down --volumes --remove-orphans`

Then commit:

```bash
git add compose.yaml demo internal/packaging/assets_test.go
git commit -m "feat: demonstrate Zabbix to VictoriaMetrics flow"
```

---

### Task 10: Documentation, CI, and Final Verification

**Files:**
- Create: `README.md`
- Create: `docs/configuration.md`
- Create: `docs/deployment.md`
- Create: `.github/workflows/ci.yml`
- Modify: `Makefile`

**Interfaces:**
- Documents the exact public YAML and environment contract, build/run commands, supported Zabbix authentication behavior, metric mapping, operational failure behavior, and all four deployment paths.
- CI executes formatting, tests, race tests, vet, build, config validation, packaging tests, and Compose config validation.

- [ ] **Step 1: Write a failing documentation-contract test**

Extend `internal/packaging/assets_test.go` to require README sections `Architecture`, `Quick start`, `Configuration`, `Metric mapping`, `Docker Compose demo`, `VM/systemd`, `Kubernetes`, `Testing`, and `Security`; require the approved-design link and verify every supported environment variable appears in `docs/configuration.md`.

- [ ] **Step 2: Run the documentation test and confirm failure**

Run: `go test ./internal/packaging -run TestDocumentationContract -count=1`

Expected: FAIL because documentation files do not exist.

- [ ] **Step 3: Write user and operator documentation**

README quick start must contain exact commands:

```bash
make build
ZABBIX_URL=http://zabbix.example/api_jsonrpc.php \
ZABBIX_TOKEN=replace-me \
VICTORIAMETRICS_REMOTE_WRITE_URL=http://victoriametrics:8428/api/v1/write \
./bin/otelcol-zabbix --config configs/otelcol.yaml
```

Document that modern Zabbix uses Bearer authorization and legacy Zabbix 5.x can use a `user.login` session token through automatic legacy fallback. Document that the fallback is cached, credentials are never logged, and long-lived API tokens require the Zabbix version that supports them.

Document metric name construction, gauge conversion, point timestamp, reserved-label precedence, filter order, discovery snapshot retention, partial-batch policy, and environment precedence. Include systemd installation commands and `kubectl apply -k deployments/kubernetes` with explicit Secret editing instructions.

- [ ] **Step 4: Add CI workflow**

Configure GitHub Actions on pushes and pull requests with Go 1.25, dependency cache, `make fmt` plus clean-diff assertion, `make test`, `make test-race`, `make vet`, `make build`, `make validate-config`, packaging tests, `docker compose config --quiet`, and Docker image build. Do not run the multi-container demo on every push; expose it as a manual `workflow_dispatch` job with a 15-minute timeout and guaranteed `docker compose down --volumes` cleanup.

- [ ] **Step 5: Run the full verification matrix from a clean process state**

Run:

```bash
make fmt
git diff --check
make test
make test-race
make vet
make build
make validate-config
make compose-config
docker build -t zabbix-otel-collector:verify .
docker run --rm zabbix-otel-collector:verify components
```

Expected: every command exits zero; container component output lists the receiver and expected pipeline components.

- [ ] **Step 6: Run final Compose verification**

Run:

```bash
make demo-up
make demo-verify
make demo-down
```

Expected: VictoriaMetrics returns the real Zabbix trapper value with `host`, `hostid`, `item_key`, `itemid`, and `env=compose`.

- [ ] **Step 7: Review secrets and repository state**

Run:

```bash
rg -n --hidden --glob '!.git/**' '(ZABBIX_TOKEN=.{8,}|Bearer [A-Za-z0-9]{16,}|Admin.*zabbix)' .
git status --short
git log --oneline --decorate -12
```

Expected: matches contain only documented placeholders or Compose-only bootstrap credentials; no generated token is tracked; working tree contains only intentional plan checkbox updates if those are being recorded.

- [ ] **Step 8: Commit documentation and CI**

```bash
git add README.md docs/configuration.md docs/deployment.md .github/workflows/ci.yml Makefile internal/packaging/assets_test.go
git commit -m "docs: document and verify Zabbix receiver"
```

- [ ] **Step 9: Perform completion review**

Compare every completion criterion in `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md` to fresh command output. Record any unavailable host-specific validator and its containerized equivalent in the final handoff; do not claim end-to-end completion unless `make demo-verify` has observed the metric in VictoriaMetrics.
