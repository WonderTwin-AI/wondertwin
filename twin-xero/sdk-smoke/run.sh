#!/usr/bin/env bash
# SDK smoke suite for the Xero app emulator.
#
# Builds twin-xero, starts it the way lstk launches an app emulator
# (--port=N, ready when GET /admin/health returns {"status":"ok"}), and runs the
# supported flows through the official xero-python SDK pinned in
# python/uv.lock. Test-only: nothing here is part of the Go build.
#
#   twin-xero/sdk-smoke/run.sh             # build and run against a fresh binary
#   BINARY=/path/to/twin-xero run.sh       # run against an existing binary
#   PORT=4195 run.sh                       # choose the port (default 4195)
#
# Needs go and uv. Prints a markdown pass/fail table and exits non-zero if any
# case fails.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
port="${PORT:-4195}"
base="http://localhost:$port"
work="$(mktemp -d)"
trap 'kill "${pid:-0}" 2>/dev/null || true; rm -rf "$work"' EXIT

if curl -s --max-time 2 "$base/admin/health" >/dev/null 2>&1; then
  echo "port $port is already in use" >&2
  exit 1
fi

binary="${BINARY:-}"
if [ -z "$binary" ]; then
  binary="$work/twin-xero"
  (cd "$repo" && go build -o "$binary" ./twin-xero/cmd/twin-xero)
fi

"$binary" --port="$port" >"$work/emulator.log" 2>&1 &
pid=$!
for _ in $(seq 1 50); do
  if [ "$(curl -s "$base/admin/health" || true)" = '{"status":"ok"}' ]; then break; fi
  sleep 0.1
done
if [ "$(curl -s "$base/admin/health" || true)" != '{"status":"ok"}' ]; then
  echo "app emulator did not become healthy on $base" >&2
  cat "$work/emulator.log" >&2
  exit 1
fi

results="$work/results.jsonl"
status=0
(cd "$here/python" && XERO_EMULATOR_URL="$base" uv run --quiet --frozen smoke.py >"$results") || status=$?

python3 - "$results" <<'PY'
import json, sys
rows = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
print("| Case | " + (rows[0]["sdk"] if rows else "xero-python") + " |")
print("|---|---|")
for r in rows:
    print(f"| {r['case']} | " + ("pass" if r["pass"] else "FAIL: " + r["detail"].replace("|", "/")) + " |")
failed = [r for r in rows if not r["pass"]]
print(f"\n{len(rows) - len(failed)}/{len(rows)} passed")
sys.exit(1 if failed or not rows else 0)
PY
