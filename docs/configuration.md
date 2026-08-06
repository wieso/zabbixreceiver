# Receiver configuration

The public Collector receiver type is `zabbix`. This document describes the implemented receiver contract; the project's [approved design](superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md) defines its compatibility boundary.

## Complete YAML schema

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
      url: ${env:ZABBIX_URL}
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

`base.address`, standalone `/metrics` or `/health` endpoints, standalone exporter flags, and agent-specific exporter settings are not supported receiver keys. The OpenTelemetry Collector owns those concerns.

## Settings and defaults

| YAML setting | Default | Behavior |
| --- | --- | --- |
| `schedule.jitter` | `5s` | Random delay from zero through the configured duration before each run. Negative values are invalid. |
| `schedule.jobs.discover.enabled` | `true` | Starts the discovery loop. At least one job must be enabled. |
| `schedule.jobs.discover.run_on_start` | `true` | When true, the first discovery runs after jitter; otherwise it waits one interval, then jitter. |
| `schedule.jobs.discover.interval` | `5m` | Wait after a run before the next run's jitter. Must be positive when enabled. |
| `schedule.jobs.discover.timeout` | `60s` | Outer deadline for one discovery cycle. Must be positive when enabled. |
| `schedule.jobs.values.enabled` | `true` | Starts the values loop. |
| `schedule.jobs.values.run_on_start` | `false` | When true, the first values collection runs after jitter; otherwise it waits one interval, then jitter. |
| `schedule.jobs.values.interval` | `30s` | Wait after a run before the next run's jitter. Must be positive when enabled. |
| `schedule.jobs.values.timeout` | `20s` | Outer deadline for one values cycle. Must be positive when enabled. |
| `prom.prefix` | `zabbix_` | Prefix joined directly to the sanitized item key. It must match `[a-zA-Z_:][a-zA-Z0-9_:]*`. |
| `prom.const_labels` | empty | String attributes added to every point before reserved identity attributes. |
| `zabbix.url` | none | Required HTTP or HTTPS Zabbix `api_jsonrpc.php` URL. |
| `zabbix.token` | none | Required opaque credential: a modern API token or a legacy `user.login` session token. |
| `zabbix.timeout` | `30s` | Per-request HTTP client timeout. Must be positive. The shorter of this and the active job deadline wins. |
| `zabbix.limits.max_metrics_per_host` | `1000` | Maximum selected items per host after filtering, preserving Zabbix item order. Must be positive. |
| `zabbix.limits.items_per_request` | `1000` | Maximum item IDs per value `item.get` request. Must be positive. |
| `zabbix.filters.*` | empty | Go regular expressions. An empty expression disables that filter. Invalid expressions fail startup. |

Configuration validation has two stages. Collector recursive validation operates on an environment-resolved clone: the receiver clones the decoded configuration, applies explicit receiver environment overrides, and validates the resolved values without mutating the decoded object. During component construction, the factory independently clones the decoded configuration, applies the same overrides, and invokes the pure resolved-config validator. Independent validation problems are returned together and name their fields. Both jobs disabled, a missing/invalid URL or token after overrides, a non-positive Zabbix request timeout, a negative jitter, invalid enabled-job timing, non-positive limits, invalid regexes, and an invalid prefix prevent Collector startup.

## Environment precedence

Collector `${env:NAME}` interpolation is evaluated while decoding the YAML. The following explicit receiver variables are then applied to a clone for Collector validation and applied again to a separate factory clone for construction. If a variable is present, including when it is present with an empty value, it replaces the decoded value at both stages.

| Environment variable | Receiver setting | Parsing |
| --- | --- | --- |
| `ZABBIX_URL` | `zabbix.url` | String |
| `ZABBIX_TOKEN` | `zabbix.token` | Opaque string |
| `ZABBIX_TIMEOUT` | `zabbix.timeout` | Go duration, such as `7s` |
| `MAX_METRICS_PER_HOST` | `zabbix.limits.max_metrics_per_host` | Base-10 integer |
| `ZABBIX_ITEMS_PER_REQUEST` | `zabbix.limits.items_per_request` | Base-10 integer |

Explicit receiver variables take precedence over decoded YAML during both stages. Environment-only `ZABBIX_URL` and `ZABBIX_TOKEN` can satisfy omitted `zabbix.url` and `zabbix.token`. Likewise, YAML with `items_per_request: 0` is valid when `ZABBIX_ITEMS_PER_REQUEST=250` is present, because validation sees the resolved positive value. Without that override, the decoded zero remains invalid.

Invalid duration or integer overrides fail validation with an error naming the environment variable. An override that parses but produces an invalid resolved value, such as `ZABBIX_ITEMS_PER_REQUEST=0` or `ZABBIX_TIMEOUT=0s`, fails with the corresponding field name. `zabbix.timeout` must remain positive after overrides. Unset variables leave the decoded YAML values unchanged. `VICTORIAMETRICS_REMOTE_WRITE_URL` in the sample configuration is standard Collector interpolation for the `prometheusremotewrite` exporter; it is not a receiver override.

Example with overrides that replace YAML values:

```bash
ZABBIX_URL=https://zabbix.example.com/api_jsonrpc.php \
ZABBIX_TOKEN=replace-me \
ZABBIX_TIMEOUT=10s \
MAX_METRICS_PER_HOST=500 \
ZABBIX_ITEMS_PER_REQUEST=250 \
VICTORIAMETRICS_REMOTE_WRITE_URL=https://vm.example.com/api/v1/write \
./bin/otelcol-zabbix --config configs/otelcol.yaml
```

