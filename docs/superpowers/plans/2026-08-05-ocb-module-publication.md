# OCB Module Publication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish the existing Zabbix receiver as an independently versioned Collector Builder component at `github.com/wieso/zabbixreceiver/receiver/zabbixreceiver` while preserving the repository's ready-made distribution and giving the remote repository exactly one commit.

**Architecture:** `receiver/zabbixreceiver` becomes a nested Go module and absorbs the three private packages it uses. The root Go module retains the executable distribution and consumes the receiver through a local `replace`. Checked-in local and remote OCB manifests prove both the unpublished workspace and the published `v0.1.0` submodule tag.

**Tech Stack:** Go 1.25+, OpenTelemetry Collector and Collector Contrib v0.154.0, stable Collector modules v1.60.0, OpenTelemetry Go v1.44.0, Collector Builder v0.154.0, Git, GitHub, Docker, GitHub Actions.

## Global Constraints

- The public receiver module path is exactly `github.com/wieso/zabbixreceiver/receiver/zabbixreceiver`.
- The public receiver component type and factory remain `zabbix` and `zabbixreceiver.NewFactory`.
- The first module version is `v0.1.0`, published with tag `receiver/zabbixreceiver/v0.1.0`.
- The tested compatibility line is Collector/Contrib `v0.154.0` with stable Collector modules `v1.60.0`.
- Preserve every receiver YAML key, default, environment override, scheduling rule, metric mapping rule, authentication behavior, and runtime failure behavior.
- The nested receiver module must not depend on the root `github.com/wieso/zabbixreceiver` module.
- The repository license is Apache License 2.0.
- The remote `main` branch must have exactly one root commit and no parent commit.
- Push only remote `main` and tag `receiver/zabbixreceiver/v0.1.0`; do not push local backup history or unrelated refs.
- If the target remote is no longer empty immediately before publication, stop without overwriting it.

---

### Task 1: Create the Independent Receiver Module

**Files:**
- Create: `receiver/zabbixreceiver/go.mod`
- Create: `receiver/zabbixreceiver/go.sum`
- Move: `internal/zabbix/*.go` to `receiver/zabbixreceiver/internal/zabbix/*.go`
- Move: `internal/discovery/*.go` to `receiver/zabbixreceiver/internal/discovery/*.go`
- Move: `internal/metrics/*.go` to `receiver/zabbixreceiver/internal/metrics/*.go`
- Modify: `receiver/zabbixreceiver/factory.go`
- Modify: `receiver/zabbixreceiver/receiver.go`
- Modify: `receiver/zabbixreceiver/receiver_test.go`
- Modify: `receiver/zabbixreceiver/telemetry_test.go`
- Modify: `receiver/zabbixreceiver/internal/discovery/select.go`
- Modify: `receiver/zabbixreceiver/internal/discovery/select_test.go`
- Modify: `receiver/zabbixreceiver/internal/metrics/builder.go`
- Modify: `receiver/zabbixreceiver/internal/metrics/builder_test.go`
- Modify: `cmd/otelcol-zabbix/components.go`
- Modify: `go.mod`
- Modify: `go.sum`
- Test: `internal/packaging/assets_test.go`

**Interfaces:**
- Produces: Go module `github.com/wieso/zabbixreceiver/receiver/zabbixreceiver` at version-compatible source state for `v0.1.0`.
- Produces: `func NewFactory() receiver.Factory` at the nested module root.
- Consumes: Collector component, configuration, consumer, pdata, and receiver APIs at `v1.60.0`; `receivertest` at `v0.154.0`.
- Preserves: root distribution import of `zabbixreceiver.NewFactory` through a local module replacement.

- [ ] **Step 1: Write the failing module-boundary contract test**

Add this test to `internal/packaging/assets_test.go`:

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

- [ ] **Step 2: Run the contract test and confirm the red state**

Run:

```bash
go test ./internal/packaging -run TestReceiverModuleContract -count=1
```

Expected: FAIL because `receiver/zabbixreceiver/go.mod` does not exist.

- [ ] **Step 3: Create the nested module manifest**

Create `receiver/zabbixreceiver/go.mod` with this direct dependency block; `go mod tidy` will add the exact transitive block and `go.sum`:

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

- [ ] **Step 4: Move private packages under the nested module**

Use file-preserving moves so Git records renames:

```text
internal/zabbix      -> receiver/zabbixreceiver/internal/zabbix
internal/discovery   -> receiver/zabbixreceiver/internal/discovery
internal/metrics     -> receiver/zabbixreceiver/internal/metrics
```

