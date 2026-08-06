# Vendor Attribution Cleanup Design

Date: 2026-08-05

## Objective

Remove the retired vendor name, its documentation URL, and all claims that the
receiver is compatible with that vendor's product. Preserve every implemented
configuration key, default, validation rule, metric-name rule, and runtime
behavior.

## Scope

The cleanup covers every current tracked text file, including historical
design and implementation-plan documents. It updates:

- `docs/configuration.md`
- `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md`
- `docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md`
- `internal/metrics/name.go`
- `internal/packaging/assets_test.go`

The Compose UI work remains a separate functional change. Files with no
retired attribution, including `demo/collector.yaml.tmpl`, remain unchanged by
this cleanup.

## Replacement Language

Documentation describes the implemented settings and defaults as the project's
public receiver contract. Metric-name construction is described as
Prometheus-compatible. Unsupported process-level settings are described by
their technical role without naming another product.

Historical design and plan documents retain their technical requirements but
use the same neutral terminology. The cleanup must not imply that configuration
keys, defaults, or behavior have changed.

## Test Contract

`TestDocumentationContract` stops requiring the retired external source link.
No replacement external link is introduced. Existing assertions for the
project design, configuration variables, validation behavior, metric delivery,
deployment, and security remain intact.

## Verification

1. Run a case-insensitive repository scan for the retired vendor name and
   require no matches in current files.
2. Run `go test ./... -count=1` and require all packages to pass.
3. Inspect the diff and confirm that the cleanup changes only prose, a Go
   comment, and the obsolete external-link assertion.
4. Confirm that `demo/collector.yaml.tmpl` has no diff.

## Non-Goals

- Removing or renaming supported configuration fields or environment
  variables.
- Changing default values, validation, scheduling, metrics, authentication, or
  exporter behavior.
- Editing the demo Collector configuration.
- Replacing the retired attribution with a different vendor attribution.
