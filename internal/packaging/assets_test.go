package packaging

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func readAsset(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

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

func TestDocumentationContract(t *testing.T) {
	readme := readAsset(t, "README.md")
	for _, heading := range []string{
		"Architecture",
		"Quick start",
		"Custom Collector Builder",
		"Configuration",
		"Metric mapping",
		"Docker Compose demo",
		"VM/systemd",
		"Kubernetes",
		"Testing",
		"Security",
	} {
		if !strings.Contains(readme, "## "+heading) {
			t.Errorf("README.md missing %q section", heading)
		}
	}
	for _, clause := range []string{
		"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver v0.1.0",
		"receiver/zabbixreceiver/v0.1.0",
		"Collector/Contrib v0.154.0",
	} {
		if !strings.Contains(readme, clause) {
			t.Errorf("README.md missing publication clause %q", clause)
		}
	}
	for name, link := range map[string]string{
		"approved design": "docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md",
	} {
		if !strings.Contains(readme, link) {
			t.Errorf("README.md missing %s link %q", name, link)
		}
	}
	for name, clause := range map[string]string{
		"two-stage validation":     "explicit receiver overrides participate in both validation stages",
		"downstream delivery gate": "only after a values cycle successfully retrieves every chunk from a non-empty snapshot and emits at least one valid point",
		"fresh demo sample":        "sample timestamp is at or after the verifier start",
	} {
		if !strings.Contains(readme, clause) {
			t.Errorf("README.md missing %s clause %q", name, clause)
		}
	}
	for name, clause := range map[string]string{
		"Zabbix demo UI URL":            "http://127.0.0.1:8080/",
		"VictoriaMetrics VMUI URL":      "http://127.0.0.1:8428/vmui/",
		"Zabbix demo UI credentials":    "`Admin` / `zabbix`",
		"VictoriaMetrics demo query":    `zabbix_demo_counter{host="otel-demo-host",env="compose"}`,
		"Zabbix UI port override":       "ZABBIX_WEB_PORT",
		"VictoriaMetrics port override": "VICTORIAMETRICS_PORT",
	} {
		if !strings.Contains(readme, clause) {
			t.Errorf("README.md missing %s clause %q", name, clause)
		}
	}

	configuration := readAsset(t, "docs/configuration.md")
	for _, environmentVariable := range []string{
		"ZABBIX_URL",
		"ZABBIX_TOKEN",
		"ZABBIX_TIMEOUT",
		"MAX_METRICS_PER_HOST",
		"ZABBIX_ITEMS_PER_REQUEST",
	} {
		if !strings.Contains(configuration, environmentVariable) {
			t.Errorf("docs/configuration.md missing environment variable %q", environmentVariable)
		}
	}
	for name, clause := range map[string]string{
		"Collector-resolved validation":    "Collector recursive validation operates on an environment-resolved clone",
		"environment-only required fields": "can satisfy omitted `zabbix.url` and `zabbix.token`",
		"invalid YAML replacement":         "`items_per_request: 0` is valid when `ZABBIX_ITEMS_PER_REQUEST=250`",
		"resolved timeout":                 "`zabbix.timeout` must remain positive after overrides",
	} {
		if !strings.Contains(configuration, clause) {
			t.Errorf("docs/configuration.md missing %s clause %q", name, clause)
		}
	}

	design := readAsset(t, "docs/superpowers/specs/2026-08-05-zabbix-opentelemetry-receiver-design.md")
	plan := readAsset(t, "docs/superpowers/plans/2026-08-05-zabbix-opentelemetry-receiver.md")
	telemetryNames := []string{
		"otelcol_receiver_zabbix_discover_attempts",
		"otelcol_receiver_zabbix_discover_errors",
		"otelcol_receiver_zabbix_discover_duration",
		"otelcol_receiver_zabbix_values_attempts",
		"otelcol_receiver_zabbix_values_errors",
		"otelcol_receiver_zabbix_values_duration",
		"otelcol_receiver_zabbix_emitted_points",
		"otelcol_receiver_zabbix_invalid_values",
		"otelcol_receiver_zabbix_filtered_items",
		"otelcol_receiver_zabbix_limited_items",
	}
	if !strings.Contains(design, "authoritative acceptance surface is exactly these ten instruments") {
		t.Error("approved design does not record the final telemetry adjudication")
	}
	for _, name := range telemetryNames {
		if strings.Count(design, name) != 1 {
			t.Errorf("approved design must define telemetry instrument %q exactly once", name)
		}
		if !strings.Contains(plan, name) {
			t.Errorf("implementation plan missing authoritative telemetry instrument %q", name)
		}
	}
	for documentName, contents := range map[string]string{"design": design, "plan": plan} {
		if !strings.Contains(contents, "sample timestamp is at or after the verifier start") {
			t.Errorf("%s missing fresh-sample verification contract", documentName)
		}
	}

	deployment := readAsset(t, "docs/deployment.md")
	safeRender := "kubectl kustomize deployments/kubernetes >/dev/null"
	secretEdit := "Only after that validation, replace both placeholders"
	apply := "kubectl apply -k deployments/kubernetes"
	renderIndex := strings.Index(deployment, safeRender)
	editIndex := strings.Index(deployment, secretEdit)
	applyIndex := strings.Index(deployment, apply)
	if renderIndex < 0 || editIndex < 0 || applyIndex < 0 || !(renderIndex < editIndex && editIndex < applyIndex) {
		t.Errorf("docs/deployment.md must validate placeholders without output before editing and applying the Secret")
	}
	for name, clause := range map[string]string{
		"real-secret render warning":   "Never run `kubectl kustomize` after inserting real credentials",
		"namespace-preserving cleanup": "kubectl -n observability delete deployment/otelcol-zabbix service/otelcol-zabbix configmap/otelcol-zabbix secret/otelcol-zabbix",
		"namespace deletion warning":   "would delete the entire `observability` namespace, including unrelated workloads",
		"port-forward terminal":        "In a second terminal, while port-forward is still running",
	} {
		if !strings.Contains(deployment, clause) {
			t.Errorf("docs/deployment.md missing %s clause %q", name, clause)
		}
	}
}

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

func TestReleasePackagingContract(t *testing.T) {
	makefile := readAsset(t, "Makefile")
	for _, want := range []string{
		"release-artifacts",
		"VERSION=$(VERSION) ./scripts/package-release.sh",
	} {
		if !strings.Contains(makefile, want) {
			t.Errorf("Makefile missing release packaging contract %q", want)
		}
	}

	script := readAsset(t, "scripts/package-release.sh")
	for _, want := range []string{
		"^v[0-9]+\\.[0-9]+\\.[0-9]+$",
		"linux/amd64",
		"linux/arm64",
		"CGO_ENABLED=0",
		"main.version",
		"checksums.txt",
		"otelcol-zabbix_",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("release packaging script missing %q", want)
		}
	}

	workflow := readAsset(t, ".github/workflows/release.yml")
	for _, want := range []string{
		"v*.*.*",
		"contents: write",
		"make release-artifacts",
		"softprops/action-gh-release@v2",
		"checksums.txt",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release workflow missing %q", want)
		}
	}
}

