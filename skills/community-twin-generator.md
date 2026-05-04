---
skill: community-twin-generator
skill_version: "2.0"
schemas:
  provenance.schema.json: "1.1"
  twin-manifest.schema.json: "1.0"
  twin.schema.json: "1.0"
  scenario.schema.json: "1.0"
---

# SKILL: Community Twin Generator

## Purpose

Generate a complete, MIT-licensed community twin from a third-party service's
public SDK reference and/or API documentation. The output is a self-contained
Go module that behaviorally clones the target service — maintaining state,
implementing business logic, and targeting compatibility with the service's
official SDK client libraries.

Community twins target **100% API surface coverage** with correct behavior
(per `wondertwin/CLAUDE.md`).

This skill is **application guidance** — how to apply the community canonical
pattern at generation time. It does not redefine the pattern. The authoritative
sources are:

1. **`wondertwin/CLAUDE.md`** — community canonical pattern, coverage policy,
   pre-push validation, scope-exception process. The source of truth.
2. **Reference twins in `wondertwin/`** — copy-and-adapt examples by contract shape.
3. **`wondertwin/schemas/`** — JSON Schemas every artifact must validate against
   (`twin.schema.json`, `twin-manifest.schema.json`, `provenance.schema.json`,
   `scenario.schema.json`).
4. **`cmd/validate-schemas/`** — pre-push validator that catches schema and
   structural defects.

When the docs in (1) and the code in (2) disagree, **the code is ground truth**.
The skill reflects what reference twins do — not what older docs claim.

## Output artifacts

A complete community twin produces:

1. **Twin source code** — a Go module following the standard layout (see CLAUDE.md "Standard structure")
2. **`twin.json`** — twin metadata (name, description, category, SDK target, port)
3. **`twin-manifest.json`** — capabilities, SDK target, service surface, coverage
4. **`provenance.json`** — generation provenance (sources, dates, archives)
5. **`workflows/{name}.arazzo.json`** — Arazzo 1.0.1 multi-step workflow file (when multi-step sequences exist)
6. **`scenarios/basic.json`** — starter scenario covering health + CRUD, ready for `wt test`
7. **Handler tests** in `internal/api/handlers_test.go`

## Inputs

- **Platform name** — service identifier (lowercase, hyphenated; e.g., `quickbooks-online`, `mercury`)
- **Research directory** — `wondertwin-docs/research/{platform}/` containing build-ready research artifacts. The research-to-build gate (`wondertwin-docs/skills/handoffs/research-to-build.md`) defines the required floor.
- **SDK reference** — URL or document containing the SDK / API reference

If the research directory is missing required artifacts or any artifact's
confidence is below the gate's floor, do NOT proceed — return to research.

## Canonical pattern (cite, don't restate)

The community canonical pattern lives in **`wondertwin/CLAUDE.md`** under
"Standard structure" and "Twin Build Process". Every twin MUST satisfy the
orchestration shape verified by `cmd/validate-schemas/` and exemplified by
existing community twins:

- **Orchestration**: `cmd/twin-{name}/main.go` calls `twincore.New(twincore.ParseFlags(...))` — never `chi.NewRouter()` directly.
- **Admin**: `admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)` from `twinkit/admin` — never a custom admin handler.
- **Store**: `internal/store/memory.go` implements `Snapshot() any`, `LoadState([]byte) error`, `Reset()`, holds `Clock *state.Clock` initialized in `New()`.
- **Routes**: `internal/api/router.go` mounts middleware (`h.authMiddleware`, `h.mw.FaultInjection`) inside the route group, then resource routes.
- **Server lifecycle**: `twin.Serve()`. Never hand-roll `http.Server` + signal handling.

For the live signatures, copy `twin-{ref}/cmd/twin-{ref}/main.go`,
`internal/api/router.go`, `internal/api/handler.go`, `internal/store/memory.go`
shapes from the matching reference twin (see "Vendor contract shape detection"
below). The skill does not duplicate those file contents — they evolve, the
references are canonical.

### Tier boundary

Community twins are MIT-licensed and have **no commercial infrastructure**:

- No telemetry instrumentation
- No quirks runtime engine (the "behavioral quirks" you discover during
  research are documented in research artifacts and implemented as twin
  behavior; the community tier has no runtime quirk-rule loader)
