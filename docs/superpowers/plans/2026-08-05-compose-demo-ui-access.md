# Docker Compose Demo UI Access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove retired vendor attribution, publish the Zabbix and VictoriaMetrics demo interfaces on configurable host-loopback ports, document how to compare the shared demo counter, and leave a verified demo stack running.

**Architecture:** First neutralize obsolete attribution in documentation, comments, and its packaging assertion without changing the receiver contract. Then use a host-connected Compose bridge and add long-form `ports` entries only to `zabbix-web` and `victoriametrics`; bind both to `127.0.0.1`, interpolate configurable host ports with stable defaults, and retain the existing verifier as the metric-contract authority.

**Tech Stack:** Docker Compose v2/v5, Go 1.25+, `gopkg.in/yaml.v3`, shell, curl, jq, Markdown

## Global Constraints

- Bind published demo interfaces to `127.0.0.1`, never `0.0.0.0`.
- Default the Zabbix host port to `8080` through `ZABBIX_WEB_PORT`.
- Default the VictoriaMetrics host port to `8428` through `VICTORIAMETRICS_PORT`.
- Keep container ports and internal URLs unchanged: Zabbix `8080`, VictoriaMetrics `8428`.
- Set the shared `demo` bridge to `internal: false`; networks with `internal: true` cannot activate host port forwarding.
- Do not change Collector code, receiver behavior, metric mapping, bootstrap logic, or producer cadence.
- Remove the retired vendor name, URL, and compatibility claims from all current files while preserving every technical requirement.
- `make demo-verify` remains authoritative for the exported metric contract.
- Leave the successful demo stack running for interactive inspection.

## File Structure

- Modify `internal/packaging/assets_test.go`: enforce the Compose publication and README documentation contracts.
- Modify `docs/configuration.md`: describe the implemented configuration without external vendor attribution.
- Modify `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md`: preserve requirements using project-owned terminology.
- Modify `docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md`: preserve historical plan details using project-owned terminology.
- Modify `internal/metrics/name.go`: describe metric naming as Prometheus-compatible.
- Modify `compose.yaml`: publish the two UI ports on configurable loopback bindings.
- Modify `README.md`: document URLs, credentials, navigation, query, overrides, and collection lag.

---

### Task 0: Remove Retired Vendor Attribution

**Files:**
- Modify: `docs/configuration.md:3`
- Modify: `docs/configuration.md:42`
- Modify: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:13`
- Modify: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:32`
- Modify: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:36`
- Modify: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:45`
- Modify: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:61`
- Modify: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:123`
- Modify: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:151`
- Modify: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:347`
- Modify: `docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md:15`
- Modify: `docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md:16`
- Modify: `docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md:36`
- Modify: `docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md:56`
- Modify: `docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md:925`
- Modify: `internal/metrics/name.go:9`
- Modify: `internal/packaging/assets_test.go:45`

**Interfaces:**
- Consumes: the existing public receiver settings, defaults, validation rules, and metric mapping.
- Produces: identical technical documentation and runtime behavior with no retired vendor name, URL, or compatibility claim.

- [ ] **Step 1: Record the existing failing baseline**

Run:

```bash
go test ./... -count=1
```

Expected: FAIL only in `TestDocumentationContract`, because it still requires
an external source link that the README intentionally omits.

- [ ] **Step 2: Replace configuration-document attribution with project-owned wording**

Make the opening of `docs/configuration.md` read:

```markdown
The public Collector receiver type is `zabbix`. This document describes the
implemented receiver contract; the project's [approved design](superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md)
defines its compatibility boundary.
```

Make the unsupported-setting paragraph read:

```markdown
`base.address`, standalone `/metrics` or `/health` endpoints, standalone
exporter flags, and agent-specific exporter settings are not supported receiver
keys. The OpenTelemetry Collector owns those concerns.
```

- [ ] **Step 3: Neutralize the approved design and historical plan**

Use these exact replacement phrases while retaining the surrounding lists,
tables, and requirements:

