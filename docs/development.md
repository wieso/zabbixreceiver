# Разработка и выпуск

[Карта документации](README.md) · [Архитектура](architecture.md)

Нужны Go 1.26.8 или новее и Make. Зависимости закреплены в [корневом go.mod](../go.mod) и [go.mod приёмника](../receiver/zabbixreceiver/go.mod). Команды ниже выполняются из корня репозитория; `make download` загружает зависимости обоих модулей.

## Проверки

Стандартный набор локальных проверок:

```bash
make fmt
git diff --check
make test
make test-race
make vet
make vulncheck
make tidy
make build
make validate-config
make compose-config
make verify-ocb-local
docker build -t zabbix-otel-collector:verify .
docker run --rm zabbix-otel-collector:verify components
```

Стандартные цели Make охватывают корневой модуль и модуль приёмника. Go-тесты используют локальные HTTP-серверы на loopback. В песочницах, запрещающих открытие loopback-портов, полный набор не запустится; используйте хост или исполнитель CI, разрешающий локальные серверы. Для проверок контейнеров, Compose и Kubernetes нужны соответствующие инструменты и службы. Сквозная демонстрация с несколькими контейнерами намеренно оформлена как ручная задача GitHub Actions `workflow_dispatch`, а не как проверка push/PR.

Простой `go test ./...` из корня не включает вложенный модуль. Для проверки только приёмника выполняйте команды из [его каталога](../receiver/zabbixreceiver/README.md).

| Область изменения | Где искать проверку |
| --- | --- |
| Значения по умолчанию и переменные окружения | `config_test.go`, `factory_test.go`, `cmd/otelcol-zabbix/config_validation_test.go` |
| JSON-RPC и аутентификация | `receiver/zabbixreceiver/internal/zabbix/client_test.go` |
| Фильтры и снимки | `receiver/zabbixreceiver/internal/discovery/*_test.go` |
| Общие имена, значения и лейблы API/Streaming | `receiver/zabbixreceiver/internal/metrics/*_test.go`, `receiver/zabbixreceiver/representation_test.go`, `receiver/zabbixreceiver/metadata_test.go` |
| Расписание и жизненный цикл | `scheduler_test.go`, `receiver_test.go` в модуле приёмника |
| HTTP Streaming | `receiver/zabbixreceiver/streaming_test.go` |
| Внутренняя телеметрия, безопасные логи и трассировки | `receiver/zabbixreceiver/telemetry_test.go`, `receiver/zabbixreceiver/observability_test.go`, `cmd/otelcol-zabbix/observability_test.go` |
| Bearer VM: 401/403 и восстановление экспорта в API/Streaming | `cmd/otelcol-zabbix/exporter_auth_test.go` |
| Документация и поставка | `internal/packaging/assets_test.go` |
| Реальный путь от Zabbix до хранилища | `make demo-up`, `make demo-verify`, `make demo-down` |

В строках с короткими именами файлов подразумевается каталог `receiver/zabbixreceiver/`.

## Интеграция с Collector Builder

Подключение опубликованного модуля и совместимые версии описаны в [README приёмника](../receiver/zabbixreceiver/README.md). Конфигурация Builder управляет компиляцией; конфигурация Collector — работой программы.

`make verify-ocb-local` использует [локальный манифест](../testdata/ocb/builder-config.yaml), проверяет регистрацию `zabbix` и валидирует [пример запуска](../examples/ocb/otelcol.yaml). [Публичный манифест](../examples/ocb/builder-config.yaml) подключает v0.3.3 без локального `replace` и требует опубликованного тега модуля.

## CI и релизы

