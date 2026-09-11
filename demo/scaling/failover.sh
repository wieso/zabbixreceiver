#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
compose() { docker compose -f demo/scaling/compose.yaml "$@"; }
compose --profile verify run --rm verify
restore() { compose start stream-1; }
trap restore EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
compose stop stream-1
compose --profile verify run --rm -e EXPECTED_STREAM_WORKERS=stream-2 verify
restore
trap - EXIT INT TERM
compose --profile verify run --rm verify
printf '%s\n' 'PASS: both workers, surviving worker, restored workers; API shards remained active'
