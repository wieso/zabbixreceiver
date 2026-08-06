# Zabbix OpenTelemetry Receiver Design

Date: 2026-08-05

## Objective

Build a native OpenTelemetry Collector metrics receiver in Go that polls numeric item values from Zabbix and passes OpenTelemetry metrics to the next component in a Collector pipeline. Provide a custom Collector distribution that can forward those metrics to VictoriaMetrics through the standard Prometheus remote-write exporter.

The component must be deployable as a container, a systemd-managed service on a Linux VM, and a Kubernetes workload. A Docker Compose environment must demonstrate the full data path using a real Zabbix installation and VictoriaMetrics.

## Compatibility Boundary

The receiver supports this public configuration interface:

- `schedule.jitter`
- `schedule.jobs.discover`
- `schedule.jobs.values`
- `prom.prefix`
- `prom.const_labels`
- `zabbix.url`
- `zabbix.token`
- `zabbix.timeout`
- `zabbix.limits`
- `zabbix.filters`
- The documented Zabbix-related environment overrides

Process-level exporter settings are intentionally excluded because the OpenTelemetry Collector owns them:

- `base.address`
- `/metrics` and `/health` exporter endpoints
- Standalone exporter CLI flags
- Agent-specific exporter configuration

Collector endpoint, health, telemetry, and command-line behavior remain standard Collector concerns.

## Architecture

### OpenTelemetry component

`receiver/zabbixreceiver` is the public Collector component. It provides:

- `NewFactory()` for registration in a Collector distribution
- Default configuration matching the documented receiver defaults
- Configuration decoding and validation
- Receiver start and shutdown lifecycle
- Delivery of `pmetric.Metrics` to the configured downstream `consumer.Metrics`

### Zabbix client

`receiver/zabbixreceiver/internal/zabbix` implements a typed JSON-RPC client for the Zabbix API. It is responsible for:

- API-token authentication
- `host.get` and `item.get` calls
- Request and response encoding
- Zabbix JSON-RPC error handling
- Per-request timeouts
- Chunked value requests

The receiver targets Zabbix 5.0 or later, subject to API-token availability in the deployed Zabbix version. The configured credential is always treated as sensitive and must not be logged.

### Discovery and snapshot management

`receiver/zabbixreceiver/internal/discovery` obtains hosts and numeric items, applies filters and limits, then creates an immutable metadata snapshot. The live snapshot is replaced atomically only after a complete successful discovery. A failed refresh leaves the previous usable snapshot in place.

### Metric conversion

`receiver/zabbixreceiver/internal/metrics` converts values into OpenTelemetry gauge points. It owns metric-name normalization, numeric parsing, attributes, descriptions, and timestamps.

### Custom Collector distribution

`cmd/otelcol-zabbix` builds a Collector binary containing:

- The Zabbix receiver
- Prometheus remote-write exporter
- Batch processor
- Memory-limiter processor
- Health-check extension
- Standard Collector telemetry support

The receiver has no direct dependency on VictoriaMetrics. VictoriaMetrics is a deployment-level destination selected with the standard Collector exporter.

## Configuration

The receiver is configured as follows:

```yaml
receivers:
  zabbix:
    schedule:
      jitter: 5s
      jobs:
        discover:
          enabled: true
          run_on_start: true
          interval: 5m
          timeout: 60s
        values:
          enabled: true
          run_on_start: false
          interval: 30s
          timeout: 20s
    prom:
      prefix: zabbix_
      const_labels:
        env: production
        instance: zabbix-01
    zabbix:
      url: http://zabbix-web:8080/api_jsonrpc.php
      token: ${env:ZABBIX_TOKEN}
      timeout: 30s
      limits:
        max_metrics_per_host: 1000
        items_per_request: 1000
      filters:
        host_include_regex: ""
        host_exclude_regex: ""
        item_key_include_regex: ""
        item_key_exclude_regex: ""
```

The receiver defaults are:

| Setting | Default |
| --- | --- |
| `schedule.jitter` | `5s` |
| `discover.enabled` | `true` |
| `discover.run_on_start` | `true` |
| `discover.interval` | `5m` |
| `discover.timeout` | `60s` |
| `values.enabled` | `true` |
| `values.run_on_start` | `false` |
| `values.interval` | `30s` |
| `values.timeout` | `20s` |
| `prom.prefix` | `zabbix_` |
| `zabbix.timeout` | `30s` |
| `max_metrics_per_host` | `1000` |
| `items_per_request` | `1000` |

The receiver honors these documented environment overrides when they are present:

| Environment variable | Receiver setting |
| --- | --- |
| `ZABBIX_URL` | `zabbix.url` |
| `ZABBIX_TOKEN` | `zabbix.token` |
| `ZABBIX_TIMEOUT` | `zabbix.timeout` |
| `MAX_METRICS_PER_HOST` | `zabbix.limits.max_metrics_per_host` |
| `ZABBIX_ITEMS_PER_REQUEST` | `zabbix.limits.items_per_request` |

