# План реализации приёмника Zabbix для OpenTelemetry

> Исторический материал. [Статус и указатель](../README.md) · [Актуальная документация](../../README.md). Не используйте как инструкцию для текущей версии.

> **Для агентов-исполнителей:** ОБЯЗАТЕЛЬНЫЙ ВСПОМОГАТЕЛЬНЫЙ НАВЫК: используйте superpowers:subagent-driven-development (рекомендуется) или superpowers:executing-plans для последовательного выполнения задач этого плана. Для отслеживания шагов используются флажки (`- [ ]`).

**Цель:** создать и развернуть нативный приёмник OpenTelemetry Collector на Go, который опрашивает числовые элементы данных Zabbix и передаёт их через конвейер метрик Collector в VictoriaMetrics.

**Архитектура:** типизированный клиент Zabbix JSON-RPC обслуживает отдельные задания обнаружения и сбора значений. Обнаружение публикует неизменяемые отфильтрованные снимки метаданных; сбор значений преобразует `lastvalue` и `lastclock` в метрики OpenTelemetry типа gauge и передаёт их в `consumer.Metrics`. Специализированная сборка Collector включает приёмник и стандартный экспортёр Prometheus remote-write.

**Технологии:** Go 1.25+, OpenTelemetry Collector v0.153.0 со стабильными модулями v1.59.0, OpenTelemetry Collector Contrib v0.153.0, демонстрационные образы Zabbix 7.4.12, общедоступный образ VictoriaMetrics v1.148.0, Docker Compose v2.24+, манифесты Kubernetes, systemd.

## Общие ограничения

- Результат — нативный приёмник метрик OpenTelemetry Collector, а не самостоятельный экспортёр Prometheus.
- Публичный тип приёмника — `zabbix`, модуль Go — `github.com/aleksandr/zabbix-otel`.
- Сохранить публичные ключи конфигурации приёмника `schedule`, `prom.prefix`, `prom.const_labels` и `zabbix`, а также документированные переопределения Zabbix через окружение.
- Не включать `base.address`, HTTP-адреса самостоятельного экспортёра, отдельные флаги CLI и настройки, специфичные для агента.
- Сохранить независимость приёмника от VictoriaMetrics; использовать `prometheusremotewrite` в конфигурации развёртывания.
- Опрашивать только числовые типы элементов данных Zabbix: число с плавающей точкой (`0`) и беззнаковое целое (`3`).
- Выдавать точки данных OpenTelemetry типа gauge с временными метками Zabbix `lastclock` и четырьмя зарезервированными атрибутами `host`, `hostid`, `item_key` и `itemid`.
- Никогда не записывать в журнал настроенный токен Zabbix или полные тела аутентифицированных запросов JSON-RPC.
- Контейнер приложения и рабочая нагрузка Kubernetes запускаются без прав root.
- Демонстрация Compose должна использовать настоящий сервер/API Zabbix и доказывать, что подготовленная метрика доступна для запросов в VictoriaMetrics.
- Каждое изменение реализации проходит цикл «красный — зелёный — рефакторинг» и завершается целевыми тестами и коммитом.

## Карта файлов

### Модуль и автоматизация проекта

- `go.mod`, `go.sum`: корневой модуль Go и закреплённые зависимости Collector.
- `Makefile`: цели форматирования, модульных тестов, проверки гонок, vet, сборки, контейнеров, Compose и проверки результата.
- `.gitignore`, `.dockerignore`: исключения для сгенерированных бинарных файлов, локальных файлов окружения, покрытия и контекста сборки.
- `.github/workflows/ci.yml`: воспроизводимые проверки Go и упаковки.

### Компонент приёмника

- `receiver/zabbixreceiver/config.go`: публичные типы конфигурации приёмника, значения по умолчанию, обработка окружения и валидация.
- `receiver/zabbixreceiver/config_test.go`: значения по умолчанию, валидация и приоритет окружения.
- `receiver/zabbixreceiver/factory.go`: `NewFactory` Collector и создание для эксплуатации.
- `receiver/zabbixreceiver/factory_test.go`: проверки типа компонента, стабильности, конфигурации по умолчанию и создания.
- `receiver/zabbixreceiver/receiver.go`: операции обнаружения/сбора значений, жизненный цикл, использование снимков и доставка следующему компоненту.
- `receiver/zabbixreceiver/receiver_test.go`: тесты операций, разбиения на порции, восстановления и интеграции с тестовым окружением.
- `receiver/zabbixreceiver/scheduler.go`: независимые циклы заданий без перекрытия запусков, с интервалом, тайм-аутом, запуском при старте и случайной задержкой.
- `receiver/zabbixreceiver/scheduler_test.go`: тесты детерминированного расчёта задержки, отмены и отсутствия перекрытия.
- `receiver/zabbixreceiver/telemetry.go`: штатные инструменты телеметрии приёмника Collector.
- `receiver/zabbixreceiver/telemetry_test.go`: базовые тесты создания инструментов и записи счётчиков.

### Внутренние пакеты

- `internal/zabbix/types.go`: `Host`, `Item`, `Value`, интерфейс API и ошибки JSON-RPC.
- `internal/zabbix/client.go`: аутентифицированный транспорт JSON-RPC и типизированные методы API.
- `internal/zabbix/client_test.go`: контракты запросов, современная/устаревшая аутентификация, декодирование, тайм-ауты и скрытие секретов.
- `internal/discovery/select.go`: фильтрация регулярными выражениями и детерминированные лимиты на узел.
- `internal/discovery/select_test.go`: тесты приоритета включения/исключения и лимитов.
- `internal/discovery/snapshot.go`: атомарное хранилище неизменяемых снимков.
- `internal/discovery/snapshot_test.go`: поведение копирования и замены.
- `internal/metrics/name.go`: построение имён метрик, совместимых с Prometheus.
- `internal/metrics/name_test.go`: табличные тесты нормализации и валидации.
- `internal/metrics/builder.go`: преобразование Zabbix в `pmetric.Metrics`.
- `internal/metrics/builder_test.go`: метрики gauge, значения, временные метки, атрибуты, описания и пропущенные значения.

### Сборка и развёртывание

