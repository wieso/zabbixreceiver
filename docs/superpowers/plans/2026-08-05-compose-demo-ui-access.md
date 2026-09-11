# План реализации доступа к интерфейсам демонстрации Docker Compose

> Исторический материал. [Статус и указатель](../README.md) · [Актуальная документация](../../README.md). Не используйте как инструкцию для текущей версии.

> **Для агентов-исполнителей:** ОБЯЗАТЕЛЬНЫЙ ВСПОМОГАТЕЛЬНЫЙ НАВЫК: используйте superpowers:subagent-driven-development (рекомендуется) или superpowers:executing-plans для последовательной реализации задач этого плана. Для отслеживания шагов используется синтаксис флажков (`- [ ]`).

**Цель:** удалить упоминания прежнего поставщика, опубликовать демонстрационные интерфейсы Zabbix и VictoriaMetrics на настраиваемых портах петлевого интерфейса хоста, описать сравнение общего демонстрационного счётчика и оставить проверенный демонстрационный стек работающим.

**Архитектура:** сначала убрать устаревшие упоминания из документации, комментариев и соответствующей проверки упаковки, не меняя контракт приёмника. Затем использовать мост Compose с подключением к хосту и добавить развёрнутые записи `ports` только для `zabbix-web` и `victoriametrics`; привязать оба к `127.0.0.1`, подставлять настраиваемые порты хоста с постоянными значениями по умолчанию и сохранить существующую проверку как основной источник подтверждения контракта метрик.

**Технологии:** Docker Compose v2/v5, Go 1.25+, `gopkg.in/yaml.v3`, оболочка, curl, jq, Markdown

## Общие ограничения

- Привязывайте опубликованные демонстрационные интерфейсы к `127.0.0.1`, никогда к `0.0.0.0`.
- По умолчанию задайте порт Zabbix на хосте `8080` через `ZABBIX_WEB_PORT`.
- По умолчанию задайте порт VictoriaMetrics на хосте `8428` через `VICTORIAMETRICS_PORT`.
- Сохраните порты контейнеров и внутренние URL: Zabbix `8080`, VictoriaMetrics `8428`.
- Задайте общему мосту `demo` значение `internal: false`; сети с `internal: true` не могут активировать перенаправление портов хоста.
- Не меняйте код Collector, поведение приёмника, сопоставление метрик, логику начальной настройки или периодичность генератора данных.
- Удалите название прежнего поставщика, URL и заявления о совместимости из всех текущих файлов, сохранив все технические требования.
- `make demo-verify` остаётся основной проверкой контракта экспортируемых метрик.
- Оставьте успешно запущенный демонстрационный стек работающим для интерактивной проверки.

## Структура файлов

- Изменить `internal/packaging/assets_test.go`: проверять контракты публикации Compose и документации README.
- Изменить `docs/configuration.md`: описать реализованную конфигурацию без упоминания внешнего поставщика.
- Изменить `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md`: сохранить требования, используя терминологию проекта.
- Изменить `docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md`: сохранить подробности исторического плана, используя терминологию проекта.
- Изменить `internal/metrics/name.go`: описать именование метрик как совместимое с Prometheus.
- Изменить `compose.yaml`: опубликовать два порта интерфейсов с настраиваемой привязкой к петлевому интерфейсу.
- Изменить `README.md`: описать URL, учётные данные, навигацию, запрос, переопределения и задержку сбора.

---

### Задача 0: удалить упоминания прежнего поставщика

**Файлы:**
- Изменить: `docs/configuration.md:3`
- Изменить: `docs/configuration.md:42`
- Изменить: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:13`
- Изменить: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:32`
- Изменить: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:36`
- Изменить: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:45`
- Изменить: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:61`
- Изменить: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:123`
- Изменить: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:151`
- Изменить: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md:347`
- Изменить: `docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md:15`
- Изменить: `docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md:16`
- Изменить: `docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md:36`
- Изменить: `docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md:56`
- Изменить: `docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md:925`
- Изменить: `internal/metrics/name.go:9`
- Изменить: `internal/packaging/assets_test.go:45`

**Интерфейсы:**
- Зависимости: существующие публичные настройки приёмника, значения по умолчанию, правила валидации и сопоставление метрик.
- Результат: та же техническая документация и поведение во время работы без названия прежнего поставщика, URL и заявления о совместимости.