- No replay, run lifecycle, or license verification

Pro twins (separately licensed) layer commercial infrastructure on top of the
same canonical orchestration. The community tier is the orchestration layer
alone, which is sufficient for community use cases.

## Vendor contract shape detection

Canonical orchestration (above) is universal. Vendor-specific request and
response handling — how the twin decodes requests and shapes responses —
depends on the vendor's contract. Pick the matching pattern, copy the
reference's resource-handler layer.

| Vendor contract shape | Reference twin | Notes |
|---|---|---|
| JSON request + JSON response | `twin-hubspot`, `twin-shopify`, `twin-resend` | Most modern APIs. Resource handlers receive `*http.Request`, decode JSON via stdlib, write JSON responses directly. |
| Form-encoded request + JSON response | `twin-stripe`, `twin-twilio` | Form parsing via `r.ParseForm()`; responses are JSON. SDK compatibility for `stripe-go`-shaped clients. |
| GraphQL | `twin-linear` | Single `POST /graphql` endpoint; query/mutation discriminator in body. Uses `twinkit/graphql` for parsing. |
| SOAP / XML | (none yet — see `twinkit/soap` package) | First community twin to implement defines the canonical reference. |

Detect shape from:
- `Content-Type` headers in the SDK's request construction
- Response envelope structure in the SDK's response decoding
- The vendor's developer documentation

Once detected, **read the matching reference twin's
`internal/api/router.go` + one `handlers_{resource}.go` end-to-end** before
writing the new twin's handlers. Copy patterns; adapt for vendor-specific
quirks. Per `CLAUDE.md` "Common pitfalls" — verify enum values, port
collisions, and nil-slice JSON behavior against existing twins before
finalizing.

## Process

### Phase 0: Verify research-to-build gate

The research-to-build gate
(`wondertwin-docs/skills/handoffs/research-to-build.md`) defines the
artifacts and confidence floor required before scaffolding. Before any code:

1. Read `wondertwin-docs/research/{platform}/RESEARCH-STATUS.md` — verify all
   required confidence scores meet their floor.
2. Confirm required artifacts exist: `system-model.md`, `record-type-catalog.json`,
   `compatibility-path.md`, `architecture-spec.md` (with engine-analysis),
   `roadmap.json`, `event-model.md` if applicable.
3. Confirm `feasibility.md` says GO.
4. Confirm engine-analysis says either "use existing engine X" or "no engine
   needed". If it says "new engine required" — stop. New engine work is a
   separate stream; community twin generation is blocked until the engine lands.

If any check fails, the agent returns to research with the highest-EV gap
from the gap list as the next target. The build is not started.

### Phase 1: Scaffold + core resources

**1. Create the directory structure** matching `CLAUDE.md` "Standard structure":

```
twin-{name}/
├── cmd/twin-{name}/main.go
├── internal/
│   ├── api/
│   │   ├── router.go
│   │   ├── handlers_{area}.go
│   │   └── handlers_test.go
│   └── store/
│       ├── memory.go
│       └── types.go
├── twin.json
├── twin-manifest.json
└── provenance.json
```

For twins with webhooks, add `internal/webhook/signer.go`.

**2. Add to `go.work`** (if applicable) and initialize the module:

```bash
go mod init github.com/wondertwin-ai/wondertwin/twin-{name}
```

**3. Write store types and memory store** — copy the matching reference twin's
`internal/store/types.go` and `internal/store/memory.go` shapes. Adapt struct
fields to match the real API's response schemas. Use `pkgstate.Store[T]` per
entity type with the service's ID prefix (`"cus"`, `"msg"`, etc.).

Rules (verify against reference twin):
- Use the EXACT JSON field names the real API returns; pointer types for nullable fields
- Always reset the Clock in `Reset()`
- Always nil-check snapshot fields in `LoadState()` to support partial seeding

**4. Write the router and handlers** — copy the matching reference twin's
`internal/api/router.go` shape. Adapt for the vendor's URL patterns, auth
scheme, and contract shape. Use `chi.Router`; apply `h.authMiddleware` and
`h.mw.FaultInjection` inside the route group.

For form-encoded vendors (Stripe pattern): `r.ParseForm()` in the handler;
responses are JSON.

For GraphQL vendors (Linear pattern): single `POST /graphql` endpoint; switch
on operation type via `twinkit/graphql` parser.

