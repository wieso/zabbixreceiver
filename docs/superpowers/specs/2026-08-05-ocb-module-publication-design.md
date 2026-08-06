# OCB Module Publication Design

Date: 2026-08-05

## Objective

Publish the Zabbix receiver at `https://github.com/wieso/zabbixreceiver` as a
versioned, independently consumable OpenTelemetry Collector Builder component.
The remote `main` branch must contain exactly one root commit and no prior
project history.

The supported integration target is OpenTelemetry Collector and Collector
Contrib `v0.154.0`, including the stable Collector modules at `v1.60.0`. The
first receiver module release is `v0.1.0`.

## Public Module Contract

The receiver is an independent nested Go module at
`receiver/zabbixreceiver` with this module path:

```text
github.com/wieso/zabbixreceiver/receiver/zabbixreceiver
```

This follows the component-level module structure used by Collector Contrib.
The module is published with the subdirectory tag:

```text
receiver/zabbixreceiver/v0.1.0
```

Users add it to an OCB manifest with no explicit `import` or local `path`:

```yaml
receivers:
  - gomod: github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.1.0
```

The module exports the existing `zabbixreceiver.NewFactory` function, and the
Collector component type remains `zabbix`. The receiver YAML schema, defaults,
environment overrides, scheduling, metric mapping, authentication, and runtime
failure behavior do not change.

## Repository and Module Layout

The existing receiver source remains in `receiver/zabbixreceiver`. Its private
implementation packages move under the nested module so the published module
has no dependency on the repository's root module:

```text
receiver/zabbixreceiver/
  go.mod
  go.sum
  factory.go
  config.go
  receiver.go
  scheduler.go
  telemetry.go
  internal/
    discovery/
    metrics/
    zabbix/
```

The root module remains responsible for the ready-made Collector distribution,
Docker image, Compose demo, deployment assets, and repository-wide packaging
tests. Its module path changes to `github.com/wieso/zabbixreceiver`, and it
requires the nested receiver module with a repository-local `replace`:

```go
require github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.1.0

replace github.com/wieso/zabbixreceiver/receiver/zabbixreceiver => ./receiver/zabbixreceiver
```

The root distribution imports the receiver through its published module path.
No dependency points back from the receiver module to the root module, so the
two modules do not form a cycle.

## Versions and Licensing

The nested module uses Go 1.25 or newer, Collector `v0.154.0` modules, stable
Collector `v1.60.0` modules, and the corresponding OpenTelemetry Go dependency
versions selected by `go mod tidy`. The root distribution is upgraded to the
same Collector/Contrib compatibility line.

The repository is published under Apache License 2.0 with a root `LICENSE`
file. Documentation identifies `v0.154.0` as the tested Collector compatibility
line rather than promising compatibility with every older or newer release.

## Documentation and Examples

The root README and a component-focused `receiver/zabbixreceiver/README.md`
document:

- the exact OCB `gomod` entry;
- the required Go and Collector versions;
- a minimal `zabbix` receiver configuration;
- the distinction between build-time OCB configuration and runtime Collector
  configuration;
- the subdirectory release-tag convention;
- how to run the repository's verification targets.

`examples/ocb/builder-config.yaml` is a copy-ready manifest that resolves the
published `v0.1.0` module. `examples/ocb/otelcol.yaml` is a minimal runtime
metrics pipeline using the `zabbix` receiver and a standard debug exporter.
The example contains only non-secret placeholders and does not contact Zabbix
during configuration validation.

A separate repository-local OCB fixture adds
`path: ./receiver/zabbixreceiver`. It exists only to verify the unpublished
working tree and is clearly separated from the copy-ready published example.

## Verification Contract

Verification covers the module boundary as well as receiver behavior:

1. Run formatting, tests, race tests, and `go vet` for the nested receiver
   module.
2. Run formatting, tests, race tests, `go vet`, build, and existing packaging
   checks for the root module.
3. Confirm that `go mod tidy` leaves both `go.mod` and `go.sum` files unchanged.
4. Run OpenTelemetry Collector Builder `v0.154.0` against the local OCB fixture.
5. Run the generated binary's `components` command and require the `zabbix`
   receiver to be present.
6. Run the generated binary's `validate` command against the minimal runtime
   configuration with placeholder credentials. Validation must succeed without
   making a Zabbix request.
7. After publication, repeat the Builder build in a clean temporary environment
   using `examples/ocb/builder-config.yaml`, with no `replace`, local `path`, or
   pre-populated module cache providing the receiver.

Any failed test, version conflict, missing component, invalid configuration, or
remote module resolution failure blocks publication or completion.

## Publication Procedure

Immediately before publication, query the remote again. If it has gained any
branch or tag, stop without overwriting it.

Local implementation commits may be used while preparing and reviewing the
project. Before publication, preserve the old local history in an unpushed
backup branch, create a new orphan `main` from the verified final tree, and
make exactly one root commit with this subject:

```text
feat: publish Zabbix OpenTelemetry receiver
```

Push only `main` and `receiver/zabbixreceiver/v0.1.0`. Do not push the backup
branch, previous branches, previous tags, or all refs. The module tag points to
the single root commit and does not introduce another commit.

After the push, use a clean clone to require all of the following:

- `git rev-list --count HEAD` returns `1`;
- `HEAD` has no parent;
- `receiver/zabbixreceiver/v0.1.0` resolves to `HEAD`;
- the remote-only OCB verification succeeds.

## Non-Goals

- Contributing the receiver to the upstream Collector Contrib repository.
- Publishing prebuilt release binaries or container images.
- Changing receiver configuration or collection behavior.
- Claiming compatibility beyond Collector/Contrib `v0.154.0`.
- Publishing the repository's existing local commit history.