func TestDockerfileContract(t *testing.T) {
	dockerfile := readAsset(t, "Dockerfile")
	for _, want := range []string{
		"FROM golang:1.25-alpine AS build",
		"CGO_ENABLED=0",
		"-trimpath",
		"-ldflags=\"-s -w\"",
		"FROM gcr.io/distroless/static-debian13:nonroot",
		"USER 10001:10001",
		"ENTRYPOINT [\"/otelcol-zabbix\"]",
		"CMD [\"--config=/etc/otelcol-zabbix/config.yaml\"]",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("Dockerfile missing %q", want)
		}
	}
	if strings.Contains(dockerfile, "ZABBIX_TOKEN=") {
		t.Error("Dockerfile must not embed a Zabbix token")
	}
	if !strings.Contains(dockerfile, "COPY receiver/zabbixreceiver/go.mod receiver/zabbixreceiver/go.sum ./receiver/zabbixreceiver/") {
		t.Error("Dockerfile must copy the nested receiver manifests before go mod download")
	}
}

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

func TestSystemdUnitContract(t *testing.T) {
	unit := readAsset(t, "deployments/systemd/otelcol-zabbix.service")
	for _, want := range []string{
		"User=otelcol-zabbix",
		"Group=otelcol-zabbix",
		"EnvironmentFile=/etc/otelcol-zabbix/otelcol-zabbix.env",
		"ExecStart=/usr/local/bin/otelcol-zabbix --config=/etc/otelcol-zabbix/config.yaml",
		"Restart=on-failure",
		"NoNewPrivileges=true",
		"ProtectSystem=strict",
		"StateDirectory=otelcol-zabbix",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("systemd unit missing %q", want)
		}
	}
}

