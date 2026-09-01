# Deployment guide

The supplied paths all run the same custom Collector distribution. Build and pin an artifact appropriate for your environment, supply credentials only at runtime, and protect the unauthenticated health and telemetry endpoints.

## Prebuilt release binary

The supported release binaries run on Linux `amd64` and `arm64`; Go is not required. Select a version from the [GitHub Releases page](https://github.com/wieso/zabbixreceiver/releases), then use the matching `ARCH` value:

```bash
VERSION=1.2.3
ARCH=amd64
curl -fLO "https://github.com/wieso/zabbixreceiver/releases/download/v${VERSION}/otelcol-zabbix_${VERSION}_linux_${ARCH}.tar.gz"
curl -fLO "https://github.com/wieso/zabbixreceiver/releases/download/v${VERSION}/checksums.txt"
sha256sum -c checksums.txt --ignore-missing
tar -xzf "otelcol-zabbix_${VERSION}_linux_${ARCH}.tar.gz"
sudo install -m 0755 "otelcol-zabbix_${VERSION}_linux_${ARCH}/otelcol-zabbix" /usr/local/bin/otelcol-zabbix
sudo install -d -m 0755 /etc/otelcol-zabbix
sudo install -m 0644 "otelcol-zabbix_${VERSION}_linux_${ARCH}/configs/otelcol.yaml" /etc/otelcol-zabbix/config.yaml
```

Set the three endpoint/credential variables at runtime and start the collector:

```bash
ZABBIX_URL=https://zabbix.example.com/api_jsonrpc.php \
ZABBIX_TOKEN=replace-with-zabbix-api-token \
VICTORIAMETRICS_REMOTE_WRITE_URL=https://victoriametrics.example.com/api/v1/write \
/usr/local/bin/otelcol-zabbix --config /etc/otelcol-zabbix/config.yaml
```

The token shown above is a placeholder. Use HTTPS, avoid putting real credentials in shared command history, and use a service secret mechanism for production. To run the downloaded binary as a managed service, continue with the [VM/systemd](#vmsystemd) section; the unit and environment-file templates are maintained in the repository under `deployments/systemd/`.

## Local binary

Requirements: Go 1.25 or newer, a Zabbix API endpoint, and a Prometheus remote-write endpoint.

```bash
make build
make validate-config
```

`make build` writes `bin/otelcol-zabbix`. `make validate-config` rebuilds it, supplies non-secret dummy endpoints, and runs the Collector's `validate` command against `configs/otelcol.yaml`. Start it with runtime values:

```bash
ZABBIX_URL=https://zabbix.example.com/api_jsonrpc.php \
ZABBIX_TOKEN=replace-me \
VICTORIAMETRICS_REMOTE_WRITE_URL=https://victoriametrics.example.com/api/v1/write \
./bin/otelcol-zabbix --config configs/otelcol.yaml
```

The sample health endpoint is `http://127.0.0.1:13133/`; Collector internal metrics are on port 8888. The configuration binds both to all interfaces, so apply host firewalling or change the listener addresses if remote access is not intended.

## Container

Build the same image name used by the Kubernetes and Compose samples:

```bash
make docker-build
docker run --rm \
  -e ZABBIX_URL=https://zabbix.example.com/api_jsonrpc.php \
  -e ZABBIX_TOKEN=replace-me \
  -e VICTORIAMETRICS_REMOTE_WRITE_URL=https://victoriametrics.example.com/api/v1/write \
  -p 127.0.0.1:13133:13133 \
  -p 127.0.0.1:8888:8888 \
  zabbix-otel-collector:local
```

The multi-stage image contains the statically linked binary and sample configuration, runs as numeric UID/GID `10001:10001`, and embeds no token. Its default command is `--config=/etc/otelcol-zabbix/config.yaml`. Avoid passing real secrets directly on a shared command line; use your container platform's secret injection. The image accepts environment variables because the bundled configuration references them.

Inspect the embedded inventory without starting a pipeline:

```bash
docker run --rm zabbix-otel-collector:local components
```

## Docker Compose demo

Requirements: Docker Engine and Docker Compose v2.24 or newer. Ensure no previous demo is running, then execute:

```bash
make demo-up
make demo-verify
make demo-down
```

`demo-up` explicitly builds/starts PostgreSQL, Zabbix server and web/API, bootstrap, producer, VictoriaMetrics, and the Collector. Bootstrap is idempotent for the host group, host, and trapper item. It rotates the current `Admin` user's named demo API token, writes the generated Collector configuration into a private named volume with UID/GID `10001:10001` and mode `0400`, and then starts the non-root Collector. The producer sends an increasing value every five seconds.

`demo-verify` activates only the `verify` profile and runs the one-shot `verify` service. It has a hard 180-second process deadline and prints the matching VictoriaMetrics series as compact JSON only after all required labels and a positive value are present and the sample timestamp is at or after the verifier start. Matching stale data is ignored. It does not traverse bootstrap dependencies, so verification does not rotate the live Collector token.

Always run `make demo-down`, including after failures. It executes `docker compose down --volumes --remove-orphans`; this removes the database, VictoriaMetrics data, generated configuration, and runtime token. The tracked `Admin`/`zabbix` login and database password are Compose-only bootstrap credentials and must not be reused outside this private demonstration network.

## VM/systemd

These commands assume a Linux distribution with systemd and conventional paths. Adjust the `nologin` path if required by the host:

```bash
make build
sudo useradd --system --home-dir /var/lib/otelcol-zabbix --shell /usr/sbin/nologin otelcol-zabbix
sudo install -d -m 0750 -o root -g otelcol-zabbix /etc/otelcol-zabbix
sudo install -m 0755 bin/otelcol-zabbix /usr/local/bin/otelcol-zabbix
sudo install -m 0640 -o root -g otelcol-zabbix deployments/systemd/otelcol-zabbix.yaml /etc/otelcol-zabbix/config.yaml
sudo install -m 0640 -o root -g otelcol-zabbix deployments/systemd/otelcol-zabbix.env.example /etc/otelcol-zabbix/otelcol-zabbix.env
sudo install -m 0644 deployments/systemd/otelcol-zabbix.service /etc/systemd/system/otelcol-zabbix.service
```

Edit `/etc/otelcol-zabbix/otelcol-zabbix.env` as root and replace all placeholders. Do not put quotes or shell commands in this systemd environment file:

```text
ZABBIX_URL=https://zabbix.example.com/api_jsonrpc.php
ZABBIX_TOKEN=replace-with-zabbix-api-token
VICTORIAMETRICS_REMOTE_WRITE_URL=https://victoriametrics.example.com/api/v1/write
```

Then enable and verify the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now otelcol-zabbix
sudo systemctl status otelcol-zabbix
sudo journalctl -u otelcol-zabbix --since today
curl --fail http://127.0.0.1:13133/
```

The unit runs with the dedicated user/group, restarts on failure, creates `/var/lib/otelcol-zabbix`, and applies filesystem, privilege, device, kernel, and address-family restrictions. Its health endpoint currently binds `0.0.0.0:13133`; restrict it with firewall policy or change `deployments/systemd/otelcol-zabbix.yaml` before installation. The systemd sample does not explicitly configure the port-8888 telemetry reader.

To rotate credentials, edit the environment file as root and restart:

```bash
sudo systemctl restart otelcol-zabbix
```

## Kubernetes

The manifests create namespace `observability`, an Opaque Secret, a ConfigMap, one Deployment, and a ClusterIP Service. They intentionally create no Role, RoleBinding, or ServiceAccount and disable automatic service-account token mounting.

First build and publish an immutable image that the cluster can pull, or load the local image into a development cluster. Update `deployments/kubernetes/deployment.yaml` from `zabbix-otel-collector:local` to that immutable reference when using a registry.

While `deployments/kubernetes/secret.example.yaml` still contains only placeholders, validate the Kustomize tree without printing its rendered Secret:

```bash
kubectl kustomize deployments/kubernetes >/dev/null
```

Only after that validation, replace both placeholders in `deployments/kubernetes/secret.example.yaml` locally:

```yaml
stringData:
  ZABBIX_URL: https://zabbix.example.com/api_jsonrpc.php
  ZABBIX_TOKEN: replace-with-zabbix-api-token
```

Also set `VICTORIAMETRICS_REMOTE_WRITE_URL` in `deployments/kubernetes/configmap.yaml`. `stringData` is plaintext input to the Kubernetes API, not encryption. Never run `kubectl kustomize` after inserting real credentials: it writes the Secret's `stringData` to standard output, where terminals and CI logs can retain it. Do not commit the edited Secret, use verbose or output-producing dry runs with it, include it in CI logs, or leave it in a shared checkout. Prefer an external secret controller or a separately managed Secret for production.

Apply the required Kustomize path directly; normal apply output reports resource identities rather than rendering Secret contents:

```bash
kubectl apply -k deployments/kubernetes
kubectl -n observability rollout status deployment/otelcol-zabbix
kubectl -n observability get pods,service
```

For a local health check, start the blocking port-forward in the first terminal:

```bash
kubectl -n observability port-forward service/otelcol-zabbix 13133:13133
```

In a second terminal, while port-forward is still running, issue the health request:

```bash
curl --fail http://127.0.0.1:13133/
```

The Pod runs as UID/GID 10001, disallows privilege escalation, drops all Linux capabilities, uses a read-only root filesystem, and has CPU/memory requests and limits. Readiness and liveness probe `/` on port 13133. The Service exposes health 13133 and Collector telemetry 8888 only inside the cluster. NetworkPolicy is not supplied; add one when namespace-wide access is too broad.

For credential rotation, update the Secret using your secret-management process and restart the Deployment so environment variables are re-read:

```bash
kubectl -n observability rollout restart deployment/otelcol-zabbix
kubectl -n observability rollout status deployment/otelcol-zabbix
```

Delete only the namespaced resources supplied by this deployment, leaving Namespace `observability` and any unrelated workloads intact:

```bash
kubectl -n observability delete deployment/otelcol-zabbix service/otelcol-zabbix configmap/otelcol-zabbix secret/otelcol-zabbix
```

Do not use `kubectl delete -k deployments/kubernetes` as routine cleanup. Because `namespace.yaml` is part of that Kustomization, the command would delete the entire `observability` namespace, including unrelated workloads in it.

## Operational failure semantics

- Invalid configuration prevents Collector startup with field-specific errors.
- Discovery errors keep the last complete metadata snapshot; only a successful discovery publishes a replacement.
- No snapshot or an empty snapshot produces no batch and is not an error.
- A failed value chunk discards the whole cycle. Malformed individual values are skipped while valid values can still be delivered.
- A downstream consumer error fails that cycle, but the scheduler continues with the next interval.
- Jobs do not overlap themselves. Discovery and values are separate loops and can overlap each other while sharing immutable snapshots.
- Job timeout bounds a complete cycle; `zabbix.timeout` also bounds each HTTP request, so the shorter active deadline wins.

Receiver attempts, failures, durations, selected-value results, and filter/limit counts are available through Collector self-telemetry. Logs name the operation and safe endpoint but do not include the configured credential or full authenticated JSON-RPC request body.

## Artifact validation

Run all validators available on the target host. The Kubernetes render command is safe only while `secret.example.yaml` still contains placeholders, and its output is discarded deliberately:

```bash
make validate-config
make compose-config
kubectl kustomize deployments/kubernetes >/dev/null
systemd-analyze verify deployments/systemd/otelcol-zabbix.service
docker build -t zabbix-otel-collector:verify .
docker run --rm zabbix-otel-collector:verify components
```

macOS does not normally provide `systemd-analyze`; use a current Debian systemd container as a host fallback and record the exact command/result. Kubernetes rendering requires `kubectl`; Docker and Compose validation require a working daemon/plugin.