[CI](../.github/workflows/ci.yml) проверяет оба модуля, форматирование, зависимости, тесты, гонки данных, vet, сборку, конфигурацию, OCB, упаковку, Compose и контейнер. Сквозная демонстрация запускается вручную через `workflow_dispatch`. CI и release используют Go 1.26.8 и проверяют достижимые уязвимости обоих модулей командой `make vulncheck` (govulncheck v1.8.0). В CI сканируется также бинарник из собранного контейнера. Скрипт упаковки дополнительно сканирует каждый целевой бинарник до создания архива; ошибка сканера или недоступность базы блокирует выпуск. Версия инструмента задаётся `GOVULNCHECK_VERSION`; обновляйте её вместе с проверками. Системный Go не требуется переустанавливать, если включён стандартный механизм загрузки toolchain: `go.mod` требует минимум 1.26.8.

[Процесс выпуска](../.github/workflows/release.yml) запускается по тегам `v*.*.*` и вызывает [скрипт упаковки](../scripts/package-release.sh). Для локальной подготовки используйте `make release-artifacts VERSION=vX.Y.Z`, заменив `X.Y.Z` числовой версией. Команда очищает `dist/`, создаёт архивы Linux amd64/arm64 и `checksums.txt`, проверяет состав и архитектуру файлов. Архивы включают исполняемый файл, README, примеры API/Streaming и руководства configuration/metadata/deployment; остальные материалы доступны в репозитории.

Теги готовой сборки (`vX.Y.Z`) и вложенного модуля (`receiver/zabbixreceiver/vX.Y.Z`) независимы. Не считайте наличие возможности в рабочей копии подтверждением её публикации. Порядок установки архивов приведён в [руководстве по развёртыванию](deployment.md).

Релизные бинарники и Docker-образ собираются с `-w`, но без `-s`: таблица символов сохраняется для точного `govulncheck -mode=binary`. Это увеличивает локально измеренный Linux amd64 файл примерно с 35 до 40 МиБ. Полная DWARF-отладочная информация удаляется. У stripped-бинарников govulncheck переходит к консервативному анализу списка модулей и может сообщать о пакетах, которые не вошли в исполняемый код. Проверка исходников остаётся обязательной наряду с проверкой артефактов.

## Порядок выпуска v0.3.3

[Заметки к выпуску](releases/v0.3.3.md) подготовлены для GitHub Release. Публикация считается завершённой после проверки обоих тегов и артефактов GitHub.

1. Отправьте PR в `main` и дождитесь успешного CI. После review объедините PR.
2. Получите итоговый коммит из `origin/main` и убедитесь, что локальный `HEAD` совпадает с ним, а рабочая копия чистая. При squash используйте коммит после merge, а не исходный коммит PR.
3. Создайте оба новых тега на этом коммите. Сначала опубликуйте тег вложенного модуля и проверьте его доступность:

   ```bash
   git tag -a receiver/zabbixreceiver/v0.3.3 -m "Zabbix receiver v0.3.3"
   git push origin refs/tags/receiver/zabbixreceiver/v0.3.3
   go mod download github.com/wieso/zabbixreceiver/receiver/zabbixreceiver@v0.3.3
   go run go.opentelemetry.io/collector/cmd/builder@v0.160.0 --skip-strict-versioning=false --config examples/ocb/builder-config.yaml
   ZABBIX_URL=http://zabbix.example/api_jsonrpc.php ZABBIX_TOKEN=dummy-token ./otelcol-zabbix-dist/otelcol-zabbix validate --config examples/ocb/otelcol.yaml
   ```

4. После успешной проверки опубликуйте тег сборки. Его push автоматически запускает Release и публикацию архивов:

   ```bash
   git tag -a v0.3.3 -m "otelcol-zabbix v0.3.3"
   git push origin refs/tags/v0.3.3
   ```

5. Дождитесь успешного Release в GitHub Actions. На странице выпуска проверьте два Linux-архива и `checksums.txt`, скачайте их и выполните `shasum -a 256 -c checksums.txt`. Добавьте подготовленные заметки к автоматически сформированному описанию выпуска.

Не перемещайте существующие теги и не заменяйте опубликованные артефакты. Если тег уже существует, сначала проверьте его коммит и состояние соответствующего workflow.