func TestKubernetesKustomizationSemanticContract(t *testing.T) {
	kustomization := readYAML(t, "deployments/kubernetes/kustomization.yaml")
	assertEqual(t, "observability", value(t, kustomization, "namespace"))

	resources := stringSlice(t, value(t, kustomization, "resources"))
	expectedResources := []string{
		"namespace.yaml",
		"secret.example.yaml",
		"configmap.yaml",
		"deployment.yaml",
		"service.yaml",
	}
	assertEqual(t, expectedResources, resources)

	objects := make(map[string]map[string]any, len(resources))
	for _, resource := range resources {
		objects[resource] = readYAML(t, filepath.Join("deployments/kubernetes", resource))
		kind := stringValue(t, value(t, objects[resource], "kind"))
		if map[string]bool{"Role": true, "ClusterRole": true, "RoleBinding": true, "ClusterRoleBinding": true, "ServiceAccount": true}[kind] {
			t.Fatalf("Kustomization must not include RBAC or ServiceAccount resource %q", resource)
		}
	}

	namespace := objects["namespace.yaml"]
	assertEqual(t, "Namespace", value(t, namespace, "kind"))
	assertEqual(t, "observability", value(t, mapValue(t, value(t, namespace, "metadata")), "name"))

	secret := objects["secret.example.yaml"]
	configMap := objects["configmap.yaml"]
	deployment := objects["deployment.yaml"]
	service := objects["service.yaml"]
	assertObjectIdentity(t, secret, "Secret", "otelcol-zabbix")
	assertObjectIdentity(t, configMap, "ConfigMap", "otelcol-zabbix")
	assertObjectIdentity(t, deployment, "Deployment", "otelcol-zabbix")
	assertObjectIdentity(t, service, "Service", "otelcol-zabbix")

	secretData := mapValue(t, value(t, secret, "stringData"))
	for _, key := range []string{"ZABBIX_URL", "ZABBIX_TOKEN"} {
		if _, ok := secretData[key]; !ok {
			t.Errorf("Secret stringData missing %q", key)
		}
	}
	configData := mapValue(t, value(t, configMap, "data"))
	for _, key := range []string{"VICTORIAMETRICS_REMOTE_WRITE_URL", "config.yaml"} {
		if _, ok := configData[key]; !ok {
			t.Errorf("ConfigMap data missing %q", key)
		}
	}
	collectorConfig := parseYAML(t, stringValue(t, configData["config.yaml"]))
	assertTelemetryPrometheusBinding(t, collectorConfig)

	deploymentSpec := mapValue(t, value(t, deployment, "spec"))
	assertEqual(t, 1, value(t, deploymentSpec, "replicas"))
	strategy := mapValue(t, value(t, deploymentSpec, "strategy"))
	assertEqual(t, "RollingUpdate", value(t, strategy, "type"))
	rollingUpdate := mapValue(t, value(t, strategy, "rollingUpdate"))
	assertEqual(t, 0, value(t, rollingUpdate, "maxUnavailable"))
	assertEqual(t, 1, value(t, rollingUpdate, "maxSurge"))

	template := mapValue(t, value(t, deploymentSpec, "template"))
	podLabels := mapValue(t, value(t, mapValue(t, value(t, template, "metadata")), "labels"))
	assertEqual(t, mapValue(t, value(t, deploymentSpec, "selector", "matchLabels")), podLabels)
	podSpec := mapValue(t, value(t, template, "spec"))
	assertEqual(t, false, value(t, podSpec, "automountServiceAccountToken"))
	assertEqual(t, 30, value(t, podSpec, "terminationGracePeriodSeconds"))
	podSecurity := mapValue(t, value(t, podSpec, "securityContext"))
	assertEqual(t, true, value(t, podSecurity, "runAsNonRoot"))
	assertEqual(t, 10001, value(t, podSecurity, "runAsUser"))
	assertEqual(t, 10001, value(t, podSecurity, "runAsGroup"))

	containers := mapSlice(t, value(t, podSpec, "containers"))
	if len(containers) != 1 {
		t.Fatalf("expected exactly one Collector container, got %d", len(containers))
	}
	container := containers[0]
	assertEqual(t, "otelcol-zabbix", value(t, container, "name"))
	assertEqual(t, "zabbix-otel-collector:local", value(t, container, "image"))
	assertContainerPorts(t, container)
	assertContainerEnvironment(t, container, secretData, configData)
	assertContainerSecurity(t, container)
	assertProbe(t, mapValue(t, value(t, container, "readinessProbe")))
	assertProbe(t, mapValue(t, value(t, container, "livenessProbe")))
	assertResources(t, mapValue(t, value(t, container, "resources")))
	assertConfigVolumeWiring(t, podSpec, container, configData)

	serviceSpec := mapValue(t, value(t, service, "spec"))
	assertEqual(t, "ClusterIP", value(t, serviceSpec, "type"))
	assertEqual(t, podLabels, mapValue(t, value(t, serviceSpec, "selector")))
	servicePorts := mapSlice(t, value(t, serviceSpec, "ports"))
	if len(servicePorts) != 2 {
		t.Fatalf("expected health and telemetry Service ports only, got %d", len(servicePorts))
	}
	assertPort(t, servicePorts, "health", 13133, "health")
	assertPort(t, servicePorts, "telemetry", 8888, "telemetry")
}

func TestContainerSampleTelemetryBinding(t *testing.T) {
	assertTelemetryPrometheusBinding(t, readYAML(t, "configs/otelcol.yaml"))
}

