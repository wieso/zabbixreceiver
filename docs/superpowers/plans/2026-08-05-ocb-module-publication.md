# План реализации публикации модуля OCB

> Исторический материал. [Статус и указатель](../README.md) · [Актуальная документация](../../README.md). Не используйте как инструкцию для текущей версии.

> **Для агентов-исполнителей:** ОБЯЗАТЕЛЬНЫЙ ВСПОМОГАТЕЛЬНЫЙ НАВЫК: используйте superpowers:subagent-driven-development (рекомендуется) или superpowers:executing-plans для последовательной реализации задач этого плана. Для отслеживания шагов используется синтаксис флажков (`- [ ]`).

**Цель:** опубликовать существующий приёмник Zabbix как компонент Collector Builder с независимыми версиями по пути `github.com/wieso/zabbixreceiver/receiver/zabbixreceiver`, сохранив готовую сборку репозитория и ровно один коммит в удалённом репозитории.

**Архитектура:** `receiver/zabbixreceiver` становится вложенным модулем Go и включает три используемых им внутренних пакета. Корневой модуль Go сохраняет исполняемую сборку и подключает приёмник через локальный `replace`. Локальный и удалённый манифесты OCB в репозитории проверяют как неопубликованную рабочую область, так и опубликованный тег подмодуля `v0.1.0`.

**Технологии:** Go 1.25+, OpenTelemetry Collector и Collector Contrib v0.154.0, стабильные модули Collector v1.60.0, OpenTelemetry Go v1.44.0, Collector Builder v0.154.0, Git, GitHub, Docker, GitHub Actions.

## Общие ограничения

- Публичный путь модуля приёмника — строго `github.com/wieso/zabbixreceiver/receiver/zabbixreceiver`.
- Публичный тип компонента приёмника и фабрика остаются `zabbix` и `zabbixreceiver.NewFactory`.
- Первая версия модуля — `v0.1.0`, опубликованная с тегом `receiver/zabbixreceiver/v0.1.0`.
- Проверенная линия совместимости — Collector/Contrib `v0.154.0` со стабильными модулями Collector `v1.60.0`.
- Сохраните все YAML-ключи приёмника, значения по умолчанию, переопределения через переменные окружения, правила расписания и сопоставления метрик, поведение аутентификации и поведение при сбоях во время работы.
- Вложенный модуль приёмника не должен зависеть от корневого модуля `github.com/wieso/zabbixreceiver`.
- Лицензия репозитория — Apache License 2.0.
- Удалённая ветка `main` должна содержать ровно один корневой коммит без родительского коммита.
- Отправляйте только удалённую ветку `main` и тег `receiver/zabbixreceiver/v0.1.0`; не отправляйте локальную резервную историю или посторонние ссылки Git.
- Если непосредственно перед публикацией целевой удалённый репозиторий уже не пуст, остановитесь, не перезаписывая его.

---

### Задача 1: создать независимый модуль приёмника

**Файлы:**
- Создать: `receiver/zabbixreceiver/go.mod`
- Создать: `receiver/zabbixreceiver/go.sum`
- Переместить: `internal/zabbix/*.go` в `receiver/zabbixreceiver/internal/zabbix/*.go`
- Переместить: `internal/discovery/*.go` в `receiver/zabbixreceiver/internal/discovery/*.go`
- Переместить: `internal/metrics/*.go` в `receiver/zabbixreceiver/internal/metrics/*.go`
- Изменить: `receiver/zabbixreceiver/factory.go`
- Изменить: `receiver/zabbixreceiver/receiver.go`
- Изменить: `receiver/zabbixreceiver/receiver_test.go`
- Изменить: `receiver/zabbixreceiver/telemetry_test.go`
- Изменить: `receiver/zabbixreceiver/internal/discovery/select.go`
- Изменить: `receiver/zabbixreceiver/internal/discovery/select_test.go`
- Изменить: `receiver/zabbixreceiver/internal/metrics/builder.go`
- Изменить: `receiver/zabbixreceiver/internal/metrics/builder_test.go`
- Изменить: `cmd/otelcol-zabbix/components.go`
- Изменить: `go.mod`
- Изменить: `go.sum`
- Тест: `internal/packaging/assets_test.go`

**Интерфейсы:**
- Результат: модуль Go `github.com/wieso/zabbixreceiver/receiver/zabbixreceiver` с исходным кодом, совместимым с версией `v0.1.0`.
- Результат: `func NewFactory() receiver.Factory` в корне вложенного модуля.
- Зависимости: API компонентов, конфигурации, потребителей, pdata и приёмников Collector версии `v1.60.0`; `receivertest` версии `v0.154.0`.
- Сохраняется: импорт `zabbixreceiver.NewFactory` корневой сборкой через локальную замену модуля.

