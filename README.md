# Zabbix receiver for the OpenTelemetry Collector

This repository provides a native OpenTelemetry Collector metrics receiver that polls numeric items from Zabbix. The custom Collector distribution sends the resulting OpenTelemetry gauges through ordinary Collector processors and exporters; the supplied pipeline uses the standard Prometheus remote-write exporter with VictoriaMetrics.

See the [approved design](docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md) for the receiver's compatibility boundary and completion criteria.

## Architecture

```text
Zabbix API -> zabbix receiver -> memory_limiter -> batch
                                              -> prometheusremotewrite -> VictoriaMetrics
```

The `zabbix` receiver has independent, non-overlapping discovery and values jobs. Discovery requests hosts and numeric items (`value_type` 0 and 3), applies filters and the per-host limit, and atomically publishes an immutable metadata snapshot. The values job reads one stable snapshot, retrieves values in bounded chunks, converts them to OpenTelemetry gauge points, and calls the next metrics consumer once per successful cycle.

The distribution in `cmd/otelcol-zabbix` embeds only the `zabbix` receiver, `memory_limiter` and `batch` processors, `prometheusremotewrite` exporter, `health_check` extension, and standard Collector telemetry. The receiver itself has no VictoriaMetrics dependency.

## Quick start

Requirements are Go 1.25 or newer and reachable Zabbix and Prometheus remote-write endpoints. The example configuration exposes health on port 13133 and internal Collector metrics on port 8888.

```bash
make build
ZABBIX_URL=http://zabbix.example/api_jsonrpc.php \
ZABBIX_TOKEN=replace-me \
VICTORIAMETRICS_REMOTE_WRITE_URL=http://victoriametrics:8428/api/v1/write \
./bin/otelcol-zabbix --config configs/otelcol.yaml
```

Use HTTPS for production credentials. `replace-me` is a placeholder, not a working token. The receiver accepts HTTP endpoints because local and private-network deployments can require them; transport security is therefore an operator responsibility.

Modern Zabbix authentication sends the configured credential as `Authorization: Bearer`. For legacy Zabbix 5.x, the configured value can instead be a session token previously obtained with `user.login`: after a Bearer request returns a Zabbix authentication error, the receiver retries that operation once with the JSON-RPC `auth` property and caches legacy mode after a successful retry. It does not accept a username/password or perform `user.login` itself. Non-authentication errors do not trigger fallback, and failed authentication is not repeatedly retried within the cycle. Long-lived API tokens require a Zabbix version that supports them.

## Custom Collector Builder

`builder-config.yaml` controls compilation, while `otelcol.yaml` controls runtime. Add the versioned receiver module to a Builder configuration:

```yaml
receivers:
  - gomod: github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.1.0
```

See the complete [build-time example](examples/ocb/builder-config.yaml) and [runtime example](examples/ocb/otelcol.yaml). Tested compatibility: Collector/Contrib v0.154.0 and stable Collector modules v1.60.0. Release tag: receiver/zabbixreceiver/v0.1.0.

## Configuration

The complete YAML schema, defaults, validation, receiver environment overrides, authentication behavior, scheduling, and failure semantics are in [docs/configuration.md](docs/configuration.md). The production sample is [configs/otelcol.yaml](configs/otelcol.yaml).

Validation happens in two stages, and the five explicit receiver overrides participate in both validation stages. After the Collector performs `${env:NAME}` interpolation and decodes YAML, the receiver's public validation method clones the decoded configuration, applies `ZABBIX_URL`, `ZABBIX_TOKEN`, `ZABBIX_TIMEOUT`, `MAX_METRICS_PER_HOST`, and `ZABBIX_ITEMS_PER_REQUEST`, and validates that resolved clone without mutating the decoded configuration. The factory independently repeats clone, resolution, and pure resolved-config validation before construction. A present override therefore takes precedence over YAML even during Collector recursive validation: environment-only URL/token values can satisfy omitted YAML fields, and a valid override can replace an otherwise invalid decoded value. Present empty or invalid overrides still fail with field- or environment-specific errors. `VICTORIAMETRICS_REMOTE_WRITE_URL` in the sample is exporter configuration, not a `zabbix` receiver setting.

## Metric mapping

Each selected item becomes one gauge metric with a double data point, including unsigned Zabbix items. Its name is `{prom.prefix}{sanitized_item_key}`: every item-key character outside `[a-zA-Z0-9_:]` becomes `_`, trailing underscores are removed, and `_` is prepended if necessary to make the first character valid. Replacements are one-for-one, so repeated punctuation can produce repeated underscores.

The item display name is the metric description. Zabbix `lastclock` seconds become the point timestamp in nanoseconds. Every point has `host`, `hostid`, `item_key`, and `itemid`; configured `prom.const_labels` are added first, then these four reserved attributes overwrite conflicts. Items whose keys produce the same metric name remain distinguishable by `item_key` and `itemid`.

Malformed numeric values, invalid timestamps, and unusable metric names are skipped individually and counted in receiver telemetry. Missing item values do not create points. If any chunked Zabbix value request fails, the entire cycle is discarded rather than emitting a partial batch. The downstream consumer is called only after a values cycle successfully retrieves every chunk from a non-empty snapshot and emits at least one valid point.

