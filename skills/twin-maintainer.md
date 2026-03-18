---
skill: twin-maintainer
skill_version: "1.0"
schemas: {}
---

# SKILL: WonderTwin Twin Maintainer

## Purpose

Keep existing WonderTwin twins healthy, current, and correctly integrated with CI/CD infrastructure. This skill covers routine maintenance tasks that are distinct from adding new capabilities (see `twin-extender`) or building from scratch (see `twin-generator`).

Maintenance does not add new endpoints, types, or stores. It keeps existing ones working correctly: updating dependencies, responding to upstream changes, fixing CI, and auditing coverage accuracy.

## When to Use

- A dependency needs updating (`twinkit`, `chi`, Go version)
- The target SDK released a new version and you need to assess impact
- CI is failing due to infrastructure issues (stale twin list, workflow changes)
- A twin is being added to or removed from the monorepo
- `wt conformance` checks have changed and twins need updating
- Periodic audit of twin health and coverage accuracy

---

## Tasks

### Task 1: CI Twin List Maintenance

The CI workflow at `.github/workflows/ci.yml` has a hardcoded list of twins in the "Build all twins" step. This list must match the actual twins in the repo.

**When to perform:** Every time a twin is added or removed.

**How to check:**

```bash
# What CI builds:
grep 'for twin in' .github/workflows/ci.yml

# What actually exists:
ls -d twin-*/cmd/twin-* | sed 's|twin-\(.*\)/cmd/twin-.*|\1|' | sort | tr '\n' ' '
```

If these don't match, update the CI file:

```yaml
# .github/workflows/ci.yml — "Build all twins" step
for twin in stripe twilio resend posthog logodev loyaltylion smile; do
```

**Common failure mode:** `stat twin-{name}/cmd/twin-{name}: directory not found` in CI. This means a twin was removed from the repo but not from the CI list (or vice versa for additions).

---

### Task 2: Dependency Updates

#### Updating twinkit

When a new version of `twinkit` is released:

```bash
cd twin-{name}
go get github.com/wondertwin-ai/wondertwin/twinkit@latest
go mod tidy
```

**Verification steps:**

1. `go build ./...` — compiles with new twinkit
2. `go test ./...` — all tests pass
3. Check for API changes — if twinkit changed an interface (e.g., `admin.StateStore`, `admin.NewHandler` signature), update the twin's implementation

**Breaking change indicators:**

- Compilation errors after updating
- New required methods on interfaces the twin implements
- Changed function signatures in `twincore`, `admin`, or `store` packages

#### Updating Go version

When the project bumps its Go version:

1. Update `go.mod` directive
2. Run `go mod tidy`
3. Run `go vet ./...` — new Go versions may add vet checks
4. Run tests

#### Updating chi

```bash
go get github.com/go-chi/chi/v5@latest
go mod tidy
```

Chi major version changes are rare. Minor updates are typically safe.

---

### Task 3: SDK Drift Detection

When the target SDK releases a new version, assess whether the twin needs changes.

**Assessment steps:**

1. **Check the SDK changelog/release notes** for breaking changes, new methods, or deprecated endpoints
2. **Compare SDK method signatures** — have request/response types changed?
3. **Check for new operations** — new SDK methods that the twin doesn't support (this crosses into extender territory if changes are needed)
4. **Check for removed operations** — SDK methods that no longer exist

**What to update if drift is found:**

- `twin-manifest.json` — update `sdk_target.primary.version`
- `provenance.json` — increment `build`, update timestamp
- If response shapes changed, update store types and handler responses
- If endpoints changed, update router and handlers

**What NOT to do:**

- Don't update the SDK version in the manifest if you haven't verified compatibility
- Don't add new capabilities — that's the extender skill's job. Only fix what's broken.

---

### Task 4: Conformance Updates

When `twinkit` adds or changes conformance checks (`wt conformance`), existing twins may need updates.

**How to check:**

```bash
go build -o ./wt-test ./cmd/wt/
go build -o ./twin-test ./twin-{name}/cmd/twin-{name}/
./wt-test conformance ./twin-test
```

**Common conformance failures after twinkit updates:**

- New required admin endpoints (e.g., `/admin/config`, `/admin/quirks`)
- Changed response shapes for existing admin endpoints
- New `StateStore` interface methods

**Fix pattern:**

1. Read the conformance check error message
2. Check the twinkit source for the new requirement
3. Update the twin's admin handler setup or store implementation
4. Re-run conformance

---

### Task 5: Coverage Audit

Periodically verify that the twin's declared coverage matches reality.

**Check manifest accuracy:**

```bash
# What the manifest claims:
jq '.coverage' twin-{name}/twin-manifest.json

# What's actually implemented — count routes:
grep -c 'r\.\(Get\|Post\|Put\|Patch\|Delete\)(' twin-{name}/internal/api/router.go

# What's actually tested:
grep -c 'func Test' twin-{name}/internal/api/handlers_test.go
```

**What to verify:**

- `resources_implemented` — are all listed resources actually working? Run the tests.
- `resources_not_implemented` — have any been implemented since the last update? Check for new handler files.
- `estimated_coverage_pct` — does it reflect the actual ratio of implemented vs. total SDK operations?
- `service_surface.resource_count` — does it match the sum of implemented + not implemented?

**Update the manifest if inaccurate.** Increment `provenance.json` build number when making changes.

---

### Task 6: Twin Removal

When removing a twin from the monorepo:

1. **Delete the twin directory:** `rm -rf twin-{name}/`
2. **Update CI twin list:** remove from `.github/workflows/ci.yml`
3. **Update `go.work`:** remove the twin's module path if listed
4. **Check for cross-references:** grep the codebase for references to the twin
5. **Registry note:** the twin remains in the registry — old versions are still installable. No registry update needed for removal.

**Do NOT:**

- Delete the twin's releases from the registry
- Remove the twin's entry from `registry.json`
- Force-push to remove the twin from git history

---

### Task 7: Twin Addition (Monorepo Integration)

When a new twin is generated (via `twin-generator`) and needs to be integrated into the monorepo:

1. **Add to `go.work`:** include the twin's module path
2. **Update CI twin list:** add to `.github/workflows/ci.yml`
3. **Verify builds:** `go build ./twin-{name}/...`
4. **Run tests:** `go test ./twin-{name}/...`
5. **Run conformance:** `wt conformance ./bin/twin-{name}`

---

### Task 8: Provenance Hygiene

When any maintenance change is made to a twin:

- Increment `build` number in `provenance.json`
- Update `generated_at` timestamp
- Do NOT change `sources` unless the actual generation sources changed

The build number tracks how many times the twin has been modified, not just generated. Each maintenance pass that changes twin behavior should bump it.

---

## Maintenance Schedule

Not all tasks need to happen on every change. Suggested cadence:

| Task | When |
|---|---|
| CI twin list | Every twin add/remove |
| Dependency updates | When twinkit releases, or quarterly |
| SDK drift detection | When target SDK releases a new major/minor version |
| Conformance updates | After twinkit updates |
| Coverage audit | Before each twin release |
| Provenance hygiene | Every maintenance change |

## Checklist

Quick verification that a twin is healthy:

- [ ] `go build ./twin-{name}/...` succeeds
- [ ] `go test ./twin-{name}/...` passes
- [ ] `go vet ./twin-{name}/...` clean
- [ ] Twin is listed in `.github/workflows/ci.yml`
- [ ] `twin-manifest.json` coverage matches reality
- [ ] `provenance.json` build number is current
- [ ] Target SDK version in manifest matches what the twin was tested against
- [ ] `wt conformance` passes (if conformance binary is available)
