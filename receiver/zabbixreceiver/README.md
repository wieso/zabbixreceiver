# Приёмник Zabbix

Приёмник `zabbix` собирает числовые элементы данных Zabbix через опрос API или HTTP Streaming и выдаёт метрики OpenTelemetry типа gauge.

## Collector Builder

```yaml
receivers:
  - gomod: github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.3.4
```

Версия v0.3.4 проверяется с Collector/Contrib v0.160.0 и стабильными модулями Collector v1.66.0; требуется Go 1.26.8 или новее. Пример требует опубликованного тега `receiver/zabbixreceiver/v0.3.4`. Для проверки рабочей копии используйте `make verify-ocb-local` из корня репозитория.

Опубликованный модуль v0.1.0 поддерживает опрос API и проверен с Collector/Contrib v0.154.0 и стабильными модулями v1.60.0. Streaming, метаданные и телеметрия доступны с v0.2.0; v0.3.0 унифицирует формат метрик API/Streaming и меняет имена и лейблы. Правила перехода — в [заметках к выпуску](../../docs/releases/v0.3.0.md).

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

По умолчанию передаются теги элементов и хостов, наследуемые теги хостов, группы и названия. Inventory запрашивается по списку `metadata.inventory_fields`. Streaming читает метаданные payload и опционально обогащается через API (`streaming.enrich_with_api: true`). [Контракт лейблов и правила нормализации](../../docs/metadata.md), [полный пример обогащения](../../configs/otelcol-streaming-enriched.yaml).

## Observability

Логи ресивера управляются через `service.telemetry.logs`; метрики публикуются штатным Prometheus reader Collector на `/metrics` (порт 8888 в примерах). Доступны стандартные accepted/refused points, циклы и ошибки сбора, длительности, размеры discovery, время последнего успеха и HTTP-статусы Streaming. Полная семантика, настройка логов и PromQL — в [разделе внутренней телеметрии](../../docs/configuration.md#внутренняя-телеметрия-приёмника). Отдельно контролируйте [успех и ошибки exporter, включая 401/403 от VM](../../docs/configuration.md#отправка-в-victoriametrics-и-ошибки-авторизации): приём receiver не подтверждает запись в хранилище.

## Разработка

Запускайте `go test ./...`, `go test -race ./...` и `go vet ./...` из этого каталога. Проверка интеграции с Builder и команды для обоих модулей описаны в [руководстве разработчика](../../docs/development.md).

С v0.3.1 названия групп передаются обычным значением `host_groups` вместо `host_group_*="true"`. Подробнее — в [заметках к выпуску v0.3.1](../../docs/releases/v0.3.1.md).

С v0.3.3 `metadata.host_groups_format` выбирает `names`, `flags` или `both`. С v0.3.4 по умолчанию используется `both`; в v0.3.3 — `names`. См. [заметки к выпуску v0.3.4](../../docs/releases/v0.3.4.md).