- `cmd/otelcol-zabbix/main.go`: точка входа команды Collector и информация о сборке.
- `cmd/otelcol-zabbix/components.go`: карты фабрик приёмника, процессоров, экспортёра и расширений.
- `cmd/otelcol-zabbix/components_test.go`: точный перечень встроенных компонентов.
- `configs/otelcol.yaml`: документированный пример конвейера VictoriaMetrics.
- `Dockerfile`: многоэтапный образ сборки с запуском без прав root.
- `deployments/systemd/otelcol-zabbix.service`: служебный юнит с усиленной защитой.
- `deployments/systemd/otelcol-zabbix.yaml`: конфигурация Collector для виртуальной машины.
- `deployments/systemd/otelcol-zabbix.env.example`: шаблон окружения с учётными данными и адресами сервисов.
- `deployments/kubernetes/*.yaml`: пространство имён, пример Secret, ConfigMap, Deployment, Service и перечень ресурсов Kustomize.
- `compose.yaml`: полная демонстрация передачи данных из Zabbix в VictoriaMetrics.
- `demo/Dockerfile.tools`: образ утилит curl/jq с закреплёнными версиями для заданий начальной настройки и проверки.
- `demo/bootstrap.sh`: идемпотентная настройка объектов/токена Zabbix и генерация конфигурации Collector.
- `demo/producer.sh`: отправка изменяющихся значений trapper в Zabbix.
- `demo/collector.yaml.tmpl`: конвейер приёмника с короткими интервалами, используемый только в демонстрации.
- `demo/verify.sh`: запрос к VictoriaMetrics и проверки меток.
- `internal/packaging/assets_test.go`: статические проверки файлов службы, Kubernetes, Docker и Compose.
- `README.md`, `docs/configuration.md`, `docs/deployment.md`: документация по использованию и эксплуатации.

---

### Задача 1: модуль Go и контракт конфигурации приёмника

**Файлы:**
- Создать: `go.mod`
- Создать: `.gitignore`
- Создать: `receiver/zabbixreceiver/config.go`
- Тест: `receiver/zabbixreceiver/config_test.go`

**Интерфейсы:**
- Предоставляет: `type Config`, `createDefaultConfig() component.Config`, `func (c *Config) Clone() *Config`, `func (c *Config) ResolveEnv(getenv func(string) (string, bool)) error` и `func (c *Config) Validate() error`. Публичный метод `Validate` применяет явные переопределения приёмника к копии и вызывает чистый валидатор итоговой конфигурации, не изменяя исходный объект.
- Предоставляет точный набор вложенных типов: `ScheduleConfig`, `JobsConfig`, `JobConfig`, `PromConfig`, `ZabbixConfig`, `LimitsConfig` и `FiltersConfig`.
- Использует `configopaque.String` для `ZabbixConfig.Token`, чтобы при выводе конфигурации Collector учётные данные скрывались.

- [ ] **Шаг 1: инициализировать модуль с закреплёнными API Collector**

Выполнить:

```bash
go mod init github.com/aleksandr/zabbix-otel
go get go.opentelemetry.io/collector/component@v1.59.0
go get go.opentelemetry.io/collector/config/configopaque@v1.59.0
go get go.opentelemetry.io/collector/receiver@v1.59.0
go get github.com/stretchr/testify@v1.11.1
```

Ожидается: `go.mod` объявляет Go 1.25 или новее и содержит требуемые версии модулей.

- [ ] **Шаг 2: написать падающие тесты значений по умолчанию, окружения и валидации**

Создать табличные тесты со следующими точными проверками:

```go
func TestCreateDefaultConfig(t *testing.T) {
    cfg := createDefaultConfig().(*Config)
    require.Equal(t, 5*time.Second, cfg.Schedule.Jitter)
    require.Equal(t, JobConfig{Enabled: true, RunOnStart: true, Interval: 5*time.Minute, Timeout: time.Minute}, cfg.Schedule.Jobs.Discover)
    require.Equal(t, JobConfig{Enabled: true, RunOnStart: false, Interval: 30*time.Second, Timeout: 20*time.Second}, cfg.Schedule.Jobs.Values)
    require.Equal(t, "zabbix_", cfg.Prom.Prefix)
    require.Equal(t, 30*time.Second, cfg.Zabbix.Timeout)
    require.Equal(t, LimitsConfig{MaxMetricsPerHost: 1000, ItemsPerRequest: 1000}, cfg.Zabbix.Limits)
}

func TestResolveEnvTakesPrecedence(t *testing.T) {
    cfg := validConfig()
    env := map[string]string{
        "ZABBIX_URL": "https://env.example/api_jsonrpc.php",
        "ZABBIX_TOKEN": "env-secret",
        "ZABBIX_TIMEOUT": "7s",
        "MAX_METRICS_PER_HOST": "12",
        "ZABBIX_ITEMS_PER_REQUEST": "34",
    }
    require.NoError(t, cfg.ResolveEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok }))
    assert.Equal(t, "https://env.example/api_jsonrpc.php", cfg.Zabbix.URL)
    assert.Equal(t, configopaque.String("env-secret"), cfg.Zabbix.Token)
    assert.Equal(t, 7*time.Second, cfg.Zabbix.Timeout)
    assert.Equal(t, 12, cfg.Zabbix.Limits.MaxMetricsPerHost)
    assert.Equal(t, 34, cfg.Zabbix.Limits.ItemsPerRequest)
}

func validConfig() *Config {
    cfg := createDefaultConfig().(*Config)
    cfg.Zabbix.URL = "https://zabbix.example/api_jsonrpc.php"
    cfg.Zabbix.Token = configopaque.String("test-secret")
    return cfg
}
```

Сценарии валидации должны проверять ошибки с указанием поля для отсутствующего URL, URL с протоколом не HTTP, пустого токена, отрицательной случайной задержки, включённого задания с неположительным интервалом или тайм-аутом, некорректного регулярного выражения, неположительных лимитов, некорректного префикса метрик и отключения обоих заданий. Сценарии разбора окружения должны охватывать некорректные строки длительности и целых чисел.

Добавить `TestCloneDoesNotAliasConstLabels`: изменить `Prom.ConstLabels` копии и убедиться, что исходная карта не изменилась.

- [ ] **Шаг 3: запустить тесты и подтвердить красное состояние**

Выполнить: `go test ./receiver/zabbixreceiver -run 'Test(CreateDefaultConfig|ResolveEnv|Validate)' -count=1`

Ожидается: FAIL, поскольку типы и функции конфигурации ещё не существуют.

- [ ] **Шаг 4: реализовать минимальный контракт конфигурации**

Использовать теги `mapstructure`, точно соответствующие утверждённому YAML:

```go
type Config struct {
    Schedule ScheduleConfig `mapstructure:"schedule"`
    Prom     PromConfig     `mapstructure:"prom"`
    Zabbix   ZabbixConfig   `mapstructure:"zabbix"`
}

type JobConfig struct {
    Enabled    bool          `mapstructure:"enabled"`
    RunOnStart bool          `mapstructure:"run_on_start"`
    Interval   time.Duration `mapstructure:"interval"`
    Timeout    time.Duration `mapstructure:"timeout"`
}

type PromConfig struct {
    Prefix      string            `mapstructure:"prefix"`
    ConstLabels map[string]string `mapstructure:"const_labels"`
}

type ZabbixConfig struct {
    URL     string              `mapstructure:"url"`
    Token   configopaque.String `mapstructure:"token"`
    Timeout time.Duration       `mapstructure:"timeout"`
    Limits  LimitsConfig        `mapstructure:"limits"`
    Filters FiltersConfig       `mapstructure:"filters"`
}
```