```text
The receiver supports this public configuration interface:
Agent-specific exporter configuration
Default configuration matching the documented receiver defaults
The receiver targets Zabbix 5.0 or later, subject to API-token availability in the deployed Zabbix version.
The receiver defaults are:
matching the public receiver interface
The supported settings and defaults decode, validate, and behave as specified.
Preserve the public receiver `schedule`, `prom.prefix`, `prom.const_labels`, and `zabbix` configuration keys and documented Zabbix environment overrides.
Omit `base.address`, standalone exporter HTTP endpoints, standalone CLI flags, and agent-specific configuration.
Public receiver configuration types, defaults, environment resolution, and validation.
Prometheus-compatible metric-name construction.
require the approved-design link and verify every supported environment variable appears in `docs/configuration.md`.
```

Delete the obsolete standalone external-reference line from the approved
design. Keep all technical key lists and defaults unchanged.

- [ ] **Step 4: Neutralize the Go comment and obsolete packaging assertion**

Make the comment in `internal/metrics/name.go` read:

```go
// Name returns the Prometheus-compatible metric name for an item key.
```

Make the README link assertion map in `TestDocumentationContract` read:

```go
	for name, link := range map[string]string{
		"approved design": "docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md",
	} {
```

- [ ] **Step 5: Prove the attribution is absent and behavior is unchanged**

Run:

```bash
rg -n -i 'as''tra' . --glob '!.git/**' --glob '!vendor/**'
go test ./... -count=1
git diff -- demo/collector.yaml.tmpl
git diff --check
```

Expected: the scan and template diff print nothing; all Go packages pass; the
whitespace check exits zero.

- [ ] **Step 6: Commit the attribution cleanup**

```bash
git add docs/configuration.md \
  docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md \
  docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md \
  internal/metrics/name.go internal/packaging/assets_test.go
git commit -m "docs: remove retired vendor attribution"
```

### Task 1: Publish Configurable Loopback Ports

**Files:**
- Modify: `internal/packaging/assets_test.go:279`
- Modify: `internal/packaging/assets_test.go:928`
- Modify: `compose.yaml:36`
- Modify: `compose.yaml:90`

**Interfaces:**
- Consumes: Compose interpolation and existing YAML assertion helpers.
- Produces: `zabbix-web` on `127.0.0.1:${ZABBIX_WEB_PORT:-8080}` and `victoriametrics` on `127.0.0.1:${VICTORIAMETRICS_PORT:-8428}`.

- [ ] **Step 1: Add failing Compose contract assertions**

Add to `TestComposeTopologyContract`:

```go
	assertComposeLoopbackPort(t, services, "zabbix-web", "ZABBIX_WEB_PORT", 8080, 8080)
	assertComposeLoopbackPort(t, services, "victoriametrics", "VICTORIAMETRICS_PORT", 8428, 8428)
```

Add next to the other Compose helpers:

```go
func assertComposeLoopbackPort(t *testing.T, services map[string]any, serviceName, environmentVariable string, defaultPort, targetPort int) {
	t.Helper()
	service := mapValue(t, value(t, services, serviceName))
	ports := mapSlice(t, value(t, service, "ports"))
	if len(ports) != 1 {
		t.Fatalf("service %q ports = %d entries, want 1", serviceName, len(ports))
	}
	port := ports[0]
	assertEqual(t, "127.0.0.1", value(t, port, "host_ip"))
	assertEqual(t, "${"+environmentVariable+":-"+strconv.Itoa(defaultPort)+"}", value(t, port, "published"))
	assertEqual(t, targetPort, value(t, port, "target"))
	assertEqual(t, "tcp", value(t, port, "protocol"))
}
```

`strconv` is already imported.

- [ ] **Step 2: Run the focused test and verify it fails**

Run:

```bash
go test ./internal/packaging -run '^TestComposeTopologyContract$' -count=1
```

Expected: FAIL because `zabbix-web` has no `ports` key.

