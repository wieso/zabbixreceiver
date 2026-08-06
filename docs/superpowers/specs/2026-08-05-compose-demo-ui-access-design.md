# Docker Compose Demo UI Access Design

Date: 2026-08-05

## Objective

Make the existing Docker Compose demonstration directly inspectable from the
host. After starting the demo, an operator must be able to open both the Zabbix
web interface and the VictoriaMetrics query interface and observe the same
demo counter as it travels through the Collector pipeline.

## Scope

The change is limited to the Compose deployment and its README instructions.
It does not change the Collector distribution, the Zabbix receiver, metric
mapping, demo bootstrap, or producer behavior.

## Network Access

The `zabbix-web` container publishes its port `8080` on host loopback. The
`victoriametrics` container publishes its port `8428` on host loopback. The
default host ports are also `8080` and `8428`.

Operators can avoid local port conflicts without editing `compose.yaml` by
setting these environment variables when running the demo:

| Variable | Default | Container port |
| --- | --- | --- |
| `ZABBIX_WEB_PORT` | `8080` | `8080` |
| `VICTORIAMETRICS_PORT` | `8428` | `8428` |

Published ports bind to `127.0.0.1` rather than all host interfaces because
the demo uses well-known credentials and VictoriaMetrics has no authentication
in this configuration.

The shared `demo` bridge network explicitly uses `internal: false`. Docker
networks with `internal: true` have no connection to host interfaces, so Docker
Engine cannot activate host port forwarding for containers attached only to
such a network. No backend service publishes a host port; changing the network
mode therefore exposes only the two explicit loopback bindings above. Existing
container-to-container addresses remain unchanged.

## Data Flow and UI Comparison

The existing producer sends an incrementing value to the Zabbix trapper item
`demo.counter` on host `otel-demo-host` every five seconds. The Collector reads
that item from the Zabbix API and writes it to VictoriaMetrics as
`zabbix_demo_counter` with the existing identity labels.

The README will document:

- Zabbix URL `http://127.0.0.1:8080/` and the Compose-only credentials
  `Admin` / `zabbix`.
- How to locate `demo.counter` for `otel-demo-host` in Zabbix latest data.
- VictoriaMetrics VMUI URL `http://127.0.0.1:8428/vmui/` and the query
  `zabbix_demo_counter{host="otel-demo-host",env="compose"}`.
- How URLs change when either host-port environment variable is overridden.
- The expected collection delay: the two interfaces represent the same source
  item, but their latest displayed values may briefly differ by a collection
  cycle while a newer Zabbix value is awaiting export.

## Failure Behavior

Compose startup fails normally if a selected host port is already in use. The
README directs the operator to choose an unused port through the corresponding
environment variable and start the demo again. No automatic port discovery is
introduced because it would make the browser URLs harder to predict.

## Verification

Static verification expands and validates the Compose configuration, including
`internal: false`, the default loopback bindings, and overridden host ports.
Runtime verification:

1. Starts the complete demo with `make demo-up`.
2. Runs `make demo-verify` to prove that a fresh, positive
   `zabbix_demo_counter` sample reached VictoriaMetrics with the required
   labels.
3. Confirms that both published HTTP endpoints respond from the host.
4. Uses the documented Zabbix and VictoriaMetrics views to inspect the source
   item and exported series.

The existing verifier remains authoritative for the end-to-end metric contract;
UI availability checks supplement it without duplicating its data assertions.

## Non-Goals

- Exposing either UI to other machines.
- Adding TLS or authentication to the demo endpoints.
- Embedding a VictoriaMetrics datasource into Zabbix or building a combined
  dashboard.
- Requiring changes to `cmd/otelcol-zabbix/main.go`.