**5. Write the entry point** (`cmd/twin-{name}/main.go`) — copy the reference
twin's main.go and adapt the twin name + port. Default port: pick from the 41xx
range; grep existing `cfg.Port` values per `CLAUDE.md` "Common pitfalls".

The canonical entry-point shape:
1. `cfg := twincore.ParseFlags("twin-{name}")`
2. Set `cfg.Port = {default}` if zero
3. `twin := twincore.New(cfg)`
4. `memStore := store.New()`
5. (Optional) Webhook dispatcher via `webhook.NewDispatcher(webhook.Config{...})`
6. (Optional) Domain engine creation
7. API handler via `api.NewHandler(memStore[, dispatcher], twin.Middleware()[, engine])`
8. `apiHandler.Routes(twin.Router)`
9. Admin handler via `admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)`
10. `adminHandler.SetConfigProvider(twin)`
11. `adminHandler.Routes(twin.Router)`
12. Load seed data if `cfg.SeedFile != ""`
13. `twin.Serve()`

For exact form, see `twin-linear/cmd/twin-linear/main.go` (51L, the smallest
canonical reference) or `twin-stripe/cmd/twin-stripe/main.go` (with webhook
dispatcher).

**6. Write `twin.json`, `twin-manifest.json`, `provenance.json`** — see
"Schema files" below. **All three must validate** via
`go run ./cmd/validate-schemas/` before pushing.

**7. Write handler tests** — see "Tests" below.

### Phase 2: Domain engine selection (when applicable)

Community twins MAY use a `twinkit` domain engine for stateful behavior modeling.
Engine selection is per-twin SDK analysis — not per-category. Engines live in
`twinkit/{engine}`. The architecture-spec.md from research names the engine.

| Service category | Engine | Package |
|---|---|---|
| Accounting / fintech (double-entry) | Ledger | `twinkit/ledger` |
| Email / SMS / notifications | Messaging | `twinkit/messaging` |
| Collaboration / CRM / project mgmt | Workspace | `twinkit/workspace` |
| Analytics / CDPs / feature flags | Events | `twinkit/events` |
| Search platforms | Search | `twinkit/search` |
| Multi-domain (lifecycle tracking) | Workspace + custom | `twinkit/workspace` |
| Simple CRUD APIs | None | Direct store operations |

The engine runs alongside the store as a parallel behavioral path. The store
remains the source of truth for API responses; the engine adds workflow
validation and state-machine enforcement.

For engine-backed twins, create the engine in `main.go`:

```go
wsEngine := workspace.NewEngine(
    workspace.WithClock(memStore.Clock),
    workspace.WithWorkflow(workspace.WorkflowConfig{
        EntityType:     "issue",
        InitialStatus:  "backlog",
        TerminalStates: []string{"done", "canceled"},
        Transitions: map[string][]string{
            "backlog":     {"todo"},
            "todo":        {"in_progress"},
            "in_progress": {"done", "canceled"},
        },
    }),
)
```

Pass the engine to `api.NewHandler`; in tests, pass `nil` and nil-check before
calling engine methods (`if h.wsEngine != nil`).

### Phase 3: Webhook signer (when applicable)

Only implement if the target service sends webhooks (per `event-model.md`).

`internal/webhook/signer.go` implements the `webhook.Signer` interface. The
signer returns header name → value pairs for every webhook POST.

Example (HMAC-SHA256 with timestamp):

```go
package webhook

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "time"
)

type ServiceSigner struct{}

func NewServiceSigner() *ServiceSigner {
    return &ServiceSigner{}
}

func (s *ServiceSigner) Sign(payload []byte, secret string) map[string]string {
    ts := fmt.Sprintf("%d", time.Now().Unix())
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(ts + "."))
    mac.Write(payload)
    sig := hex.EncodeToString(mac.Sum(nil))

    return map[string]string{
        "X-Service-Signature": fmt.Sprintf("t=%s,v1=%s", ts, sig),
        "X-Service-Timestamp": ts,
    }
}
```

The exact header names and signing scheme come from the vendor's webhook
documentation. `twin-stripe/internal/webhook/signer.go` is the canonical
reference.

When the twin has webhooks, the Handler struct gains a `dispatcher *webhook.Dispatcher`
field, and `main.go` constructs the dispatcher with the vendor signer.

