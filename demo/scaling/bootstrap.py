"""Provision only the isolated Compose Zabbix; keep credentials in its volume."""
import json
import os
from pathlib import Path
import secrets
import urllib.request

url = os.environ["ZABBIX_API_URL"]
session = None


def rpc(method, params):
    headers = {"Content-Type": "application/json-rpc"}
    if session:
        headers["Authorization"] = "Bearer " + session
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params}).encode()
    with urllib.request.urlopen(urllib.request.Request(url, data=body, headers=headers), timeout=20) as response:
        data = json.load(response)
    if "error" in data:
        # Never print request parameters, which may contain credentials.
        raise RuntimeError(f"{method}: {data['error']}")
    return data["result"]


session = rpc("user.login", {"username": os.environ["ZABBIX_USERNAME"], "password": os.environ["ZABBIX_PASSWORD"]})
try:
    groups = rpc("hostgroup.get", {"output": ["groupid"], "filter": {"name": ["Scaling demo"]}})
    group_id = groups[0]["groupid"] if groups else rpc("hostgroup.create", {"name": "Scaling demo"})["groupids"][0]
    for host in ("scale-a", "scale-b"):
        hosts = rpc("host.get", {"output": ["hostid"], "filter": {"host": [host]}})
        host_id = hosts[0]["hostid"] if hosts else rpc("host.create", {"host": host, "groups": [{"groupid": group_id}]})["hostids"][0]
        items = rpc("item.get", {"output": ["itemid"], "hostids": [host_id], "filter": {"key_": ["demo.counter"]}})
        params = {"name": "Scaling counter", "key_": "demo.counter", "hostid": host_id, "type": 2, "value_type": 0, "delay": "0", "tags": [{"tag": "scaling-demo", "value": "true"}]}
        if items:
            rpc("item.update", {"itemid": items[0]["itemid"], "tags": params["tags"]})
        else:
            rpc("item.create", params)

    # Stable tokens allow a repeated 'up' without breaking already-running workers.
    credential_path = Path("/generated/credentials.json")
    if credential_path.exists():
        credentials = json.loads(credential_path.read_text())
    else:
        token_id = rpc("token.create", {"name": "scaling-demo"})["tokenids"][0]
        credentials = {"api": rpc("token.generate", [token_id])[0]["token"], "stream": secrets.token_hex(32)}
        fd = os.open(credential_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as output:
            json.dump(credentials, output)

    connectors = rpc("connector.get", {"output": ["connectorid"], "filter": {"name": ["scaling-demo"]}})
    params = {"name": "scaling-demo", "url": "http://streaming-lb:8081/v1/history", "data_type": 0, "item_value_type": 9, "authtype": 5, "token": credentials["stream"], "max_records": 100, "max_senders": 2, "max_attempts": 5, "attempt_interval": "2s", "timeout": "10s", "tags": [{"tag": "scaling-demo", "operator": 0, "value": "true"}]}
    if connectors:
        params["connectorid"] = connectors[0]["connectorid"]
        rpc("connector.update", params)
    else:
        rpc("connector.create", params)

    for name, mode, host in (("api-a", "api", "scale-a"), ("api-b", "api", "scale-b"), ("stream-1", "stream", ""), ("stream-2", "stream", "")):
        template = Path(f"/demo/{mode}.yaml.tmpl").read_text()
        for key, value in {"API_TOKEN": credentials["api"], "STREAM_TOKEN": credentials["stream"], "COLLECTOR": name, "HOST": host}.items():
            template = template.replace(f"@@{key}@@", value)
        path = Path(f"/generated/{name}.yaml")
        temp = path.with_suffix(".tmp")
        fd = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(fd, "w") as output:
            output.write(template)
        os.chown(temp, 10001, 10001)
        os.chmod(temp, 0o400)
        temp.replace(path)
    print("Scaling demo ready: two API shards, two Streaming workers")
finally:
    rpc("user.logout", [])
