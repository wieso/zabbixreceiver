"""Verify fresh series and disjoint API ownership in real VictoriaMetrics."""
import json
import os
import time
import urllib.parse
import urllib.request

start = int(time.time())
timeout = int(os.environ.get("VERIFY_TIMEOUT_SECONDS", "180"))
if timeout <= 0:
    raise SystemExit("VERIFY_TIMEOUT_SECONDS must be positive")
workers = os.environ.get("EXPECTED_STREAM_WORKERS", "stream-1,stream-2").split(",")
expected_api = {("scale-a", "api-a"), ("scale-b", "api-b")}
expected_stream = {(host, worker) for host in ("scale-a", "scale-b") for worker in workers}
selector = '{__name__=~"scale_(api_demo_counter|stream_Scaling_counter)",env="scaling"}'
query = f"{selector} and (timestamp({selector}) >= {start})"
url = "http://victoriametrics:8428/api/v1/query?" + urllib.parse.urlencode({"query": query})
deadline = time.monotonic() + timeout
last = None
while time.monotonic() < deadline:
    try:
        with urllib.request.urlopen(url, timeout=min(10, max(1, deadline-time.monotonic()))) as response:
            last = json.load(response)
        if last.get("status") != "success":
            raise ValueError("query failed")
        rows = last["data"]["result"]
        api = {(r["metric"]["host"], r["metric"].get("collector")) for r in rows if r["metric"]["__name__"] == "scale_api_demo_counter"}
        stream = {(r["metric"]["host"], r["metric"].get("collector")) for r in rows if r["metric"]["__name__"] == "scale_stream_Scaling_counter"}
        if api == expected_api and stream == expected_stream and len(rows) == len(api) + len(stream) and all(float(r["value"][1]) > 0 for r in rows):
            print(json.dumps({"fresh_api_ownership": sorted(api), "fresh_stream_routes": sorted(stream)}, ensure_ascii=False))
            raise SystemExit(0)
    except (OSError, ValueError, KeyError) as error:
        last = str(error)
    time.sleep(2)
raise SystemExit(f"Fresh series did not match within {timeout}s: {last}")