### Phase 4: Arazzo workflow generation

After implementing handlers, identify multi-step workflows that span multiple
resources or require sequenced operations. Produce an Arazzo 1.0.1 JSON file at
`workflows/{name}.arazzo.json`.

**When to produce a workflow:**
- A resource must be created before it can be used (e.g., create a sender before sending a message)
- Operations have dependencies (e.g., verify a domain, then send from that domain)
- A common integration pattern involves 3+ sequential SDK calls

**Arazzo 1.0.1 structure:**

```json
{
  "arazzo": "1.0.1",
  "info": {
    "title": "{Service} Twin Workflows",
    "version": "1.0.0",
    "description": "Multi-step workflows discovered during twin generation."
  },
  "sourceDescriptions": [
    {
      "name": "{name}-twin",
      "type": "openapi",
      "url": "./specs/{name}-openapi.json"
    }
  ],
  "workflows": [
    {
      "workflowId": "create-and-send-message",
      "description": "Create a contact, then send a message to that contact.",
      "steps": [
        {
          "stepId": "create-contact",
          "operationId": "createContact",
          "requestBody": {
            "payload": { "email": "$inputs.recipient_email" }
          },
          "successCriteria": [
            { "condition": "$statusCode == 201" }
          ],
          "outputs": { "contact_id": "$response.body.id" }
        },
        {
          "stepId": "send-message",
          "operationId": "sendMessage",
          "requestBody": {
            "payload": {
              "to": "$steps.create-contact.outputs.contact_id",
              "body": "$inputs.message_body"
            }
          },
          "successCriteria": [
            { "condition": "$statusCode == 201" }
          ],
          "outputs": { "message_id": "$response.body.id" }
        }
      ]
    }
  ]
}
```

**Rules:**
- Each workflow has a descriptive `workflowId` (kebab-case)
- Steps reference `operationId` values that match the twin's handlers
- Use runtime expressions (`$response.body.*`, `$statusCode`, `$inputs.*`) to wire steps together
- Include `successCriteria` for every step (at minimum, the expected status code)
- If no OpenAPI spec exists, use `operationPath` with method + path instead of `operationId` (e.g., `"operationPath": "POST /v1/contacts"`)

**Update provenance** after generating Arazzo:

```json
{
  "sources": {
    "arazzo": {
      "origin": "generated",
      "generated_from": ["sdk_analysis", "handler_implementation"],
      "sha256": "{hash of arazzo file}"
    }
  }
}
```

### Phase 5: Starter scenario

Produce a starter scenario JSON at `scenarios/basic.json`. Validates against
`schemas/scenario.schema.json`. Provides baseline test coverage for `wt test`.

**The starter scenario MUST include:**

1. A health check step (verifying `/admin/health`)
2. A state reset step (calling `/admin/reset`)
3. One complete CRUD cycle per primary resource (create, get, update, list, delete)
4. Variable capture between steps (capturing the created resource ID for subsequent get/update/delete)

**Starter scenario template:**

```json
{
  "name": "Basic CRUD - {Service}",
  "description": "Health check and CRUD cycle for primary {Service} resources.",
  "setup": { "reset": ["{name}"] },
  "variables": {
    "auth_token": "Bearer test_123"
  },
  "steps": [
    {
      "name": "Health check",
      "request": { "method": "GET", "url": "{{base_url}}/admin/health" },
      "assert": { "status": 200, "body": { "$.status": "ok" } }
    },
    {
      "name": "Reset state",
      "request": { "method": "POST", "url": "{{base_url}}/admin/reset" },
      "assert": { "status": 200 }
    },
    {
      "name": "Create contact",
      "request": {
        "method": "POST",
        "url": "{{base_url}}/v1/contacts",
        "headers": { "Authorization": "{{auth_token}}" },
        "body": { "email": "test@example.com", "first_name": "Test" }
      },
      "capture": { "contact_id": "$.id" },
      "assert": {
        "status": 201,
        "body": { "$.email": "test@example.com" }
      }
    },
    {
      "name": "Get contact",
      "request": {
        "method": "GET",
        "url": "{{base_url}}/v1/contacts/{{contact_id}}",
        "headers": { "Authorization": "{{auth_token}}" }
      },
      "assert": {
        "status": 200,
        "body": {
          "$.id": "{{contact_id}}",
          "$.email": "test@example.com"
        }
      }
    },
    {
      "name": "Update contact",
      "request": {
        "method": "PATCH",
        "url": "{{base_url}}/v1/contacts/{{contact_id}}",
        "headers": { "Authorization": "{{auth_token}}" },
        "body": { "first_name": "Updated" }
      },
      "assert": {
        "status": 200,
        "body": { "$.first_name": "Updated" }
      }
    },
    {
      "name": "List contacts",
      "request": {
        "method": "GET",
        "url": "{{base_url}}/v1/contacts?limit=10",
        "headers": { "Authorization": "{{auth_token}}" }
      },
      "assert": { "status": 200 }
    },
    {
      "name": "Delete contact",
      "request": {
        "method": "DELETE",
        "url": "{{base_url}}/v1/contacts/{{contact_id}}",
        "headers": { "Authorization": "{{auth_token}}" }
      },
      "assert": {
        "status": 200,
        "body": { "$.deleted": true }
      }
    }
  ]
}
```

