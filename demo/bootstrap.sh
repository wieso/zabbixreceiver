#!/bin/sh
set -eu

: "${ZABBIX_API_URL:?ZABBIX_API_URL is required}"
: "${ZABBIX_USERNAME:?ZABBIX_USERNAME is required}"
: "${ZABBIX_PASSWORD:?ZABBIX_PASSWORD is required}"

rpc_result() {
	jq -e '
		if has("error") then
			error("Zabbix API error: \(.error.code) \(.error.message) \(.error.data)")
		elif has("result") then
			.result
		else
			error("Zabbix API response is missing result")
		end
	'
}

rpc_unauthenticated() (
	method=$1
	params=$2
	request=$(jq -nc --arg method "$method" --argjson params "$params" \
		'{jsonrpc:"2.0",method:$method,params:$params,id:1}')
	response=$(printf '%s' "$request" | curl --silent --show-error --fail-with-body \
		--request POST \
		--header 'Content-Type: application/json-rpc' \
		--data-binary @- \
		"$ZABBIX_API_URL")
	printf '%s\n' "$response" | rpc_result
)

rpc_session() (
	method=$1
	params=$2
	request=$(jq -nc --arg method "$method" --argjson params "$params" \
		'{jsonrpc:"2.0",method:$method,params:$params,id:1}')
	response=$(printf 'header = "Authorization: Bearer %s"\n' "$session_token" | \
		curl --config - --silent --show-error --fail-with-body \
		--request POST \
		--header 'Content-Type: application/json-rpc' \
		--data-binary "$request" \
		"$ZABBIX_API_URL")
	printf '%s\n' "$response" | rpc_result
)

rpc_bearer() (
	method=$1
	params=$2
	request=$(jq -nc --arg method "$method" --argjson params "$params" \
		'{jsonrpc:"2.0",method:$method,params:$params,id:1}')
	response=$(printf 'header = "Authorization: Bearer %s"\n' "$api_token" | \
		curl --config - --silent --show-error --fail-with-body \
		--request POST \
		--header 'Content-Type: application/json-rpc' \
		--data-binary "$request" \
		"$ZABBIX_API_URL")
	printf '%s\n' "$response" | rpc_result
)

until rpc_unauthenticated apiinfo.version '{}' >/dev/null 2>&1; do
	sleep 2
done

login_params=$(jq -nc \
	--arg username "$ZABBIX_USERNAME" \
	--arg password "$ZABBIX_PASSWORD" \
	'{username:$username,password:$password}')
session_result=$(rpc_unauthenticated user.login "$login_params")
session_token=$(printf '%s\n' "$session_result" | jq -er \
	'if type == "string" and length > 0 then . else error("user.login returned no session") end')

groups=$(rpc_session hostgroup.get \
	'{"output":["groupid"],"filter":{"name":["OpenTelemetry Demo"]}}')
group_id=$(printf '%s\n' "$groups" | jq -r '.[0].groupid // empty')
if [ -z "$group_id" ]; then
	created_group=$(rpc_session hostgroup.create '{"name":"OpenTelemetry Demo"}')
	group_id=$(printf '%s\n' "$created_group" | jq -er '.groupids[0]')
fi

host_params='{"output":["hostid"],"filter":{"host":["otel-demo-host"]}}'
hosts=$(rpc_session host.get "$host_params")
host_id=$(printf '%s\n' "$hosts" | jq -r '.[0].hostid // empty')
if [ -z "$host_id" ]; then
	create_host_params=$(jq -nc --arg group_id "$group_id" \
		'{host:"otel-demo-host",groups:[{groupid:$group_id}]}')
	created_host=$(rpc_session host.create "$create_host_params")
	host_id=$(printf '%s\n' "$created_host" | jq -er '.hostids[0]')
fi

item_params=$(jq -nc --arg host_id "$host_id" \
	'{output:["itemid"],hostids:[$host_id],filter:{key_:["demo.counter"]}}')
items=$(rpc_session item.get "$item_params")
item_id=$(printf '%s\n' "$items" | jq -r '.[0].itemid // empty')
if [ -z "$item_id" ]; then
	create_item_params=$(jq -nc --arg host_id "$host_id" \
		'{name:"OpenTelemetry demo counter",key_:"demo.counter",hostid:$host_id,type:2,value_type:0,delay:"0"}')
	created_item=$(rpc_session item.create "$create_item_params")
	item_id=$(printf '%s\n' "$created_item" | jq -er '.itemids[0]')
fi

user_get_params=$(jq -nc --arg username "$ZABBIX_USERNAME" \
	'{output:["userid"],filter:{username:[$username]}}')
users=$(rpc_session user.get "$user_get_params")
user_id=$(printf '%s\n' "$users" | jq -er \
	'if length == 1 then .[0].userid else error("expected the authenticated user") end')

token_get_params=$(jq -nc --arg user_id "$user_id" \
	'{output:["tokenid"],userids:[$user_id],filter:{name:["otel-demo-receiver"]}}')
tokens=$(rpc_session token.get "$token_get_params")
token_ids=$(printf '%s\n' "$tokens" | jq -c '[.[].tokenid]')
if [ "$token_ids" != '[]' ]; then
	rpc_session token.delete "$token_ids" >/dev/null
fi

create_token_params=$(jq -nc --arg user_id "$user_id" \
	'{name:"otel-demo-receiver",userid:$user_id}')
created_token=$(rpc_session token.create "$create_token_params")
token_id=$(printf '%s\n' "$created_token" | jq -er '.tokenids[0]')
generate_token_params=$(jq -nc --arg token_id "$token_id" '[$token_id]')
generated_token=$(rpc_session token.generate "$generate_token_params")
api_token=$(printf '%s\n' "$generated_token" | jq -er \
	'if length == 1 and .[0].token != null and (.[0].token | length) > 0 then .[0].token else error("token.generate returned no token") end')

rpc_bearer user.get "$user_get_params" >/dev/null

rm -f /generated/ready /generated/otelcol.yaml
umask 0377
while IFS= read -r line || [ -n "$line" ]; do
	case "$line" in
		*'@@ZABBIX_TOKEN@@'*)
			prefix=${line%%'@@ZABBIX_TOKEN@@'*}
			suffix=${line#*'@@ZABBIX_TOKEN@@'}
			printf '%s%s%s\n' "$prefix" "$api_token" "$suffix"
			;;
		*)
			printf '%s\n' "$line"
			;;
	esac
done < /demo/collector.yaml.tmpl > /generated/otelcol.yaml
chown 10001:10001 /generated/otelcol.yaml
chmod 0400 /generated/otelcol.yaml
: > /generated/ready

printf '%s\n' 'Zabbix demo bootstrap complete'
