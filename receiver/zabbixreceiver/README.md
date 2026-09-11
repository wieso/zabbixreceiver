# Приёмник Zabbix

Приёмник `zabbix` собирает числовые элементы данных Zabbix через опрос API или HTTP Streaming и выдаёт метрики OpenTelemetry типа gauge.

## Collector Builder

```yaml
receivers:
  - gomod: github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.2.0
```

Текущая рабочая копия готовится к v0.2.0 и проверяется с Collector/Contrib v0.160.0 и стабильными модулями Collector v1.66.0; требуется Go 1.26.8 или новее. Пример выше станет доступен после публикации тега `receiver/zabbixreceiver/v0.2.0`. До публикации используйте `make verify-ocb-local` из корня репозитория.

Опубликованный модуль v0.1.0 поддерживает опрос API и проверен с Collector/Contrib v0.154.0 и стабильными модулями v1.60.0. Streaming и описанные ниже расширения относятся к v0.2.0.

## Конфигурация запуска

```yaml
receivers:
  zabbix:
    zabbix:
      url: ${env:ZABBIX_URL}
      token: ${env:ZABBIX_TOKEN}
```

Добавьте приёмник в конвейер метрик. Полная схема и оба режима описаны в [справочнике настройки](../../docs/configuration.md). Примеры: [конфигурация Builder](../../examples/ocb/builder-config.yaml), [конфигурация запуска API](../../examples/ocb/otelcol.yaml), [конфигурация Streaming](../../configs/otelcol-streaming.yaml).

## Метаданные

По умолчанию передаются теги элементов и хостов, наследуемые теги хостов, группы и названия. Inventory запрашивается по списку `metadata.inventory_fields`. Streaming читает метаданные payload и опционально обогащается через API (`streaming.enrich_with_api: true`). [Контракт лейблов и правила кодирования](../../docs/metadata.md), [полный пример обогащения](../../configs/otelcol-streaming-enriched.yaml).

## Observability

Логи ресивера управляются через `service.telemetry.logs`; метрики публикуются штатным Prometheus reader Collector на `/metrics` (порт 8888 в примерах). Доступны стандартные accepted/refused points, циклы и ошибки сбора, длительности, размеры discovery, время последнего успеха и HTTP-статусы Streaming. Полная семантика, настройка логов и PromQL — в [разделе внутренней телеметрии](../../docs/configuration.md).

## Разработка

Запускайте `go test ./...`, `go test -race ./...` и `go vet ./...` из этого каталога. Проверка интеграции с Builder и команды для обоих модулей описаны в [руководстве разработчика](../../docs/development.md).