func TestComposeTopologyContract(t *testing.T) {
	compose := readYAML(t, "compose.yaml")
	services := mapValue(t, value(t, compose, "services"))

	images := map[string]string{
		"postgres":        "postgres:17-alpine",
		"zabbix-server":   "zabbix/zabbix-server-pgsql:alpine-7.4.12",
		"zabbix-web":      "zabbix/zabbix-web-nginx-pgsql:alpine-7.4.12",
		"producer":        "zabbix/zabbix-agent2:alpine-7.4.12",
		"victoriametrics": "victoriametrics/victoria-metrics:v1.148.0",
	}
	for name, image := range images {
		service := mapValue(t, value(t, services, name))
		assertEqual(t, image, value(t, service, "image"))
	}
	for _, name := range []string{"bootstrap", "otelcol-zabbix", "verify"} {
		mapValue(t, value(t, services, name))
	}
	assertComposeLoopbackPort(t, services, "zabbix-web", "ZABBIX_WEB_PORT", 8080, 8080)
	assertComposeLoopbackPort(t, services, "victoriametrics", "VICTORIAMETRICS_PORT", 8428, 8428)

	for _, name := range []string{"postgres", "zabbix-server", "zabbix-web"} {
		value(t, mapValue(t, value(t, services, name)), "healthcheck")
	}
	assertComposeDependency(t, services, "zabbix-server", "postgres", "service_healthy")
	assertComposeDependency(t, services, "zabbix-web", "postgres", "service_healthy")
	assertComposeDependency(t, services, "zabbix-web", "zabbix-server", "service_healthy")
	assertComposeDependency(t, services, "bootstrap", "zabbix-web", "service_healthy")
	assertComposeDependency(t, services, "producer", "bootstrap", "service_completed_successfully")
	assertComposeDependency(t, services, "producer", "zabbix-server", "service_healthy")
	assertComposeDependency(t, services, "otelcol-zabbix", "bootstrap", "service_completed_successfully")

	verify := mapValue(t, value(t, services, "verify"))
	assertEqual(t, []string{"verify"}, stringSlice(t, value(t, verify, "profiles")))
	assertEqual(t, []string{"/bin/sh", "/demo/verify-entrypoint.sh"}, stringSlice(t, value(t, verify, "entrypoint")))
	verifyDependencies := mapValue(t, value(t, verify, "depends_on"))
	if len(verifyDependencies) != 1 {
		t.Fatalf("verifier must not traverse and rerun bootstrap dependencies, got %d dependencies", len(verifyDependencies))
	}
	assertComposeDependency(t, services, "verify", "victoriametrics", "service_started")

	networks := mapValue(t, value(t, compose, "networks"))
	demoNetwork := mapValue(t, value(t, networks, "demo"))
	assertEqual(t, false, value(t, demoNetwork, "internal"))
	for name, rawService := range services {
		service := mapValue(t, rawService)
		assertEqual(t, []string{"demo"}, stringSlice(t, value(t, service, "networks")))
		if name == "bootstrap" {
			assertEqual(t, "0:0", value(t, service, "user"))
		}
	}

	volumes := mapValue(t, value(t, compose, "volumes"))
	mapValue(t, value(t, volumes, "demo-config"))
	var demoConfigUsers []string
	for name, rawService := range services {
		service := mapValue(t, rawService)
		for _, volume := range anySlice(t, service["volumes"]) {
			if composeVolumeSource(t, volume) == "demo-config" {
				demoConfigUsers = append(demoConfigUsers, name)
			}
		}
	}
	slices.Sort(demoConfigUsers)
	assertEqual(t, []string{"bootstrap", "otelcol-zabbix"}, demoConfigUsers)
}

func TestProducerRetriesAfterTransientSenderFailure(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(tempDir, "nc"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(tempDir, "zabbix_sender"), `#!/bin/sh
count=0
if [ -f "$TEST_STATE/count" ]; then
	read -r count < "$TEST_STATE/count"
fi
count=$((count + 1))
printf '%s\n' "$count" > "$TEST_STATE/count"
if [ "$count" -eq 1 ]; then
	exit 1
fi
: > "$TEST_STATE/continued"
`)
	writeExecutable(t, filepath.Join(tempDir, "sleep"), `#!/bin/sh
if [ -f "$TEST_STATE/continued" ]; then
	kill -TERM "$PPID"
fi
`)

	command := exec.Command("/bin/sh", filepath.Join("..", "..", "demo", "producer.sh"))
	command.Env = append(os.Environ(), "PATH="+tempDir, "TEST_STATE="+stateDir)
	_ = command.Run()

	countContents, err := os.ReadFile(filepath.Join(stateDir, "count"))
	if err != nil {
		t.Fatalf("producer stopped after the first transient sender failure: %v", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(countContents)))
	if err != nil {
		t.Fatalf("parse sender attempt count: %v", err)
	}
	if count < 2 {
		t.Fatalf("sender attempts = %d, want at least 2", count)
	}
}

func TestBootstrapScopesTokenRotationToAuthenticatedUser(t *testing.T) {
	bootstrap := readAsset(t, "demo/bootstrap.sh")
	for name, want := range map[string]string{
		"authenticated user lookup": "user_get_params=$(jq -nc --arg username \"$ZABBIX_USERNAME\" \\\n\t'{output:[\"userid\"],filter:{username:[$username]}}')\nusers=$(rpc_session user.get \"$user_get_params\")",
		"current-user token lookup": "token_get_params=$(jq -nc --arg user_id \"$user_id\" \\\n\t'{output:[\"tokenid\"],userids:[$user_id],filter:{name:[\"otel-demo-receiver\"]}}')\ntokens=$(rpc_session token.get \"$token_get_params\")",
	} {
		if !strings.Contains(bootstrap, want) {
			t.Errorf("bootstrap current-user token rotation missing %s", name)
		}
	}
	if strings.Contains(bootstrap, `filter":{"username":["Admin"]}`) {
		t.Error("bootstrap must derive the token owner from the authenticated username, not a hard-coded Admin lookup")
	}
}