An explicit environment override takes precedence over the decoded YAML value, matching the public receiver interface. Standard Collector environment interpolation remains supported as well. Public configuration validation clones the decoded configuration, resolves these overrides, and validates the clone without mutating its caller; the factory independently resolves its own clone and invokes the pure resolved-config validator before construction. Consequently, environment-only URL/token values and valid environment replacements for invalid decoded values participate in Collector recursive validation as well as factory construction.

Validation rejects:

- A missing or invalid Zabbix URL
- An empty API token after environment overrides
- A non-positive Zabbix per-request timeout after environment overrides
- Non-positive enabled-job intervals or timeouts
- Negative jitter
- Non-positive request or per-host limits
- Invalid regular expressions
- A metric prefix that cannot produce valid Prometheus-compatible metric names
- Configurations with both jobs disabled

## Runtime Data Flow

### Discover job

1. Apply a random delay in the range from zero through `schedule.jitter`.
2. Call `host.get` to obtain available host IDs and names.
3. Call `item.get` for numeric item types only: float (`value_type=0`) and unsigned integer (`value_type=3`).
4. Apply non-empty host and item-key include expressions.
5. Apply host and item-key exclude expressions after the include expressions.
6. Enforce `max_metrics_per_host` deterministically in the item order returned by Zabbix.
7. Build and atomically publish a new immutable metadata snapshot.

The first discover invocation occurs at startup when `run_on_start` is true. Otherwise, it occurs after one interval. The job never overlaps itself.

### Values job

1. Apply bounded positive jitter.
2. Read one stable discovery snapshot.
3. Return successfully without emitting data if no snapshot is available or the snapshot is empty.
4. Split item IDs into chunks no larger than `items_per_request`.
5. Call `item.get` for each chunk to obtain `lastvalue` and `lastclock`.
6. Ignore missing values and report malformed numeric values through receiver self-telemetry without failing the entire batch.
7. Convert valid values to OpenTelemetry metrics.
8. Deliver one metrics batch to the next consumer.

The first values invocation follows `run_on_start`; it may safely run before discovery. The job never overlaps itself.

Each job invocation uses the job timeout as its outer deadline. Individual Zabbix calls are additionally capped by `zabbix.timeout`; the shorter active deadline wins.

## Metric Model

Each Zabbix item becomes an OpenTelemetry gauge data point with a double value. Unsigned integer values are represented as doubles because Prometheus remote write uses floating-point samples.

Metric names follow `{prefix}{sanitized_item_key}`:

- Characters outside the Prometheus metric-name character set are replaced with `_`.
- Repeated invalid characters may produce repeated underscores; this preserves direct replacement semantics.
- Trailing underscores are removed.
- An invalid leading character is replaced or prefixed with `_` as required.

Each point includes these attributes:

| Attribute | Source |
| --- | --- |
| `host` | Zabbix host name |
| `hostid` | Zabbix host ID |
| `item_key` | Zabbix item key |
| `itemid` | Zabbix item ID |
| Configured constant labels | `prom.const_labels` |

Automatic attributes take precedence if a constant label uses one of the four reserved names. This prevents configuration from changing the identity metadata defined by the compatibility contract.

The point timestamp is parsed from Zabbix `lastclock`. The Zabbix item display name is used as the metric description. Items that sanitize to the same metric name remain distinguishable by `item_key` and `itemid` attributes.

## Concurrency and Lifecycle

- Receiver startup validates configuration, constructs the client, and starts enabled job loops.
- Discovery snapshots use atomic replacement, allowing values collection to proceed without holding a lock across network calls.
- Each job is serialized independently.
- Collector cancellation propagates into scheduler waits and Zabbix requests.
- Shutdown cancels both loops and waits for them to finish within the Collector-provided context.
- No goroutines remain after successful shutdown.

## Error Handling and Observability

Configuration errors fail Collector startup with field-specific messages.

Runtime failures are recoverable:

- Discovery errors retain the last good snapshot.
- A failed value request prevents emission of a partial batch for that cycle, except that individual malformed values are skipped.
- Downstream consumer errors are logged and counted; the next scheduled collection still runs.
- Authentication failures are never retried within the same cycle.
- Network and HTTP errors include safe endpoint and operation context but never credentials or full request bodies.

The final human adjudication makes the precise Task 6 list implemented in `telemetry.go`, covered by tests, and published in the configuration documentation authoritative. The authoritative acceptance surface is exactly these ten instruments; the older broad concepts of separate success, host-count, item-count, requested-item, or downstream-failure instruments are not additional requirements:

| Instrument | Semantics |
| --- | --- |
| `otelcol_receiver_zabbix_discover_attempts` | Counter incremented at the start of every discovery cycle. |
| `otelcol_receiver_zabbix_discover_errors` | Counter incremented when a discovery cycle returns an error. |
| `otelcol_receiver_zabbix_discover_duration` | Histogram recording every discovery cycle's duration in seconds, including errors. |
| `otelcol_receiver_zabbix_values_attempts` | Counter incremented at the start of every values cycle. |
| `otelcol_receiver_zabbix_values_errors` | Counter incremented when a values cycle returns an error, including a downstream consumer error. |
| `otelcol_receiver_zabbix_values_duration` | Histogram recording every values cycle's duration in seconds, including errors. |
| `otelcol_receiver_zabbix_emitted_points` | Counter increased by valid gauge points built from retrieved values before the downstream consumer returns. |
| `otelcol_receiver_zabbix_invalid_values` | Counter increased by retrieved values skipped for malformed numbers, invalid timestamps, or unusable metric names. |
| `otelcol_receiver_zabbix_filtered_items` | Counter increased by discovery items excluded for an unknown host or a configured host/item filter. |
| `otelcol_receiver_zabbix_limited_items` | Counter increased by otherwise selected discovery items excluded by the per-host limit. |

## Deployment

### Container

A multi-stage Dockerfile builds a statically linked Linux binary. The runtime image:

- Uses a non-root user
- Contains only the binary and required trust roots
- Exposes the Collector health and telemetry ports used by sample configuration
- Defines no embedded credentials

### systemd

VM assets include:

- A sample Collector YAML configuration
- An environment-file template for `ZABBIX_TOKEN`
- A hardened systemd unit
- Installation and verification instructions

The service runs as a dedicated unprivileged account, restarts on failure, loads credentials from an administrator-readable environment file, and uses the Collector health extension for operational checks.

### Kubernetes

Kubernetes assets include:

- Namespace
- Secret example for the Zabbix token
- ConfigMap for Collector configuration
- Deployment
- Service for health and telemetry endpoints
- Readiness and liveness probes
- Non-root security context
- Resource requests and limits

The receiver does not require Kubernetes API permissions, so no Role or RoleBinding is created.

### Docker Compose demonstration

The demonstration contains:

- PostgreSQL for Zabbix
- Zabbix server
- Zabbix web/API
- A bootstrap job
- A metric producer
- The custom OpenTelemetry Collector
- VictoriaMetrics

The bootstrap job waits for the Zabbix API, creates a monitored host and trapper item, creates and generates an API token, and writes a generated Collector configuration into a shared initialization volume. The Collector starts only after bootstrap succeeds. The producer regularly sends changing numeric values into the Zabbix trapper item.

A verification script records its start epoch, waits for the pipeline, queries the VictoriaMetrics query API for the expected `zabbix_` metric, and checks the automatic attributes and configured constant labels. It accepts only a positive sample whose sample timestamp is at or after the verifier start, so persisted stale data cannot prove the current run. This proves the path:

```text
Zabbix item -> Zabbix API -> zabbix receiver -> OpenTelemetry metrics pipeline -> Prometheus remote write -> VictoriaMetrics
```

## Testing Strategy

### Unit tests

- Default configuration and all validation branches
- Environment-override precedence and invalid override values
- JSON-RPC request structure, token handling, response decoding, API errors, HTTP errors, and secret redaction
- Include/exclude filter precedence
- Deterministic per-host limiting
- Immutable snapshot replacement and retention after discovery failure
- Request chunking
- Metric-name sanitization, reserved labels, descriptions, numeric parsing, and timestamps
- Job startup semantics, intervals, jitter bounds, timeout propagation, non-overlap, and shutdown
- Consumer errors and recovery on the next cycle

### Integration tests

An in-process HTTP fixture implements the required Zabbix JSON-RPC methods. Tests start the real receiver with an OpenTelemetry consuming sink and verify emitted `pmetric.Metrics`, multiple discovery cycles, failed refresh retention, chunking, and cancellation.

### Build and packaging checks

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- Collector binary smoke test with configuration validation
- Container image build
- Static checks for deployment manifests
- Docker Compose configuration validation
- End-to-end Compose verification when Docker is available

## Completion Criteria

The work is complete when:

1. The receiver builds into the supplied custom Collector distribution.
2. The supported settings and defaults decode, validate, and behave as specified.
3. Numeric Zabbix items produce correctly named and attributed OpenTelemetry gauge points.
4. The sample Collector pipeline writes those points to VictoriaMetrics.
5. Unit and integration tests pass, including the race detector.
6. Container, systemd, Kubernetes, and Docker Compose assets are documented and validated.
7. The Compose verification observes the seeded Zabbix metric in VictoriaMetrics.
