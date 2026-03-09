# SKILL: WonderTwin Twin Extender

## Purpose

Expand the capabilities of an existing WonderTwin twin to cover more of the target SDK's surface area. This skill handles the gap analysis, incremental store expansion, new handler implementation, and coverage updates needed to bring a twin from partial to full SDK coverage.

This skill is for **adding new capabilities** to a working twin — new endpoints, new domain types, new logical operations. For dependency updates, CI fixes, and drift detection, see the `twin-maintainer` skill.

## When to Use

- A twin exists and passes tests but only covers a subset of SDK operations
- The target SDK supports operations the twin doesn't handle
- A user reports that an SDK method fails against the twin
- Coverage needs to increase for a release milestone

## Prerequisites

- An existing twin that builds and passes `go test ./...`
- Access to the target SDK source code or documentation
- Understanding of the twin's current store types and handler structure

## Inputs

The user will provide:

- **Twin name**: Which twin to extend (e.g., "posthog")
- **Target operations** (optional): Specific SDK methods or endpoints to add support for
- **Scope** (optional): "full SDK coverage" or a specific subset

If no specific operations are given, perform a full gap analysis.

---

## Process

### Phase 1: Gap Analysis

Before writing any code, determine exactly what the twin is missing.

**1. Inventory current coverage:**

Read the twin's files to understand what's already implemented:

```
twin-{name}/
├── internal/store/types.go    → What domain types exist
├── internal/store/memory.go   → What stores are initialized
├── internal/api/router.go     → What routes are mounted
├── internal/api/handlers_*.go → What handlers exist
├── twin-manifest.json         → What coverage is declared
```

**2. Analyze the target SDK:**

Enumerate all operations the SDK supports. For Go SDKs, look at:

- Exported methods on the client struct
- Message/request types the SDK can send
- Endpoint constants or URL construction
- Test files showing expected usage patterns

**3. Produce a coverage delta table:**

| SDK Operation | SDK Method | Twin Endpoint | Status |
|---|---|---|---|
| Capture | `Enqueue(Capture{})` | `POST /capture` | DONE |
| Identify | `Enqueue(Identify{})` | `POST /capture` (event=`$identify`) | MISSING |
| ... | ... | ... | ... |

**4. Identify the implementation pattern:**

Determine how the SDK sends each missing operation:

- **Separate endpoint**: Each operation has its own URL (standard REST)
- **Shared endpoint with discriminator**: Multiple operations go through one endpoint, differentiated by a field (see "Single-Endpoint Message Routing" in `twin-generator.md`)
- **Separate endpoint with shared response shape**: Different URLs but the same response format (e.g., `/decide` and `/flags` both return feature flag maps)

### Phase 2: New Store Types

Add types for new domain objects to `internal/store/types.go`.

**Rules:**

- Add to the existing file — don't create a new types file
- Match the real service's field names exactly
- If expanding an existing type (e.g., adding `Payload` to `FeatureFlag`), add the field with `omitempty` to avoid breaking existing seed data

```go
// Add to existing types.go:

// Person represents an identified user with properties.
type Person struct {
    DistinctID string         `json:"distinct_id"`
    Properties map[string]any `json:"properties,omitempty"`
    CreatedAt  string         `json:"created_at"`
}
```

### Phase 3: Expand MemoryStore

Update `internal/store/memory.go` to include new stores.

**Four places to update:**

1. **Struct fields** — add new `*pkgstore.Store[T]` fields
2. **`New()` constructor** — initialize new stores with appropriate ID prefixes
3. **`stateSnapshot` struct** — add new fields for JSON serialization
4. **`Snapshot()`** — include new stores in output
5. **`LoadState()`** — load new stores from snapshot (nil-check each)
6. **`Reset()`** — reset new stores

All six changes must stay in sync. Missing any one will cause state to be lost on reset, not persisted in snapshots, or not loadable from seed data.

### Phase 4: Handle New Operations

How you add handlers depends on the implementation pattern identified in Phase 1.

#### Pattern A: Shared endpoint with side effects

If the new operations route through an existing endpoint (e.g., `$identify` through `/capture`):

1. Modify the existing handler's internal processing method (e.g., `storeEvent()`)
2. Add a switch/case for the new event type
3. Write a processing method for each new operation
4. The raw event/message should still be stored for observability

```go
// In the existing storeEvent method:
switch req.Event {
case "$identify":
    h.processIdentify(req, ts)
case "$create_alias":
    h.processAlias(req, ts)
}
```

#### Pattern B: New endpoints

If the new operations need new routes:

1. Create a new handler file (`handlers_{resource}.go`) if the operations are logically distinct from existing handlers
2. Add to an existing handler file if they're closely related (e.g., `/flags` alongside `/decide`)
3. Add routes in `router.go`

