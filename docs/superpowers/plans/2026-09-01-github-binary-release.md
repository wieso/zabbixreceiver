# GitHub Binary Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish verified Linux `amd64` and `arm64` collector binaries as GitHub Release assets with checksums and no-Go startup instructions.

**Architecture:** Keep release packaging in a repository script invoked by `make release-artifacts`; the script creates only explicitly allowed files in deterministic archive layouts and validates them. A tag-triggered GitHub Actions workflow runs the existing verification suite, invokes the packaging target, revalidates checksums/assets, and creates the release with GitHub's official release action.

**Tech Stack:** Go 1.25, Make, POSIX shell, tar, sha256sum, GitHub Actions, `softprops/action-gh-release@v2`.

**Spec:** `docs/superpowers/specs/2026-09-01-github-binary-release-design.md`

## Global Constraints

- Support only Linux `amd64` and Linux `arm64` in the first release.
- Accept only versions matching `vMAJOR.MINOR.PATCH`.
- Build with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags "-s -w -X main.version=$(VERSION)"`.
- Never copy generated runtime configuration or real credentials into release staging directories.
- Publish exactly two archives and `checksums.txt`.
- Preserve the existing Docker, Compose, Kubernetes, systemd, and demo workflows.

## Files and responsibilities

- Create: `scripts/package-release.sh` — cross-compiles, stages, archives, checksums, and validates release assets.
- Modify: `Makefile` — expose `release-artifacts` and include it in `.PHONY`.
- Create: `.github/workflows/release.yml` — tag-triggered verification and GitHub Release publication.
- Modify: `README.md` — download-first quick start for release users.
- Modify: `docs/deployment.md` — prebuilt binary installation instructions.
- Modify: `internal/packaging/assets_test.go` — add static contract assertions for the release script, Make target, and release workflow so the packaging interface cannot drift silently.

### Task 1: Add deterministic local release packaging

**Files:**
- Create: `scripts/package-release.sh`
- Modify: `Makefile`

**Interfaces:**
- Consumes: `VERSION` from the Make environment and repository paths `cmd/otelcol-zabbix`, `configs/otelcol.yaml`, `README.md`, `docs/configuration.md`, and `docs/deployment.md`.
- Produces: `dist/otelcol-zabbix_<version>_linux_amd64.tar.gz`, `dist/otelcol-zabbix_<version>_linux_arm64.tar.gz`, and `dist/checksums.txt`.

- [ ] **Step 1: Define the Make target and script contract**

Add `release-artifacts` to `.PHONY` and invoke `scripts/package-release.sh` with `VERSION`:

```make
release-artifacts:
	VERSION=$(VERSION) ./scripts/package-release.sh
```

Make the script executable.

- [ ] **Step 2: Implement version and target validation**

In `scripts/package-release.sh`, enable strict shell options, require `VERSION` to match `^v[0-9]+\.[0-9]+\.[0-9]+$`, derive the archive version by removing the leading `v`, and define the fixed target list `linux/amd64` and `linux/arm64`. Exit with a field-specific error for an unset or malformed version.

- [ ] **Step 3: Implement isolated staging and cross-compilation**

Create a temporary directory with a cleanup trap. For each target, build `./cmd/otelcol-zabbix` using `GOOS=linux`, the target `GOARCH`, `CGO_ENABLED=0`, `-trimpath`, and `-ldflags="-s -w -X main.version=${VERSION}"`. Copy only the binary and the four explicitly listed documentation/configuration paths into the target directory, preserving `configs/` and `docs/` subdirectories.

- [ ] **Step 4: Create archives and checksums**

Create deterministic gzip tar archives from the staging parent so each archive has exactly one top-level directory. Use stable file ordering and normalized metadata where supported. Write `dist/checksums.txt` from the two archive paths in lexical order, with filenames relative to `dist/`.

- [ ] **Step 5: Add script-level asset validation**

Validate the exact two archive filenames, archive member paths, executable permission, absence of `..` or absolute paths, SHA-256 verification, and ELF architecture (`x86-64` for `amd64`, `ARM aarch64` for `arm64`) using available host tools. Run the target binary's `components` command only when its architecture matches the current runner; otherwise report that cross-compiled inspection was used.

- [ ] **Step 6: Run the local packaging contract**

Run:

```bash
make release-artifacts VERSION=v1.2.3
sha256sum -c dist/checksums.txt
tar -tzf dist/otelcol-zabbix_1.2.3_linux_amd64.tar.gz
tar -tzf dist/otelcol-zabbix_1.2.3_linux_arm64.tar.gz
```

Expected: the target exits successfully, checksums report `OK`, and each archive contains only its documented top-level directory and five required files.

- [ ] **Step 7: Commit the packaging unit**

```bash
git add Makefile scripts/package-release.sh
git commit -m "build: add release artifact packaging"
```

### Task 2: Document downloading and installing a release

**Files:**
- Modify: `README.md`
- Modify: `docs/deployment.md`

**Interfaces:**
- Consumes: The artifact names and archive layout produced by Task 1.
- Produces: Copy-pasteable public download, checksum, extraction, installation, configuration, and startup instructions.

- [ ] **Step 1: Add the README release quick start**

Add a section before the Go-based local build instructions. Use the repository's Releases page link and the URL pattern `https://github.com/wieso/zabbixreceiver/releases/download/vX.Y.Z/otelcol-zabbix_X.Y.Z_linux_ARCH.tar.gz`. Show architecture selection, `curl -fL`, checksum verification against `checksums.txt`, extraction, and the runtime command with the three required environment variables.