- [ ] **Шаг 1: написать падающий тест контракта границ модуля**

Добавьте этот тест в `internal/packaging/assets_test.go`:

```go
func TestReceiverModuleContract(t *testing.T) {
	module := readAsset(t, "receiver/zabbixreceiver/go.mod")
	for _, want := range []string{
		"module github.com/wieso/zabbixreceiver/receiver/zabbixreceiver",
		"go 1.25.0",
		"go.opentelemetry.io/collector/component v1.60.0",
		"go.opentelemetry.io/collector/receiver v1.60.0",
		"go.opentelemetry.io/collector/receiver/receivertest v0.154.0",
	} {
		if !strings.Contains(module, want) {
			t.Errorf("receiver go.mod missing %q", want)
		}
	}
	if strings.Contains(module, "github.com/wieso/zabbixreceiver v") {
		t.Error("receiver module must not depend on the repository root module")
	}

	for _, path := range []string{
		"receiver/zabbixreceiver/internal/zabbix/client.go",
		"receiver/zabbixreceiver/internal/discovery/select.go",
		"receiver/zabbixreceiver/internal/metrics/builder.go",
	} {
		if _, err := os.Stat(filepath.Join("..", "..", path)); err != nil {
			t.Errorf("receiver-private package %q is unavailable: %v", path, err)
		}
	}
}
```

- [ ] **Шаг 2: запустить тест контракта и подтвердить его падение**

Выполните:

```bash
go test ./internal/packaging -run TestReceiverModuleContract -count=1
```

Ожидаемый результат: FAIL, поскольку `receiver/zabbixreceiver/go.mod` не существует.

- [ ] **Шаг 3: создать манифест вложенного модуля**

Создайте `receiver/zabbixreceiver/go.mod` с этим блоком прямых зависимостей; `go mod tidy` добавит точный блок транзитивных зависимостей и `go.sum`:

```go
module github.com/wieso/zabbixreceiver/receiver/zabbixreceiver

go 1.25.0

require (
	github.com/stretchr/testify v1.11.1
	go.opentelemetry.io/collector/component v1.60.0
	go.opentelemetry.io/collector/config/configopaque v1.60.0
	go.opentelemetry.io/collector/consumer v1.60.0
	go.opentelemetry.io/collector/pdata v1.60.0
	go.opentelemetry.io/collector/receiver v1.60.0
	go.opentelemetry.io/collector/receiver/receivertest v0.154.0
	go.opentelemetry.io/otel/metric v1.44.0
	go.opentelemetry.io/otel/sdk/metric v1.44.0
	go.uber.org/zap v1.28.0
)
```

- [ ] **Шаг 4: переместить внутренние пакеты во вложенный модуль**

Перемещайте файлы с сохранением содержимого, чтобы Git распознал переименования:

```text
internal/zabbix      -> receiver/zabbixreceiver/internal/zabbix
internal/discovery   -> receiver/zabbixreceiver/internal/discovery
internal/metrics     -> receiver/zabbixreceiver/internal/metrics
```

Не перемещайте `internal/packaging`: он проверяет ресурсы уровня репозитория и остаётся в корневом модуле.

- [ ] **Шаг 5: переписать импорты внутренних пакетов приёмника**

Во всех перемещённых пакетах, исходном коде и тестах приёмника замените три старых импорта эквивалентами из вложенного модуля:

```go
"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/discovery"
otelmetrics "github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/metrics"
"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
```

Псевдоним `otelmetrics` остаётся только там, где локальное имя пакета иначе было бы неоднозначным.

- [ ] **Шаг 6: подключить корневую сборку к публичному модулю приёмника**

Измените импорт в `cmd/otelcol-zabbix/components.go` на:

```go
"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver"
```

Измените строку модуля в корневом `go.mod` на:

```go
module github.com/wieso/zabbixreceiver
```

Обновите прямые зависимости до линии v0.154.0/v1.60.0 и включите:

```go
require (
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter v0.154.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension v0.154.0
	github.com/stretchr/testify v1.11.1
	github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.1.0
	go.opentelemetry.io/collector/component v1.60.0
	go.opentelemetry.io/collector/confmap/provider/envprovider v1.60.0
	go.opentelemetry.io/collector/confmap/provider/fileprovider v1.60.0
	go.opentelemetry.io/collector/otelcol v0.154.0
	go.opentelemetry.io/collector/processor v1.60.0
	go.opentelemetry.io/collector/processor/batchprocessor v0.154.0
	go.opentelemetry.io/collector/processor/memorylimiterprocessor v0.154.0
	go.opentelemetry.io/collector/service v0.154.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/wieso/zabbixreceiver/receiver/zabbixreceiver => ./receiver/zabbixreceiver
```

