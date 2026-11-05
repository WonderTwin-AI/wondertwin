#!/usr/bin/env bash
# SDK smoke suite for the Resend app emulator.
#
# Builds twin-resend, starts it the way lstk launches an app emulator
# (--port=N, ready when GET /admin/health returns {"status":"ok"}), and runs
# the same cases through both official SDKs that pass a recipient through as
# given:
#
#   python/  resend-python 2.48.0, pinned by uv.lock
#   node/    resend-node 6.30.0, pinned by package-lock.json
#
# Test-only: nothing here is part of the Go build, and the Node client is
# installed into a scratch copy, never into the tree.
#
#   twin-resend/sdk-smoke/run.sh             # build and run against a fresh binary
#   BINARY=/path/to/twin-resend run.sh       # run against an existing binary
#   PORT=4113 run.sh                         # choose the port (default 4113)
#
# Needs go, uv and node (with npm). Prints a markdown pass/fail table and exits
# non-zero if any case fails.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
port="${PORT:-4113}"
base="http://localhost:$port"
work="$(mktemp -d)"
trap 'kill "${pid:-0}" 2>/dev/null || true; rm -rf "$work"' EXIT

if curl -s --max-time 2 "$base/admin/health" >/dev/null 2>&1; then
  echo "port $port is already in use" >&2
  exit 1
fi

binary="${BINARY:-}"
if [ -z "$binary" ]; then
  binary="$work/twin-resend"
  (cd "$repo" && go build -o "$binary" ./twin-resend/cmd/twin-resend)
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
(cd "$here/python" && RESEND_EMULATOR_URL="$base" uv run --quiet --frozen smoke.py >>"$results") 2>"$work/python.log" || status=1

mkdir -p "$work/node"
cp "$here/node/package.json" "$here/node/package-lock.json" "$here/node/smoke.mjs" "$work/node/"
(cd "$work/node" && npm ci --no-audit --no-fund --loglevel=error >/dev/null && RESEND_EMULATOR_URL="$base" node smoke.mjs >>"$results") 2>"$work/node.log" || status=1

python3 - "$results" <<'PY' || status=1
import json, sys
sdks = ["resend-python 2.48.0", "resend-node 6.30.0"]
rows = {}
for line in open(sys.argv[1]):
    if line.strip():
        r = json.loads(line)
        rows.setdefault(r["case"], {})[r["sdk"]] = "pass" if r["pass"] else "FAIL: " + r["detail"].replace("|", "/")
print("| Case | " + " | ".join(sdks) + " |")
print("|---|" + "---|" * len(sdks))
for case, got in rows.items():
    print(f"| {case} | " + " | ".join(got.get(s, "not run") for s in sdks) + " |")
cells = [got.get(s, "not run") for got in rows.values() for s in sdks]
passed = sum(c == "pass" for c in cells)
print(f"\n{passed}/{len(cells)} passed")
sys.exit(0 if cells and passed == len(cells) else 1)
PY

if [ "$status" -ne 0 ]; then
  echo "--- resend-python log"; tail -n 40 "$work/python.log"
  echo "--- resend-node log"; tail -n 40 "$work/node.log"
fi
exit "$status"
