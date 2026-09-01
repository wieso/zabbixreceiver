#!/usr/bin/env bash

set -euo pipefail

version=${VERSION:-}
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "VERSION must match vMAJOR.MINOR.PATCH, got: ${version:-<empty>}" >&2
  exit 1
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
archive_version=${version#v}
dist_dir="$root_dir/dist"
stage_dir=$(mktemp -d "${TMPDIR:-/tmp}/otelcol-zabbix-release.XXXXXX")

cleanup() {
  rm -rf "$stage_dir"
}
trap cleanup EXIT

rm -rf "$dist_dir"
mkdir -p "$dist_dir"

target_specs=(linux/amd64 linux/arm64)
for target in "${target_specs[@]}"; do
  arch=${target#*/}
  archive_base="otelcol-zabbix_${archive_version}_linux_${arch}"
  package_dir="$stage_dir/$archive_base"
  mkdir -p "$package_dir/configs" "$package_dir/docs"

  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build \
    -trimpath \
    -ldflags="-s -w -X main.version=$version" \
    -o "$package_dir/otelcol-zabbix" \
    "$root_dir/cmd/otelcol-zabbix"

  cp "$root_dir/configs/otelcol.yaml" "$package_dir/configs/otelcol.yaml"
  cp "$root_dir/README.md" "$package_dir/README.md"
  cp "$root_dir/docs/configuration.md" "$package_dir/docs/configuration.md"
  cp "$root_dir/docs/deployment.md" "$package_dir/docs/deployment.md"

  chmod 0755 "$package_dir/otelcol-zabbix"
  archive_path="$dist_dir/$archive_base.tar.gz"
  if tar --help 2>&1 | grep -q -- '--sort'; then
    tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
      -czf "$archive_path" -C "$stage_dir" "$archive_base"
  else
    tar -czf "$archive_path" -C "$stage_dir" "$archive_base"
  fi
done

if command -v sha256sum >/dev/null 2>&1; then
  checksum_command=sha256sum
else
  checksum_command='shasum -a 256'
fi
(cd "$dist_dir" && $checksum_command \
  "otelcol-zabbix_${archive_version}_linux_amd64.tar.gz" \
  "otelcol-zabbix_${archive_version}_linux_arm64.tar.gz" \
  > checksums.txt)

expected_assets=(
  "otelcol-zabbix_${archive_version}_linux_amd64.tar.gz"
  "otelcol-zabbix_${archive_version}_linux_arm64.tar.gz"
  checksums.txt
)
expected_asset_list=$(printf '%s\n' "${expected_assets[@]}" | sort)
actual_asset_list=$(find "$dist_dir" -maxdepth 1 -type f -print \
  | sed "s#^$dist_dir/##" | sort)
if [[ "$actual_asset_list" != "$expected_asset_list" ]]; then
  echo "unexpected release assets: $actual_asset_list" >&2
  exit 1
fi

host_arch=$(uname -m)
for target in "${target_specs[@]}"; do
  arch=${target#*/}
  archive_base="otelcol-zabbix_${archive_version}_linux_${arch}"
  archive_path="$dist_dir/$archive_base.tar.gz"
  expected_members=$(printf '%s\n' \
    "$archive_base" \
    "$archive_base/README.md" \
    "$archive_base/configs" \
    "$archive_base/configs/otelcol.yaml" \
    "$archive_base/docs" \
    "$archive_base/docs/configuration.md" \
    "$archive_base/docs/deployment.md" \
    "$archive_base/otelcol-zabbix" | sort)
  actual_members=$(tar -tzf "$archive_path" | sed 's#/$##' | sort)
  if [[ "$actual_members" != "$expected_members" ]]; then
    echo "unexpected members in $archive_path" >&2
    diff -u <(printf '%s\n' "$expected_members") <(printf '%s\n' "$actual_members") >&2 || true
    exit 1
  fi
  if tar -tzf "$archive_path" | grep -Eq '(^/|(^|/)\.\.(/|$))'; then
    echo "unsafe path in $archive_path" >&2
    exit 1
  fi

  binary="$stage_dir/$archive_base/otelcol-zabbix"
  file_output=$(file "$binary")
  case "$arch" in
    amd64) printf '%s\n' "$file_output" | grep -Eq 'ELF 64-bit.*x86-64' ;;
    arm64) printf '%s\n' "$file_output" | grep -Eq 'ELF 64-bit.*ARM aarch64' ;;
  esac
  if [[ "$host_arch" == "x86_64" && "$arch" == "amd64" ]] || \
     [[ "$host_arch" == "aarch64" && "$arch" == "arm64" ]]; then
    "$binary" --version | grep -Fq "$version"
    "$binary" components >/dev/null
  fi
done

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$dist_dir" && sha256sum -c checksums.txt >/dev/null)
else
  while read -r expected_hash filename; do
    actual_hash=$(cd "$dist_dir" && shasum -a 256 "$filename" | awk '{print $1}')
    [[ "$actual_hash" == "$expected_hash" ]]
  done < "$dist_dir/checksums.txt"
fi
echo "release artifacts written to $dist_dir"