- [ ] **Шаг 7: разрешить оба графа модулей**

Выполните:

```bash
cd receiver/zabbixreceiver
go mod tidy
cd ../..
go mod tidy
```

Ожидаемый результат: обе команды завершаются успешно; вложенный `go.mod` не содержит зависимости от `github.com/wieso/zabbixreceiver`.

- [ ] **Шаг 8: запустить целевые и полные тесты модулей**

Выполните:

```bash
cd receiver/zabbixreceiver
go test ./... -count=1
cd ../..
go test ./internal/packaging -run TestReceiverModuleContract -count=1
go test ./... -count=1
```

Ожидаемый результат: PASS. Корневой обход `./...` останавливается на границе вложенного модуля, а явный запуск во вложенном модуле охватывает тесты приёмника и внутренних пакетов.

- [ ] **Шаг 9: убедиться, что устаревшие пути импорта удалены**

Выполните:

```bash
rg -n 'github\.com/aleksandr/zabbix-otel|github\.com/wieso/zabbixreceiver/internal/' --glob '*.go' --glob 'go.mod'
```

Ожидаемый результат: вывод отсутствует.

- [ ] **Шаг 10: закоммитить разделение модулей**

```bash
git add -A -- go.mod go.sum cmd/otelcol-zabbix/components.go receiver/zabbixreceiver internal/packaging/assets_test.go internal/zabbix internal/discovery internal/metrics
git commit -m "refactor: publish receiver as nested Go module"
```

Ожидаемый результат: удалённые пути внутренних пакетов корня и добавленные пути вложенного модуля зафиксированы, желательно как переименования. Этот локальный коммит позже будет включён в корневой коммит публикации.

---

### Задача 2: адаптировать всю автоматизацию сборки к нескольким модулям

**Файлы:**
- Изменить: `internal/packaging/assets_test.go`
- Изменить: `Makefile`
- Изменить: `Dockerfile`
- Изменить: `.github/workflows/ci.yml`

**Интерфейсы:**
- Результат: `make download`, `make tidy`, `make test`, `make test-race` и `make vet`, охватывающие оба модуля.
- Результат: сборка Docker, в которой вложенный манифест доступен до загрузки зависимостей корня.
- Результат: кеш CI и шаги проверки, учитывающие оба файла `go.sum`.

- [ ] **Шаг 1: написать падающие проверки автоматизации для двух модулей**

Добавьте этот тест в `internal/packaging/assets_test.go`:

```go
func TestDualModuleAutomationContract(t *testing.T) {
	makefile := readAsset(t, "Makefile")
	for _, want := range []string{
		"download:",
		"tidy:",
		"cd receiver/zabbixreceiver && go mod download",
		"cd receiver/zabbixreceiver && go mod tidy",
		"cd receiver/zabbixreceiver && go test ./... -count=1",
		"cd receiver/zabbixreceiver && go test -race ./... -count=1",
		"cd receiver/zabbixreceiver && go vet ./...",
	} {
		if !strings.Contains(makefile, want) {
			t.Errorf("Makefile missing dual-module command %q", want)
		}
	}

	workflow := readAsset(t, ".github/workflows/ci.yml")
	if !strings.Contains(workflow, "receiver/zabbixreceiver/go.sum") {
		t.Error("CI cache must include the receiver module go.sum")
	}
}
```

Дополните `TestDockerfileContract` следующим кодом:

```go
if !strings.Contains(dockerfile, "COPY receiver/zabbixreceiver/go.mod receiver/zabbixreceiver/go.sum ./receiver/zabbixreceiver/") {
	t.Error("Dockerfile must copy the nested receiver manifests before go mod download")
}
```

- [ ] **Шаг 2: запустить тесты контракта автоматизации и подтвердить падение**

Выполните:

```bash
go test ./internal/packaging -run 'Test(DualModuleAutomation|Dockerfile)Contract' -count=1
```

Ожидаемый результат: FAIL из-за отсутствующих элементов Makefile, CI и Dockerfile.

- [ ] **Шаг 3: расширить проверки в Makefile**

Используйте эти цели, сохранив существующие цели сборки и демонстрации:

```make
.PHONY: fmt download tidy test test-race vet build validate-config docker-build compose-config demo-up demo-verify demo-down

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*' -not -path './.ocb/*')

download:
	go mod download
	cd receiver/zabbixreceiver && go mod download

tidy:
	go mod tidy
	cd receiver/zabbixreceiver && go mod tidy

test:
	go test ./... -count=1
	cd receiver/zabbixreceiver && go test ./... -count=1

test-race:
	go test -race ./... -count=1
	cd receiver/zabbixreceiver && go test -race ./... -count=1

vet:
	go vet ./...
	cd receiver/zabbixreceiver && go vet ./...
```

- [ ] **Шаг 4: учесть вложенный манифест при загрузке зависимостей Docker**

Начало этапа сборки Docker должно выглядеть так:

```dockerfile
WORKDIR /src

COPY receiver/zabbixreceiver/go.mod receiver/zabbixreceiver/go.sum ./receiver/zabbixreceiver/
COPY go.mod go.sum ./
RUN go mod download
```

Это предоставляет локальному `replace` корневого модуля корректный каталог модуля до запуска `go mod download`.

- [ ] **Шаг 5: обновить кеширование CI и проверки зависимостей**

В `actions/setup-go@v6` задайте:

```yaml
cache-dependency-path: |
  go.sum
  receiver/zabbixreceiver/go.sum
```

Замените команду работы с зависимостями на `make download`. Перед форматированием добавьте:

```yaml
- name: Enforce tidy module files
  run: |
    make tidy
    git diff --check
    git diff --exit-code
```

Существующие шаги `make test`, `make test-race` и `make vet` теперь охватывают оба модуля.

- [ ] **Шаг 6: запустить проверки автоматизации**

Выполните:

```bash
make tidy
make fmt
git diff --check
make test
make vet
go test ./internal/packaging -run 'Test(DualModuleAutomation|Dockerfile)Contract' -count=1
```

Ожидаемый результат: PASS.

- [ ] **Шаг 7: закоммитить автоматизацию для нескольких модулей**

```bash
git add Makefile Dockerfile .github/workflows/ci.yml internal/packaging/assets_test.go go.mod go.sum receiver/zabbixreceiver/go.mod receiver/zabbixreceiver/go.sum
git commit -m "build: verify root and receiver modules"
```

---

### Задача 3: добавить локальные и опубликованные примеры Collector Builder

**Файлы:**
- Создать: `examples/ocb/builder-config.yaml`
- Создать: `examples/ocb/otelcol.yaml`
- Создать: `testdata/ocb/builder-config.yaml`
- Изменить: `internal/packaging/assets_test.go`
- Изменить: `Makefile`
- Изменить: `.github/workflows/ci.yml`
- Изменить: `.gitignore`

**Интерфейсы:**
- Результат: готовый к копированию удалённый манифест без отдельного ключа `path` компонента, `replace` или переопределения неопубликованного импорта.
- Результат: манифест только для локального использования с `path: ./receiver/zabbixreceiver`.
- Результат: `make verify-ocb-local`, выполняющий сборку с Builder v0.154.0, проверку состава компонентов и валидацию YAML рабочей конфигурации.

- [ ] **Шаг 1: написать падающий тест контракта ресурсов OCB**

Добавьте этот тест в `internal/packaging/assets_test.go`:

```go
func TestOCBExamplesContract(t *testing.T) {
	published := readAsset(t, "examples/ocb/builder-config.yaml")
	wantModule := "github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.1.0"
	if !strings.Contains(published, wantModule) {
		t.Errorf("published OCB example missing %q", wantModule)
	}
	for _, forbidden := range []string{"\n    path:", "replaces:", "github.com/aleksandr"} {
		if strings.Contains(published, forbidden) {
			t.Errorf("published OCB example contains local-only clause %q", forbidden)
		}
	}

	local := readAsset(t, "testdata/ocb/builder-config.yaml")
	for _, want := range []string{wantModule, "path: ./receiver/zabbixreceiver"} {
		if !strings.Contains(local, want) {
			t.Errorf("local OCB fixture missing %q", want)
		}
	}

	runtimeConfig := readAsset(t, "examples/ocb/otelcol.yaml")
	for _, want := range []string{"zabbix:", "${env:ZABBIX_URL}", "${env:ZABBIX_TOKEN}", "debug:", "receivers: [zabbix]"} {
		if !strings.Contains(runtimeConfig, want) {
			t.Errorf("runtime OCB example missing %q", want)
		}
	}
}
```

- [ ] **Шаг 2: запустить тест ресурсов OCB и подтвердить падение**

Выполните:

```bash
go test ./internal/packaging -run TestOCBExamplesContract -count=1
```