Do not move `internal/packaging`; it tests repository-level assets and remains in the root module.

- [ ] **Step 5: Rewrite receiver-private imports**

In all moved packages and receiver source/tests, replace the three old imports with the nested module equivalents:

```go
"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/discovery"
otelmetrics "github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/metrics"
"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
```

The alias `otelmetrics` remains only where the local package name would otherwise be ambiguous.

- [ ] **Step 6: Point the root distribution at the public receiver module**

Change `cmd/otelcol-zabbix/components.go` to import:

```go
"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver"
```

Change the root `go.mod` module line to:

```go
module github.com/wieso/zabbixreceiver
```

Update its direct dependencies to the v0.154.0/v1.60.0 line and include:

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

- [ ] **Step 7: Resolve both module graphs**

Run:

```bash
cd receiver/zabbixreceiver
go mod tidy
cd ../..
go mod tidy
```

Expected: both commands succeed; the nested `go.mod` has no requirement on `github.com/wieso/zabbixreceiver`.

- [ ] **Step 8: Run focused and complete module tests**

Run:

```bash
cd receiver/zabbixreceiver
go test ./... -count=1
cd ../..
go test ./internal/packaging -run TestReceiverModuleContract -count=1
go test ./... -count=1
```

Expected: PASS. The root `./...` traversal stops at the nested module boundary, and the explicit nested invocation covers receiver and private-package tests.

- [ ] **Step 9: Confirm obsolete import paths are gone**

Run:

```bash
rg -n 'github\.com/aleksandr/zabbix-otel|github\.com/wieso/zabbixreceiver/internal/' --glob '*.go' --glob 'go.mod'
```

Expected: no output.

- [ ] **Step 10: Commit the module split**

```bash
git add -A -- go.mod go.sum cmd/otelcol-zabbix/components.go receiver/zabbixreceiver internal/packaging/assets_test.go internal/zabbix internal/discovery internal/metrics
git commit -m "refactor: publish receiver as nested Go module"
```

Expected: the deleted root-private paths and added nested-private paths are recorded, preferably as renames. This local commit will later be folded into the publication root commit.

---

### Task 2: Make All Build Automation Multi-Module Aware

**Files:**
- Modify: `internal/packaging/assets_test.go`
- Modify: `Makefile`
- Modify: `Dockerfile`
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- Produces: `make download`, `make tidy`, `make test`, `make test-race`, and `make vet` that cover both modules.
- Produces: a Docker build that exposes the nested manifest before root dependency download.
- Produces: CI cache and verification steps keyed by both `go.sum` files.

- [ ] **Step 1: Write failing dual-module automation assertions**

Add this test to `internal/packaging/assets_test.go`:

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

Extend `TestDockerfileContract` with:

```go
if !strings.Contains(dockerfile, "COPY receiver/zabbixreceiver/go.mod receiver/zabbixreceiver/go.sum ./receiver/zabbixreceiver/") {
	t.Error("Dockerfile must copy the nested receiver manifests before go mod download")
}
```

- [ ] **Step 2: Run the automation contract tests and confirm failure**

Run:

```bash
go test ./internal/packaging -run 'Test(DualModuleAutomation|Dockerfile)Contract' -count=1
```

Expected: FAIL with missing Makefile, CI, and Dockerfile clauses.

- [ ] **Step 3: Extend the Makefile verification surface**

Use these targets and preserve the existing build/demo targets:

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

- [ ] **Step 4: Make Docker dependency download aware of the nested manifest**

The start of the Docker build stage must be:

```dockerfile
WORKDIR /src

COPY receiver/zabbixreceiver/go.mod receiver/zabbixreceiver/go.sum ./receiver/zabbixreceiver/
COPY go.mod go.sum ./
RUN go mod download
```

This gives the root module's local `replace` a valid module directory before `go mod download` runs.

- [ ] **Step 5: Update CI caching and dependency checks**

Under `actions/setup-go@v6`, set:

```yaml
cache-dependency-path: |
  go.sum
  receiver/zabbixreceiver/go.sum
```

Replace the dependency command with `make download`. Before formatting, add:

```yaml
- name: Enforce tidy module files
  run: |
    make tidy
    git diff --check
    git diff --exit-code
```

The existing `make test`, `make test-race`, and `make vet` steps now cover both modules.

- [ ] **Step 6: Run the automation checks**

Run:

```bash
make tidy
make fmt
git diff --check
make test
make vet
go test ./internal/packaging -run 'Test(DualModuleAutomation|Dockerfile)Contract' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit multi-module automation**

```bash
git add Makefile Dockerfile .github/workflows/ci.yml internal/packaging/assets_test.go go.mod go.sum receiver/zabbixreceiver/go.mod receiver/zabbixreceiver/go.sum
git commit -m "build: verify root and receiver modules"
```

---

### Task 3: Add Local and Published Collector Builder Fixtures

**Files:**
- Create: `examples/ocb/builder-config.yaml`
- Create: `examples/ocb/otelcol.yaml`
- Create: `testdata/ocb/builder-config.yaml`
- Modify: `internal/packaging/assets_test.go`
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`
- Modify: `.gitignore`

**Interfaces:**
- Produces: copy-ready remote manifest with no standalone component `path` key, `replace`, or unpublished import override.
- Produces: local-only manifest with `path: ./receiver/zabbixreceiver`.
- Produces: `make verify-ocb-local`, which builds with Builder v0.154.0, checks component inventory, and validates runtime YAML.

- [ ] **Step 1: Write the failing OCB asset contract test**

Add this test to `internal/packaging/assets_test.go`:

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

- [ ] **Step 2: Run the OCB asset test and confirm failure**

Run:

```bash
go test ./internal/packaging -run TestOCBExamplesContract -count=1
```

Expected: FAIL because the OCB example files do not exist.

- [ ] **Step 3: Create the published Builder manifest**

Create `examples/ocb/builder-config.yaml`:

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

- [ ] **Step 4: Create the local Builder fixture**

Create `testdata/ocb/builder-config.yaml` with the same component versions but a separate output and the one local override:

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

- [ ] **Step 5: Create the minimal runtime configuration**

Create `examples/ocb/otelcol.yaml`:

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

- [ ] **Step 6: Add the local OCB verification target**

Add variables, the phony target, and recipe to `Makefile`:

```make
OCB_VERSION ?= v0.154.0
OCB_LOCAL_OUTPUT ?= .ocb/local

verify-ocb-local:
	mkdir -p $(OCB_LOCAL_OUTPUT)
	go run go.opentelemetry.io/collector/cmd/builder@$(OCB_VERSION) --skip-strict-versioning=false --config testdata/ocb/builder-config.yaml
	$(OCB_LOCAL_OUTPUT)/otelcol-zabbix components | grep -q 'zabbix'
	ZABBIX_URL=http://zabbix.example/api_jsonrpc.php ZABBIX_TOKEN=dummy-token $(OCB_LOCAL_OUTPUT)/otelcol-zabbix validate --config examples/ocb/otelcol.yaml
```

Add `verify-ocb-local` to `.PHONY`.

- [ ] **Step 7: Ignore generated and editor-local paths**

Append to `.gitignore`:

```gitignore
.idea/
.ocb/
otelcol-zabbix-dist/
```

- [ ] **Step 8: Add OCB verification to CI**

After the ordinary Collector configuration validation step, add:

```yaml
- name: Verify Collector Builder integration
  run: make verify-ocb-local
```

- [ ] **Step 9: Run local Builder verification**

Run:

```bash
go test ./internal/packaging -run TestOCBExamplesContract -count=1
make verify-ocb-local
```

Expected: the target creates its ignored output parent; Builder v0.154.0 completes strict version checks and compiles `.ocb/local/otelcol-zabbix`; `components` contains `zabbix`; `validate` exits successfully without contacting the placeholder endpoint.

- [ ] **Step 10: Commit Builder integration assets**

```bash
git add .gitignore Makefile .github/workflows/ci.yml examples/ocb testdata/ocb internal/packaging/assets_test.go
git commit -m "test: verify Collector Builder integration"
```

---

### Task 4: Add Public Documentation and Apache-2.0 Licensing

