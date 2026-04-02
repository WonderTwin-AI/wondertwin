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