- [ ] **Шаг 1: зафиксировать исходное падение теста**

Выполните:

```bash
go test ./... -count=1
```

Ожидаемый результат: FAIL только в `TestDocumentationContract`, поскольку он всё ещё требует
ссылку на внешний источник, намеренно исключённую из README.

- [ ] **Шаг 2: заменить упоминание поставщика в документации конфигурации формулировкой проекта**

Задайте следующее начало `docs/configuration.md`:

```markdown
Публичный тип приёмника Collector — `zabbix`. Этот документ описывает
реализованный контракт приёмника; [утверждённый проект](superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md)
определяет границы его совместимости.
```

Задайте следующий абзац о неподдерживаемых настройках:

```markdown
`base.address`, отдельные конечные точки `/metrics` или `/health`, отдельные
флаги экспортёра и настройки экспортёра, специфичные для агента, не поддерживаются как ключи
приёмника. За эти возможности отвечает OpenTelemetry Collector.
```

- [ ] **Шаг 3: убрать упоминания из утверждённого проекта и исторического плана**

Используйте следующие точные формулировки для замены, сохранив окружающие списки,
таблицы и требования:

```text
Приёмник поддерживает следующий публичный интерфейс конфигурации:
Конфигурация экспортёра, специфичная для агента
Конфигурация по умолчанию, соответствующая документированным значениям приёмника
Приёмник рассчитан на Zabbix 5.0 или новее при условии доступности API-токенов в развёрнутой версии Zabbix.
Значения по умолчанию для приёмника:
соответствие публичному интерфейсу приёмника
Поддерживаемые настройки и значения по умолчанию декодируются, проходят валидацию и работают согласно спецификации.
Сохраните публичные ключи конфигурации приёмника `schedule`, `prom.prefix`, `prom.const_labels` и `zabbix`, а также документированные переопределения Zabbix через переменные окружения.
Исключите `base.address`, отдельные HTTP-точки экспортёра, отдельные флаги командной строки и конфигурацию, специфичную для агента.
Публичные типы конфигурации приёмника, значения по умолчанию, разрешение переменных окружения и валидация.
Формирование имён метрик, совместимых с Prometheus.
требовать ссылку на утверждённый проект и проверять наличие каждой поддерживаемой переменной окружения в `docs/configuration.md`.
```

Удалите устаревшую отдельную строку с внешней ссылкой из утверждённого
проекта. Сохраните все технические списки ключей и значения по умолчанию.

- [ ] **Шаг 4: обновить комментарий Go и устаревшую проверку упаковки**

Задайте следующий комментарий в `internal/metrics/name.go`:

```go
// Name возвращает совместимое с Prometheus имя метрики для ключа элемента данных.
```

Задайте следующую карту проверок ссылок README в `TestDocumentationContract`:

```go
	for name, link := range map[string]string{
		"approved design": "docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md",
	} {
```

- [ ] **Шаг 5: подтвердить отсутствие упоминаний и неизменность поведения**

Выполните:

```bash
rg -n -i 'as''tra' . --glob '!.git/**' --glob '!vendor/**'
go test ./... -count=1
git diff -- demo/collector.yaml.tmpl
git diff --check
```

Ожидаемый результат: поиск и сравнение шаблона ничего не выводят; тесты всех пакетов Go проходят;
проверка пробельных символов завершается с кодом ноль.

- [ ] **Шаг 6: закоммитить удаление упоминаний**

```bash
git add docs/configuration.md \
  docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md \
  docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md \
  internal/metrics/name.go internal/packaging/assets_test.go
git commit -m "docs: remove retired vendor attribution"
```

### Задача 1: опубликовать настраиваемые порты петлевого интерфейса

**Файлы:**
- Изменить: `internal/packaging/assets_test.go:279`
- Изменить: `internal/packaging/assets_test.go:928`
- Изменить: `compose.yaml:36`
- Изменить: `compose.yaml:90`

**Интерфейсы:**
- Зависимости: интерполяция Compose и существующие вспомогательные функции проверки YAML.
- Результат: `zabbix-web` на `127.0.0.1:${ZABBIX_WEB_PORT:-8080}` и `victoriametrics` на `127.0.0.1:${VICTORIAMETRICS_PORT:-8428}`.

- [ ] **Шаг 1: добавить падающие проверки контракта Compose**

