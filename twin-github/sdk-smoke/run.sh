#!/usr/bin/env bash
# SDK smoke suite for the GitHub app emulator.
#
# Runs every MVP use case, plus the bad-token, missing-resource and
# unsupported-version errors, through two clients against a running binary:
#
#   octokit/   Octokit.js (official): octokit 5.0.5 with
#              @octokit/plugin-rest-endpoint-methods 17.0.0, pinned by
#              package-lock.json
#   gogithub/  google/go-github v92.0.0 (third party), pinned by
#              go-github.mod and go-github.sum
#
# The suite is test-only and outside the repository's Go module: both
# clients are installed into a scratch directory, never into the tree. The
# Go pins are not named go.mod because no go.mod may live under an emulator
# directory (scripts/check-twinkit-resolution).
#
# Usage:
#   twin-github/sdk-smoke/run.sh
#       builds twin-github, starts it with --port=N and tears it down
#   GITHUB_EMULATOR_URL=http://github.localhost.localstack.cloud:4566 twin-github/sdk-smoke/run.sh
#       runs against an emulator that is already up (for example through lstk)
#
# Needs go, node (20 or later) and npm. Prints a pass/fail table and exits
# non-zero if any case fails.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/../.." && pwd)"
work="$(mktemp -d "${TMPDIR:-/tmp}/github-sdk-smoke.XXXXXX")"
pid=""
cleanup() {
  if [ -n "$pid" ]; then kill "$pid" 2>/dev/null || true; fi
  rm -rf "$work"
}
trap cleanup EXIT

if [ -z "${GITHUB_EMULATOR_URL:-}" ]; then
  port="${PORT:-$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')}"
  (cd "$root" && go build -o "$work/twin-github" ./twin-github/cmd/twin-github/)
  "$work/twin-github" --port="$port" >"$work/emulator.log" 2>&1 &
  pid=$!
  export GITHUB_EMULATOR_URL="http://127.0.0.1:$port"
fi

for _ in $(seq 1 50); do
  if [ "$(curl -fsS "$GITHUB_EMULATOR_URL/admin/health" 2>/dev/null | tr -d '\n')" = '{"status":"ok"}' ]; then
    break
  fi
  sleep 0.2
done
echo "emulator: $GITHUB_EMULATOR_URL"

export SMOKE_RESULTS="$work/results.tsv"
: >"$SMOKE_RESULTS"
status=0

mkdir -p "$work/octokit"
cp "$here/octokit/package.json" "$here/octokit/package-lock.json" "$here/octokit/smoke.test.mjs" "$work/octokit/"
(cd "$work/octokit" && npm ci --no-audit --no-fund --loglevel=error >/dev/null && npm test --silent) >"$work/octokit.log" 2>&1 || status=1

mkdir -p "$work/gogithub"
cp "$here/gogithub/smoke_test.go" "$work/gogithub/"
cp "$here/gogithub/go-github.mod" "$work/gogithub/go.mod"
cp "$here/gogithub/go-github.sum" "$work/gogithub/go.sum"
(cd "$work/gogithub" && go test -tags sdksmoke -count=1 ./) >"$work/gogithub.log" 2>&1 || status=1

python3 - "$SMOKE_RESULTS" <<'PY'
import sys
rows, clients = {}, ["octokit", "go-github"]
for line in open(sys.argv[1]):
    client, case, result = line.rstrip("\n").split("\t")
    rows.setdefault(case, {})[client] = result
print("| Case | Octokit.js 17.0.0 | go-github v92.0.0 |")
print("|---|---|---|")
for case, got in rows.items():
    print(f"| {case} | " + " | ".join(got.get(c, "not run") for c in clients) + " |")
PY

if [ "$status" -ne 0 ]; then
  echo
  echo "--- octokit log"; tail -n 60 "$work/octokit.log"
  echo "--- go-github log"; tail -n 60 "$work/gogithub.log"
fi
exit "$status"
