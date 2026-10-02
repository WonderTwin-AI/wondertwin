#!/usr/bin/env bash
# SDK smoke suite for the Slack app emulator.
#
# Builds twin-slack, starts it the way lstk launches an app emulator
# (--port=N, ready when GET /admin/health returns {"status":"ok"}), and runs
# every covered MVP use case and error case through the official SDKs pinned in
# node/package-lock.json (@slack/web-api) and python/uv.lock (slack-sdk).
# Test-only: nothing here is part of the Go build.
#
#   twin-slack/sdk-smoke/run.sh             # build and run against a fresh binary
#   BINARY=/path/to/twin-slack run.sh       # run against an existing binary
#   PORT=4197 run.sh                        # choose the port (default 4197)
#
# Needs go, node (>=20) with npm, and uv. Prints a markdown pass/fail table
# and exits non-zero if any case fails.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
port="${PORT:-4197}"
base="http://localhost:$port"
work="$(mktemp -d)"
trap 'kill "${pid:-0}" 2>/dev/null || true; rm -rf "$work"' EXIT

binary="${BINARY:-}"
if [ -z "$binary" ]; then
  binary="$work/twin-slack"
  (cd "$repo" && go build -o "$binary" ./twin-slack/cmd/twin-slack)
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
  SLACK_EMULATOR_URL="$base" node smoke.mjs >>"$results") || true
curl -s -X POST "$base/admin/reset" >/dev/null
(cd "$here/python" && SLACK_EMULATOR_URL="$base" uv run --quiet --frozen smoke.py >>"$results") || true

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