**Rules:**
- Use the v2 JSON format matching `schemas/scenario.schema.json` (NOT YAML)
- Always start with health check and reset steps
- Capture IDs from create responses; reuse via `{{variable}}` syntax
- Include at least one assertion per step (status code at minimum, body assertions preferred)

### Phase 6: Local-dev loop with `wt`

`wt` is the canonical local-dev CLI for community twins. The full loop is
**mandatory**, not optional. Every community twin must be exercised via `wt`
before opening the PR.

**1. Build the twin locally:**

```bash
go build -o ./bin/twin-{name} ./twin-{name}/cmd/twin-{name}/
```

Or use the Makefile: `make build-twins`.

**2. Add to your project's `wondertwin.json`:**

```json
{
  "twins": {
    "{name}": {
      "binary": "./bin/twin-{name}",
      "port": {port}
    }
  }
}
```

The `binary` field supports relative paths — they resolve against the
`wondertwin.json` location.

**3. Run the offline workflow:**

```bash
wt up        # Start the twin
wt status    # Verify it's healthy
wt test      # Run scenarios against it
wt down      # Stop when done
```

**4. Run the starter scenario** generated in Phase 5:

```bash
wt test scenarios/basic.json
```

**5. Run conformance to validate the admin contract:**

```bash
wt conformance ./bin/twin-{name} --port 9999
```

This validates the standard admin checks: health, reset, state POST/GET, fault
injection, time advance, clean shutdown.

**6. Iterate:**

```bash
make build-twins && wt down && wt up && wt test
```

**Test-as-deliverable for community:** the tools (`wt up`/`test`/`conformance`)
are **available**; running them is **expected** for development; passing them
is **required** before the PR opens. The community tier does not enforce
test-suite coverage as a CI contract — that's a pro-tier-only concern. For
community, "the tools work and the local loop passes" is the deliverable.

### Phase 7: Pre-push validation

Per `CLAUDE.md` "Pre-push validation":

```bash
go run ./cmd/validate-schemas/   # All twin JSON files validate
go test ./twin-{name}/...        # New twin's tests pass
go test ./... -short             # No regressions in other twins
go build -o /dev/null ./twin-{name}/cmd/twin-{name}/   # Compiles
```

The `validate-schemas` tool catches missing files, invalid enum values, and
structural errors before they reach CI. **All four checks must pass before
push.**

### Phase 8: Gap audit (final)

Before declaring the twin complete, per `CLAUDE.md` "Final Phase: Gap Audit":

1. Cross-reference every endpoint from `record-type-catalog.json` against the
   handlers actually implemented.
2. Implement every missing endpoint, no matter how niche. Per `CLAUDE.md`
   "Twin Coverage Policy": **the agent does not make scope decisions** — if
   the real API supports it, the twin must support it. Use the Scope Exception
   Request Template (in CLAUDE.md) when surfacing genuine blockers.
3. Update `twin-manifest.json` to `coverage.estimated_coverage_pct: 100` with
   empty `resources_not_implemented`.
4. Re-run the local-dev loop (Phase 6) and pre-push validation (Phase 7).

A twin is not complete until coverage is 100%, all artifacts validate, and the
local-dev loop passes end-to-end.

