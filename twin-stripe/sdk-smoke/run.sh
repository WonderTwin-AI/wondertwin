#!/usr/bin/env bash
# SDK smoke suite for the Stripe app emulator.
#
# Builds twin-stripe, starts it the way lstk launches an app emulator
# (--port=N, ready when GET /admin/health returns {"status":"ok"}), and runs
# every MVP use case plus the error cases through the official SDKs pinned in
# node/package-lock.json (stripe-node) and python/uv.lock (stripe-python).
# Test-only: nothing here is part of the Go build.
#
#   twin-stripe/sdk-smoke/run.sh            # build and run against a fresh binary
#   BINARY=/path/to/twin-stripe run.sh      # run against an existing binary
#   PORT=4199 run.sh                        # choose the port (default 4199)
#
# Needs go, node (>=20) with npm, and uv. Prints a markdown pass/fail table
# and exits non-zero if any case fails.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
port="${PORT:-4199}"
base="http://localhost:$port"
work="$(mktemp -d)"
trap 'kill "${pid:-0}" 2>/dev/null || true; rm -rf "$work"' EXIT

binary="${BINARY:-}"
if [ -z "$binary" ]; then
  binary="$work/twin-stripe"
  (cd "$repo" && go build -o "$binary" ./twin-stripe/cmd/twin-stripe)
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
: >"$results"

(cd "$here/node" && { [ -d node_modules ] || npm ci --no-audit --no-fund >/dev/null; } &&
  STRIPE_EMULATOR_URL="$base" node smoke.mjs >>"$results") || true
curl -s -X POST "$base/admin/reset" >/dev/null
(cd "$here/python" && STRIPE_EMULATOR_URL="$base" uv run --quiet --frozen smoke.py >>"$results") || true

python3 - "$results" <<'PY'
import json, sys
rows = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
sdks = list(dict.fromkeys(r["sdk"] for r in rows))
cases = list(dict.fromkeys(r["case"] for r in rows))
by = {(r["sdk"], r["case"]): r for r in rows}
print("| Case | " + " | ".join(sdks) + " |")
print("|---|" + "---|" * len(sdks))
for c in cases:
    cells = []
    for s in sdks:
        r = by.get((s, c))
        cells.append("missing" if r is None else ("pass" if r["pass"] else "FAIL: " + r["detail"].replace("|", "/")))
    print(f"| {c} | " + " | ".join(cells) + " |")
failed = [r for r in rows if not r["pass"]]
expected = len(sdks) * len(cases)
print(f"\n{len(rows) - len(failed)}/{expected} passed")
sys.exit(1 if failed or len(rows) != expected or len(sdks) < 2 else 0)
PY