`Clone` копирует структуру и создаёт глубокую копию `Prom.ConstLabels`. `ResolveEnv` должен разбирать значения через `time.ParseDuration` и `strconv.Atoi`, оставлять поля неизменными, если переменная не задана, и возвращать сообщения с именем некорректной переменной окружения. Публичный `Validate` создаёт копию, применяет окружение через `os.LookupEnv` и вызывает чистый валидатор для полученной копии. Валидация итоговой конфигурации должна требовать положительный `zabbix.timeout`, компилировать все четыре поля регулярных выражений, требовать итоговое имя метрики, совместимое с `[a-zA-Z_:][a-zA-Z0-9_:]*`, и объединять независимые ошибки полей через `errors.Join`.

- [ ] **Шаг 5: запустить целевые тесты и тесты пакета**

Выполнить:

```bash
go test ./receiver/zabbixreceiver -count=1
go test ./... -count=1
```

Ожидается: PASS.

- [ ] **Шаг 6: закоммитить контракт конфигурации**

```bash
git add go.mod go.sum .gitignore receiver/zabbixreceiver/config.go receiver/zabbixreceiver/config_test.go
git commit -m "feat: define Zabbix receiver configuration"
```

---

### Задача 2: типизированный клиент Zabbix JSON-RPC

**Файлы:**
- Создать: `internal/zabbix/types.go`
- Создать: `internal/zabbix/client.go`
- Тест: `internal/zabbix/client_test.go`

**Интерфейсы:**
- Предоставляет:

```go
type API interface {
    Hosts(context.Context) ([]Host, error)
    Items(context.Context, []string) ([]Item, error)
    Values(context.Context, []string) ([]Value, error)
}

type Host struct { ID, Name string }
type Item struct { ID, HostID, Name, Key, ValueType string }
type Value struct { ItemID, LastValue, LastClock string }
type ClientConfig struct { URL, Token string; Timeout time.Duration }
func NewClient(ClientConfig, *http.Client) (*Client, error)
```

- `Items` принимает идентификаторы узлов; `Values` принимает идентификаторы элементов данных. Пустые срезы идентификаторов возвращают пустой результат без сетевых вызовов.
- Аутентификация начинается с `Authorization: Bearer`; при ошибке аутентификации старого Zabbix клиент повторяет запрос один раз со свойством JSON-RPC `auth` и кеширует успешный режим. Это сохраняет совместимость с токенами сеанса Zabbix 5.x без использования устаревшей аутентификации на современных серверах.

- [ ] **Шаг 1: написать падающие тесты контракта запросов**

Использовать `httptest.Server` для перехвата запросов. Проверить:

```go
func TestHostsUsesBearerAndExactFields(t *testing.T) {
    // Сервер проверяет Content-Type application/json-rpc и Authorization Bearer secret.
    // Декодировать тело и проверить method=host.get, output=[hostid,host], ненулевой id
    // и отсутствие свойства auth. Вернуть два объекта узлов.
}

func TestItemsRequestsNumericItems(t *testing.T) {
    // Проверить, что параметры item.get содержат output itemid,hostid,name,key_,value_type;
    // hostids совпадают с переданными идентификаторами; filter.value_type равен ["0","3"].
}

func TestValuesRequestsLastFields(t *testing.T) {
    // Проверить, что параметры item.get содержат output itemid,lastvalue,lastclock и точные itemids.
}
```

Добавить сценарии для пустых срезов, HTTP 503, некорректного JSON, несовпадения идентификатора ответа, полей ошибок JSON-RPC, отмены контекста, тайм-аута клиента, повтора/кеширования устаревшей аутентификации и некорректного URL. Строки ошибок должны содержать операцию и безопасный адрес сервиса, но никогда — токен.

- [ ] **Шаг 2: запустить тесты клиента и подтвердить сбой**

Выполнить: `go test ./internal/zabbix -count=1`

Ожидается: FAIL, поскольку пакет ещё не существует.

- [ ] **Шаг 3: реализовать типизированный транспорт и методы**

Определить внутреннюю оболочку с результатом `json.RawMessage` и типизированной ошибкой `RPCError`:

```go
type RPCError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    string `json:"data"`
}

func (e *RPCError) Error() string {
    return fmt.Sprintf("zabbix API error %d: %s: %s", e.Code, e.Message, e.Data)
}
```

Тело запроса использует JSON-RPC `2.0`, атомарный числовой идентификатор, метод и параметры; `auth` включается только в устаревшем режиме. Ограничить тела ответов 8 МиБ, требовать HTTP 2xx, закрывать тело каждого ответа, проверять идентификаторы ответов и оборачивать ошибки в формат `zabbix <method> at <scheme://host/path>: ...`.

Использовать следующие точные параметры API:

```go
host.get: {"output": ["hostid", "host"], "sortfield": "hostid"}
item.get discovery: {
  "output": ["itemid", "hostid", "name", "key_", "value_type"],
  "hostids": hostIDs,
  "filter": {"value_type": ["0", "3"]},
  "sortfield": ["hostid", "itemid"]
}
item.get values: {
  "output": ["itemid", "lastvalue", "lastclock"],
  "itemids": itemIDs,
  "sortfield": "itemid"
}
```

- [ ] **Шаг 4: запустить тесты, проверку гонок и vet для клиента**

Выполнить:

```bash
go test ./internal/zabbix -count=1
go test -race ./internal/zabbix -count=1
go vet ./internal/zabbix
```

Ожидается: PASS, токен не появляется в перехваченном тексте ошибок.

- [ ] **Шаг 5: закоммитить клиент Zabbix**

```bash
git add internal/zabbix
git commit -m "feat: add typed Zabbix API client"
```

---

### Задача 3: фильтры обнаружения, лимиты и атомарные снимки

**Файлы:**
- Создать: `internal/discovery/select.go`
- Создать: `internal/discovery/snapshot.go`
- Тест: `internal/discovery/select_test.go`
- Тест: `internal/discovery/snapshot_test.go`

**Интерфейсы:**
- Принимает: `zabbix.Host` и `zabbix.Item` из задачи 2.
- Предоставляет:

```go
type Filters struct {
    HostInclude, HostExclude, ItemKeyInclude, ItemKeyExclude *regexp.Regexp
}
type ItemMeta struct { ID, HostID, Host, Name, Key, ValueType string }
func Select([]zabbix.Host, []zabbix.Item, Filters, int) (selected []ItemMeta, filtered, limited int)
type Snapshot struct { /* immutable */ }
func NewSnapshot([]ItemMeta) *Snapshot
func (s *Snapshot) Items() []ItemMeta
type Store struct { /* atomic pointer */ }
func (s *Store) Load() *Snapshot
func (s *Store) Replace(*Snapshot)
```

- [ ] **Шаг 1: написать падающие тесты отбора**

Создать табличные сценарии, доказывающие следующее:

```go
// Узлы: prod-a, prod-a-backup, dev-a.
// Включить prod-.*, затем исключить .*-backup => остаётся только prod-a.
// Элементы: system.cpu.util, vm.memory.size, system.log.
// Включить system\..*|vm\..*, затем исключить .*\.log => процессор и память.
// max=1 сохраняет первый элемент каждого узла в порядке Zabbix и увеличивает limited.
// Элементы с неизвестными идентификаторами узлов отфильтровываются.
```

Тесты снимков должны изменять входной срез после `NewSnapshot` и срез, возвращённый `Items`; ни одно изменение не должно влиять на сохранённое содержимое. Конкурентный тест многократно вызывает `Replace` и `Load` под детектором гонок.

- [ ] **Шаг 2: запустить тесты обнаружения и подтвердить сбой**

Выполнить: `go test ./internal/discovery -count=1`

Ожидается: FAIL, поскольку пакет ещё не существует.

- [ ] **Шаг 3: реализовать отбор и неизменяемое хранилище**

Создать индекс узлов по идентификаторам, проверять ненулевые выражения включения перед исключениями, учитывать все удаления в `filtered`, а элементы сверх лимита — в `limited`. Сохранить порядок входных элементов. Реализовать `Store` через `atomic.Pointer[Snapshot]`; копировать срезы при создании снимка и при доступе к нему.

- [ ] **Шаг 4: запустить обычные тесты и проверку гонок**

Выполнить:

```bash
go test ./internal/discovery -count=1
go test -race ./internal/discovery -count=1
```

Ожидается: PASS.

- [ ] **Шаг 5: закоммитить логику обнаружения**

```bash
git add internal/discovery
git commit -m "feat: add Zabbix discovery snapshots"
```

---

### Задача 4: преобразование метрик OpenTelemetry

**Файлы:**
- Создать: `internal/metrics/name.go`
- Создать: `internal/metrics/builder.go`
- Тест: `internal/metrics/name_test.go`
- Тест: `internal/metrics/builder_test.go`

**Интерфейсы:**
- Принимает: `discovery.ItemMeta` и `zabbix.Value`.
- Предоставляет:

```go
func Name(prefix, itemKey string) (string, error)
type Config struct { Prefix string; ConstLabels map[string]string; ScopeName string }
type Stats struct { Emitted, Invalid, Missing int64 }
func Build([]discovery.ItemMeta, []zabbix.Value, Config) (pmetric.Metrics, Stats)
```

- [ ] **Шаг 1: написать падающие тесты имён и преобразования**

Таблица имён:

```go
{"zabbix_", "system.cpu.util", "zabbix_system_cpu_util"}
{"zabbix_", `vfs.fs.size[/,free]`, "zabbix_vfs_fs_size___free"}
{"", "9bad", "_9bad"}
{"zabbix_", "ends...", "zabbix_ends"}
```

Входные данные теста построителя содержат одно число с плавающей точкой, одно беззнаковое целое, некорректное числовое значение, значение без метаданных и постоянные метки, включая конфликтующую `host`. Проверить следующее:

- Результат содержит одну запись resource-metrics и одну запись scope-metrics.
- Корректные элементы представлены метриками gauge со значениями double.
- `lastclock=1700000000` преобразуется в `pcommon.Timestamp(1700000000 * 1_000_000_000)`.
- Описание совпадает с отображаемым именем элемента данных Zabbix.
- Присутствует `env=production`.
- Зарезервированный атрибут `host` берётся из обнаружения, а не из постоянных меток.
- `Stats` точно учитывает корректные, некорректные и отсутствующие значения.

- [ ] **Шаг 2: запустить тесты преобразования и подтвердить сбой**

Выполнить: `go test ./internal/metrics -count=1`

Ожидается: FAIL, поскольку пакет ещё не существует.

- [ ] **Шаг 3: реализовать построение имён и данных pdata**

Нормализовать ключ элемента посимвольно, заменяя символы вне `[a-zA-Z0-9_:]` на `_`, удаляя завершающие подчёркивания и обеспечивая соответствие первого символа итогового имени `[a-zA-Z_:]`. Возвращать ошибку, если корректного имени не осталось.

Создавать одну метрику для каждого значения, сопоставленного с элементом. Установить переданное имя области инструментирования, имя/описание метрики, значение и временную метку точки gauge, затем добавить постоянные атрибуты и перезаписать четыре зарезервированных атрибута. Разбирать значения через `strconv.ParseFloat`, а временные метки — через `strconv.ParseInt`.

- [ ] **Шаг 4: запустить тесты преобразования и все внутренние тесты**

Выполнить:

```bash
go test ./internal/metrics -count=1
go test ./internal/... -count=1
```

Ожидается: PASS.

- [ ] **Шаг 5: закоммитить преобразование**

```bash
git add internal/metrics go.mod go.sum
git commit -m "feat: convert Zabbix values to OTel metrics"
```

---

### Задача 5: планировщик двух заданий и жизненный цикл приёмника

**Файлы:**
- Создать: `receiver/zabbixreceiver/scheduler.go`
- Создать: `receiver/zabbixreceiver/receiver.go`
- Тест: `receiver/zabbixreceiver/scheduler_test.go`
- Тест: `receiver/zabbixreceiver/receiver_test.go`

**Интерфейсы:**
- Принимает: конфигурацию из задачи 1, `zabbix.API`, `discovery.Store`, `discovery.Select`, `metrics.Build` и последующий `consumer.Metrics`.
- Предоставляет неэкспортируемый приёмник, реализующий `receiver.Metrics`:

```go
type zabbixReceiver struct { /* config, API, next consumer, store, cancellation, wait group */ }
func newReceiver(receiver.Settings, *Config, consumer.Metrics, zabbix.API) (*zabbixReceiver, error)
func (r *zabbixReceiver) Start(context.Context, component.Host) error
func (r *zabbixReceiver) Shutdown(context.Context) error
func (r *zabbixReceiver) discover(context.Context) error
func (r *zabbixReceiver) values(context.Context) error
```

- [ ] **Шаг 1: написать падающие тесты чистой логики планировщика**

Выделить внедряемые функции:

