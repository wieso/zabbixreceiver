#!/bin/sh
set -u

until nc -z -w 2 zabbix-server 10051; do
	sleep 2
done

value=1
while :; do
	zabbix_sender -z zabbix-server -s otel-demo-host -k demo.counter -o "$value"
	value=$((value + 1))
	sleep 5
done