## Tests

Use `twinkit/testutil` helpers. Tests create the handler directly:

```go
package api_test

import (
    "testing"

    "github.com/wondertwin-ai/wondertwin/twinkit/admin"
    "github.com/wondertwin-ai/wondertwin/twinkit/testutil"
    "github.com/wondertwin-ai/wondertwin/twinkit/twincore"
    "github.com/wondertwin-ai/wondertwin/twin-{name}/internal/api"
    "github.com/wondertwin-ai/wondertwin/twin-{name}/internal/store"
)

func setupTestServer(t *testing.T) (*testutil.TwinClient, *testutil.AdminClient) {
    cfg := &twincore.Config{Name: "twin-{name}-test"}
    twin := twincore.New(cfg)
    memStore := store.New()

    apiHandler := api.NewHandler(memStore, twin.Middleware())
    apiHandler.Routes(twin.Router)

    adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
    adminHandler.Routes(twin.Router)

    server := httptest.NewServer(twin)
    t.Cleanup(server.Close)

    tc := testutil.NewTwinClient(t, server)
    ac := testutil.NewAdminClient(tc)
    return tc, ac
}
```

For exact test shapes, see `twin-{ref}/internal/api/handlers_test.go` of the
matching reference twin. Pass `nil` for optional engine/dispatcher parameters
in tests where they aren't needed for the test case.

**Coverage floor for every twin:**

- Create, get, list, update, delete for each resource (happy path)
- Pagination test with multiple items
- Admin reset clears all state
- Admin seed loads fixtures correctly
- 404 for nonexistent resources
- 401 for missing auth header
- Required field validation returns the vendor's error format

## Schema files

### `twin.json`

```json
{
  "name": "{name}",
  "description": "Behavioral clone of the {Service} API.",
  "category": "{category}",
  "sdk": {
    "package": "{sdk_import_path}",
    "version": "{version}"
  },
  "default_port": {port}
}
```

### `twin-manifest.json`

Minimum shape (validates against `schemas/twin-manifest.schema.json`):

```json
{
  "twin": "{name}",
  "display_name": "{Service}",
  "tier": "community",
  "category": "{category}",
  "description": "Behavioral clone of the {Service} service for SDK-compatible local testing.",
  "sdk_target": {
    "primary": {
      "package": "{sdk_import_path}",
      "language": "go",
      "version": "{version}",
      "repo_url": "https://github.com/{org}/{sdk-repo}"
    }
  },
  "service_surface": {
    "openapi_spec": {
      "available": true,
      "origin": "vendor_published",
      "url": "https://api.example.com/openapi.json"
    },
    "auth_pattern": "api_key",
    "has_webhooks": false,
    "resource_count": 0
  },
  "coverage": {
    "resources_implemented": [],
    "resources_not_implemented": [],
    "estimated_coverage_pct": 0
  }
}
```

`auth_pattern` must be one of: `api_key`, `oauth2`, `basic`, `jwt`, `custom`,
`none` (per `CLAUDE.md` "Common pitfalls" — do not invent values).

### `provenance.json`

```json
{
  "twin": "twin-{name}",
  "version": "0.1.0",
  "api_version": "{pinned API version}",
  "platform": "{Service}",
  "platform_url": "{developer docs URL}",
  "category": "{category}",
  "research_archive": "wondertwin-docs/research/{platform}",
  "research_completed": "{ISO 8601}",
  "generated_at": "{ISO 8601 timestamp}",
  "sources": [
    {
      "type": "{source_type}",
      "url": "{url}",
      "accessed": "{date}",
      "archived_at": "{archive_path}"
    }
  ],
  "sdk_target": {
    "package": "{sdk_import_path}",
    "version": "{pinned version}",
    "language": "go"
  }
}
```

## Common mistakes

Per `CLAUDE.md` "Common pitfalls" plus mistakes surfaced from prior twin builds:

1. **Using `time.Now()` instead of `store.Clock.Now()`** — breaks simulated time and `wt` time-advance tests
2. **Hardcoding the port** — always use `twincore.ParseFlags()` and allow `--port` override
3. **Inventing `auth_pattern` enum values** — must be one of the schema's allowed values; `validate-schemas` catches this
4. **Port collisions** — grep existing `cfg.Port` values before assigning a new one
5. **Nil-slice JSON drift** — `json.Marshal(nil_slice)` produces `null`, not `[]`. Tests must handle both when asserting on empty lists
6. **Skipping `validate-schemas`** — runs in seconds locally; catches errors that would otherwise surface in CI
7. **Hand-rolling `http.Server` + signal handling in main.go** — bypasses canonical lifecycle in `twin.Serve()`
8. **Missing `FaultInjection` middleware** — must be applied inside the route group with `r.Use(h.mw.FaultInjection)`
9. **Returning the wrong error format** — each service has its own error envelope; match it exactly
10. **Using `http.StatusOK` for creates** — check what the real service returns (often 201)
11. **Skipping `omitempty` on optional JSON fields** — SDK clients may break on unexpected null fields
12. **Not nil-checking in `LoadState()`** — partial seed data should work
13. **Skipping any of `twin.json` / `twin-manifest.json` / `provenance.json`** — all three are required, all three validate against schemas
14. **Writing scenarios in YAML instead of JSON** — the v2 scenario format is JSON, validated against `schemas/scenario.schema.json`
15. **Omitting health check and reset steps from starter scenarios** — every scenario should begin with these
16. **Forgetting to update provenance after Arazzo generation** — add the `arazzo` source entry with `origin` and `sha256`
17. **Making scope decisions silently** — per `CLAUDE.md` "Twin Coverage Policy", the agent never decides what to skip; surface scope exceptions via the template, do not omit endpoints

## Completion checklist

A community twin is complete when ALL of these are true:

**Source code and structure:**
- [ ] Directory structure matches `CLAUDE.md` "Standard structure"
- [ ] `cmd/twin-{name}/main.go` follows canonical pattern (`twincore.New` + `twin.Serve()`)
- [ ] `internal/api/router.go` mounts `h.authMiddleware` and `h.mw.FaultInjection` inside the route group
- [ ] `internal/store/memory.go` implements `Snapshot`, `LoadState`, `Reset`; holds `Clock`
- [ ] All routes match the real service's URL patterns exactly
- [ ] Request parsing matches the SDK's content type (JSON, form-encoded, GraphQL)
- [ ] Response format matches the real service's envelope and field names
- [ ] Error responses match the real service's error format
- [ ] ID generation matches the real service's ID format and prefix
- [ ] Timestamps use `store.Clock.Now()` (not `time.Now()`)
- [ ] Pagination matches the real service's pagination pattern
- [ ] Auth middleware validates header presence (accepts any value)

**Pipeline artifacts (same PR as code):**
- [ ] `twin.json` validates against `schemas/twin.schema.json`
- [ ] `twin-manifest.json` validates against `schemas/twin-manifest.schema.json`
- [ ] `provenance.json` validates against `schemas/provenance.schema.json`
- [ ] `workflows/{name}.arazzo.json` present (if multi-step sequences exist)
- [ ] `scenarios/basic.json` validates against `schemas/scenario.schema.json`
- [ ] `coverage.resources_implemented` lists every resource the twin handles
- [ ] `coverage.resources_not_implemented` empty (gap audit complete)
- [ ] `coverage.estimated_coverage_pct: 100`

**Local-dev loop and validation:**
- [ ] `wt up` + `wt status` + `wt test` + `wt down` works end-to-end
- [ ] `wt test scenarios/basic.json` passes
- [ ] `wt conformance ./bin/twin-{name} --port 9999` passes
- [ ] `go run ./cmd/validate-schemas/` clean
- [ ] `go test ./twin-{name}/...` passes
- [ ] `go test ./... -short` clean (no regressions in other twins)
- [ ] `go build -o /dev/null ./twin-{name}/cmd/twin-{name}/` succeeds
- [ ] `go vet ./...` clean

**Domain engine (if applicable):**
- [ ] Engine created with `WithClock(memStore.Clock)` for simulated time
- [ ] Handlers call engine methods after store writes
- [ ] Engine passed to Handler struct and nil-safe (`if h.engine != nil`)

**Webhooks (if applicable):**
- [ ] Signer implements `webhook.Signer`
- [ ] Dispatcher created via `webhook.NewDispatcher(webhook.Config{...})`

A twin is NOT complete until coverage is 100%, all artifacts validate, and the
local-dev loop passes end-to-end.