Ожидаемый результат: FAIL, поскольку файлы примеров OCB не существуют.

- [ ] **Шаг 3: создать манифест Builder для опубликованного модуля**

Создайте `examples/ocb/builder-config.yaml`:

```yaml
dist:
  module: github.com/wieso/zabbixreceiver/examples/ocb/otelcol-zabbix
  name: otelcol-zabbix
  description: OpenTelemetry Collector with the Zabbix receiver
  output_path: ./otelcol-zabbix-dist
  version: 0.1.0

receivers:
  - gomod: github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.1.0

processors:
  - gomod: go.opentelemetry.io/collector/processor/batchprocessor v0.154.0

exporters:
  - gomod: go.opentelemetry.io/collector/exporter/debugexporter v0.154.0

providers:
  - gomod: go.opentelemetry.io/collector/confmap/provider/envprovider v1.60.0
  - gomod: go.opentelemetry.io/collector/confmap/provider/fileprovider v1.60.0

telemetry:
  gomod: go.opentelemetry.io/collector/service v0.154.0
  import: go.opentelemetry.io/collector/service/telemetry/otelconftelemetry
```

- [ ] **Шаг 4: создать локальный тестовый пример Builder**

Создайте `testdata/ocb/builder-config.yaml` с теми же версиями компонентов, но отдельным каталогом вывода и одним локальным переопределением:

```yaml
dist:
  module: github.com/wieso/zabbixreceiver/internal/ocbtest
  name: otelcol-zabbix
  description: Local Zabbix receiver OCB verification binary
  output_path: ./.ocb/local
  version: 0.1.0-dev

receivers:
  - gomod: github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.1.0
    path: ./receiver/zabbixreceiver

processors:
  - gomod: go.opentelemetry.io/collector/processor/batchprocessor v0.154.0

exporters:
  - gomod: go.opentelemetry.io/collector/exporter/debugexporter v0.154.0

providers:
  - gomod: go.opentelemetry.io/collector/confmap/provider/envprovider v1.60.0
  - gomod: go.opentelemetry.io/collector/confmap/provider/fileprovider v1.60.0

telemetry:
  gomod: go.opentelemetry.io/collector/service v0.154.0
  import: go.opentelemetry.io/collector/service/telemetry/otelconftelemetry
```

- [ ] **Шаг 5: создать минимальную рабочую конфигурацию**

Создайте `examples/ocb/otelcol.yaml`:

```yaml
receivers:
  zabbix:
    zabbix:
      url: ${env:ZABBIX_URL}
      token: ${env:ZABBIX_TOKEN}

processors:
  batch: {}

exporters:
  debug:
    verbosity: basic

service:
  pipelines:
    metrics:
      receivers: [zabbix]
      processors: [batch]
      exporters: [debug]
```

- [ ] **Шаг 6: добавить цель локальной проверки OCB**

Добавьте переменные, фиктивную цель и рецепт в `Makefile`:

```make
OCB_VERSION ?= v0.154.0
OCB_LOCAL_OUTPUT ?= .ocb/local

verify-ocb-local:
	mkdir -p $(OCB_LOCAL_OUTPUT)
	go run go.opentelemetry.io/collector/cmd/builder@$(OCB_VERSION) --skip-strict-versioning=false --config testdata/ocb/builder-config.yaml
	$(OCB_LOCAL_OUTPUT)/otelcol-zabbix components | grep -q 'zabbix'
	ZABBIX_URL=http://zabbix.example/api_jsonrpc.php ZABBIX_TOKEN=dummy-token $(OCB_LOCAL_OUTPUT)/otelcol-zabbix validate --config examples/ocb/otelcol.yaml
```

Добавьте `verify-ocb-local` в `.PHONY`.

- [ ] **Шаг 7: исключить генерируемые и локальные пути редактора**

Добавьте в `.gitignore`:

```gitignore
.idea/
.ocb/
otelcol-zabbix-dist/
```

- [ ] **Шаг 8: добавить проверку OCB в CI**

После обычного шага валидации конфигурации Collector добавьте:

```yaml
- name: Verify Collector Builder integration
  run: make verify-ocb-local
```

- [ ] **Шаг 9: запустить локальную проверку Builder**

Выполните:

```bash
go test ./internal/packaging -run TestOCBExamplesContract -count=1
make verify-ocb-local
```

Ожидаемый результат: цель создаёт игнорируемый родительский каталог вывода; Builder v0.154.0 успешно выполняет строгие проверки версий и компилирует `.ocb/local/otelcol-zabbix`; `components` содержит `zabbix`; `validate` завершается успешно без обращения к адресу-заглушке.

