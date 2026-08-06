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

query='zabbix_demo_counter{host="otel-demo-host",env="compose"}'
verify_start_epoch=$(date +%s)
deadline=$((verify_start_epoch + verify_timeout_seconds))
last_response=
while :; do
	now=$(date +%s)
	remaining=$((deadline - now))
	if [ "$remaining" -le 1 ]; then
		break
	fi
	connect_timeout=5
	if [ "$remaining" -lt "$connect_timeout" ]; then
		connect_timeout=$remaining
	fi
	if response=$(curl --silent --show-error --fail-with-body \
		--connect-timeout "$connect_timeout" \
		--max-time "$remaining" \
		--get \
		--data-urlencode "query=$query" \
		http://victoriametrics:8428/api/v1/query); then
		last_response=$response
		if printf '%s\n' "$response" | jq -e --argjson verify_start_epoch "$verify_start_epoch" '
			.status == "success" and
			(.data.result | length) == 1 and
			.data.result[0].metric.host == "otel-demo-host" and
			.data.result[0].metric.env == "compose" and
			.data.result[0].metric.item_key == "demo.counter" and
			(.data.result[0].metric.hostid | type == "string" and length > 0) and
			(.data.result[0].metric.itemid | type == "string" and length > 0) and
			(try (.data.result[0].value[0] | tonumber >= $verify_start_epoch) catch false) and
			(try (.data.result[0].value[1] | tonumber > 0) catch false)
		' >/dev/null; then
			printf '%s\n' "$response" | jq -c '.data.result[0]'
			exit 0
		fi
	fi
	now=$(date +%s)
	remaining=$((deadline - now))
	if [ "$remaining" -le 1 ]; then
		break
	fi
	sleep_time=2
	if [ "$remaining" -lt "$sleep_time" ]; then
		sleep_time=$remaining
	fi
	sleep "$sleep_time"
done

printf 'expected zabbix_demo_counter metric was not observed within %s seconds\n' "$verify_timeout_seconds" >&2
if [ -n "$last_response" ]; then
	printf '%s\n' "$last_response" | jq -c . >&2
fi
exit 1