```go
type timerFunc func(context.Context, time.Duration) error
type jitterFunc func(time.Duration) time.Duration
func runJob(context.Context, JobConfig, time.Duration, timerFunc, jitterFunc, func(context.Context) error, func(error))
```

Тесты должны подтверждать последовательность задержек:

- `run_on_start=true`: случайная задержка, запуск, интервал, случайная задержка, запуск.
- `run_on_start=false`: интервал, случайная задержка, запуск.
- Каждый запуск получает собственный контекст с тайм-аутом.
- Отмена останавливает цикл до следующего запуска.
- Заблокированный запуск не может перекрываться с другим запуском того же задания.
- Ошибки передаются в функцию обратного вызова для отчётности и не останавливают последующие циклы.

- [ ] **Шаг 2: написать падающие тесты операций приёмника**

Использовать подставной `zabbix.API` и записывающий `consumer.Metrics`. Покрыть:

```go
func TestDiscoverReplacesSnapshotOnlyAfterSuccess(t *testing.T)
func TestDiscoverRetainsSnapshotOnItemFailure(t *testing.T)
func TestValuesNoSnapshotIsNoOp(t *testing.T)
func TestValuesChunksRequestsAndConsumesOneBatch(t *testing.T)
func TestValuesAbortsBatchWhenAChunkFails(t *testing.T)
func TestValuesReturnsConsumerError(t *testing.T)
func TestStartAndShutdownStopAllJobs(t *testing.T)
```

Для разбиения на порции настроить `items_per_request=2`, подготовить пять элементов и проверить размеры запросов `2,2,1`, а также один пакет из пяти точек, переданный следующему компоненту.

- [ ] **Шаг 3: запустить тесты приёмника и подтвердить сбой**

Выполнить: `go test ./receiver/zabbixreceiver -run 'Test(Discover|Values|Start|RunJob)' -count=1`

Ожидается: FAIL, поскольку функции жизненного цикла и планировщика ещё не существуют.

- [ ] **Шаг 4: реализовать планировщик и операции приёмника**

`Start` создаёт один отменяемый контекст жизненного цикла, запускает только включённые циклы и добавляет две записи в группу ожидания. `Shutdown` выполняет отмену один раз и возвращается либо после завершения группы ожидания, либо по истечении контекста вызывающей стороны.

`discover` вызывает `Hosts`, собирает их идентификаторы, вызывает `Items`, компилирует настроенные фильтры один раз при создании, отбирает метаданные и заменяет хранилище только после успеха обоих вызовов.

`values` копирует элементы снимка, перебирает точные порции, накапливает все полученные значения, прерывается при любой ошибке запроса, строит метрики, пропускает доставку следующему компоненту при отсутствии корректных точек и в остальных случаях вызывает `ConsumeMetrics` один раз.

- [ ] **Шаг 5: запустить тесты приёмника, проверку гонок и тесты репозитория**

Выполнить:

```bash
go test ./receiver/zabbixreceiver -count=1
go test -race ./receiver/zabbixreceiver -count=1
go test ./... -count=1
```

Ожидается: PASS без утечек горутин и сообщений о гонках.

- [ ] **Шаг 6: закоммитить рабочую логику приёмника**

```bash
git add receiver/zabbixreceiver
git commit -m "feat: run Zabbix discovery and values jobs"
```

---

### Задача 6: фабрика Collector, собственная телеметрия и интеграция с HTTP-стендом

**Файлы:**
- Создать: `receiver/zabbixreceiver/factory.go`
- Создать: `receiver/zabbixreceiver/telemetry.go`
- Тест: `receiver/zabbixreceiver/factory_test.go`
- Тест: `receiver/zabbixreceiver/telemetry_test.go`
- Изменить: `receiver/zabbixreceiver/receiver.go`
- Изменить: `receiver/zabbixreceiver/receiver_test.go`

**Интерфейсы:**
- Предоставляет: `func NewFactory() receiver.Factory` для типа компонента `zabbix` со стабильностью метрик уровня development.
- Предоставляет неэкспортируемые инструменты телеметрии с именами:
  - `otelcol_receiver_zabbix_discover_attempts`
  - `otelcol_receiver_zabbix_discover_errors`
  - `otelcol_receiver_zabbix_discover_duration`
  - `otelcol_receiver_zabbix_values_attempts`
  - `otelcol_receiver_zabbix_values_errors`
  - `otelcol_receiver_zabbix_values_duration`
  - `otelcol_receiver_zabbix_emitted_points`
  - `otelcol_receiver_zabbix_invalid_values`
  - `otelcol_receiver_zabbix_filtered_items`
  - `otelcol_receiver_zabbix_limited_items`

Этот точный список из десяти инструментов является обязательным контрактом приёмки телеметрии. Попытки и длительности охватывают каждый соответствующий цикл; ошибки — циклы, вернувшие ошибку; выданные точки учитывают корректно построенные gauge до возврата потребителя; некорректные значения включают неверные числа, временные метки и непригодные имена метрик; счётчики отфильтрованных и ограниченных элементов соответствуют отбору при обнаружении. Не добавлять отдельные инструменты для успешных операций, числа узлов, числа элементов, запрошенных элементов или сбоев следующего компонента.

- [ ] **Шаг 1: написать падающие тесты фабрики и телеметрии**

Проверить `NewFactory().Type().String() == "zabbix"`, равенство конфигураций по умолчанию и успешное создание с `receivertest.NewNopSettings`. Использовать ручной считыватель SDK, чтобы после записи проверить одну попытку обнаружения, одну выданную точку и один замер длительности.

- [ ] **Шаг 2: написать интеграционный тест внутри процесса**

Запустить один `httptest.Server`, отвечающий на `host.get` и две формы `item.get`. Создать приёмник через фабрику с интервалами заданий 5 мс, использовать записывающий потребитель, запустить приёмник и потребовать выдачу метрики в течение двух секунд. Проверить точные имя, значение, временную метку и атрибуты, затем остановить приёмник и убедиться, что число запросов перестало меняться.

- [ ] **Шаг 3: запустить интеграционные тесты и подтвердить сбой**

Выполнить: `go test ./receiver/zabbixreceiver -run 'Test(NewFactory|Telemetry|ReceiverHTTPIntegration)' -count=1`

Ожидается: FAIL, поскольку фабрика и телеметрия ещё не существуют.

- [ ] **Шаг 4: реализовать фабрику и подключение телеметрии**

Создание через фабрику должно проверять тип `*Config`, создавать копию перед применением окружения, вызывать `ResolveEnv(os.LookupEnv)`, вызывать только чистый валидатор итоговой конфигурации, создавать HTTP-клиент, инициализировать телеметрию и вызывать `newReceiver`. Операции приёмника записывают попытки и длительности один раз за цикл, счётчики ошибок при возврате ошибок, счётчики отбора при обнаружении и счётчики преобразования.

