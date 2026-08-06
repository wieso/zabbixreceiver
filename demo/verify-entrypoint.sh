#!/bin/sh
set -eu

verify_timeout_seconds=${VERIFY_TIMEOUT_SECONDS:-180}
case "$verify_timeout_seconds" in
	'' | *[!0-9]*)
		printf '%s\n' 'VERIFY_TIMEOUT_SECONDS must be a positive integer' >&2
		exit 2
		;;
esac
if [ "$verify_timeout_seconds" -le 0 ]; then
	printf '%s\n' 'VERIFY_TIMEOUT_SECONDS must be a positive integer' >&2
	exit 2
fi

timeout_bin=${VERIFY_TIMEOUT_BIN:-/usr/bin/timeout}
verify_script=${VERIFY_SCRIPT_PATH:-/demo/verify.sh}
exec "$timeout_bin" -s KILL "$verify_timeout_seconds" /bin/sh "$verify_script"