func TestVerifierCapsCurlAndDoesNotSleepPastDeadline(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(tempDir, "date"), `#!/bin/sh
count=0
if [ -f "$TEST_STATE/date-count" ]; then
	read -r count < "$TEST_STATE/date-count"
fi
count=$((count + 1))
printf '%s\n' "$count" > "$TEST_STATE/date-count"
if [ -f "$TEST_STATE/sleep-count" ]; then
	printf '%s\n' 280
	exit 0
fi
case "$count" in
	1) printf '%s\n' 100 ;;
	2) printf '%s\n' 278 ;;
	*) printf '%s\n' 279 ;;
esac
`)
	writeExecutable(t, filepath.Join(tempDir, "curl"), `#!/bin/sh
count=0
if [ -f "$TEST_STATE/curl-count" ]; then
	read -r count < "$TEST_STATE/curl-count"
fi
count=$((count + 1))
printf '%s\n' "$count" > "$TEST_STATE/curl-count"
connect_timeout=
max_time=
while [ "$#" -gt 0 ]; do
	case "$1" in
		--connect-timeout)
			connect_timeout=$2
			shift 2
			;;
		--max-time)
			max_time=$2
			shift 2
			;;
		*) shift ;;
	esac
done
if [ "$connect_timeout" != 2 ] || [ "$max_time" != 2 ]; then
	: > "$TEST_STATE/curl-timeout-violation"
fi
printf '%s\n' '{"status":"success","data":{"resultType":"vector","result":[]}}'
`)
	writeExecutable(t, filepath.Join(tempDir, "jq"), `#!/bin/sh
validate=false
while [ "$#" -gt 0 ]; do
	if [ "$1" = -e ]; then
		validate=true
	fi
	shift
done
while IFS= read -r line; do :; done
if [ "$validate" = true ]; then
	exit 1
fi
`)
	writeExecutable(t, filepath.Join(tempDir, "sleep"), `#!/bin/sh
count=0
if [ -f "$TEST_STATE/sleep-count" ]; then
	read -r count < "$TEST_STATE/sleep-count"
fi
count=$((count + 1))
printf '%s\n' "$count" > "$TEST_STATE/sleep-count"
`)

	command := exec.Command("/bin/sh", filepath.Join("..", "..", "demo", "verify.sh"))
	command.Env = append(os.Environ(), "PATH="+tempDir, "TEST_STATE="+stateDir)
	_ = command.Run()

	assertFileCount(t, filepath.Join(stateDir, "curl-count"), 1)
	assertFileCount(t, filepath.Join(stateDir, "sleep-count"), 0)
	if _, err := os.Stat(filepath.Join(stateDir, "curl-timeout-violation")); !os.IsNotExist(err) {
		t.Error("verifier did not cap curl connect and total time by the remaining wall-clock deadline")
	}
}

func TestVerifierWaitsForSampleAtOrAfterItsStart(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(tempDir, "date"), "#!/bin/sh\nprintf '%s\\n' 100\n")
	writeExecutable(t, filepath.Join(tempDir, "curl"), `#!/bin/sh
count=0
if [ -f "$TEST_STATE/curl-count" ]; then
	read -r count < "$TEST_STATE/curl-count"
fi
count=$((count + 1))
printf '%s\n' "$count" > "$TEST_STATE/curl-count"
if [ "$count" -eq 1 ]; then
	printf '%s\n' '{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"zabbix_demo_counter","env":"compose","host":"otel-demo-host","hostid":"1","item_key":"demo.counter","itemid":"2"},"value":[99,"1"]}]}}'
else
	printf '%s\n' '{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"zabbix_demo_counter","env":"compose","host":"otel-demo-host","hostid":"1","item_key":"demo.counter","itemid":"2"},"value":[100,"2"]}]}}'
fi
`)
	writeExecutable(t, filepath.Join(tempDir, "jq"), `#!/bin/sh
validate=false
verify_start_epoch=
filter=
while [ "$#" -gt 0 ]; do
	case "$1" in
	-e)
		validate=true
		shift
		;;
	-c)
		shift
		;;
	--argjson)
		if [ "$2" = verify_start_epoch ]; then
			verify_start_epoch=$3
		fi
		shift 3
		;;
	*)
		filter=$1
		shift
		;;
	esac
done
input=
while IFS= read -r line; do
	input=${input}${line}
done
case "$input" in
*'"value":[99,'*) sample_epoch=99 ;;
*'"value":[100,'*) sample_epoch=100 ;;
*) exit 1 ;;
esac
if [ "$validate" = true ]; then
	case "$filter" in
	*'.value[0]'*'tonumber >= $verify_start_epoch'*) ;;
	*) exit 0 ;;
	esac
	[ -n "$verify_start_epoch" ] && [ "$sample_epoch" -ge "$verify_start_epoch" ]
	exit
fi
if [ "$sample_epoch" -eq 99 ]; then
	printf '%s\n' '{"metric":{"__name__":"zabbix_demo_counter","env":"compose","host":"otel-demo-host","hostid":"1","item_key":"demo.counter","itemid":"2"},"value":[99,"1"]}'
else
	printf '%s\n' '{"metric":{"__name__":"zabbix_demo_counter","env":"compose","host":"otel-demo-host","hostid":"1","item_key":"demo.counter","itemid":"2"},"value":[100,"2"]}'
fi
`)
	writeExecutable(t, filepath.Join(tempDir, "sleep"), `#!/bin/sh
count=0
if [ -f "$TEST_STATE/sleep-count" ]; then
	read -r count < "$TEST_STATE/sleep-count"
fi
count=$((count + 1))
printf '%s\n' "$count" > "$TEST_STATE/sleep-count"
`)

	command := exec.Command("/bin/sh", filepath.Join("..", "..", "demo", "verify.sh"))
	command.Env = append(os.Environ(), "PATH="+tempDir, "TEST_STATE="+stateDir)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("verifier failed: %v: %s", err, output)
	}

	assertFileCount(t, filepath.Join(stateDir, "curl-count"), 2)
	assertFileCount(t, filepath.Join(stateDir, "sleep-count"), 1)
	want := `{"metric":{"__name__":"zabbix_demo_counter","env":"compose","host":"otel-demo-host","hostid":"1","item_key":"demo.counter","itemid":"2"},"value":[100,"2"]}`
	if strings.TrimSpace(string(output)) != want {
		t.Fatalf("verifier output = %q, want only the fresh sample %q", output, want)
	}
}