## Authentication

The client begins in modern mode and sends `Authorization: Bearer <credential>` without a JSON-RPC `auth` property. If Zabbix returns a JSON-RPC authentication error, it retries that operation exactly once with the credential in the legacy JSON-RPC `auth` property. A successful retry caches legacy mode for later calls. A failed retry is returned and does not change the cached mode.

For modern Zabbix, supply an API token. Long-lived API tokens require a Zabbix version that implements them. For legacy Zabbix 5.x, obtain a session token separately with `user.login` and supply that token as `zabbix.token`; the receiver does not accept or exchange a username/password. HTTP, transport, decoding, and non-authentication API errors do not cause legacy fallback. Credentials and complete authenticated request bodies are not logged, and occurrences of the configured credential in returned errors are replaced with `[REDACTED]`.

## Discovery, filters, and snapshots

One discovery cycle:

1. Requests available hosts, ordered by `hostid`.
2. Requests items for those hosts and asks Zabbix for numeric value types only: float (`0`) and unsigned integer (`3`), ordered by `itemid`.
3. Rejects items with an unknown host.
4. Applies a non-empty host include regex, then the host exclude regex.
5. Applies a non-empty item-key include regex, then the item-key exclude regex.
6. Applies `max_metrics_per_host` after all filters, preserving the returned item order.
7. Atomically replaces the immutable metadata snapshot.

The snapshot is replaced only after both host and item requests succeed. A discovery error therefore retains the last good snapshot. A successful discovery with no selected items publishes an empty snapshot. Discovery and values jobs are independently serialized; they can run concurrently with each other, while values always sees one stable snapshot.

## Values, batches, and failures

With no snapshot or an empty snapshot, a values cycle succeeds without emitting a batch. Otherwise, item IDs are divided into chunks no larger than `items_per_request`. All chunk responses are accumulated before conversion.

If any chunk request fails, no partial batch is delivered for that cycle. Individual values with a malformed number, invalid/out-of-range `lastclock`, missing metadata, or unusable metric name are skipped while other valid values are emitted. If no points remain, the downstream consumer is not called. Otherwise exactly one metrics batch is delivered. A downstream consumer error is returned, logged by the job loop, and counted; scheduled collection continues on the next cycle. Discovery, value-request, and downstream errors do not terminate their job loops.

There is no same-cycle retry beyond the single modern-to-legacy authentication fallback. Each job cycle has its own timeout context. Collector shutdown cancels scheduler waits and in-flight Zabbix requests and waits for enabled loops within the provided shutdown context.

## Metric contract

For every matched valid Zabbix value:

- Metric type: OpenTelemetry gauge.
- Point value: double, including Zabbix unsigned integer items.
- Name: `{prom.prefix}{sanitized_item_key}`. Each item-key rune outside `[a-zA-Z0-9_:]` is replaced with `_`; trailing underscores are removed; `_` is prepended if the combined name begins with an invalid character. Invalid characters are replaced independently, so underscores are not collapsed.
- Description: Zabbix item display name.
- Timestamp: Zabbix `lastclock`, interpreted as non-negative Unix seconds and converted to nanoseconds.
- Attributes: all `prom.const_labels`, followed by `host`, `hostid`, `item_key`, and `itemid` from discovery metadata.

Because the four reserved attributes are written last, their discovery values take precedence over constant labels with the same names. Name collisions remain distinguishable through `item_key` and `itemid`.

## Receiver self-telemetry

The authoritative receiver telemetry surface is exactly the ten instruments below:

| Instrument | Semantics |
| --- | --- |
| `otelcol_receiver_zabbix_discover_attempts` | Counter incremented at the start of every discovery cycle. |
| `otelcol_receiver_zabbix_discover_errors` | Counter incremented when a discovery cycle returns an error. |
| `otelcol_receiver_zabbix_discover_duration` | Histogram recording every discovery cycle's duration in seconds, including failed cycles. |
| `otelcol_receiver_zabbix_values_attempts` | Counter incremented at the start of every values cycle. |
| `otelcol_receiver_zabbix_values_errors` | Counter incremented when a values cycle returns an error, including downstream consumer errors. |
| `otelcol_receiver_zabbix_values_duration` | Histogram recording every values cycle's duration in seconds, including failed cycles. |
| `otelcol_receiver_zabbix_emitted_points` | Counter increased by valid gauge points built from retrieved values before the downstream consumer returns. |
| `otelcol_receiver_zabbix_invalid_values` | Counter increased by retrieved values skipped for malformed numbers, invalid timestamps, or unusable metric names. |
| `otelcol_receiver_zabbix_filtered_items` | Counter increased by discovery items excluded because their host is unknown or a configured host/item filter rejects them. |
| `otelcol_receiver_zabbix_limited_items` | Counter increased by otherwise selected discovery items excluded by the per-host limit. |

These are Collector-native instruments when the configured telemetry reader exposes them. Separate success, host-count, item-count, requested-item, and downstream-failure instruments are not part of the accepted surface.

The production container and Kubernetes samples expose standard Collector telemetry on port 8888. Treat it as an unauthenticated operational endpoint and restrict network access appropriately.
