---
skill: community-twin-generator
skill_version: "1.0"
schemas:
  provenance.schema.json: "1.1"
  twin-manifest.schema.json: "1.0"
  twin.schema.json: "1.0"
---

# SKILL: Community Twin Generator

## Purpose

Generate a complete, MIT-licensed community twin from a third-party service's public SDK reference and/or API documentation. The output is a self-contained Go module that behaviorally clones the target service — maintaining state, implementing business logic, and targeting compatibility with the service's official SDK client libraries.

Community twins provide 100% API surface coverage with correct behavior.

## Output Artifacts

1. **Twin source code** — a Go module following the standard directory layout
2. **`twin-manifest.json`** — describes the twin's capabilities, SDK target, service surface, and coverage
3. **`provenance.json`** — records what sources were used during generation and when
4. **`twin.json`** — twin metadata (name, description, category, SDK target, port)

## Prerequisites

1. The WonderTwin shared libraries via `github.com/wondertwin-ai/wondertwin/twinkit`:
   - Core: `twincore`, `state`, `admin`, `webhook`, `testutil`, `pagination`
   - Domain engines: `workspace`, `ledger`, `messaging`, `events`, `search`
2. The target service's public API documentation or SDK reference
3. Optionally, the official SDK client library source code for compatibility verification

---

## Standard Directory Layout

```
twin-{name}/
├── cmd/twin-{name}/main.go          # entry point
├── internal/
│   ├── api/
│   │   ├── router.go                # routes + auth middleware
│   │   ├── handlers_{area}.go       # one file per resource area
│   │   └── handlers_test.go         # tests using testutil
│   └── store/
│       ├── memory.go                # in-memory store with snapshot/load/reset
│       └── types.go                 # domain structs matching real service
├── twin.json                        # twin metadata
├── twin-manifest.json               # coverage manifest
├── provenance.json                  # build provenance
└── go.mod                           # module definition
```

For twins with webhooks, add:
```
├── internal/
│   └── webhook/
│       └── signer.go                # service-specific webhook signer
```

---

## Entry Point Template

Every community twin's `main.go` follows this pattern:

```go
package main

import (
    "log"
    "os"

    "github.com/wondertwin-ai/wondertwin/twinkit/admin"
    "github.com/wondertwin-ai/wondertwin/twinkit/twincore"
    "github.com/wondertwin-ai/wondertwin/twin-{name}/internal/api"
    "github.com/wondertwin-ai/wondertwin/twin-{name}/internal/store"
)

func main() {
    cfg := twincore.ParseFlags("twin-{name}")
    if cfg.Port == 0 {
        cfg.Port = {default_port}
    }

    twin := twincore.New(cfg)
    memStore := store.New()

    // API handlers
    handler := api.NewHandler(memStore, twin.Middleware())
    handler.Routes(twin.Router)

    // Admin control plane
    adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
    adminHandler.Routes(twin.Router)

    // Load seed data if provided
    if cfg.SeedFile != "" {
        data, err := os.ReadFile(cfg.SeedFile)
        if err != nil {
            log.Fatalf("failed to read seed file: %v", err)
        }
        if err := memStore.LoadState(data); err != nil {
            log.Fatalf("failed to load seed data: %v", err)
        }
        twin.Logger.Info("loaded seed data", "file", cfg.SeedFile)
    }

    twin.Logger.Info("twin-{name} ready", "port", cfg.Port)

    if err := twin.Serve(); err != nil {
        log.Fatalf("server error: %v", err)
    }
}
```

### With webhook support

Add the dispatcher between store creation and handler creation:

```go
    dispatcher := webhook.NewDispatcher(webhook.Config{
        URL:         cfg.WebhookURL,
        Secret:      webhookSecret,
        Signer:      {service}wh.New{Service}Signer(),
        Logger:      twin.Logger,
        EventPrefix: "{prefix}",
        AutoDeliver: cfg.WebhookURL != "",
    })

    handler := api.NewHandler(memStore, dispatcher, twin.Middleware())
```

