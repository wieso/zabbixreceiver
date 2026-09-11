"""Paced synthetic NDJSON -> HAProxy -> Collectors -> VictoriaMetrics probe.

Bypasses Zabbix; checks exported sample count, not just HTTP acknowledgements.
Uses 1,000 series and fresh host names to avoid earlier runs affecting counts.
"""
from concurrent.futures import ThreadPoolExecutor
import json
import math
import os
from pathlib import Path
import time
import urllib.parse
import urllib.request

rate = int(os.environ.get("RATE", "1000"))
duration = int(os.environ.get("DURATION", "30"))
if not 1 <= rate <= 100000 or not 1 <= duration <= 3600:
    raise SystemExit("RATE must be 1..100000 and DURATION 1..3600")
token = json.loads(Path("/generated/credentials.json").read_text())["stream"]
host = f"load-{time.time_ns()}"
batch_size = min(1000, rate)
total = rate * duration
start_ns = time.time_ns()
start = time.monotonic()


def send(offset):
    count = min(batch_size, total - offset)
    lines = []
    for i in range(offset, offset + count):
        stamp = start_ns + i * 1000000000 // rate
        lines.append(json.dumps({"host": {"host": host}, "name": "Load probe", "itemid": i % 1000 + 1, "clock": stamp // 1000000000, "ns": stamp % 1000000000, "value": i + 1, "type": 0}, separators=(",", ":")))
    payload = ("\n".join(lines) + "\n").encode()
    request = urllib.request.Request("http://streaming-lb:8081/v1/history", data=payload, headers={"Content-Type": "application/x-ndjson", "Authorization": "Bearer " + token})
    before = time.monotonic()
    with urllib.request.urlopen(request, timeout=20) as response:
        if response.status != 200:
            raise RuntimeError(f"HTTP {response.status}")
        response.read()
    return count, len(payload), time.monotonic() - before


futures = []
with ThreadPoolExecutor(max_workers=2) as pool:
    for offset in range(0, total, batch_size):
        delay = start + offset / rate - time.monotonic()
        if delay > 0:
            time.sleep(delay)
        futures.append(pool.submit(send, offset))
    results = [future.result() for future in futures]
    # Include the full offered-load window, even when the last request finishes early.
    time.sleep(max(0, start + duration - time.monotonic()))
elapsed = time.monotonic() - start
latencies = sorted(result[2] for result in results)
summary = {"offered_points_s": rate, "duration_s": duration, "elapsed_s": round(elapsed, 3), "accepted_points": sum(r[0] for r in results), "accepted_points_s": round(total / elapsed, 1), "wire_bytes": sum(r[1] for r in results), "http_p95_ms": round(latencies[math.ceil(len(latencies)*.95)-1]*1000, 2), "host": host}

# Export raw samples: instant queries cannot prove how many points arrived.
selector = f'scale_stream_Load_probe{{host="{host}"}}'
url = "http://victoriametrics:8428/api/v1/export?" + urllib.parse.urlencode({"match[]": selector})
deadline = time.monotonic() + 60
exported = 0
while time.monotonic() < deadline:
    with urllib.request.urlopen(url, timeout=10) as response:
        exported = sum(len(json.loads(line)["values"]) for line in response if line.strip())
    if exported >= total:
        break
    time.sleep(2)
summary["exported_points"] = exported
summary["pass"] = exported == total and total / elapsed >= rate * .95
print(json.dumps(summary))
if not summary["pass"]:
    raise SystemExit(1)