- [ ] **Шаг 10: закоммитить ресурсы интеграции Builder**

```bash
git add .gitignore Makefile .github/workflows/ci.yml examples/ocb testdata/ocb internal/packaging/assets_test.go
git commit -m "test: verify Collector Builder integration"
```

---

### Задача 4: добавить публичную документацию и лицензию Apache-2.0

**Файлы:**
- Создать: `LICENSE`
- Создать: `receiver/zabbixreceiver/README.md`
- Изменить: `README.md`
- Изменить: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md`
- Изменить: `internal/packaging/assets_test.go`

**Интерфейсы:**
- Результат: готовые к копированию инструкции OCB и руководство по настройке компонента.
- Результат: каноническая лицензия Apache License 2.0 в корне репозитория.
- Сохраняется: подробная документация по конфигурации, развёртыванию, безопасности и демонстрации, ссылки на которую уже есть в корневом README.

- [ ] **Шаг 1: написать падающие проверки публичной документации**

В `TestDocumentationContract` добавьте `Custom Collector Builder` к обязательным заголовкам README и проверьте:

```go
for _, clause := range []string{
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.1.0",
	"receiver/zabbixreceiver/v0.1.0",
	"Collector/Contrib v0.154.0",
} {
	if !strings.Contains(readme, clause) {
		t.Errorf("README.md missing publication clause %q", clause)
	}
}
```

Добавьте:

```go
func TestLicenseContract(t *testing.T) {
	license := readAsset(t, "LICENSE")
	for _, want := range []string{
		"Apache License",
		"Version 2.0, January 2004",
		"http://www.apache.org/licenses/",
	} {
		if !strings.Contains(license, want) {
			t.Errorf("LICENSE missing %q", want)
		}
	}
}
```

- [ ] **Шаг 2: запустить тесты документации и подтвердить падение**

Выполните:

```bash
go test ./internal/packaging -run 'Test(Documentation|License)Contract' -count=1
```

Ожидаемый результат: FAIL, поскольку новый заголовок, инструкции по модулю и `LICENSE` отсутствуют.

- [ ] **Шаг 3: добавить каноническую лицензию**

Создайте `LICENSE` из неизменённого текста Apache License 2.0, поставляемого с OpenTelemetry Collector Builder v0.154.0. Проверьте точное содержимое командой:

```bash
shasum -a 256 LICENSE
```

Ожидаемый результат:

```text
cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30  LICENSE
```

- [ ] **Шаг 4: добавить инструкции Builder в корневую документацию**

Добавьте раздел `## Custom Collector Builder` рядом с быстрым стартом. Он должен объяснять, что `builder-config.yaml` управляет компиляцией, а `otelcol.yaml` — работой программы, показывать именно эту запись компонента и содержать ссылки на оба примера:

```yaml
receivers:
  - gomod: github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.1.0
```

Задокументируйте следующие точные сведения:

```text
Проверенная совместимость: Collector/Contrib v0.154.0 и стабильные модули Collector v1.60.0.
Тег выпуска: receiver/zabbixreceiver/v0.1.0.
```

Добавьте `make tidy` и `make verify-ocb-local` в таблицу команд тестирования; укажите, что стандартные цели Make охватывают корневой модуль и модуль приёмника.

- [ ] **Шаг 5: создать README компонента**

Создайте `receiver/zabbixreceiver/README.md` со следующими разделами и конкретным содержимым:

````markdown

# Приёмник Zabbix

Приёмник `zabbix` опрашивает числовые элементы данных Zabbix и выдаёт метрики OpenTelemetry типа gauge.

## Collector Builder

```yaml
receivers:
  - gomod: github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.1.0
```

Этот выпуск проверен с Collector/Contrib v0.154.0 и стабильными модулями Collector v1.60.0. Тег репозитория — `receiver/zabbixreceiver/v0.1.0`.

## Рабочая конфигурация

```yaml
receivers:
  zabbix:
    zabbix:
      url: ${env:ZABBIX_URL}
      token: ${env:ZABBIX_TOKEN}
```

Добавьте приёмник в конвейер метрик. Полная схема приведена в `../../docs/configuration.md`, а полные примеры конфигурации сборки и выполнения — в `../../examples/ocb`.

## Разработка

Запустите `go test ./...`, `go test -race ./...` и `go vet ./...` из этого каталога. Из корня репозитория команда `make verify-ocb-local` проверяет полную интеграцию Builder.
````

При добавлении этого содержимого правильно вкладывайте ограждения блоков Markdown.