Добавьте в `TestComposeTopologyContract`:

```go
	assertComposeLoopbackPort(t, services, "zabbix-web", "ZABBIX_WEB_PORT", 8080, 8080)
	assertComposeLoopbackPort(t, services, "victoriametrics", "VICTORIAMETRICS_PORT", 8428, 8428)
```

Добавьте рядом с остальными вспомогательными функциями Compose:

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

`strconv` уже импортирован.

- [ ] **Шаг 2: запустить целевой тест и убедиться в его падении**

Выполните:

```bash
go test ./internal/packaging -run '^TestComposeTopologyContract$' -count=1
```

Ожидаемый результат: FAIL, поскольку у `zabbix-web` нет ключа `ports`.

- [ ] **Шаг 3: добавить минимальные сопоставления портов Compose**

Добавьте в `zabbix-web` после проверки работоспособности:

```yaml
    ports:
      - target: 8080
        published: "${ZABBIX_WEB_PORT:-8080}"
        host_ip: 127.0.0.1
        protocol: tcp
```

Добавьте в `victoriametrics` после `volumes`:

```yaml
    ports:
      - target: 8428
        published: "${VICTORIAMETRICS_PORT:-8428}"
        host_ip: 127.0.0.1
        protocol: tcp
```

- [ ] **Шаг 4: отформатировать код и повторить целевой тест**

Выполните:

```bash
gofmt -w internal/packaging/assets_test.go
go test ./internal/packaging -run '^TestComposeTopologyContract$' -count=1
```

Ожидаемый результат: PASS.

- [ ] **Шаг 5: проверить раскрытие Compose со стандартными и переопределёнными значениями**

Выполните:

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

Ожидаемый результат: обе команды выводят `true` и завершаются с кодом ноль.

- [ ] **Шаг 6: закоммитить контракт портов и изменение Compose**

```bash
git add internal/packaging/assets_test.go compose.yaml
git commit -m "feat: expose demo UIs on loopback"
```

### Задача 1.5: включить перенаправление портов хоста на демонстрационном мосту

**Файлы:**
- Изменить: `internal/packaging/assets_test.go:318`
- Изменить: `compose.yaml:139`

**Интерфейсы:**
- Зависимости: сопоставления портов петлевого интерфейса из задачи 1 и общий мост `demo`.
- Результат: мост с подключением к хосту, активирующий только явно опубликованные порты петлевого интерфейса.

- [ ] **Шаг 1: изменить контракт сети, потребовав подключение к хосту**

В `TestComposeTopologyContract` замените существующую проверку сети на:

```go
	networks := mapValue(t, value(t, compose, "networks"))
	demoNetwork := mapValue(t, value(t, networks, "demo"))
	assertEqual(t, false, value(t, demoNetwork, "internal"))
```

- [ ] **Шаг 2: запустить целевой тест и убедиться в его падении**

Выполните:

```bash
go test ./internal/packaging -run '^TestComposeTopologyContract$' -count=1
```

Ожидаемый результат: FAIL, поскольку текущее значение YAML — `true`, а не `false`.

- [ ] **Шаг 3: явно подключить мост к хосту**

Измените объявление сети верхнего уровня в `compose.yaml` на:

```yaml
networks:
  demo:
    internal: false
```

Не добавляйте опубликованные порты внутренним сервисам.

- [ ] **Шаг 4: проверить целевой контракт и раскрытую модель Compose**

Выполните:

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

Ожидаемый результат: тест Go проходит, jq выводит `true`.

- [ ] **Шаг 5: закоммитить исправленный контракт сети**

```bash
git add internal/packaging/assets_test.go compose.yaml
git commit -m "fix: enable demo host port forwarding"
```

### Задача 2: описать и запустить демонстрацию для интерактивной проверки

**Файлы:**
- Изменить: `internal/packaging/assets_test.go:29`
- Изменить: `README.md:62`

**Интерфейсы:**
- Зависимости: адреса из задачи 1, учётные данные Zabbix только для Compose и существующие демонстрационные цели Make.
- Результат: инструкции по поиску `demo.counter` и выполнению запроса `zabbix_demo_counter{host="otel-demo-host",env="compose"}`.

- [ ] **Шаг 1: добавить падающие проверки контракта README**

После существующих проверок содержимого README в `TestDocumentationContract` добавьте:

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

