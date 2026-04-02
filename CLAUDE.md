# CLAUDE.md — wondertwin

## Twin Coverage Policy

The goal for all twins is **100% API parity** with the real service. Every endpoint, every parameter, every error shape.

**You may never make scope decisions.** Do not decide what is "niche", "low priority", or "not worth implementing". If the real API supports it, the twin must support it.

If you believe there is a legitimate reason to consider cutting or deferring scope, you must surface it using the template below. The decision is never yours — bring it to me.

### Scope Exception Request Template

When you encounter a potential reason to not cover some part of an API surface, present it like this:

```
## Scope Exception Request: [twin-name] — [feature/endpoint]

**What:** [Exact endpoint or behavior from the real API]

**Why consider excluding:**
- [Technical blocker, dependency, or constraint]

**Impact of not covering:**
- [Who uses this? What SDK paths hit it? What tests would be uncoverable?]

**Effort to cover:**
- [Rough size: trivial / small / medium / large]
- [Any new twinkit infrastructure required?]

**Recommendation:** [Cover now / Defer until X / Needs discussion because Y]
```

Do not skip this process. Do not silently omit endpoints. If you're unsure whether something is in the real API's surface, research it.

## Twin Development Checklist

Every new twin must include all of the following before pushing. Run these checks locally — CI will fail if any are missing.

### Required files (3 schema files + manifest)
- `twin-{name}/twin.json` — name, description, category, sdk, default_port
- `twin-{name}/twin-manifest.json` — full coverage manifest
- `twin-{name}/provenance.json` — build provenance and source metadata

### Pre-push validation
1. **Schema validation**: `go run ./cmd/validate-schemas/` — catches missing files, invalid enum values, structural errors
2. **Tests pass**: `go test ./twin-{name}/...`
3. **Full suite clean**: `go test ./... -short` — ensure no regressions in other twins
4. **Binary compiles**: `go build -o /dev/null ./twin-{name}/cmd/twin-{name}/`

### Common pitfalls
- **`auth_pattern` enum**: must be one of `api_key`, `oauth2`, `basic`, `jwt`, `custom`, `none`. Do not invent values.
- **Port collisions**: grep existing `cfg.Port` values before assigning a new one.
- **Nil slices in Go JSON**: `json.Marshal(nil slice)` produces `null`, not `[]`. Tests must handle both when asserting on empty lists.

### Standard structure
```
twin-{name}/
  cmd/twin-{name}/main.go          # entry point
  internal/api/router.go            # routes + auth middleware
  internal/api/handlers_{area}.go   # one file per resource area
  internal/api/handlers_test.go     # tests
  internal/store/types.go           # domain types
  internal/store/memory.go          # in-memory store + snapshot/load/reset
  twin.json                         # twin metadata
  twin-manifest.json                # coverage manifest
  provenance.json                   # build provenance
```

## Twin Build Process

This is the proven process for building a new twin from scratch. Follow it in order.

### Phase 0: Research
Before writing any code, research the full API surface of the target service. Use a dedicated research agent that returns a structured inventory of:
- Every endpoint (method + path)
- Authentication model
- Request/response formats and standard envelopes
- Error response shapes and codes
- Webhooks (events, signing, delivery format)
- Rate limits
- SDK compatibility targets

Save the research output. It is the source of truth for 100% parity.

### Phase 1: Scaffold + Core
1. Create directory structure following the standard layout
2. Write store types for all known resource areas (even ones you won't implement until later phases)
3. Write the memory store with snapshot/load/reset
4. Write the router with auth middleware and response helpers matching the service's style
5. Implement the highest-traffic endpoints first (the ones every integration test hits)
6. Include all 3 schema files (`twin.json`, `twin-manifest.json`, `provenance.json`) from commit 1
7. Write tests, run the pre-push checklist, PR

### Phase 2+: Expand to 100%
1. Work through remaining resource areas in logical batches
2. One PR per logical batch (not one PR per endpoint)
3. After each phase, update the manifest with current coverage %

### Final Phase: Gap Audit
Before declaring a twin complete:
1. Run a gap audit — cross-reference every endpoint from the Phase 0 research against the router
2. Implement every missing endpoint, no matter how niche
3. Update manifest to `estimated_coverage_pct: 100` with empty `resources_not_implemented`
4. Only then is the twin done

### Process Retros
When anything goes wrong (CI failure, missing file, wrong enum, broken test), immediately:
1. Identify the root cause
2. Determine what process change would have prevented it
3. Codify that change in this file or in memory
4. Do not just fix the symptom and move on