- [ ] **Шаг 6: обновить пути в утверждённой архитектуре**

В `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md` замените три ссылки на пути следующими:

```text
receiver/zabbixreceiver/internal/zabbix
receiver/zabbixreceiver/internal/discovery
receiver/zabbixreceiver/internal/metrics
```

Не меняйте требования к поведению в утверждённом проекте приёмника.

- [ ] **Шаг 7: запустить проверки документации и поиск по репозиторию**

Выполните:

```bash
go test ./internal/packaging -run 'Test(Documentation|License|OCBExamples|ReceiverModule)Contract' -count=1
rg -n 'github\.com/aleksandr/zabbix-otel' --glob '!docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md'
git diff --check
```

Ожидаемый результат: тесты PASS; поиск не выдаёт результатов вне явно исторического плана реализации; проверка различий проходит успешно.

- [ ] **Шаг 8: закоммитить публичную документацию**

```bash
git add LICENSE README.md receiver/zabbixreceiver/README.md docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md internal/packaging/assets_test.go
git commit -m "docs: explain versioned receiver integration"
```

---

### Задача 5: выполнить полный набор проверок перед публикацией

**Файлы:**
- Только проверка; изменяйте файлы лишь при обнаружении дефекта, затем повторяйте цикл «падение — успех» для затронутой задачи.

**Интерфейсы:**
- Зависимости: оба модуля Go, локальный тестовый пример OCB, готовая сборка, конфигурация Compose и сборка контейнера.
- Результат: свидетельства того, что итоговое дерево отслеживаемых файлов готово стать корневым коммитом публикации.

- [ ] **Шаг 1: применить правила проверки перед завершением**

Используйте `superpowers:verification-before-completion` перед любым заявлением об успехе или созданием коммита публикации.

- [ ] **Шаг 2: проверить форматирование и актуальность файлов модулей**

Выполните:

```bash
make fmt
make tidy
git diff --check
git diff --exit-code
```

Ожидаемый результат: все команды завершаются с кодом 0, дерево остаётся неизменным.

- [ ] **Шаг 3: проверить оба модуля Go**

Выполните:

```bash
make test
make test-race
make vet
```

Ожидаемый результат: тесты корневого и вложенного модулей, тесты с детектором гонок и vet — все PASS.

- [ ] **Шаг 4: проверить оба способа сборки Collector**

Выполните:

```bash
make build
make validate-config
make verify-ocb-local
```

Ожидаемый результат: вручную поддерживаемая и генерируемая OCB сборки успешно собираются; обе проходят валидацию рабочей конфигурации; список компонентов OCB включает `zabbix`.

- [ ] **Шаг 5: проверить упаковку**

Выполните:

```bash
make compose-config
docker build -t zabbix-otel-collector:verify .
docker run --rm zabbix-otel-collector:verify components
```

Ожидаемый результат: конфигурация Compose корректна, образ собирается, список компонентов контейнера включает `zabbix`.

- [ ] **Шаг 6: проверить итоговое состояние репозитория**

Выполните:

```bash
git status --short
git log -6 --oneline --decorate
git tag --list
```

Ожидаемый результат: рабочее дерево чистое; локальные коммиты реализации доступны для проверки; тег выпуска пока отсутствует.

---

### Задача 6: опубликовать один корневой коммит и проверить удалённый модуль

**Файлы:**
- Только ссылки Git; проверенное дерево отслеживаемых файлов не должно измениться.

**Интерфейсы:**
- Зависимости: чистый проверенный `HEAD` и пустой `https://github.com/wieso/zabbixreceiver.git`.
- Результат: удалённая ветка `main` с одним коммитом и указывающий на него тег `receiver/zabbixreceiver/v0.1.0`.
- Сохраняется: прежняя локальная история в неотправленной ветке `feat/ocb-module-publication`.

- [ ] **Шаг 1: повторно проверить удалённый репозиторий непосредственно перед изменением**

Выполните:

```bash
git ls-remote https://github.com/wieso/zabbixreceiver.git
```

Ожидаемый результат: вывод отсутствует. При появлении любой ссылки остановитесь и спросите пользователя; не выполняйте принудительную отправку или перезапись.

- [ ] **Шаг 2: создать корневой коммит из проверенного дерева без изменения файлов**

Убедитесь, что `git status --short` не выводит изменений, затем выполните в одной оболочке:

```bash
publication_tree="$(git rev-parse HEAD^{tree})"
publication_commit="$(printf '%s\n' 'feat: publish Zabbix OpenTelemetry receiver' | git commit-tree "$publication_tree")"
git branch publication-main "$publication_commit"
```

