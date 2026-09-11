#!/bin/sh
set -u
value=1
while :; do
    # Both hosts in every submission: each balanced request can demonstrate both routes.
    printf 'scale-a demo.counter %s\nscale-b demo.counter %s\n' "$value" "$value" |
        zabbix_sender -z zabbix-server -i -
    value=$((value + 1))
    sleep 1
done
