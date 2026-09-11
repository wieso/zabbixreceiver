.PHONY: fmt download tidy test test-race vet vulncheck build validate-config verify-ocb-local release-artifacts docker-build compose-config demo-up demo-verify demo-down

VERSION ?= dev
IMAGE ?= zabbix-otel-collector:local
GOVULNCHECK_VERSION ?= v1.8.0
OCB_VERSION ?= v0.160.0
OCB_LOCAL_OUTPUT ?= .ocb/local

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

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...
	cd receiver/zabbixreceiver && go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

build:
	mkdir -p bin
	go build -ldflags '-X main.version=$(VERSION)' -o bin/otelcol-zabbix ./cmd/otelcol-zabbix

validate-config: build
	ZABBIX_URL=http://zabbix.example/api_jsonrpc.php ZABBIX_TOKEN=dummy-token VICTORIAMETRICS_REMOTE_WRITE_URL=http://victoriametrics.example/api/v1/write bin/otelcol-zabbix validate --config configs/otelcol.yaml

verify-ocb-local:
	mkdir -p $(OCB_LOCAL_OUTPUT)
	go run go.opentelemetry.io/collector/cmd/builder@$(OCB_VERSION) --skip-strict-versioning=false --config testdata/ocb/builder-config.yaml
	$(OCB_LOCAL_OUTPUT)/otelcol-zabbix components | grep -q 'zabbix'
	ZABBIX_URL=http://zabbix.example/api_jsonrpc.php ZABBIX_TOKEN=dummy-token $(OCB_LOCAL_OUTPUT)/otelcol-zabbix validate --config examples/ocb/otelcol.yaml

release-artifacts:
	VERSION=$(VERSION) ./scripts/package-release.sh

docker-build:
	docker build -t $(IMAGE) .

compose-config:
	docker compose config --quiet

demo-up:
	docker compose up -d --build postgres zabbix-server zabbix-web bootstrap producer victoriametrics otelcol-zabbix grafana

demo-verify:
	docker compose --profile verify run --rm verify
	docker compose --profile verify run --rm --no-deps --entrypoint /bin/sh verify /demo/verify-grafana.sh

demo-down:
	docker compose down --volumes --remove-orphans

.PHONY: scaling-up scaling-verify scaling-failover scaling-down scaling-bench scaling-load
scaling-up:
	docker compose -f demo/scaling/compose.yaml up -d --build

scaling-verify:
	docker compose -f demo/scaling/compose.yaml --profile verify run --rm verify

scaling-failover:
	sh demo/scaling/failover.sh

scaling-down:
	docker compose -f demo/scaling/compose.yaml down --volumes --remove-orphans

scaling-bench:
	cd receiver/zabbixreceiver && go test -run '^$$' -bench BenchmarkScaling -benchmem -benchtime=1s -count=3 -cpu=1

scaling-load:
	docker compose -f demo/scaling/compose.yaml --profile load run --rm load