- [ ] **Step 3: Add the minimal Compose port mappings**

Add to `zabbix-web`, after its health check:

```yaml
    ports:
      - target: 8080
        published: "${ZABBIX_WEB_PORT:-8080}"
        host_ip: 127.0.0.1
        protocol: tcp
```

Add to `victoriametrics`, after `volumes`:

```yaml
    ports:
      - target: 8428
        published: "${VICTORIAMETRICS_PORT:-8428}"
        host_ip: 127.0.0.1
        protocol: tcp
```

- [ ] **Step 4: Format and rerun the focused test**

Run:

```bash
gofmt -w internal/packaging/assets_test.go
go test ./internal/packaging -run '^TestComposeTopologyContract$' -count=1
```

Expected: PASS.

- [ ] **Step 5: Verify default and overridden Compose expansion**

Run:

```bash
ZABBIX_WEB_PORT=8080 VICTORIAMETRICS_PORT=8428 docker compose config --format json | jq -e '
  any(.services["zabbix-web"].ports[];
    .host_ip == "127.0.0.1" and .published == "8080" and .target == 8080) and
  any(.services.victoriametrics.ports[];
    .host_ip == "127.0.0.1" and .published == "8428" and .target == 8428)
'
ZABBIX_WEB_PORT=18080 VICTORIAMETRICS_PORT=18428 docker compose config --format json | jq -e '
  any(.services["zabbix-web"].ports[];
    .host_ip == "127.0.0.1" and .published == "18080" and .target == 8080) and
  any(.services.victoriametrics.ports[];
    .host_ip == "127.0.0.1" and .published == "18428" and .target == 8428)
'
```

Expected: both commands print `true` and exit zero.

- [ ] **Step 6: Commit the port contract and Compose change**

```bash
git add internal/packaging/assets_test.go compose.yaml
git commit -m "feat: expose demo UIs on loopback"
```

### Task 1.5: Enable Host Port Forwarding on the Demo Bridge

**Files:**
- Modify: `internal/packaging/assets_test.go:318`
- Modify: `compose.yaml:139`

**Interfaces:**
- Consumes: the loopback port mappings from Task 1 and the shared `demo` bridge.
- Produces: a host-connected bridge that activates only the explicitly published loopback ports.

- [ ] **Step 1: Change the network contract to require host connectivity**

In `TestComposeTopologyContract`, change the existing network assertion to:

```go
	networks := mapValue(t, value(t, compose, "networks"))
	demoNetwork := mapValue(t, value(t, networks, "demo"))
	assertEqual(t, false, value(t, demoNetwork, "internal"))
```

- [ ] **Step 2: Run the focused test and verify it fails**

Run:

```bash
go test ./internal/packaging -run '^TestComposeTopologyContract$' -count=1
```

Expected: FAIL because the current YAML value is `true`, not `false`.

- [ ] **Step 3: Make the bridge explicitly host-connected**

Change the top-level network declaration in `compose.yaml` to:

```yaml
networks:
  demo:
    internal: false
```

Do not add published ports to any backend service.

- [ ] **Step 4: Verify the focused contract and expanded Compose model**

Run:

```bash
gofmt -w internal/packaging/assets_test.go
go test ./internal/packaging -run '^TestComposeTopologyContract$' -count=1
docker compose config --format json | jq -e '
  (.networks.demo.internal // false) == false and
  any(.services["zabbix-web"].ports[];
    .host_ip == "127.0.0.1" and .published == "8080" and .target == 8080) and
  any(.services.victoriametrics.ports[];
    .host_ip == "127.0.0.1" and .published == "8428" and .target == 8428)
'
```

Expected: the Go test passes and jq prints `true`.

- [ ] **Step 5: Commit the corrected network contract**

```bash
git add internal/packaging/assets_test.go compose.yaml
git commit -m "fix: enable demo host port forwarding"
```

### Task 2: Document and Run the Inspectable Demo

**Files:**
- Modify: `internal/packaging/assets_test.go:29`
- Modify: `README.md:62`

