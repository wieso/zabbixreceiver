# GitHub Binary Release Design

## Goal

Publish downloadable Linux binaries of `otelcol-zabbix` from GitHub Releases so an operator can install and run the collector without installing Go or building the repository.

## Scope

The first release supports:

- Linux `amd64`.
- Linux `arm64`.
- Version tags named `vX.Y.Z`.
- Compressed tar archives, SHA-256 checksums, and release-page startup instructions.

macOS, Windows, package-manager repositories, container publishing, signing, and automatic changelog generation are out of scope for this release implementation.

## Existing constraints

- The module requires Go 1.25 or newer.
- The entrypoint is `./cmd/otelcol-zabbix`.
- The binary version is set through `-ldflags '-X main.version=...'`.
- The binary can be built with `CGO_ENABLED=0` and therefore cross-compiles for the two target architectures from the Linux GitHub Actions runner.
- Runtime credentials are supplied through environment variables and must not be embedded in release artifacts.

## Release workflow

`.github/workflows/release.yml` runs when a tag matching `v*.*.*` is pushed. It has `contents: write` permission and performs these phases:

1. Check out the tagged commit.
2. Set up Go 1.25.x.
3. Download dependencies.
4. Run the repository verification commands: formatting check, unit tests, race tests, vet, and collector configuration validation.
5. Build and package both target architectures.
6. Verify the generated assets before publication.
7. Create a non-draft GitHub Release for the tag and upload both archives plus `checksums.txt`.

The workflow must fail before publishing if any verification or packaging check fails. It must not print credentials or expand the sample configuration with real values.

## Artifact contract

For a tag `v1.2.3`, the assets are:

```text
otelcol-zabbix_1.2.3_linux_amd64.tar.gz
otelcol-zabbix_1.2.3_linux_arm64.tar.gz
checksums.txt
```

The version in the filename omits the leading `v`, while the binary reports the tag version including `v` through the existing build-info path.

Each archive contains a single top-level directory named after the archive without `.tar.gz`:

```text
otelcol-zabbix_1.2.3_linux_amd64/
├── otelcol-zabbix
├── configs/otelcol.yaml
├── README.md
├── docs/configuration.md
└── docs/deployment.md
```

The binary is executable. Documentation and configuration are regular files. No `.git` data, build directory, token, password, or generated runtime configuration is included.

`checksums.txt` contains one SHA-256 entry per archive, using the exact asset filenames and a stable ordering.

## Local packaging interface

The repository exposes a `make release-artifacts` target. It accepts `VERSION` (for example, `VERSION=v1.2.3`), creates a clean output directory under `dist/`, cross-compiles the two binaries, creates the archives, writes `checksums.txt`, and performs the same structural checks used by CI.

The packaging implementation should be kept in a focused script under `scripts/` rather than duplicated in YAML. The script must use a temporary staging directory and only copy the explicitly listed release files. It must reject versions that do not match `vMAJOR.MINOR.PATCH` and reject empty or unexpected target values.

## Documentation

`README.md` gets a download-first quick-start section linking to the repository Releases page. It explains how to select the architecture, verify checksums, unpack the archive, set `ZABBIX_URL`, `ZABBIX_TOKEN`, and `VICTORIAMETRICS_REMOTE_WRITE_URL`, and start the binary with `configs/otelcol.yaml`.

`docs/deployment.md` gets a “Prebuilt release binary” section. It documents Linux prerequisites, the exact download URL pattern, checksum verification, installation under `/usr/local/bin`, configuration placement, and a minimal start command. It points to the existing systemd section for service installation.

The instructions must state that the sample token is a placeholder and that HTTPS plus secret-management practices are required for production.

## Verification

Packaging tests and CI checks must verify:

- Both expected archives exist and no unexpected release assets are generated.
- Archive paths match the artifact contract and contain no path traversal.
- Each archive contains the executable binary and the three documented files.
- The binary is executable and `components` succeeds for both architectures where the runner can execute it; cross-compiled artifacts must at least pass `file`/ELF architecture inspection and checksum verification.
- The version is embedded in the build output.
- `sha256sum -c checksums.txt` succeeds.
- The release staging tree contains no configured credential names with values, token placeholders beyond the tracked documentation/configuration examples, or untracked build output.
- Existing Go, configuration, Compose, Docker, and demo verification remains unchanged and passing.

## Failure and rerun behavior

The workflow is tag-driven and immutable at the commit level. If publication fails before creating the release, rerunning the workflow for the same tag is allowed. If a release already exists, the workflow must fail clearly rather than silently replacing assets. A maintainer may delete or edit the GitHub Release manually before retrying; the workflow does not delete releases or rewrite repository history.

## Acceptance criteria

An operator on supported Linux can:

1. Open the GitHub Releases page and select the correct architecture archive.
2. Verify the archive with `checksums.txt`.
3. Extract and install the binary without Go.
4. Copy or use the included sample configuration.
5. Supply runtime endpoints and credentials through environment variables.
6. Start the collector and reach its configured health endpoint.

The repository can also produce and validate the exact release assets locally with one documented make target.