**Files:**
- Create: `LICENSE`
- Create: `receiver/zabbixreceiver/README.md`
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md`
- Modify: `internal/packaging/assets_test.go`

**Interfaces:**
- Produces: copy-ready OCB instructions and a component-focused configuration guide.
- Produces: canonical Apache License 2.0 grant at repository root.
- Preserves: detailed configuration, deployment, security, and demo documentation already linked by the root README.

- [ ] **Step 1: Write failing public-documentation assertions**

In `TestDocumentationContract`, add `Custom Collector Builder` to the required README headings and assert:

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

Add:

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

- [ ] **Step 2: Run the documentation tests and confirm failure**

Run:

```bash
go test ./internal/packaging -run 'Test(Documentation|License)Contract' -count=1
```

Expected: FAIL because the new heading, module instructions, and `LICENSE` are absent.

- [ ] **Step 3: Add the canonical license**

Create `LICENSE` from the unmodified Apache License 2.0 text distributed with OpenTelemetry Collector Builder v0.154.0. Verify exact content with:

```bash
shasum -a 256 LICENSE
```

Expected:

```text
cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30  LICENSE
```

- [ ] **Step 4: Add the root Builder instructions**

Add a `## Custom Collector Builder` section near Quick start. It must explain that `builder-config.yaml` controls compilation while `otelcol.yaml` controls runtime, show this exact component entry, and link both examples:

```yaml
receivers:
  - gomod: github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.1.0
```

Document these exact facts:

```text
Tested compatibility: Collector/Contrib v0.154.0 and stable Collector modules v1.60.0.
Release tag: receiver/zabbixreceiver/v0.1.0.
```

Extend the Testing command matrix with `make tidy` and `make verify-ocb-local`; state that the standard Make targets cover both root and receiver modules.

- [ ] **Step 5: Create the component README**

Create `receiver/zabbixreceiver/README.md` with these sections and concrete content:

````markdown
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
````

Use correctly nested Markdown fences when implementing this content.

- [ ] **Step 6: Update the approved architecture paths**

In `docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md`, replace the three path references with:

```text
receiver/zabbixreceiver/internal/zabbix
receiver/zabbixreceiver/internal/discovery
receiver/zabbixreceiver/internal/metrics
```

Do not change behavior requirements in the approved receiver design.

- [ ] **Step 7: Run documentation and repository scans**

Run:

```bash
go test ./internal/packaging -run 'Test(Documentation|License|OCBExamples|ReceiverModule)Contract' -count=1
rg -n 'github\.com/aleksandr/zabbix-otel' --glob '!docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md'
git diff --check
```

Expected: tests PASS; the scan has no output outside the explicitly historical implementation plan; diff check succeeds.

- [ ] **Step 8: Commit public documentation**

```bash
git add LICENSE README.md receiver/zabbixreceiver/README.md docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md internal/packaging/assets_test.go
git commit -m "docs: explain versioned receiver integration"
```

---

### Task 5: Run the Complete Pre-Publication Verification Matrix

**Files:**
- Verify only; modify files only if a check exposes a defect, then rerun the affected task's red-green cycle.

**Interfaces:**
- Consumes: both Go modules, local OCB fixture, ready-made distribution, Compose configuration, and container build.
- Produces: evidence that the final tracked tree is ready to become the publication root commit.

- [ ] **Step 1: Invoke the completion-verification discipline**

Use `superpowers:verification-before-completion` before making any success claim or creating the publication commit.

- [ ] **Step 2: Verify formatting and module tidiness**

Run:

```bash
make fmt
make tidy
git diff --check
git diff --exit-code
```

Expected: all commands exit 0 and the tree remains unchanged.

- [ ] **Step 3: Verify both Go modules**

Run:

```bash
make test
make test-race
make vet
```

Expected: root and nested module tests, race tests, and vet all PASS.

- [ ] **Step 4: Verify both Collector assembly paths**

Run:

```bash
make build
make validate-config
make verify-ocb-local
```

Expected: the hand-maintained distribution and OCB-generated distribution both build; both validate their runtime configuration; OCB component inventory includes `zabbix`.

- [ ] **Step 5: Verify packaging surfaces**

Run:

```bash
make compose-config
docker build -t zabbix-otel-collector:verify .
docker run --rm zabbix-otel-collector:verify components
```

Expected: Compose configuration is valid, the image builds, and container component inventory includes `zabbix`.

- [ ] **Step 6: Verify final repository state**

Run:

```bash
git status --short
git log -6 --oneline --decorate
git tag --list
```

Expected: status is clean; local implementation commits are present for review; no release tag exists yet.

---

### Task 6: Publish One Root Commit and Verify the Remote Module

**Files:**
- Git refs only; the verified tracked tree must not change.

**Interfaces:**
- Consumes: clean verified `HEAD` and empty `https://github.com/wieso/zabbixreceiver.git`.
- Produces: remote `main` with one commit and tag `receiver/zabbixreceiver/v0.1.0` pointing to it.
- Preserves: prior local history under unpushed branch `feat/ocb-module-publication`.

- [ ] **Step 1: Recheck the remote immediately before mutation**

