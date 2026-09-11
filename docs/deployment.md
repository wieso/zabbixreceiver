# Руководство по развёртыванию

[Обзор проекта](../README.md) · [Настройка приёмника](configuration.md)

Все предложенные способы запускают одну и ту же специализированную сборку Collector. Соберите и закрепите артефакт, подходящий вашей среде, передавайте учётные данные только при запуске и защитите адреса проверки работоспособности и телеметрии, не требующие аутентификации.

## Готовый исполняемый файл релиза

Поддерживаемые исполняемые файлы релизов работают в Linux на `amd64` и `arm64`; Go не требуется. Выберите версию на [странице релизов GitHub](https://github.com/wieso/zabbixreceiver/releases), затем используйте соответствующее значение `ARCH`:

```bash
VERSION=1.2.3 # пример: замените на выбранную опубликованную версию
ARCH=amd64
curl -fLO "https://github.com/wieso/zabbixreceiver/releases/download/v${VERSION}/otelcol-zabbix_${VERSION}_linux_${ARCH}.tar.gz"
curl -fLO "https://github.com/wieso/zabbixreceiver/releases/download/v${VERSION}/checksums.txt"
sha256sum -c checksums.txt --ignore-missing
tar -xzf "otelcol-zabbix_${VERSION}_linux_${ARCH}.tar.gz"
sudo install -m 0755 "otelcol-zabbix_${VERSION}_linux_${ARCH}/otelcol-zabbix" /usr/local/bin/otelcol-zabbix
sudo install -d -m 0755 /etc/otelcol-zabbix
sudo install -m 0644 "otelcol-zabbix_${VERSION}_linux_${ARCH}/configs/otelcol.yaml" /etc/otelcol-zabbix/config.yaml
```

Задайте три переменные адресов и учётных данных при запуске и запустите Collector:

```bash
ZABBIX_URL=https://zabbix.example.com/api_jsonrpc.php \
ZABBIX_TOKEN=replace-with-zabbix-api-token \
VICTORIAMETRICS_REMOTE_WRITE_URL=https://victoriametrics.example.com/api/v1/write \
/usr/local/bin/otelcol-zabbix --config /etc/otelcol-zabbix/config.yaml
```

Токен выше — заполнитель. Используйте HTTPS, не оставляйте настоящие учётные данные в общей истории команд и применяйте механизм секретов службы в рабочей среде. Чтобы запустить загруженный файл как управляемую службу, продолжите с раздела VM/systemd; шаблоны unit и файла окружения находятся в репозитории в `deployments/systemd/`.

## Локальный исполняемый файл

Требования: Go 1.26.8 или новее, доступ к Zabbix API и адресу Prometheus remote-write.

```bash
make build
make validate-config
```

`make build` создаёт `bin/otelcol-zabbix`. `make validate-config` пересобирает его, задаёт фиктивные адреса без секретов и запускает команду Collector `validate` для `configs/otelcol.yaml`. Запустите программу со значениями для вашей среды:

```bash
ZABBIX_URL=https://zabbix.example.com/api_jsonrpc.php \
ZABBIX_TOKEN=replace-me \
VICTORIAMETRICS_REMOTE_WRITE_URL=https://victoriametrics.example.com/api/v1/write \
./bin/otelcol-zabbix --config configs/otelcol.yaml
```

В примере адрес проверки работоспособности — `http://127.0.0.1:13133/`; внутренние метрики Collector доступны на порту 8888. Конфигурация привязывает оба сервера ко всем интерфейсам, поэтому настройте межсетевой экран хоста или измените адреса прослушивания, если удалённый доступ не нужен.

## Контейнер

Соберите образ с тем же именем, что используется в примерах Kubernetes и Compose:

```bash
make docker-build
docker run --rm \
  -e ZABBIX_URL=https://zabbix.example.com/api_jsonrpc.php \
  -e ZABBIX_TOKEN=replace-me \
  -e VICTORIAMETRICS_REMOTE_WRITE_URL=https://victoriametrics.example.com/api/v1/write \
  -p 127.0.0.1:13133:13133 \
  -p 127.0.0.1:8888:8888 \
  zabbix-otel-collector:local
```

Образ с многоэтапной сборкой содержит статически скомпонованный исполняемый файл и пример конфигурации, запускается с числовыми UID/GID `10001:10001` и не содержит встроенного токена. Его команда по умолчанию — `--config=/etc/otelcol-zabbix/config.yaml`. Не передавайте настоящие секреты напрямую в общей командной строке; используйте подстановку секретов вашей контейнерной платформы. Образ принимает переменные окружения, поскольку конфигурация из комплекта ссылается на них.

Просмотрите состав встроенных компонентов без запуска конвейера:

```bash
docker run --rm zabbix-otel-collector:local components
```

## Демонстрация в Docker Compose

Требуются Docker Engine и Compose v2.24 или новее. Демонстрационная среда запускает PostgreSQL, Zabbix 7.4.12, задачу начальной настройки, генератор данных, специализированную сборку Collector, VictoriaMetrics и Grafana:

```bash
make demo-up
make demo-verify
```

Веб-интерфейсы доступны только через loopback хоста:

- Откройте Zabbix по адресу <http://127.0.0.1:8080/> и войдите с учётными данными `Admin` / `zabbix`, предназначенными только для Compose. Откройте **Мониторинг → Последние данные** (в английском интерфейсе **Monitoring → Latest data**), выберите узел `otel-demo-host` и найдите ключ элемента `demo.counter`.
- Откройте [дашборд мониторинга ресивера](http://127.0.0.1:3000/d/zabbix-receiver) в Grafana. Просмотр доступен без входа; источник VictoriaMetrics и дашборд загружаются автоматически из `demo/grafana/`.
- Откройте VictoriaMetrics VMUI по адресу <http://127.0.0.1:8428/vmui/> и выполните запросы `zabbix_api_demo_counter{host="otel-demo-host",env="compose"}` и `zabbix_stream_OpenTelemetry_demo_counter{host="otel-demo-host",env="compose"}`.

VictoriaMetrics каждые 5 секунд собирает внутреннюю телеметрию с `otelcol-zabbix:8888/metrics` через встроенный scraper (`demo/victoriametrics-scrape.yaml`). Дашборд позволяет выбрать `zabbix/api` или `zabbix/stream` и показывает accepted/refused, ошибки, p95 длительности, discovery, пропуски, HTTP-коды Streaming, экспорт, очередь, CPU и память. Панели экспортёра и процесса относятся ко всему Collector. Discovery неприменим к чистому Streaming; до первого успешного сбора время последнего успеха показывает No data. Для графиков скоростей подождите несколько scrape-интервалов.

Генератор увеличивает значение элемента Zabbix каждые пять секунд, а Collector опрашивает значения API каждые пять секунд, одновременно принимая данные Streaming вторым приёмником. Два ряда представляют один исходный счётчик с разным временем доставки. Экземпляры называются `zabbix/api` и `zabbix/stream`, их префиксы — `zabbix_api_` и `zabbix_stream_`.

Если порты хоста заняты, выберите другие без редактирования Compose:

```bash
ZABBIX_WEB_PORT=18080 VICTORIAMETRICS_PORT=18428 GRAFANA_PORT=13000 make demo-up
```

С этими переопределениями откройте `http://127.0.0.1:18080/`, `http://127.0.0.1:18428/vmui/` и `http://127.0.0.1:13000/d/zabbix-receiver`. Сохраняйте привязку интерфейсов к loopback: демонстрация использует общеизвестные учётные данные Zabbix и не настраивает аутентификацию VictoriaMetrics.

`make demo-verify` завершается успешно, только когда VictoriaMetrics возвращает и `zabbix_api_demo_counter`, и `zabbix_stream_OpenTelemetry_demo_counter` с `host="otel-demo-host"`, непустым `itemid`, `env="compose"` и положительным значением. API также должен содержать `hostid` и `item_key="demo.counter"`; в Streaming их быть не должно. Временная метка исходного образца должна быть не раньше запуска проверки, что обеспечивается через `timestamp(...)`; подходящие устаревшие ряды игнорируются. Проверка выводит только принятые свежие ряды метрик в компактном JSON и имеет жёсткий предел длительности процесса 180 секунд.

Сервер включает два рабочих процесса коннекторов (`ZBX_STARTCONNECTORS=2`). Начальная настройка создаёт или обновляет `otel-demo-streaming`, ограничивает его тегом демонстрационного элемента и генерирует отдельный Bearer-токен Streaming. Для активации конфигурации коннектора может потребоваться одно обновление кэша Zabbix. Адрес приёмника доступен внутри сети Compose и не публикуется на хосте.

Начальная настройка использует учётные данные `Admin`/`zabbix` только для Compose, создаёт или находит демонстрационный узел и элемент типа trapper, удаляет только предыдущий токен `otel-demo-receiver` текущего пользователя, генерирует замену и проверяет её. Токен записывается только в рабочий том `demo-config` в виде конфигурации Collector с правами `0400` и UID/GID `10001:10001`; он не выводится и не подставляется в `compose.yaml`. Эта среда служит демонстрацией и не является образцом управления учётными данными для рабочей эксплуатации.

После просмотра интерфейсов, в том числе после ошибок, выполните `make demo-down`. Команда удаляет контейнеры и все именованные тома демонстрации: базу Zabbix, данные VictoriaMetrics и сгенерированную конфигурацию с токенами. `demo-verify` запускает только одноразовый сервис профиля `verify`, без повторной начальной настройки и замены действующего токена.

## VM/systemd

Эти команды предполагают дистрибутив Linux с systemd и стандартными путями. При необходимости скорректируйте путь `nologin` для вашего хоста:

```bash
make build
sudo useradd --system --home-dir /var/lib/otelcol-zabbix --shell /usr/sbin/nologin otelcol-zabbix
sudo install -d -m 0750 -o root -g otelcol-zabbix /etc/otelcol-zabbix
sudo install -m 0755 bin/otelcol-zabbix /usr/local/bin/otelcol-zabbix
sudo install -m 0640 -o root -g otelcol-zabbix deployments/systemd/otelcol-zabbix.yaml /etc/otelcol-zabbix/config.yaml
sudo install -m 0640 -o root -g otelcol-zabbix deployments/systemd/otelcol-zabbix.env.example /etc/otelcol-zabbix/otelcol-zabbix.env
sudo install -m 0644 deployments/systemd/otelcol-zabbix.service /etc/systemd/system/otelcol-zabbix.service
```

Отредактируйте `/etc/otelcol-zabbix/otelcol-zabbix.env` от имени root и замените все заполнители. Не добавляйте кавычки или команды оболочки в этот файл окружения systemd:

```text
ZABBIX_URL=https://zabbix.example.com/api_jsonrpc.php
ZABBIX_TOKEN=replace-with-zabbix-api-token
VICTORIAMETRICS_REMOTE_WRITE_URL=https://victoriametrics.example.com/api/v1/write
```

Затем включите и проверьте службу:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now otelcol-zabbix
sudo systemctl status otelcol-zabbix
sudo journalctl -u otelcol-zabbix --since today
curl --fail http://127.0.0.1:13133/
```

Unit работает от выделенных пользователя и группы, перезапускается при сбое, создаёт `/var/lib/otelcol-zabbix` и применяет ограничения файловой системы, привилегий, устройств, ядра и семейств адресов. Адрес проверки работоспособности сейчас привязан к `0.0.0.0:13133`; ограничьте доступ политикой межсетевого экрана или измените `deployments/systemd/otelcol-zabbix.yaml` перед установкой. В примере systemd явно не настроен считыватель телеметрии на порту 8888.

Для ротации учётных данных отредактируйте файл окружения от имени root и перезапустите службу:

```bash
sudo systemctl restart otelcol-zabbix
```

## Kubernetes

Манифесты создают пространство имён `observability`, Secret типа Opaque, ConfigMap, один Deployment и Service типа ClusterIP. Они намеренно не создают Role, RoleBinding или ServiceAccount и отключают автоматическое монтирование токена служебной учётной записи.

Сначала соберите и опубликуйте неизменяемый образ, который кластер сможет загрузить, либо загрузите локальный образ в кластер разработки. При использовании реестра замените `zabbix-otel-collector:local` в `deployments/kubernetes/deployment.yaml` на неизменяемую ссылку.

Пока `deployments/kubernetes/secret.example.yaml` содержит только заполнители, проверьте дерево Kustomize без вывода сформированного Secret:

```bash
kubectl kustomize deployments/kubernetes >/dev/null
```

Только после этой проверки локально замените оба заполнителя в `deployments/kubernetes/secret.example.yaml`:

```yaml
stringData:
  ZABBIX_URL: https://zabbix.example.com/api_jsonrpc.php
  ZABBIX_TOKEN: replace-with-zabbix-api-token
```

Также задайте `VICTORIAMETRICS_REMOTE_WRITE_URL` в `deployments/kubernetes/configmap.yaml`. `stringData` — открытый текст, передаваемый в Kubernetes API, а не шифрование. Никогда не запускайте `kubectl kustomize` после внесения настоящих учётных данных: команда записывает `stringData` объекта Secret в стандартный вывод, где терминалы и журналы CI могут его сохранить. Не сохраняйте изменённый Secret в коммите, не выполняйте с ним подробные или печатающие содержимое пробные запуски, не включайте его в журналы CI и не оставляйте в общей рабочей копии. Для рабочей среды предпочтительны внешний контроллер секретов или отдельно управляемый Secret.

Примените нужный путь Kustomize напрямую; обычный вывод применения сообщает идентификаторы ресурсов, не раскрывая содержимое Secret:

```bash
kubectl apply -k deployments/kubernetes
kubectl -n observability rollout status deployment/otelcol-zabbix
kubectl -n observability get pods,service
```

Для локальной проверки работоспособности запустите блокирующее перенаправление порта в первом терминале:

```bash
kubectl -n observability port-forward service/otelcol-zabbix 13133:13133
```

Во втором терминале, пока перенаправление порта работает, выполните запрос проверки работоспособности:

```bash
curl --fail http://127.0.0.1:13133/
```

Pod работает с UID/GID 10001, запрещает повышение привилегий, сбрасывает все Linux capabilities, использует корневую файловую систему только для чтения и задаёт запросы и лимиты CPU/памяти. Пробы готовности и работоспособности обращаются к `/` на порту 13133. Service предоставляет проверку работоспособности на порту 13133 и телеметрию Collector на порту 8888 только внутри кластера. NetworkPolicy не входит в комплект; добавьте её, если доступ из всего пространства имён слишком широк.

API Deployment использует одну реплику и `Recreate`: при штатном обновлении старый Pod завершается до запуска нового. Это устраняет перекрытие API-опроса при rollout ценой перерыва сбора на время запуска и discovery. Если требуется HA, нужно внешнее управление владельцем шарда с lease/fencing; простое увеличение replicas или переход на RollingUpdate этого не обеспечивает. Recreate не является fencing при сетевом разделении или принудительном удалении Pod. Для отдельного Streaming Deployment rolling update допустим.

Для ротации учётных данных обновите Secret через ваш процесс управления секретами и перезапустите Deployment, чтобы переменные окружения были прочитаны заново:

```bash
kubectl -n observability rollout restart deployment/otelcol-zabbix
kubectl -n observability rollout status deployment/otelcol-zabbix
```

Удалите только ресурсы пространства имён, входящие в это развёртывание, сохранив Namespace `observability` и все посторонние рабочие нагрузки:

```bash
kubectl -n observability delete deployment/otelcol-zabbix service/otelcol-zabbix configmap/otelcol-zabbix secret/otelcol-zabbix
```

Не используйте `kubectl delete -k deployments/kubernetes` для обычной очистки. Поскольку `namespace.yaml` входит в эту Kustomization, команда удалит всё пространство имён `observability`, включая посторонние рабочие нагрузки в нём.

## Развёртывание Streaming

Используйте [configs/otelcol-streaming.yaml](../configs/otelcol-streaming.yaml) с тем же исполняемым файлом, образом или модулем Collector Builder. Передайте только `ZABBIX_STREAM_TOKEN` для входящей аутентификации и `VICTORIAMETRICS_REMOTE_WRITE_URL` для экспорта. Существующие примеры systemd и Kubernetes по умолчанию работают в режиме API; для переключения замените их конфигурацию Collector этим примером.

1. Привяжите `streaming.endpoint` к адресу, доступному из Zabbix, например `0.0.0.0:8081` внутри контейнера. Для Kubernetes добавьте TCP-порт 8081 контейнера и порт Service; для Docker опубликуйте порт или обеспечьте маршрутизацию в общей сети; для systemd разрешите входящий трафик от Zabbix в межсетевом экране хоста. Сохраните существующие порты проверки работоспособности и телеметрии.
2. Используйте частную сеть или завершайте HTTPS на обратном прокси. Задайте на прокси лимит тела запроса не ниже лимита приёмника и достаточный тайм-аут запроса. Собственная поддержка TLS в приёмнике отсутствует.
3. Задайте `StartConnectors=2` в `zabbix_server.conf` (переменная окружения официального контейнера: `ZBX_STARTCONNECTORS=2`) и перезапустите Zabbix. Подберите число рабочих процессов под ваши коннекторы и одновременные сеансы.
4. В разделе **Администрирование → Общие → Коннекторы** (в английском интерфейсе **Administration → General → Connectors**) создайте коннектор **Значения элементов данных** (**Item values**) с числовыми типами «с плавающей точкой» и «беззнаковое целое», URL `https://collector.example/v1/history` и аутентификацией **Bearer** с использованием `ZABBIX_STREAM_TOKEN`. По возможности задайте фильтр тегов. Для создания через API используйте `data_type: 0`, `item_value_type: 9` и `authtype: 5`.
5. Начните со 100 записей на сообщение, одного одновременного сеанса, пяти попыток, интервала между попытками 5 секунд и тайм-аута коннектора 10 секунд. Подберите число записей/сеансов, тайм-аут приёмника и лимит тела под вашу нагрузку. Перезагрузите кэш Zabbix (`zabbix_server -R config_cache_reload`) или дождитесь штатного обновления.
6. Убедитесь, что метрики поступают в последующие компоненты, и проверьте `values_errors` Collector и `zabbix[connector_queue]` Zabbix. Для автоматизированного примера двух режимов выполните `make demo-up` и `make demo-verify`.

Эти настройки коннектора соответствуют [протоколу Zabbix Streaming](https://www.zabbix.com/documentation/7.4/en/manual/config/export/streaming) и [объекту коннектора](https://www.zabbix.com/documentation/7.4/en/manual/api/reference/connector/object). Приёмнику Streaming не нужна учётная запись Zabbix API. Создание коннектора в Zabbix требует административных прав; это однократная настройка отправителя, а не зависимость приёмника во время работы. Приёмник не создаёт коннекторы самостоятельно.

Перед переключением адаптируйте запросы и панели к [контракту Streaming](configuration.md). Там же описаны фильтрация, HTTP-ответы и гарантии доставки.

## Диагностика

Проверка работоспособности Collector подтверждает доступность процесса, но не свежесть метрик в хранилище. Проверяйте время последних образцов и [внутреннюю телеметрию приёмника](configuration.md).

| Симптом | Что проверить |
| --- | --- |
| Collector не запускается | Сообщения валидации и [приоритет окружения](configuration.md) |
| API не обнаруживает элементы | Права токена, [фильтры и снимки](configuration.md) |
| Значения API перестали обновляться | Ошибки запросов, тайм-ауты и [поведение цикла сбора](configuration.md) |
| Коннектор не доставляет историю | Доступность адреса, HTTP-код ответа, очередь Zabbix и [контракт Streaming](configuration.md) |

## Сеть и секреты

Передавайте токены через механизм секретов платформы или файл окружения с ограниченными правами. Не сохраняйте их в репозитории, истории команд и выводе CI. Для API используйте HTTPS в рабочей среде; приёмник также допускает HTTP для локальных и частных сетей. Для Streaming настройка TLS и входящей аутентификации описана в инструкции подключения.

Служебные адреса проверки работоспособности и телеметрии не требуют аутентификации: ограничьте их сетевую доступность. Права файлов, настройки контейнеров и ротация учётных данных приведены в разделах соответствующих платформ выше. После обновления источника токенов перезапустите Collector. Срок действия и отзыв API-токенов и устаревших сеансов определяются в Zabbix; особенности клиента описаны в [аутентификации API](configuration.md).

## Проверка артефактов

Запустите все средства проверки, доступные на целевом хосте. Команда рендеринга Kubernetes безопасна, только пока `secret.example.yaml` содержит заполнители; её вывод намеренно отбрасывается:

```bash
make validate-config
make compose-config
kubectl kustomize deployments/kubernetes >/dev/null
systemd-analyze verify deployments/systemd/otelcol-zabbix.service
docker build -t zabbix-otel-collector:verify .
docker run --rm zabbix-otel-collector:verify components
```

В macOS обычно нет `systemd-analyze`; в качестве замены используйте актуальный контейнер Debian с systemd и зафиксируйте точную команду и результат. Для рендеринга Kubernetes требуется `kubectl`; для проверок Docker и Compose — работающие демон и плагин.

`make demo-up` запускает также Grafana. `make demo-verify` сначала проверяет свежие метрики API и Streaming в VictoriaMetrics, затем здоровье Grafana и наличие дашборда `zabbix-receiver`. Проверка выполняется из сети Compose и не зависит от выбранного внешнего порта Grafana.