Запрос использует необработанную строку Go, ограниченную обратными кавычками.

- [ ] **Шаг 2: запустить тест документации и убедиться в его падении**

Выполните:

```bash
go test ./internal/packaging -run '^TestDocumentationContract$' -count=1
```

Ожидаемый результат: FAIL из-за отсутствующих URL интерфейсов, учётных данных, запроса и имён переменных переопределения портов.

- [ ] **Шаг 3: добавить конкретные инструкции по интерфейсам в README**

Вставьте после трёх демонстрационных команд make:

````markdown
Оба веб-интерфейса опубликованы только на петлевом интерфейсе хоста:

- Откройте Zabbix по адресу <http://127.0.0.1:8080/> и войдите с учётными данными,
  используемыми только в Compose: `Admin` / `zabbix`. Откройте **Мониторинг → Последние данные (Monitoring → Latest data)**, выберите
  узел `otel-demo-host` и найдите элемент данных с ключом `demo.counter`.
- Откройте VictoriaMetrics VMUI по адресу <http://127.0.0.1:8428/vmui/> и выполните
  `zabbix_demo_counter{host="otel-demo-host",env="compose"}`.

Генератор увеличивает значение элемента данных Zabbix каждые пять секунд, а Collector
считывает значения каждые пять секунд. Поэтому оба интерфейса показывают один исходный
счётчик, хотя последние отображаемые значения могут ненадолго различаться на один
цикл сбора.

Чтобы обойти занятые порты хоста, выберите другие без изменения Compose:

```bash
ZABBIX_WEB_PORT=18080 VICTORIAMETRICS_PORT=18428 make demo-up
```

С этими переопределениями откройте `http://127.0.0.1:18080/` и
`http://127.0.0.1:18428/vmui/`. Оставляйте интерфейсы на петлевом адресе: демонстрация
использует общеизвестные учётные данные Zabbix и не настраивает аутентификацию
VictoriaMetrics.
````

- [ ] **Шаг 4: выполнить целевые и статические проверки**

Выполните:

```bash
go test ./internal/packaging -run '^(TestDocumentationContract|TestComposeTopologyContract)$' -count=1
docker compose config --quiet
git diff --check
```

Ожидаемый результат: все команды завершаются с кодом ноль.

- [ ] **Шаг 5: выбрать свободные порты и запустить реальную демонстрацию**

Проверьте порты по умолчанию:

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
lsof -nP -iTCP:8428 -sTCP:LISTEN
```

Пересоздайте контейнеры и сеть, не удаляя именованные тома:

```bash
docker compose down --remove-orphans
make demo-up
```

Если любой порт по умолчанию занят, выберите свободные значения и сохраняйте одни и те же
присваивания в обеих командах и во всех последующих командах Compose, например:

```bash
ZABBIX_WEB_PORT=18080 VICTORIAMETRICS_PORT=18428 docker compose down --remove-orphans
ZABBIX_WEB_PORT=18080 VICTORIAMETRICS_PORT=18428 make demo-up
```

Ожидаемый результат: именованные тома сохраняются; сеть `demo` пересоздаётся с
`internal=false`; PostgreSQL, сервер Zabbix и веб-интерфейс Zabbix проходят проверку работоспособности;
начальная настройка завершается; генератор, VictoriaMetrics и Collector продолжают
работать.

- [ ] **Шаг 6: проверить метрику и оба адреса хоста**

С портами по умолчанию выполните:

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

При переопределении подставьте выбранные порты хоста. Ожидаемый результат: проверка выводит один
принятый временной ряд, оба запроса к интерфейсам завершаются с кодом ноль, а проверка API выводит
`true`.

- [ ] **Шаг 7: проверить оба интерфейса в браузере и оставить стек работающим**

Откройте документированные URL. В Zabbix убедитесь, что у
`otel-demo-host` / `demo.counter` есть свежее растущее значение. В VMUI
выполните документированный запрос и подтвердите `item_key="demo.counter"` и свежее
положительное значение. Дождитесь одного цикла сбора для сближения значений.

Не запускайте `make demo-down`: оператор запросил интерактивную демонстрацию.

- [ ] **Шаг 8: закоммитить документацию и её контракт**

```bash
git add internal/packaging/assets_test.go README.md
git commit -m "docs: explain demo UI comparison"
```