**When to create a new file vs. add to existing:**

- New file: different resource domain (e.g., flags vs. capture)
- Existing file: variation of same resource (e.g., `/decide` and `/flags` both evaluate feature flags)

#### Pattern C: Upsert logic for derived entities

When operations create or update entities that may already exist (e.g., re-identifying a user):

1. Use `store.Filter()` or `store.FilterWithIDs()` to find existing records
2. Merge properties (new values override, existing values preserved)
3. Use the existing store ID for updates, generate new ID for creates

```go
existing := h.store.Persons.Filter(func(_ string, p store.Person) bool {
    return p.DistinctID == req.DistinctID
})
if len(existing) > 0 {
    // Merge and update
    ids, _ := h.store.Persons.FilterWithIDs(...)
    h.store.Persons.Set(ids[0], mergedPerson)
} else {
    // Create new
    id := h.store.Persons.NextID()
    h.store.Persons.Set(id, newPerson)
}
```

### Phase 5: New Admin Endpoints

Every new domain entity should have a corresponding admin endpoint for test observability.

**Pattern:**

```go
// GET /admin/{entities} — list with optional filter query params
func (h *Handler) AdminList{Entities}(w http.ResponseWriter, r *http.Request) {
    filter := r.URL.Query().Get("{filter_field}")
    items := h.store.{Entities}.List()
    if filter != "" {
        var filtered []store.{Entity}
        for _, item := range items {
            if item.{Field} == filter {
                filtered = append(filtered, item)
            }
        }
        items = filtered
    }
    twincore.JSON(w, http.StatusOK, map[string]any{
        "{entities}": items,
        "total":      len(items),
    })
}
```

Mount in `router.go` under the admin group (no auth required):

```go
r.Get("/admin/{entities}", h.AdminList{Entities})
```

### Phase 6: Update Routes

Add new routes to `internal/api/router.go`.

**Rules:**

- Service API routes go inside the fault injection middleware group
- Admin routes go outside it (no auth, no fault injection)
- Include both with and without trailing slash variants for service routes
- Auth requirements may differ per endpoint (e.g., local evaluation uses Bearer token, capture uses project API key)

### Phase 7: Add Tests

Add tests to the existing `handlers_test.go` file.

**Rules:**

- Reuse the existing `setup` function — don't create a new one
- Test the new operations through the SDK-facing endpoints (not just admin)
- Verify side effects via admin endpoints (e.g., send `$identify` via `/capture`, then check `/admin/persons`)
- Test upsert/merge behavior where applicable
- Test auth requirements for new endpoints
- Group new tests with a comment header (e.g., `// --- Identify Tests ---`)

### Phase 8: Update Manifest and Provenance

**`twin-manifest.json`:**

- Add new resources to `coverage.resources_implemented`
- Remove them from `coverage.resources_not_implemented`
- Update `estimated_coverage_pct`
- Update `description` to mention new capabilities
- Update `service_surface.resource_count`

**`provenance.json`:**

- Increment `build` number
- Update `generated_at` timestamp

### Phase 9: Update CI

If the extension changes the twin's build path or adds new twins, update the CI twin list in `.github/workflows/ci.yml`. See "CI Twin List Maintenance" in `twin-generator.md`.

---

## Checklist

Before considering the extension complete:

- [ ] Gap analysis documented — know exactly what was missing and what was added
- [ ] New store types added to `types.go`
- [ ] `MemoryStore` expanded — struct, `New()`, `stateSnapshot`, `Snapshot()`, `LoadState()`, `Reset()` all updated
- [ ] New operations handled — either via shared endpoint routing or new endpoints
- [ ] Admin endpoints added for each new domain entity
- [ ] Routes mounted in `router.go`
- [ ] Tests added for all new operations
- [ ] Tests verify side effects via admin endpoints
- [ ] `twin-manifest.json` updated — coverage, description, resource count
- [ ] `provenance.json` updated — build number incremented
- [ ] `go build ./...` succeeds
- [ ] `go test ./...` passes
- [ ] Existing tests still pass (no regressions)

## Common Mistakes

1. **Updating `Snapshot()` but forgetting `LoadState()` or `Reset()`** — all three must stay in sync when adding stores
2. **Not storing the raw event for routed operations** — even when an event triggers side effects (creating a person, alias, etc.), always store the original event too for observability
3. **Creating a new test setup function** — reuse the existing one; the new stores are initialized by `store.New()` already
4. **Overwriting properties on upsert instead of merging** — new properties should override, but existing properties not in the update should be preserved
5. **Forgetting to nil-check new fields in `LoadState()`** — existing seed data won't have the new fields
6. **Updating coverage percentage without updating the resource lists** — `resources_implemented` and `resources_not_implemented` must match the declared percentage
