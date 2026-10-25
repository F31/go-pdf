#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

log=$(mktemp)
trap 'rm -f "$log"' EXIT
PDF_SLO=1 go test . -run '^TestServerCoreSLO$' -v -count=1 | tee "$log"

python3 - "$log" "${PERF_OPEN_P95_MS:-0.700}" "${PERF_EXTRACT_P95_MS:-3.380}" "${PERF_PREFLIGHT_P95_MS:-1.620}" <<'PY'
import re, sys
from pathlib import Path

text = Path(sys.argv[1]).read_text()
limits = {
    "open50": float(sys.argv[2]),
    "extract50": float(sys.argv[3]),
    "preflight50": float(sys.argv[4]),
}

def ms(value):
    m = re.fullmatch(r"([0-9.]+)(ns|µs|ms|s)", value)
    if not m:
        raise SystemExit(f"cannot parse duration {value!r}")
    n, unit = float(m.group(1)), m.group(2)
    return {"ns": n / 1e6, "µs": n / 1e3, "ms": n, "s": n * 1e3}[unit]

failed = False
for key, limit in limits.items():
    m = re.search(rf"{key} .*?p95=([^ ]+)", text)
    if not m:
        print(f"{key}: missing p95", file=sys.stderr)
        failed = True
        continue
    got = ms(m.group(1))
    status = "ok" if got <= limit else "FAIL"
    print(f"{key}: p95={got:.3f}ms limit={limit:.3f}ms {status}")
    if got > limit:
        failed = True
if failed:
    sys.exit(1)
PY