## Docker Compose demo

Docker Engine with Compose v2 is required. The real demo starts PostgreSQL, Zabbix 7.4.12, a bootstrap job, a producer, the custom Collector, and VictoriaMetrics:

```bash
make demo-up
make demo-verify
make demo-down
```

Both web interfaces are published only on host loopback:

- Open Zabbix at <http://127.0.0.1:8080/> and sign in with the Compose-only
  credentials `Admin` / `zabbix`. Open **Monitoring → Latest data**, select
  host `otel-demo-host`, and find item key `demo.counter`.
- Open VictoriaMetrics VMUI at <http://127.0.0.1:8428/vmui/> and run
  `zabbix_demo_counter{host="otel-demo-host",env="compose"}`.

The producer increments the Zabbix item every five seconds, and the Collector
reads values every five seconds. Both views therefore show the same source
counter, although their latest displayed values can briefly differ by one
collection cycle.

To avoid occupied host ports, choose alternatives without editing Compose:

```bash
ZABBIX_WEB_PORT=18080 VICTORIAMETRICS_PORT=18428 make demo-up
```

With those overrides, open `http://127.0.0.1:18080/` and
`http://127.0.0.1:18428/vmui/`. Keep these interfaces on loopback: the demo
uses well-known Zabbix credentials and does not configure VictoriaMetrics
authentication.

`make demo-verify` succeeds only after VictoriaMetrics returns `zabbix_demo_counter` with `host="otel-demo-host"`, non-empty `hostid` and `itemid`, `item_key="demo.counter"`, `env="compose"`, and a positive value. The sample timestamp is at or after the verifier start; matching stale series are ignored. Verification prints only the accepted fresh metric series as compact JSON and has a hard 180-second process deadline.

The bootstrap uses the Compose-only `Admin`/`zabbix` credential, creates or finds the demo host and trapper item, deletes only the current user's prior `otel-demo-receiver` token, generates a replacement, and verifies it. The token is written only to the `demo-config` runtime volume as a mode `0400`, UID/GID `10001:10001` Collector configuration; it is not printed or interpolated into `compose.yaml`. `make demo-down` removes containers, named volumes, and orphans, including the runtime token. This environment is a demonstration, not a production credential pattern.

## VM/systemd

Build the binary, create the dedicated `otelcol-zabbix` service account, install the supplied configuration, environment file, and hardened unit, then enable the service. Exact commands and file permissions are in [docs/deployment.md](docs/deployment.md#vmsystemd). The sample health endpoint binds `0.0.0.0:13133`; restrict it with host firewalling or change the bind address when it must not be remotely reachable.

## Kubernetes

The Kustomize deployment runs as UID/GID 10001, disables service-account token mounting, uses a read-only root filesystem, and exposes health and internal telemetry through a ClusterIP Service. Validate the manifests while the Secret still contains placeholders, redirecting the rendered output so even placeholder data is not printed:

```bash
kubectl kustomize deployments/kubernetes >/dev/null
```

Then replace the placeholders in `deployments/kubernetes/secret.example.yaml` locally, do not commit the edited Secret, and apply without running the standalone render command again:

```bash
kubectl apply -k deployments/kubernetes
```

The manifest uses image `zabbix-otel-collector:local`; make that image available to the cluster or change the Deployment to an immutable registry reference. See [docs/deployment.md](docs/deployment.md#kubernetes) for preparation, verification, rotation, and cleanup.

## Testing

The standard local matrix is:

```bash
make fmt
git diff --check
make test
make test-race
make vet
make tidy
make build
make validate-config
make compose-config
make verify-ocb-local
docker build -t zabbix-otel-collector:verify .
docker run --rm zabbix-otel-collector:verify components
```

The standard Make targets cover both the root and receiver modules. Go tests use local loopback listeners for HTTP fixtures. Sandboxes that prohibit binding loopback ports cannot run the complete suite; run it on a host or CI runner that permits local listeners. Container, Compose, and Kubernetes checks require their respective tools and daemons. The multi-container end-to-end demo is intentionally a manual GitHub Actions `workflow_dispatch` job rather than a push/PR check.

## Security

- `zabbix.token` uses Collector opaque-string handling. The client never logs credentials or full authenticated JSON-RPC bodies and redacts the configured token from its error chain.
- Keep tokens out of YAML, shell history, source control, and CI output. Prefer runtime environment injection from a permission-restricted file or secret manager.
- The image and Kubernetes workload run as numeric UID/GID 10001 with no embedded credentials. The systemd service uses a dedicated unprivileged account and hardened unit settings.
- Kubernetes `Secret.stringData` is not encryption; protect cluster API and etcd access and use your platform's secret integration where appropriate.
- The Compose bootstrap token lives only in a named volume and is deleted with `make demo-down`; abrupt interruption can leave it until cleanup is run.
- Health and internal telemetry endpoints are unauthenticated sample endpoints. Limit network access and change bind addresses to suit the deployment.
- Rotate a production credential by updating its runtime source and restarting the Collector. Legacy sessions expire according to Zabbix policy; modern API-token lifetime and revocation are managed in Zabbix.