### With domain engine

Add the engine after store creation:

```go
    wsEngine := workspace.NewEngine(
        workspace.WithClock(memStore.Clock),
        workspace.WithWorkflow(workspace.WorkflowConfig{
            // ... workflow definitions
        }),
    )

    handler := api.NewHandler(memStore, twin.Middleware(), wsEngine)
```

---

## Handler Pattern

### Handler struct and constructor

```go
type Handler struct {
    store *store.MemoryStore
    mw    *twincore.Middleware
    // Add domain engine fields as needed (ws, msgs, events, etc.)
}

func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
    return &Handler{store: s, mw: mw}
}
```

### Routes method

```go
func (h *Handler) Routes(r chi.Router) {
    r.Route("/v1", func(r chi.Router) {
        r.Use(h.authMiddleware)
        r.Use(h.mw.FaultInjection)

        r.Post("/{resources}", h.Create{Resource})
        r.Get("/{resources}/{id}", h.Get{Resource})
        // ...
    })
}
```

---

## Domain Engine Selection

Choose the domain engine based on the service category:

| Category | Engine | Package |
|----------|--------|---------|
| Payments, fintech, accounting | `ledger` | `twinkit/ledger` |
| CRM, project mgmt, collaboration | `workspace` | `twinkit/workspace` |
| Email, SMS, notifications | `messaging` | `twinkit/messaging` |
| Analytics, CDPs, feature flags | `events` | `twinkit/events` |
| Search platforms | `search` | `twinkit/search` |
| Simple CRUD APIs | None needed | Direct store operations |

When using a domain engine, the engine runs alongside the store. The store is the source of truth for API responses; the engine provides workflow validation and state machine enforcement.

---

## Build Process

### Phase 0: Research
Use the `twin-researcher` skill to investigate the target API. Research artifacts go to `wondertwin-docs/research/{platform}/`.

### Phase 1: Scaffold + Core
1. Create directory structure following the standard layout
2. Write store types for all known resource areas
3. Write the memory store with snapshot/load/reset
4. Write the router with auth middleware matching the service's style
5. Implement the highest-traffic endpoints first
6. Include all 3 schema files from commit 1
7. Write tests, PR

### Phase 2+: Expand to 100%
1. Work through remaining resource areas in logical batches
2. One PR per logical batch
3. Update manifest coverage after each phase

### Final: Gap Audit
1. Cross-reference every endpoint from research against the router
2. Implement every missing endpoint
3. Update manifest to 100% coverage

---

## Testing

Use `twinkit/testutil` for test helpers. Tests create a handler directly:

```go
func TestCreateCustomer(t *testing.T) {
    s := store.New()
    mw := twincore.NewTestMiddleware()
    h := NewHandler(s, mw)

    r := chi.NewRouter()
    h.Routes(r)

    // ... make requests against r
}
```

Pass `nil` for optional domain engine parameters in tests where the engine isn't needed for the test case.

---

## Schema Files

### twin.json
```json
{
    "name": "{name}",
    "description": "Simulates the {Service} API surface...",
    "category": "{category}",
    "sdk": {
        "package": "{sdk_import_path}",
        "version": "{version}"
    },
    "default_port": {port}
}
```

### twin-manifest.json
```json
{
    "twin": "{name}",
    "display_name": "{Service}",
    "tier": "community",
    "category": "{category}",
    "description": "...",
    "sdk_target": { ... },
    "service_surface": { ... },
    "coverage": { ... }
}
```

### provenance.json
```json
{
    "twin": "twin-{name}",
    "version": "0.1.0",
    "generated_at": "{ISO 8601}",
    "api_version": "{version}",
    "platform": "{Service}",
    "platform_url": "{docs_url}",
    "category": "{category}",
    "sources": [ ... ],
    "sdk_target": {
        "package": "{sdk_import_path}",
        "version": "{version}",
        "language": "go"
    }
}
```