- [ ] **Шаг 5: выполнить все проверки качества Go**

Выполнить:

```bash
gofmt -w receiver internal
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
```

Ожидается: все команды завершаются успешно.

- [ ] **Шаг 6: закоммитить публичный компонент Collector**

```bash
git add receiver/zabbixreceiver go.mod go.sum
git commit -m "feat: expose Zabbix Collector receiver"
```

---

### Задача 7: специализированная сборка Collector и пример конвейера

**Файлы:**
- Создать: `cmd/otelcol-zabbix/main.go`
- Создать: `cmd/otelcol-zabbix/components.go`
- Тест: `cmd/otelcol-zabbix/components_test.go`
- Создать: `configs/otelcol.yaml`
- Создать: `Makefile`

**Интерфейсы:**
- Принимает: `zabbixreceiver.NewFactory()`.
- Включает стандартные фабрики `batch`, `memory_limiter`, `prometheusremotewrite` и `health_check` версии v0.153.0.
- Создаёт бинарный файл `bin/otelcol-zabbix`.

- [ ] **Шаг 1: добавить зависимости сборки и падающий тест перечня компонентов**

Выполнить:

```bash
go get go.opentelemetry.io/collector/otelcol@v0.153.0
go get go.opentelemetry.io/collector/processor/batchprocessor@v0.153.0
go get go.opentelemetry.io/collector/processor/memorylimiterprocessor@v0.153.0
go get github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter@v0.153.0
go get github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension@v0.153.0
```

Написать `TestComponents`, вызывающий `components()` и проверяющий точные ключи карт: приёмники `zabbix`; процессоры `batch,memory_limiter`; экспортёры `prometheusremotewrite`; расширения `health_check`.

- [ ] **Шаг 2: запустить тест перечня компонентов и подтвердить сбой**

Выполнить: `go test ./cmd/otelcol-zabbix -count=1`

Ожидается: FAIL, поскольку `components()` ещё не существует.

- [ ] **Шаг 3: реализовать команду Collector**

Создать карты фабрик с помощью функций Collector `MakeFactoryMap`. `main.go` вызывает `otelcol.NewCommand` с информацией о сборке: команда `otelcol-zabbix`, описание `OpenTelemetry Collector with Zabbix receiver`, версия, переданная через `-ldflags`, и `components` как функция обратного вызова фабрик. При сбое команды завершаться с ненулевым кодом.

- [ ] **Шаг 4: добавить пример конфигурации для эксплуатации**

`configs/otelcol.yaml` должен настраивать:

```yaml
extensions:
  health_check:
    endpoint: 0.0.0.0:13133
receivers:
  zabbix:
    schedule:
      jitter: 5s
      jobs:
        discover: {enabled: true, run_on_start: true, interval: 5m, timeout: 60s}
        values: {enabled: true, run_on_start: false, interval: 30s, timeout: 20s}
    prom:
      prefix: zabbix_
      const_labels: {env: production, instance: zabbix-01}
    zabbix:
      url: ${env:ZABBIX_URL}
      token: ${env:ZABBIX_TOKEN}
      timeout: 30s
      limits: {max_metrics_per_host: 1000, items_per_request: 1000}
      filters: {host_include_regex: "", host_exclude_regex: "", item_key_include_regex: "", item_key_exclude_regex: ""}
processors:
  memory_limiter: {check_interval: 1s, limit_mib: 256}
  batch: {}
exporters:
  prometheusremotewrite:
    endpoint: ${env:VICTORIAMETRICS_REMOTE_WRITE_URL}
service:
  extensions: [health_check]
  pipelines:
    metrics:
      receivers: [zabbix]
      processors: [memory_limiter, batch]
      exporters: [prometheusremotewrite]
```

- [ ] **Шаг 5: добавить воспроизводимые цели Make**

Создать цели `fmt`, `test`, `test-race`, `vet`, `build`, `validate-config`, `docker-build`, `compose-config`, `demo-up`, `demo-verify` и `demo-down`. `build` записывает `bin/otelcol-zabbix`; `validate-config` задаёт фиктивные несекретные значения окружения и вызывает `bin/otelcol-zabbix validate --config configs/otelcol.yaml`.

- [ ] **Шаг 6: собрать и проверить сборку**

Выполнить:

```bash
make fmt
make test
make vet
make build
make validate-config
bin/otelcol-zabbix components
```

Ожидается: сборка и валидация проходят успешно; вывод компонентов содержит `zabbix` и `prometheusremotewrite`.

- [ ] **Шаг 7: закоммитить сборку**

```bash
git add cmd configs Makefile go.mod go.sum
git commit -m "feat: build custom Zabbix Collector distribution"
```

---

### Задача 8: упаковка для контейнера, systemd и Kubernetes

**Файлы:**
- Создать: `Dockerfile`
- Создать: `.dockerignore`
- Создать: `deployments/systemd/otelcol-zabbix.service`
- Создать: `deployments/systemd/otelcol-zabbix.yaml`
- Создать: `deployments/systemd/otelcol-zabbix.env.example`
- Создать: `deployments/kubernetes/namespace.yaml`
- Создать: `deployments/kubernetes/secret.example.yaml`
- Создать: `deployments/kubernetes/configmap.yaml`
- Создать: `deployments/kubernetes/deployment.yaml`
- Создать: `deployments/kubernetes/service.yaml`
- Создать: `deployments/kubernetes/kustomization.yaml`
- Тест: `internal/packaging/assets_test.go`

**Интерфейсы:**
- Создаёт образ OCI `zabbix-otel-collector:local` с точкой входа `/otelcol-zabbix` и конфигурацией по умолчанию `/etc/otelcol-zabbix/config.yaml`.
- Создаёт пользователя/группу службы systemd `otelcol-zabbix` и рабочую нагрузку Kubernetes `otelcol-zabbix` в пространстве имён `observability`.

- [ ] **Шаг 1: написать падающие статические тесты файлов развёртывания**

Тесты читают файлы от корня репозитория и проверяют:

- Dockerfile содержит этап сборки Go, числовой непривилегированный `USER 10001:10001` и не содержит значения токена.
- Юнит systemd содержит `User=otelcol-zabbix`, `EnvironmentFile=/etc/otelcol-zabbix/otelcol-zabbix.env`, `Restart=on-failure`, `NoNewPrivileges=true`, `ProtectSystem=strict` и точный `ExecStart`.
- Kubernetes Deployment содержит `runAsNonRoot: true`, `readOnlyRootFilesystem: true`, сброшенные capabilities, проверки состояния на порту 13133, запросы/лимиты ресурсов, ссылки на Secret и не содержит объектов RBAC.
- Kustomization перечисляет пространство имён, пример Secret, ConfigMap, Deployment и Service.