- [ ] **Step 2: Add the deployment guide prebuilt-binary section**

Document Linux prerequisites, installation under `/usr/local/bin/otelcol-zabbix`, copying `configs/otelcol.yaml`, environment-variable precedence, health endpoint verification, and the link to the existing systemd procedure. Explicitly call the token value a placeholder and recommend HTTPS and secret injection for production.

- [ ] **Step 3: Check documentation commands and links**

Run Markdown link/path checks available in the repository and inspect every command for matching archive names, architecture placeholders, and the actual configuration path. Ensure no command asks users to install Go for the release path.

- [ ] **Step 4: Commit documentation**

```bash
git add README.md docs/deployment.md
git commit -m "docs: explain installing release binaries"
```

### Task 3: Add the tag-triggered GitHub Release workflow

**Files:**
- Create: `.github/workflows/release.yml`

**Interfaces:**
- Consumes: Task 1's `make release-artifacts VERSION=...` interface and Task 2's documented artifact contract.
- Produces: A non-draft GitHub Release containing the two archives and `checksums.txt`.

- [ ] **Step 1: Define the tag trigger and permissions**

Configure `push.tags` for `v*.*.*`, `ubuntu-latest`, Go `1.25.x`, and job-level `permissions: contents: write`. Do not add a manual workflow input that could publish an untagged or mismatched version.

- [ ] **Step 2: Reuse repository verification**

Run checkout, setup-go, dependency download, `make fmt` plus `git diff --check` and `git diff --exit-code`, `make test`, `make test-race`, `make vet`, and `make validate-config`. The validation command must use dummy endpoints only and must not print a secret.

- [ ] **Step 3: Build and validate release assets**

Derive the version from the tag or dispatch input, run `make release-artifacts VERSION="$VERSION"`, run `sha256sum -c dist/checksums.txt`, and list the output directory. Fail if the expected three files are not the only files in `dist/`.

- [ ] **Step 4: Publish the release**

Use `softprops/action-gh-release@v2` with `tag_name`, `name`, generated release notes, and the exact three asset paths. Do not enable replacement or deletion behavior; an existing release/assets conflict must fail visibly.

- [ ] **Step 5: Validate workflow syntax and policy**

Parse the YAML with an available YAML parser, inspect the workflow for `contents: write`, tag filters, exact asset paths, and absence of secret values. If `actionlint` is available, run it and record the result.

- [ ] **Step 6: Commit the workflow**

```bash
git add .github/workflows/release.yml
git commit -m "ci: publish tagged collector binaries"
```

### Task 4: Run the complete verification suite

**Files:**
- Test: repository-wide commands and generated `dist/` artifacts only; do not commit generated `dist/` output.

- [ ] **Step 1: Run Go and configuration verification**

```bash
make fmt
git diff --check
make test
make test-race
make vet
make validate-config
```

- [ ] **Step 2: Run packaging and existing deployment verification**

```bash
make release-artifacts VERSION=v1.2.3
sha256sum -c dist/checksums.txt
make compose-config
go test ./internal/packaging -count=1
```

Run Docker, Kubernetes, systemd, and demo checks only when their required host tools/daemon are available, following `docs/deployment.md`.

- [ ] **Step 3: Inspect the final change set**

```bash
git status --short
git diff HEAD~3 --check
git diff HEAD~3 --stat
```

Confirm `.idea/` and generated `dist/` remain untracked/ignored as appropriate, release archives contain no credentials, and all acceptance criteria in the spec are covered.

- [ ] **Step 4: Report evidence and integration options**

Report exact command results, note any unavailable host-only checks, and present the resulting commits for review. Do not claim the GitHub Release exists until a real tag workflow has run on GitHub.
