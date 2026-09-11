#!/bin/sh
set -eu

verify_timeout_seconds=${VERIFY_TIMEOUT_SECONDS:-180}
case "$verify_timeout_seconds" in
  ''|*[!0-9]*) echo 'VERIFY_TIMEOUT_SECONDS must be a positive integer' >&2; exit 2 ;;
esac
if [ "$verify_timeout_seconds" -le 0 ]; then
  echo 'VERIFY_TIMEOUT_SECONDS must be a positive integer' >&2
  exit 2
fi
grafana_url=${GRAFANA_URL:-http://grafana:3000}
deadline=$(($(date +%s) + verify_timeout_seconds))
fetch() {
  remaining=$((deadline - $(date +%s)))
  [ "$remaining" -gt 0 ] || return 1
  curl --silent --show-error --fail --connect-timeout "$remaining" --max-time "$remaining" "$grafana_url$1"
}
while [ "$(date +%s)" -lt "$deadline" ]; do
  if health=$(fetch /api/health) &&
     printf '%s\n' "$health" | jq -e '.database == "ok"' >/dev/null &&
     dashboard=$(fetch /api/dashboards/uid/zabbix-receiver) &&
     printf '%s\n' "$dashboard" | jq -e '.dashboard.uid == "zabbix-receiver" and (.dashboard.panels | length > 0)' >/dev/null; then
    echo 'Grafana and zabbix-receiver dashboard are ready'
    exit 0
  fi
  [ "$(date +%s)" -lt "$deadline" ] || break
  sleep 1
done
echo 'Grafana or zabbix-receiver dashboard was not ready before the deadline' >&2
exit 1