**Interfaces:**
- Consumes: Task 1 endpoints, Compose-only Zabbix credentials, and existing demo Make targets.
- Produces: instructions for finding `demo.counter` and querying `zabbix_demo_counter{host="otel-demo-host",env="compose"}`.

- [ ] **Step 1: Add failing README contract assertions**

After the existing README clause checks in `TestDocumentationContract`, add:

```go
	for name, clause := range map[string]string{
		"Zabbix demo UI URL":             "http://127.0.0.1:8080/",
		"VictoriaMetrics VMUI URL":       "http://127.0.0.1:8428/vmui/",
		"Zabbix demo UI credentials":     "`Admin` / `zabbix`",
		"VictoriaMetrics demo query":     `zabbix_demo_counter{host="otel-demo-host",env="compose"}`,
		"Zabbix UI port override":        "ZABBIX_WEB_PORT",
		"VictoriaMetrics port override": "VICTORIAMETRICS_PORT",
	} {
		if !strings.Contains(readme, clause) {
			t.Errorf("README.md missing %s clause %q", name, clause)
		}
	}
```

The query string uses a Go raw string delimited by backticks.

- [ ] **Step 2: Run the documentation test and verify it fails**

Run:

```bash
go test ./internal/packaging -run '^TestDocumentationContract$' -count=1
```

Expected: FAIL with missing UI URLs, credentials, query, and port override names.

- [ ] **Step 3: Add concrete UI instructions to README**

Insert after the three demo make commands:

````markdown
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
````

- [ ] **Step 4: Run focused and static verification**

Run:

```bash
go test ./internal/packaging -run '^(TestDocumentationContract|TestComposeTopologyContract)$' -count=1
docker compose config --quiet
git diff --check
```

Expected: all commands exit zero.

- [ ] **Step 5: Choose free ports and start the real demo**

Check defaults:

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
lsof -nP -iTCP:8428 -sTCP:LISTEN
```

Recreate the containers and network without deleting named volumes:

```bash
docker compose down --remove-orphans
make demo-up
```

If either default port is occupied, choose unused values and keep the same
assignments for both commands and all later Compose commands, for example:

```bash
ZABBIX_WEB_PORT=18080 VICTORIAMETRICS_PORT=18428 docker compose down --remove-orphans
ZABBIX_WEB_PORT=18080 VICTORIAMETRICS_PORT=18428 make demo-up
```

Expected: named volumes remain; the `demo` network is recreated with
`internal=false`; PostgreSQL, Zabbix server, and Zabbix web become healthy;
bootstrap completes; producer, VictoriaMetrics, and the Collector remain
running.

- [ ] **Step 6: Verify the metric and both host endpoints**

With default ports, run:

```bash
make demo-verify
curl --fail --silent --show-error --output /dev/null http://127.0.0.1:8080/
curl --fail --silent --show-error --output /dev/null http://127.0.0.1:8428/vmui/
curl --fail --silent --show-error --get \
  --data-urlencode 'query=zabbix_demo_counter{host="otel-demo-host",env="compose"}' \
  http://127.0.0.1:8428/api/v1/query | jq -e '
    .status == "success" and
    (.data.result | length) == 1 and
    (.data.result[0].value[1] | tonumber) > 0
  '
```

Substitute selected host ports if overridden. Expected: the verifier prints one
accepted series, both UI requests exit zero, and the API assertion prints
`true`.

- [ ] **Step 7: Inspect both browser views and leave the stack running**

Open the documented URLs. In Zabbix, confirm that
`otel-demo-host` / `demo.counter` has a recent increasing value. In VMUI,
run the documented query and confirm `item_key="demo.counter"` and a recent
positive value. Allow one collection cycle for convergence.

Do not run `make demo-down`; the operator requested an interactive demo.

- [ ] **Step 8: Commit documentation and its contract**

```bash
git add internal/packaging/assets_test.go README.md
git commit -m "docs: explain demo UI comparison"
```