Run:

```bash
git ls-remote https://github.com/wieso/zabbixreceiver.git
```

Expected: no output. If any ref appears, stop and ask the user; do not force-push or overwrite it.

- [ ] **Step 2: Create a root commit from the verified tree without changing files**

Require clean `git status --short`, then run in one shell:

```bash
publication_tree="$(git rev-parse HEAD^{tree})"
publication_commit="$(printf '%s\n' 'feat: publish Zabbix OpenTelemetry receiver' | git commit-tree "$publication_tree")"
git branch publication-main "$publication_commit"
```

Expected: `publication-main` has the same tree as `feat/ocb-module-publication` but is a new root commit. The implementation branch remains checked out and local.

- [ ] **Step 3: Prove the local publication commit has no parent**

Run:

```bash
git rev-list --count publication-main
git rev-list --parents -n 1 publication-main
git diff --exit-code publication-main feat/ocb-module-publication
```

Expected: count is `1`; the second command prints only the new commit hash with no parent hash; the tree diff is empty.

- [ ] **Step 4: Create the nested-module release tag**

Run:

```bash
git tag -a receiver/zabbixreceiver/v0.1.0 -m "Zabbix receiver v0.1.0" publication-main
git rev-parse receiver/zabbixreceiver/v0.1.0^{commit}
git rev-parse publication-main
```

Expected: both hashes are identical.

- [ ] **Step 5: Configure and push only the approved refs**

Run:

```bash
git remote add origin https://github.com/wieso/zabbixreceiver.git
git push --atomic origin \
  publication-main:refs/heads/main \
  refs/tags/receiver/zabbixreceiver/v0.1.0
```

If `origin` already exists, verify its URL is exactly the target and use `git remote set-url origin https://github.com/wieso/zabbixreceiver.git` only if needed. Do not use `--mirror`, `--all`, `--tags`, or force push.

Expected: the atomic push publishes both refs using the user's configured Git credential helper, or publishes neither ref if either update fails.

- [ ] **Step 6: Verify remote ref inventory**

Run:

```bash
git ls-remote --heads --tags origin
```

Expected: only `refs/heads/main`, the annotated tag ref, and its peeled `^{}` tag line are present.

- [ ] **Step 7: Verify history and tag from a clean clone**

Create a temporary directory, clone `main`, and run:

```bash
publication_tmp="$(mktemp -d)"
git clone --branch main https://github.com/wieso/zabbixreceiver.git "$publication_tmp/repo"
cd "$publication_tmp/repo"
git rev-list --count HEAD
git rev-list --parents -n 1 HEAD
git rev-parse receiver/zabbixreceiver/v0.1.0^{commit}
git rev-parse HEAD
```

Expected: count is `1`; `HEAD` has no parent; tag and `HEAD` hashes match.

- [ ] **Step 8: Build from the remote tag with isolated caches**

From the clean clone, set task-specific temporary `GOMODCACHE` and `GOCACHE` directories and force only this repository's module lookup to bypass proxy/sumdb lag:

```bash
GONOPROXY=github.com/wieso/zabbixreceiver \
GONOSUMDB=github.com/wieso/zabbixreceiver \
GOMODCACHE="$publication_tmp/modcache" \
GOCACHE="$publication_tmp/buildcache" \
go run go.opentelemetry.io/collector/cmd/builder@v0.154.0 --skip-strict-versioning=false --config examples/ocb/builder-config.yaml
```

Do not add a `replace`, `path`, or Go workspace. Expected: Builder fetches `github.com/wieso/zabbixreceiver/receiver/zabbixreceiver@v0.1.0` from GitHub and compiles `otelcol-zabbix-dist/otelcol-zabbix`.

- [ ] **Step 9: Verify the remotely resolved binary**

Run from the clean clone:

```bash
otelcol-zabbix-dist/otelcol-zabbix components | grep -q 'zabbix'
ZABBIX_URL=http://zabbix.example/api_jsonrpc.php \
ZABBIX_TOKEN=dummy-token \
otelcol-zabbix-dist/otelcol-zabbix validate --config examples/ocb/otelcol.yaml
```

Expected: both commands exit 0; validation does not contact the placeholder Zabbix endpoint.

- [ ] **Step 10: Report the publication evidence**

Report the GitHub repository URL, remote root commit hash, module tag, exact OCB `gomod` line, one-commit count, and the passing local/remote Builder verification. Also state that `feat/ocb-module-publication` exists only locally and was not pushed.
