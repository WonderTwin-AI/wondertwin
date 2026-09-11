# CLAUDE.md — wondertwin

## Repository Scope

This is the **public, MIT-licensed** WonderTwin repository. It contains:
- **twinkit** — the shared library for building API twins
- **Community twins** — 10 open-source twins
- **wt CLI** — the command-line tool for managing twins

Pro twins are available separately for commercial customers. Code contributions,
including the skill-driven build process below, are paused while the architecture
is reworked. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Product docs live in Notion

As of 2026-08-20, product and strategy documentation is canonical in Notion, not in `wondertwin-docs` or any repo. Do not write product strategy, positioning, roadmap, pricing, GTM or planning docs into this repository. Code-level docs, READMEs, and ADRs about this code stay here. Vendor research artifacts stay in `wondertwin-docs/research/`, because the pipeline reads them as files. See `wondertwin-docs/AGENTS.md` for the full split.

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

Every new community twin must include all of the following before pushing.

### Required files (3 schema files + manifest)
- `twin-{name}/twin.json` — name, description, category, sdk, default_port
- `twin-{name}/twin-manifest.json` — full coverage manifest
- `twin-{name}/provenance.json` — build provenance and source metadata

### Pre-push validation
1. **Schema validation**: `go run ./cmd/validate-schemas/` — catches missing files, invalid enum values, structural errors
2. **Tests pass**: `go test ./twin-{name}/...`
3. **Full suite clean**: `go test ./... -short` — ensure no regressions in other twins
4. **Binary compiles**: `go build -o /dev/null ./twin-{name}/cmd/twin-{name}/`

### Community twins in this repo
stripe, posthog, resend, twilio, logodev, slack, github, shopify, linear, hubspot

### Common pitfalls
- **`auth_pattern` enum**: must be one of `api_key`, `oauth2`, `basic`, `jwt`, `custom`, `none`. Do not invent values.
- **Port collisions**: grep existing `cfg.Port` values before assigning a new one.
- **Nil slices in Go JSON**: `json.Marshal(nil slice)` produces `null`, not `[]`. Tests must handle both when asserting on empty lists.
- **This repo contains community twins only.** Do not add dependencies on external private packages.

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

This is the proven process for building a new community twin from scratch. Follow it in order.

### Phase 0: Research
Before writing any code, research the full API surface of the target service. The
`twin-researcher` skill that drives this step has moved to a private library and
is no longer in this repo. Research artifacts still land in
`wondertwin-docs/research/` and remain the source of truth for 100% parity.

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

## Fleet-shaped changes record their governing skill

A change touching two or more `twin-*` directories, or anything under
`twinkit/` or `schemas/`, is fleet-shaped: it reaches emulators beyond the one
in front of you. CI requires a skill citation naming the skill that governed
it.

```
Skill: fleet-wide-hygiene-sweep
```

If no skill applies, record that decision rather than omitting the citation:

```
Skill: none (single-vendor SDK bump, no cross-emulator pattern)
```

This exists because a four-emulator sweep once shipped while the sweep and
synthesis skills sat unused, and the omission left no trace to review. The
requirement is that the choice is auditable, not that it is approved, so a
reasoned `none` passes. Run it locally with
`scripts/check-governance origin/main HEAD`.

## The public kit carries primitives, not domain engines

`twinkit/` is an allowlist. A store, a clock, seeded randomness and transport
shims are primitives and belong here. Double-entry, period close and approval
graphs are semantics and ship in the commercial kit: if the caller can be
wrong about it, it is not a primitive.

CI enforces this by presence, not by imports, because the ledger engine sat on
the public default branch for months with no consumer to give it away
(CTO-46). Any Go package directory under `twinkit/` that is not on the
allowlist fails, whether or not anything uses it. Adding a genuinely new
primitive means adding its path to `ALLOWED_PACKAGES` in the script and saying
why in the commit. Run it locally with `scripts/check-engine-boundary`.

It reads the git index, not the worktree. An engine you have copied in but
not yet `git add`ed is invisible here, so a local run can print green on a
tree CI will reject. Stage it first, or CI will see what your local run did
not.

The gate is a path rule, so it is package-granular, not file-granular. It
catches a new package directory, nested or top-level, which is the shape an
engine has and the shape the ledger arrived in. A single `.go` file added to a
package that is already allowlisted is indistinguishable from that package's
own code to any path rule. That case, and only that case, is left to code
review.

## Concurrent Agent Work Uses Git Worktrees

When more than one agent session needs to work in this repo at the same time, each session runs in its own git worktree rather than sharing this checkout. Two sessions sharing one working tree will clobber each other the moment either one switches branches or rebases, since both operations rewrite the same working directory and index the other session may be mid-edit in.

Worktrees for this purpose live under `.claude/worktrees/` (already ignored in `.gitignore`) and branch from `origin/main` by default, not from whatever branch happens to be checked out in the primary working tree.
