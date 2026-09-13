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

selector='{__name__=~"zabbix_(api_demo_counter|stream_open_telemetry_demo_counter)",host="otel-demo-host",env="compose"}'
verify_start_epoch=$(date +%s)
# Instant-query result timestamps are evaluation times; filter source timestamps.
query="$selector and (timestamp($selector) >= $verify_start_epoch)"
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
			(.data.result | length) == 2 and
            ([.data.result[].metric.__name__] | sort) == ["zabbix_api_demo_counter", "zabbix_stream_open_telemetry_demo_counter"] and
            all(.data.result[];
			.metric.host == "otel-demo-host" and
			.metric.env == "compose" and
            (if .metric.__name__ == "zabbix_api_demo_counter" then
                .metric.item_key == "demo.counter" and
                (.metric.hostid | type == "string" and length > 0)
             else
                (.metric | has("item_key") | not) and
                (.metric | has("hostid") | not)
             end) and
			(.metric.itemid | type == "string" and length > 0) and
			(try (.value[0] | tonumber >= $verify_start_epoch) catch false) and
			(try (.value[1] | tonumber > 0) catch false))
		' >/dev/null; then
			printf '%s\n' "$response" | jq -c '.data.result[]'
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

printf 'expected API and Streaming metrics was not observed within %s seconds\n' "$verify_timeout_seconds" >&2
if [ -n "$last_response" ]; then
	printf '%s\n' "$last_response" | jq -c . >&2
fi
exit 1
