#!/usr/bin/env bash
# SDK smoke suite for the PostHog app emulator.
#
# Builds twin-posthog, starts it the way lstk launches an app emulator
# (--port=N, ready when GET /admin/health returns {"status":"ok"}), and runs
# the same cases through the official SDKs:
#
#   go/      posthog-go v1.44.1, pinned by posthog-go.mod and posthog-go.sum
#   python/  posthog-python 7.91.1, pinned by uv.lock
#
# Test-only: nothing here is part of the Go build (the Go client builds only
# under the sdksmoke tag, in a copy outside the module), and every SDK client
# is installed into scratch space, never into the tree. A case an SDK cannot
# express reports n/a.
#
#   twin-posthog/sdk-smoke/run.sh             # build and run against a fresh binary
#   BINARY=/path/to/twin-posthog run.sh       # run against an existing binary
#   PORT=4114 run.sh                          # choose the port (default 4114)
#
# Needs go and uv. Prints a markdown pass/fail table and exits non-zero if any
# case fails.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
port="${PORT:-4114}"
base="http://localhost:$port"
work="$(mktemp -d)"
trap 'kill "${pid:-0}" 2>/dev/null || true; rm -rf "$work"' EXIT

if curl -s --max-time 2 "$base/admin/health" >/dev/null 2>&1; then
  echo "port $port is already in use" >&2
  exit 1
fi

binary="${BINARY:-}"
if [ -z "$binary" ]; then
  binary="$work/twin-posthog"
  (cd "$repo" && go build -o "$binary" ./twin-posthog/cmd/twin-posthog)
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
(cd "$here/python" && UV_PROJECT_ENVIRONMENT="$work/venv" POSTHOG_EMULATOR_URL="$base" uv run --quiet --frozen smoke.py >>"$results") 2>"$work/python.log" || status=1

mkdir -p "$work/go"
cp "$here/go/smoke.go" "$work/go/"
cp "$here/go/posthog-go.mod" "$work/go/go.mod"
cp "$here/go/posthog-go.sum" "$work/go/go.sum"
(cd "$work/go" && POSTHOG_EMULATOR_URL="$base" go run -tags sdksmoke . >>"$results") 2>"$work/go.log" || status=1

python3 - "$results" <<'PY' || status=1
import json, sys
sdks = ["posthog-go v1.44.1", "posthog-python 7.91.1"]
rows = {}
for line in open(sys.argv[1]):
    if line.strip():
        r = json.loads(line)
        cell = "n/a" if r["pass"] is None else "pass" if r["pass"] else "FAIL: " + r["detail"].replace("|", "/")
        rows.setdefault(r["case"], {})[r["sdk"]] = cell
print("| Case | " + " | ".join(sdks) + " |")
print("|---|" + "---|" * len(sdks))
for case, got in rows.items():
    print(f"| {case} | " + " | ".join(got.get(s, "not run") for s in sdks) + " |")
cells = [got.get(s, "not run") for got in rows.values() for s in sdks]
cells = [c for c in cells if c != "n/a"]
passed = sum(c == "pass" for c in cells)
print(f"\n{passed}/{len(cells)} passed")
sys.exit(0 if cells and passed == len(cells) else 1)
PY

if [ "$status" -ne 0 ]; then
  echo "--- posthog-go log"; tail -n 40 "$work/go.log"
  echo "--- posthog-python log"; tail -n 40 "$work/python.log"
fi
exit "$status"