func TestVerifierProcessDeadlineStopsStalledToolsAndLateSuccess(t *testing.T) {
	for _, stalledTool := range []string{"curl", "jq"} {
		t.Run(stalledTool, func(t *testing.T) {
			tempDir := t.TempDir()
			stateDir := filepath.Join(tempDir, "state")
			if err := os.Mkdir(stateDir, 0o700); err != nil {
				t.Fatal(err)
			}
			writeTimeoutTestDouble(t, filepath.Join(tempDir, "timeout"))
			writeExecutable(t, filepath.Join(tempDir, "date"), "#!/bin/sh\nprintf '%s\\n' 100\n")
			staller := `#!/bin/sh
printf '%s\n' "$$" > "$TEST_STATE/stalled-pid"
while :; do :; done
`
			if stalledTool == "curl" {
				writeExecutable(t, filepath.Join(tempDir, "curl"), staller)
				writeExecutable(t, filepath.Join(tempDir, "jq"), "#!/bin/sh\nexit 1\n")
			} else {
				writeExecutable(t, filepath.Join(tempDir, "curl"), `#!/bin/sh
printf '%s\n' '{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"zabbix_demo_counter","env":"compose","host":"otel-demo-host","hostid":"1","item_key":"demo.counter","itemid":"2"},"value":[1,"1"]}]}}'
`)
				writeExecutable(t, filepath.Join(tempDir, "jq"), staller)
			}

			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "demo", "verify.sh"))
			if err != nil {
				t.Fatal(err)
			}
			entrypointPath, err := filepath.Abs(filepath.Join("..", "..", "demo", "verify-entrypoint.sh"))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3500*time.Millisecond)
			defer cancel()
			command := exec.CommandContext(ctx, "/bin/sh", entrypointPath)
			command.Env = append(os.Environ(),
				"PATH="+tempDir+":"+os.Getenv("PATH"),
				"TEST_STATE="+stateDir,
				"VERIFY_TIMEOUT_BIN="+filepath.Join(tempDir, "timeout"),
				"VERIFY_TIMEOUT_SECONDS=2",
				"VERIFY_SCRIPT_PATH="+scriptPath,
			)
			outputPath := filepath.Join(tempDir, "output")
			outputFile, err := os.Create(outputPath)
			if err != nil {
				t.Fatal(err)
			}
			command.Stdout = outputFile
			command.Stderr = outputFile
			started := time.Now()
			runErr := command.Run()
			elapsed := time.Since(started)
			if err := outputFile.Close(); err != nil {
				t.Fatal(err)
			}
			killRecordedProcess(t, filepath.Join(stateDir, "stalled-pid"))
			output, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatal(err)
			}

			if ctx.Err() != nil || elapsed >= 3*time.Second {
				t.Fatalf("verifier escaped its two-second process deadline: elapsed=%s context=%v", elapsed, ctx.Err())
			}
			if runErr == nil {
				t.Fatal("verifier reported success after a tool stalled past the deadline")
			}
			var exitError *exec.ExitError
			if !errors.As(runErr, &exitError) || exitError.ExitCode() != 137 {
				t.Fatalf("stalled verifier exit = %v, want timeout status 137", runErr)
			}
			if strings.Contains(string(output), `"__name__":"zabbix_demo_counter"`) {
				t.Fatalf("verifier emitted late success JSON after deadline: %s", output)
			}
		})
	}
}

func TestVerifierProcessDeadlinePreservesNormalSuccess(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTimeoutTestDouble(t, filepath.Join(tempDir, "timeout"))
	writeExecutable(t, filepath.Join(tempDir, "curl"), `#!/bin/sh
printf '%s\n' '{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"zabbix_demo_counter","env":"compose","host":"otel-demo-host","hostid":"1","item_key":"demo.counter","itemid":"2"},"value":[1,"1"]}]}}'
`)
	writeExecutable(t, filepath.Join(tempDir, "jq"), `#!/bin/sh
validate=false
while [ "$#" -gt 0 ]; do
	if [ "$1" = -e ]; then
		validate=true
	fi
	shift
done
while IFS= read -r line; do :; done
if [ "$validate" = true ]; then
	exit 0
fi
printf '%s\n' '{"metric":{"__name__":"zabbix_demo_counter","env":"compose","host":"otel-demo-host","hostid":"1","item_key":"demo.counter","itemid":"2"},"value":[1,"1"]}'
`)

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "demo", "verify.sh"))
	if err != nil {
		t.Fatal(err)
	}
	entrypointPath, err := filepath.Abs(filepath.Join("..", "..", "demo", "verify-entrypoint.sh"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/sh", entrypointPath)
	command.Env = append(os.Environ(),
		"PATH="+tempDir+":"+os.Getenv("PATH"),
		"TEST_STATE="+stateDir,
		"VERIFY_TIMEOUT_BIN="+filepath.Join(tempDir, "timeout"),
		"VERIFY_TIMEOUT_SECONDS=3",
		"VERIFY_SCRIPT_PATH="+scriptPath,
	)
	output, runErr := command.CombinedOutput()
	if runErr != nil {
		t.Fatalf("normal verification failed under process guard: %v: %s", runErr, output)
	}
	want := `{"metric":{"__name__":"zabbix_demo_counter","env":"compose","host":"otel-demo-host","hostid":"1","item_key":"demo.counter","itemid":"2"},"value":[1,"1"]}`
	if strings.TrimSpace(string(output)) != want {
		t.Fatalf("normal verification output = %q, want %q", output, want)
	}
}

func readYAML(t *testing.T, path string) map[string]any {
	t.Helper()
	return parseYAML(t, readAsset(t, path))
}

func parseYAML(t *testing.T, contents string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(contents), &parsed); err != nil {
		t.Fatalf("parse YAML: %v", err)
	}
	return parsed
}