- [ ] **Шаг 2: запустить тесты упаковки и подтвердить сбой**

Выполнить: `go test ./internal/packaging -count=1`

Ожидается: FAIL, поскольку файлы развёртывания ещё не существуют.

- [ ] **Шаг 3: реализовать контейнер без прав root**

Использовать `golang:1.25-alpine` для сборки и `gcr.io/distroless/static-debian13:nonroot` для запуска. Собирать с `CGO_ENABLED=0`, `-trimpath` и флагами компоновщика для удаления отладочной информации. Копировать бинарный файл и `configs/otelcol.yaml`; запускать с числовыми UID/GID 10001 и объявить порты 13133 и 8888.

- [ ] **Шаг 4: реализовать файлы развёртывания на виртуальной машине**

Юнит читает `/etc/otelcol-zabbix/otelcol-zabbix.env`, выполняет `/usr/local/bin/otelcol-zabbix --config=/etc/otelcol-zabbix/config.yaml`, использует отдельный каталог состояния и применяет меры защиты, проверяемые тестами. Шаблон окружения определяет `ZABBIX_URL`, `ZABBIX_TOKEN` и `VICTORIAMETRICS_REMOTE_WRITE_URL` с безопасными примерами значений.

- [ ] **Шаг 5: реализовать файлы Kubernetes**

Deployment содержит одну реплику, последовательные обновления, специализированный образ, значения окружения из Secret/ConfigMap, том конфигурации, порт 13133, HTTP-проверки `/`, запросы ресурсов 100m/128Mi, лимиты 500m/512Mi и 30 секунд на корректное завершение. Service имеет тип `ClusterIP` и публикует только проверку состояния и внутреннюю телеметрию. ClusterRole, Role и подключение токена служебной учётной записи не требуются.

- [ ] **Шаг 6: проверить упаковку**

Выполнить:

```bash
go test ./internal/packaging -count=1
docker build -t zabbix-otel-collector:local .
kubectl kustomize deployments/kubernetes >/tmp/zabbix-otel-rendered.yaml
systemd-analyze verify deployments/systemd/otelcol-zabbix.service
```

Ожидается: тесты и доступные валидаторы проходят успешно; образ Docker сообщает пользователя `10001:10001`. Если `systemd-analyze` недоступен в ОС разработки, запустить его внутри актуального контейнера Debian с systemd и записать команду в заметках о проверке.

- [ ] **Шаг 7: закоммитить упаковку для развёртывания**

```bash
git add Dockerfile .dockerignore deployments internal/packaging
git commit -m "feat: package Collector for VM and Kubernetes"
```

---

### Задача 9: демонстрация реальной передачи из Zabbix в VictoriaMetrics через Docker Compose

**Файлы:**
- Создать: `compose.yaml`
- Создать: `demo/Dockerfile.tools`
- Создать: `demo/bootstrap.sh`
- Создать: `demo/producer.sh`
- Создать: `demo/collector.yaml.tmpl`
- Создать: `demo/verify.sh`
- Изменить: `internal/packaging/assets_test.go`

**Интерфейсы:**
- Использует образы сервера/веб-интерфейса Zabbix 7.4.12 для PostgreSQL, PostgreSQL 17 Alpine и VictoriaMetrics v1.148.0.
- Создаёт метрику `zabbix_demo_counter` с `host="otel-demo-host"`, `item_key="demo.counter"`, `env="compose"` и изменяющимися числовыми значениями.

- [ ] **Шаг 1: расширить падающие тесты упаковки проверками топологии Compose**

Разобрать `compose.yaml` как YAML и проверить сервисы `postgres`, `zabbix-server`, `zabbix-web`, `bootstrap`, `producer`, `otelcol-zabbix` и `victoriametrics`; точные закреплённые теги Zabbix и VictoriaMetrics; условия состояния/зависимостей; частную сеть; общий том `demo-config`, используемый только начальной настройкой и Collector.

- [ ] **Шаг 2: написать скрипт начальной настройки с идемпотентными функциями API**

Реализовать функции POSIX shell `rpc_unauthenticated`, `rpc_session` и `rpc_bearer` с использованием `curl --fail-with-body` и `jq -e`. Скрипт должен:

1. Опрашивать `apiinfo.version` до готовности.
2. Войти как `Admin` с паролем, используемым только в Compose.
3. Найти или создать группу узлов `OpenTelemetry Demo`.
4. Найти или создать узел `otel-demo-host`.
5. Найти или создать элемент данных trapper `demo.counter` с типом значения float.
6. Удалить предыдущий токен с именем `otel-demo-receiver`, затем создать и сгенерировать токен для текущего пользователя.
7. Сформировать `/generated/otelcol.yaml` из `demo/collector.yaml.tmpl`, включив сгенерированный токен только в этот файл рабочего тома. Запускать начальную настройку от root исключительно для установки владельца `10001:10001` и режима `0400`, чтобы непривилегированный Collector мог читать файл без раскрытия токена через подстановки Compose или отслеживаемые файлы.
8. Последним записать `/generated/ready`.

Каждый ответ API необходимо проверять на `.error`; вывод токена никогда не должен печататься.

- [ ] **Шаг 3: написать скрипты генерации и проверки**

`producer.sh` ожидает порт 10051 сервера Zabbix и отправляет одно возрастающее значение каждые пять секунд:

```sh
value=1
while :; do
  zabbix_sender -z zabbix-server -s otel-demo-host -k demo.counter -o "$value"
  value=$((value + 1))
  sleep 5
done
```

`verify.sh` опрашивает до 180 секунд:

```text
GET http://victoriametrics:8428/api/v1/query?query=zabbix_demo_counter{host="otel-demo-host",env="compose"}
```

Он завершается успешно только если `.status == "success"`, существует ровно один результат, `item_key == "demo.counter"`, `hostid` и `itemid` непустые, значение образца разбирается как положительное число, а временная метка образца не предшествует запуску проверки. Он игнорирует подходящие устаревшие образцы и печатает только JSON принятой свежей метрики, никогда — учётные данные.

- [ ] **Шаг 4: реализовать топологию Compose и демонстрационную конфигурацию приёмника**

Использовать короткое расписание демонстрации: нулевая случайная задержка, обнаружение при старте и каждые 15 секунд, сбор значений при старте и каждые 5 секунд, лимиты запросов 100. Настроить расширение проверки состояния Collector и адрес `prometheusremotewrite` `http://victoriametrics:8428/api/v1/write`.

Собрать `demo/Dockerfile.tools` на основе `alpine:3.24.1`, включив только `curl`, `jq` и сертификаты центров сертификации. Использовать этот локальный образ утилит для начальной настройки и проверки; для `zabbix_sender` в генераторе использовать официальный образ агента Zabbix.