Ожидаемый результат: дерево `publication-main` совпадает с `feat/ocb-module-publication`, но это новый корневой коммит. Ветка реализации остаётся текущей и локальной.

- [ ] **Шаг 3: доказать отсутствие родителя у локального коммита публикации**

Выполните:

```bash
git rev-list --count publication-main
git rev-list --parents -n 1 publication-main
git diff --exit-code publication-main feat/ocb-module-publication
```

Ожидаемый результат: число коммитов — `1`; вторая команда выводит только хеш нового коммита без хеша родителя; различий между деревьями нет.

- [ ] **Шаг 4: создать тег выпуска вложенного модуля**

Выполните:

```bash
git tag -a receiver/zabbixreceiver/v0.1.0 -m "Zabbix receiver v0.1.0" publication-main
git rev-parse receiver/zabbixreceiver/v0.1.0^{commit}
git rev-parse publication-main
```

Ожидаемый результат: оба хеша совпадают.

- [ ] **Шаг 5: настроить и отправить только утверждённые ссылки**

Выполните:

```bash
git remote add origin https://github.com/wieso/zabbixreceiver.git
git push --atomic origin \
  publication-main:refs/heads/main \
  refs/tags/receiver/zabbixreceiver/v0.1.0
```

Если `origin` уже существует, убедитесь, что его URL точно совпадает с целевым, и используйте `git remote set-url origin https://github.com/wieso/zabbixreceiver.git` только при необходимости. Не используйте `--mirror`, `--all`, `--tags` или принудительную отправку.

Ожидаемый результат: атомарная отправка публикует обе ссылки с помощью настроенного пользователем помощника учётных данных Git либо не публикует ни одну, если любое из обновлений завершается ошибкой.

- [ ] **Шаг 6: проверить список удалённых ссылок**

Выполните:

```bash
git ls-remote --heads --tags origin
```

Ожидаемый результат: присутствуют только `refs/heads/main`, ссылка аннотированного тега и его строка `^{}`, указывающая на коммит.

- [ ] **Шаг 7: проверить историю и тег в чистом клоне**

Создайте временный каталог, клонируйте `main` и выполните:

```bash
publication_tmp="$(mktemp -d)"
git clone --branch main https://github.com/wieso/zabbixreceiver.git "$publication_tmp/repo"
cd "$publication_tmp/repo"
git rev-list --count HEAD
git rev-list --parents -n 1 HEAD
git rev-parse receiver/zabbixreceiver/v0.1.0^{commit}
git rev-parse HEAD
```

Ожидаемый результат: число коммитов — `1`; у `HEAD` нет родителя; хеши тега и `HEAD` совпадают.

- [ ] **Шаг 8: собрать из удалённого тега с изолированными кешами**

В чистом клоне задайте отдельные временные каталоги `GOMODCACHE` и `GOCACHE` для этой задачи и обойдите задержки proxy/sumdb только при поиске модуля этого репозитория:

```bash
GONOPROXY=github.com/wieso/zabbixreceiver \
GONOSUMDB=github.com/wieso/zabbixreceiver \
GOMODCACHE="$publication_tmp/modcache" \
GOCACHE="$publication_tmp/buildcache" \
go run go.opentelemetry.io/collector/cmd/builder@v0.154.0 --skip-strict-versioning=false --config examples/ocb/builder-config.yaml
```

Не добавляйте `replace`, `path` или рабочую область Go. Ожидаемый результат: Builder загружает `github.com/wieso/zabbixreceiver/receiver/zabbixreceiver@v0.1.0` с GitHub и компилирует `otelcol-zabbix-dist/otelcol-zabbix`.

- [ ] **Шаг 9: проверить бинарный файл, собранный с удалённым модулем**

Выполните из чистого клона:

```bash
otelcol-zabbix-dist/otelcol-zabbix components | grep -q 'zabbix'
ZABBIX_URL=http://zabbix.example/api_jsonrpc.php \
ZABBIX_TOKEN=dummy-token \
otelcol-zabbix-dist/otelcol-zabbix validate --config examples/ocb/otelcol.yaml
```

Ожидаемый результат: обе команды завершаются с кодом 0; валидация не обращается к адресу-заглушке Zabbix.

- [ ] **Шаг 10: сообщить результаты проверки публикации**

Сообщите URL репозитория GitHub, хеш удалённого корневого коммита, тег модуля, точную строку OCB `gomod`, число коммитов, равное одному, и результаты успешной локальной и удалённой проверки Builder. Также укажите, что `feat/ocb-module-publication` существует только локально и не была отправлена.
