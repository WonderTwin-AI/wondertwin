# Twin determinism playbook

Every twin is supposed to honor the contract: two runs of the same
seeded scenario produce byte-for-byte identical replay JSONL bundles
modulo timestamps and durations. This is the foundation for the
"twins beat vendor sandboxes in CI" pitch — without it, captured
replay artifacts are noisy and the parity scorecard claim is empty.

twin-stripe is the reference implementation. This playbook is the
recipe for applying the same contract to every other twin.

## Step 1 — Audit randomness sources

For the target twin, grep for every leak:

```bash
grep -rn 'rand\.\|crypto/rand\|uuid\.\|math/rand\|time\.Now()\|hmac\.' \
  twin-<name>/ | grep -v '_test.go'
```

For each hit, classify into one of:

| Kind | Route to |
| --- | --- |
| Entity ID (counter-shaped, e.g. `cus_*`) | `pkgstate.Store.NextID()` (already deterministic) |
| Token-shaped string (e.g. `client_secret`, `whsec_`, promo code) | `MemoryStore.RandHex(n)` backed by `twin.Rand` |
| Random byte buffer (`crypto/rand.Read`) | Replace with `MemoryStore.RandHex` or a domain-specific helper |
| `time.Now()` in handler | `h.store.Clock.Now()` (returns the simulated clock) |
| `time.Now().Unix()` in object construction | `h.store.Now()` (added as a method) |
| Webhook signing timestamp | inject `ClockSource` into the signer; pass `memStore.Clock` |
| Latency / fault injection RNG | already `twin.Rand`-derived via twincore middleware |

Document the audit in the PR body as a `file:line | kind | source |
routing` table. Don't proceed until every leak has a planned route.

## Step 2 — Refactor

Order the changes so each commit builds:

1. Add `MemoryStore.Now()` method backed by `s.Clock`. Replace
   `store.Now()` callers with `h.store.Now()` via sed.
2. Add `MemoryStore.Rand *sim.Rand` and `MemoryStore.RandHex(n)`.
3. Convert package-private `randomHex` (or equivalent) into a
   method on `*Handler` that delegates to `h.store.RandHex`.
4. Drop any `crypto/rand` import.
5. Inject `memStore.Clock` into webhook signers and other utilities
   that take their own timestamp.
6. In `cmd/twin-<name>/main.go`, set `memStore.Rand = twin.Rand`
   immediately after constructing both. Test setups must do the
   same.

## Step 3 — Pin the clock at run start

This is twinkit-side and should be free for free per twin once 2A-1
landed: `/admin/runs/start` with a non-zero `seed` calls
`h.clock.Pin(epoch + seed*time.Second)`. Handlers reading
`h.store.Clock.Now()` see the same wall clock value across runs.

If a twin uses its own clock (rare), confirm the admin handler is
configured with that same `*pkgstate.Clock` instance — `admin.NewHandler`
takes the clock as its third argument.

## Step 4 — Determinism contract test

Add `twin-<name>/internal/api/determinism_test.go`. Write a scenario
that:

- Hits at least one endpoint per major resource type the twin
  manages.
- Includes one error path (e.g. `GET` on a missing resource).
- Exercises each handler that previously generated a token-shaped
  string.

Wire the scenario through `testutil.AssertDeterministic`:

```go
func TestDeterminism_X(t *testing.T) {
    scenario := []testutil.Request{ /* ... */ }
    testutil.AssertDeterministic(t, newDeterministicX(t), 42, scenario)
}
```

`newDeterministicX` is a factory that builds a fresh twin per
invocation, wires `memStore.Rand = twin.Rand`, mounts handlers and
the admin handler, and returns `(*TwinClient, *AdminClient, cleanup)`.

The test runs the scenario twice; the helper canonicalizes the
captured bundles and asserts byte-for-byte equality. On failure it
logs both bundles and points at the first divergent line so the
twin author can locate the leak quickly.

## Canonicalization rules (shared with all twins)

`testutil.AssertDeterministic` strips:

- `timestamp`, `duration_ns`, `sim_time` per entry
- `started_at`, `finished_at`, `run_id` from manifests
- `X-Request-Id` headers (chi assigns these per request)
- Any entry whose `path` starts with `/admin/` (control-plane only)

Map keys are emitted sorted before comparison so map-iteration noise
can't produce spurious diffs.

## Webhook signing trade-off

Twin webhook secrets in CI mode are deterministic per seed. This is
a feature, not a bug: it lets test code assert against fixed
signatures without recapturing fixtures every run. Twins are
simulators, not security boundaries — do not treat their secret
material as cryptographically sensitive.

## Verifying

Before opening the PR:

- `go test -count=5 ./twin-<name>/internal/api/ -run TestDeterminism_<X>`
  must pass five consecutive runs without intermittent failures.
- Manual smoke: two `wt runs start --seed 42` invocations produce
  JSONL bundles whose canonicalized diff is empty.
- Existing `*_test.go` files must continue to pass (test setups may
  need `memStore.Rand = twin.Rand`).