Закрепить следующие образы:

```yaml
postgres:17-alpine
zabbix/zabbix-server-pgsql:alpine-7.4.12
zabbix/zabbix-web-nginx-pgsql:alpine-7.4.12
zabbix/zabbix-agent2:alpine-7.4.12
victoriametrics/victoria-metrics:v1.148.0
```

Собирать сервис Collector из локального Dockerfile. Collector зависит от успешной начальной настройки, генератор — от успешной начальной настройки и исправного сервера Zabbix, а проверяющий сервис запускается в профиле Compose `verify`.

- [ ] **Шаг 5: проверить синтаксис и статический контракт**

Выполнить:

```bash
go test ./internal/packaging -count=1
docker compose config --quiet
shellcheck demo/*.sh
```

Ожидается: все проверки проходят успешно, а раскрытая конфигурация Compose не содержит токена приёмника.

- [ ] **Шаг 6: запустить реальную сквозную демонстрацию**

Выполнить:

```bash
docker compose up -d --build postgres zabbix-server zabbix-web bootstrap producer victoriametrics otelcol-zabbix
docker compose --profile verify run --rm verify
docker compose logs --no-color otelcol-zabbix
```

Ожидается: проверка печатает один ряд `zabbix_demo_counter` со всеми обязательными метками; журналы Collector показывают успешные циклы обнаружения и сбора значений без ошибок аутентификации.

- [ ] **Шаг 7: остановить демонстрацию и закоммитить её**

Выполнить: `docker compose down --volumes --remove-orphans`

Затем закоммитить:

```bash
git add compose.yaml demo internal/packaging/assets_test.go
git commit -m "feat: demonstrate Zabbix to VictoriaMetrics flow"
```

---

### Задача 10: документация, CI и итоговая проверка

**Файлы:**
- Создать: `README.md`
- Создать: `docs/configuration.md`
- Создать: `docs/deployment.md`
- Создать: `.github/workflows/ci.yml`
- Изменить: `Makefile`

**Интерфейсы:**
- Документирует точный публичный контракт YAML и окружения, команды сборки/запуска, поддерживаемое поведение аутентификации Zabbix, сопоставление метрик, поведение при эксплуатационных сбоях и все четыре способа развёртывания.
- CI выполняет форматирование, тесты, проверку гонок, vet, сборку, валидацию конфигурации, тесты упаковки и валидацию конфигурации Compose.

- [ ] **Шаг 1: написать падающий тест контракта документации**

Расширить `internal/packaging/assets_test.go`, потребовав разделы README `Architecture`, `Quick start`, `Configuration`, `Metric mapping`, `Docker Compose demo`, `VM/systemd`, `Kubernetes`, `Testing` и `Security`; потребовать ссылку на утверждённый проект и проверить наличие каждой поддерживаемой переменной окружения в `docs/configuration.md`.

- [ ] **Шаг 2: запустить тест документации и подтвердить сбой**

Выполнить: `go test ./internal/packaging -run TestDocumentationContract -count=1`

Ожидается: FAIL, поскольку файлы документации ещё не существуют.

- [ ] **Шаг 3: написать документацию для пользователей и операторов**

Быстрый старт README должен содержать точные команды:

```bash
make build
ZABBIX_URL=http://zabbix.example/api_jsonrpc.php \
ZABBIX_TOKEN=replace-me \
VICTORIAMETRICS_REMOTE_WRITE_URL=http://victoriametrics:8428/api/v1/write \
./bin/otelcol-zabbix --config configs/otelcol.yaml
```

Документировать, что современный Zabbix использует авторизацию Bearer, а старый Zabbix 5.x может использовать токен сеанса `user.login` через автоматический переход к устаревшему режиму. Указать, что этот режим кешируется, учётные данные никогда не записываются в журнал, а долгоживущие токены API требуют поддерживающей их версии Zabbix.

Документировать построение имён метрик, преобразование в gauge, временную метку точки, приоритет зарезервированных меток, порядок фильтров, сохранение снимка обнаружения, политику частичных пакетов и приоритет окружения. Включить команды установки systemd и `kubectl apply -k deployments/kubernetes` с явными указаниями по редактированию Secret.

- [ ] **Шаг 4: добавить процесс CI**

Настроить GitHub Actions для отправок в репозиторий и pull request с Go 1.25, кешем зависимостей, `make fmt` и проверкой отсутствия изменений, `make test`, `make test-race`, `make vet`, `make build`, `make validate-config`, тестами упаковки, `docker compose config --quiet` и сборкой образа Docker. Не запускать демонстрацию из нескольких контейнеров при каждой отправке; предоставить её как ручное задание `workflow_dispatch` с тайм-аутом 15 минут и гарантированной очисткой через `docker compose down --volumes`.

- [ ] **Шаг 5: выполнить полную матрицу проверок из чистого состояния процессов**

Выполнить:

```bash
make fmt
git diff --check
make test
make test-race
make vet
make build
make validate-config
make compose-config
docker build -t zabbix-otel-collector:verify .
docker run --rm zabbix-otel-collector:verify components
```

Ожидается: каждая команда завершается с кодом ноль; вывод компонентов контейнера перечисляет приёмник и ожидаемые компоненты конвейера.

- [ ] **Шаг 6: выполнить итоговую проверку Compose**

Выполнить:

```bash
make demo-up
make demo-verify
make demo-down
```

Ожидается: VictoriaMetrics возвращает реальное значение trapper Zabbix с `host`, `hostid`, `item_key`, `itemid` и `env=compose`.

- [ ] **Шаг 7: проверить секреты и состояние репозитория**

Выполнить:

```bash
rg -n --hidden --glob '!.git/**' '(ZABBIX_TOKEN=.{8,}|Bearer [A-Za-z0-9]{16,}|Admin.*zabbix)' .
git status --short
git log --oneline --decorate -12
```

Ожидается: совпадения содержат только документированные заглушки или учётные данные начальной настройки, используемые только в Compose; сгенерированные токены не отслеживаются; рабочее дерево содержит только намеренные обновления флажков плана, если они ведутся.

- [ ] **Шаг 8: закоммитить документацию и CI**

```bash
git add README.md docs/configuration.md docs/deployment.md .github/workflows/ci.yml Makefile internal/packaging/assets_test.go
git commit -m "docs: document and verify Zabbix receiver"
```

- [ ] **Шаг 9: выполнить проверку завершения**

Сопоставить каждый критерий завершения из `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md` со свежим выводом команд. В итоговой передаче результата указать все недоступные валидаторы для хостовой ОС и их контейнерные эквиваленты; не заявлять о завершении сквозной проверки, пока `make demo-verify` не обнаружит метрику в VictoriaMetrics.