func value(t *testing.T, current map[string]any, path ...string) any {
	t.Helper()
	var result any = current
	for _, key := range path {
		result = mapValue(t, result)[key]
		if result == nil {
			t.Fatalf("YAML value %q is missing", strings.Join(path, "."))
		}
	}
	return result
}

func mapValue(t *testing.T, raw any) map[string]any {
	t.Helper()
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected mapping, got %T", raw)
	}
	return result
}

func mapSlice(t *testing.T, raw any) []map[string]any {
	t.Helper()
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected list, got %T", raw)
	}
	result := make([]map[string]any, len(items))
	for index, item := range items {
		result[index] = mapValue(t, item)
	}
	return result
}

func stringSlice(t *testing.T, raw any) []string {
	t.Helper()
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected list, got %T", raw)
	}
	result := make([]string, len(items))
	for index, item := range items {
		result[index] = stringValue(t, item)
	}
	return result
}

func anySlice(t *testing.T, raw any) []any {
	t.Helper()
	if raw == nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected list, got %T", raw)
	}
	return items
}

func stringValue(t *testing.T, raw any) string {
	t.Helper()
	result, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string, got %T", raw)
	}
	return result
}

func assertEqual(t *testing.T, want, got any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func assertObjectIdentity(t *testing.T, object map[string]any, kind, name string) {
	t.Helper()
	assertEqual(t, kind, value(t, object, "kind"))
	assertEqual(t, name, value(t, mapValue(t, value(t, object, "metadata")), "name"))
}

func assertTelemetryPrometheusBinding(t *testing.T, collectorConfig map[string]any) {
	t.Helper()
	service := mapValue(t, value(t, collectorConfig, "service"))
	telemetry := mapValue(t, value(t, service, "telemetry"))
	metrics := mapValue(t, value(t, telemetry, "metrics"))
	readers := mapSlice(t, value(t, metrics, "readers"))
	if len(readers) != 1 {
		t.Fatalf("expected one Prometheus telemetry reader, got %d", len(readers))
	}
	pull := mapValue(t, value(t, readers[0], "pull"))
	exporter := mapValue(t, value(t, pull, "exporter"))
	prometheus := mapValue(t, value(t, exporter, "prometheus"))
	assertEqual(t, "0.0.0.0", value(t, prometheus, "host"))
	assertEqual(t, 8888, value(t, prometheus, "port"))
}

func assertContainerPorts(t *testing.T, container map[string]any) {
	t.Helper()
	ports := mapSlice(t, value(t, container, "ports"))
	if len(ports) != 2 {
		t.Fatalf("expected health and telemetry container ports, got %d", len(ports))
	}
	for name, port := range map[string]int{"health": 13133, "telemetry": 8888} {
		found := false
		for _, current := range ports {
			if value(t, current, "name") != name {
				continue
			}
			found = true
			assertEqual(t, port, value(t, current, "containerPort"))
			assertEqual(t, "TCP", value(t, current, "protocol"))
			break
		}
		if !found {
			t.Errorf("missing container port %q", name)
		}
	}
}

func assertContainerEnvironment(t *testing.T, container, secretData, configData map[string]any) {
	t.Helper()
	env := mapSlice(t, value(t, container, "env"))
	byName := make(map[string]map[string]any, len(env))
	for _, variable := range env {
		byName[stringValue(t, value(t, variable, "name"))] = variable
	}
	for _, key := range []string{"ZABBIX_URL", "ZABBIX_TOKEN"} {
		ref := mapValue(t, value(t, byName[key], "valueFrom", "secretKeyRef"))
		assertEqual(t, "otelcol-zabbix", value(t, ref, "name"))
		assertEqual(t, key, value(t, ref, "key"))
		if _, ok := secretData[key]; !ok {
			t.Errorf("Secret reference %q has no corresponding Secret key", key)
		}
	}
	vmRef := mapValue(t, value(t, byName["VICTORIAMETRICS_REMOTE_WRITE_URL"], "valueFrom", "configMapKeyRef"))
	assertEqual(t, "otelcol-zabbix", value(t, vmRef, "name"))
	assertEqual(t, "VICTORIAMETRICS_REMOTE_WRITE_URL", value(t, vmRef, "key"))
	if _, ok := configData["VICTORIAMETRICS_REMOTE_WRITE_URL"]; !ok {
		t.Error("ConfigMap reference has no corresponding ConfigMap key")
	}
}

func assertContainerSecurity(t *testing.T, container map[string]any) {
	t.Helper()
	security := mapValue(t, value(t, container, "securityContext"))
	assertEqual(t, false, value(t, security, "allowPrivilegeEscalation"))
	assertEqual(t, true, value(t, security, "readOnlyRootFilesystem"))
	capabilities := mapValue(t, value(t, security, "capabilities"))
	assertEqual(t, []string{"ALL"}, stringSlice(t, value(t, capabilities, "drop")))
}

func assertProbe(t *testing.T, probe map[string]any) {
	t.Helper()
	httpGet := mapValue(t, value(t, probe, "httpGet"))
	assertEqual(t, "/", value(t, httpGet, "path"))
	assertEqual(t, 13133, value(t, httpGet, "port"))
}

func assertResources(t *testing.T, resources map[string]any) {
	t.Helper()
	requests := mapValue(t, value(t, resources, "requests"))
	limits := mapValue(t, value(t, resources, "limits"))
	assertEqual(t, "100m", value(t, requests, "cpu"))
	assertEqual(t, "128Mi", value(t, requests, "memory"))
	assertEqual(t, "500m", value(t, limits, "cpu"))
	assertEqual(t, "512Mi", value(t, limits, "memory"))
}

func assertConfigVolumeWiring(t *testing.T, podSpec, container, configData map[string]any) {
	t.Helper()
	volumes := mapSlice(t, value(t, podSpec, "volumes"))
	if len(volumes) != 1 {
		t.Fatalf("expected one config volume, got %d", len(volumes))
	}
	volume := volumes[0]
	assertEqual(t, "config", value(t, volume, "name"))
	configMap := mapValue(t, value(t, volume, "configMap"))
	assertEqual(t, "otelcol-zabbix", value(t, configMap, "name"))
	items := mapSlice(t, value(t, configMap, "items"))
	if len(items) != 1 {
		t.Fatalf("expected one config map item, got %d", len(items))
	}
	assertEqual(t, "config.yaml", value(t, items[0], "key"))
	assertEqual(t, "config.yaml", value(t, items[0], "path"))
	if _, ok := configData["config.yaml"]; !ok {
		t.Error("config volume key has no corresponding ConfigMap key")
	}
	mounts := mapSlice(t, value(t, container, "volumeMounts"))
	if len(mounts) != 1 {
		t.Fatalf("expected one config volume mount, got %d", len(mounts))
	}
	assertEqual(t, "config", value(t, mounts[0], "name"))
	assertEqual(t, "/etc/otelcol-zabbix", value(t, mounts[0], "mountPath"))
	assertEqual(t, true, value(t, mounts[0], "readOnly"))
}

func assertPort(t *testing.T, ports []map[string]any, name string, port int, targetPort any) {
	t.Helper()
	for _, current := range ports {
		if value(t, current, "name") != name {
			continue
		}
		assertEqual(t, port, value(t, current, "port"))
		if targetPort != nil {
			assertEqual(t, targetPort, value(t, current, "targetPort"))
		}
		assertEqual(t, "TCP", value(t, current, "protocol"))
		return
	}
	t.Errorf("missing port %q", name)
}

func assertComposeDependency(t *testing.T, services map[string]any, serviceName, dependencyName, condition string) {
	t.Helper()
	service := mapValue(t, value(t, services, serviceName))
	dependencies := mapValue(t, value(t, service, "depends_on"))
	dependency := mapValue(t, value(t, dependencies, dependencyName))
	assertEqual(t, condition, value(t, dependency, "condition"))
}

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

func composeVolumeSource(t *testing.T, volume any) string {
	t.Helper()
	switch typed := volume.(type) {
	case string:
		return strings.SplitN(typed, ":", 2)[0]
	case map[string]any:
		return stringValue(t, value(t, typed, "source"))
	default:
		t.Fatalf("expected volume string or mapping, got %T", volume)
		return ""
	}
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
}

func writeTimeoutTestDouble(t *testing.T, path string) {
	t.Helper()
	writeExecutable(t, path, `#!/bin/sh
if [ "$1" != -s ] || [ "$2" != KILL ]; then
	exit 2
fi
shift 2
seconds=$1
shift
"$@" &
command_pid=$!
(
	/bin/sleep "$seconds"
	if [ -f "$TEST_STATE/stalled-pid" ]; then
		read -r stalled_pid < "$TEST_STATE/stalled-pid"
		kill -KILL "$stalled_pid" 2>/dev/null || :
	fi
	kill -KILL "$command_pid" 2>/dev/null || :
) &
watchdog_pid=$!
set +e
wait "$command_pid"
status=$?
set -e
kill -KILL "$watchdog_pid" 2>/dev/null || :
wait "$watchdog_pid" 2>/dev/null || :
exit "$status"
`)
}

func assertFileCount(t *testing.T, path string, want int) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) && want == 0 {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	got, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s = %d, want %d", filepath.Base(path), got, want)
	}
}

func killRecordedProcess(t *testing.T, path string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	if err != nil {
		t.Fatal(err)
	}
	process, err := os.FindProcess(pid)
	if err == nil {
		_ = process.Kill()
	}
}
