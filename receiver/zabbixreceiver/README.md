# Zabbix Receiver

The `zabbix` receiver polls numeric Zabbix items and emits OpenTelemetry gauge metrics.

## Collector Builder

```yaml
receivers:
  - gomod: github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.1.0
```

This release is tested with Collector/Contrib v0.154.0 and stable Collector modules v1.60.0. The repository tag is `receiver/zabbixreceiver/v0.1.0`.

## Runtime configuration

```yaml
receivers:
  zabbix:
    zabbix:
      url: ${env:ZABBIX_URL}
      token: ${env:ZABBIX_TOKEN}
```

Add the receiver to a metrics pipeline. See `../../docs/configuration.md` for the full schema and `../../examples/ocb` for complete build-time and runtime examples.

## Development

Run `go test ./...`, `go test -race ./...`, and `go vet ./...` from this directory. From the repository root, `make verify-ocb-local` verifies the complete Builder integration.
