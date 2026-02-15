# SKILL: WonderTwin Twin Generator

## Purpose

Generate a complete, production-ready WonderTwin behavioral service twin from a third-party service's public SDK reference and/or API documentation. The output is a self-contained Go module that behaviorally clones the target service — maintaining state, implementing business logic, and targeting compatibility with the service's official SDK client libraries.

In addition to the twin source code, generation produces a set of pipeline artifacts:

1. **`twin-manifest.json`** — describes the twin's capabilities, SDK target, service surface, and coverage (see `schemas/twin-manifest.schema.json`)
2. **`provenance.json`** — records what sources were used during generation and when (see `schemas/provenance.schema.json`)
3. **Arazzo workflow file** — captures discovered multi-step sequences as Arazzo 1.0.1 JSON (e.g., `workflows/{name}.arazzo.json`)
4. **Starter scenario file** — a JSON scenario covering health check and basic CRUD, ready for `wt test` (see `schemas/scenario.schema.json`)

## Prerequisites

Before using this skill, ensure you have access to:

1. The WonderTwin shared libraries via `github.com/wondertwin-ai/twinkit` (`twincore`, `store`, `admin`, `webhook`, `testutil`)
2. The target service's public API documentation or SDK reference
3. Optionally, the official SDK client library source code for compatibility verification

## Inputs

The user will provide:

- **Service name**: The SaaS service to twin (e.g., "Stripe", "SendGrid", "Auth0")
- **SDK reference**: URL or document containing the SDK or API reference
- **OpenAPI spec URL** (optional): Direct URL to the service's OpenAPI/Swagger spec, if available
- **SDK target** (optional): Official SDK client library for compatibility (e.g., `github.com/stripe/stripe-go`)
- **Scope** (optional): Specific SDK resources to prioritize, or "full coverage"
- **Known quirks** (optional): Undocumented behaviors or edge cases to encode (see `schemas/quirk.schema.json`)

## Output Structure

Every twin MUST produce this exact directory structure:

```
twin-{name}/
├── cmd/
│   └── twin-{name}/
│       └── main.go                   # Entry point
├── internal/
│   ├── api/
│   │   ├── router.go                 # Handler struct, Routes(), auth middleware
│   │   ├── handlers_{resource}.go    # One file per SDK resource group
│   │   ├── helpers.go                # Request parsing, response formatting helpers
│   │   └── handlers_test.go          # Handler tests
│   └── store/
│       ├── types.go                  # Domain structs with JSON tags
│       └── memory.go                 # MemoryStore implementing admin.StateStore
├── twin-manifest.json                # Twin capabilities and coverage (schemas/twin-manifest.schema.json)
├── provenance.json                   # Generation provenance record (schemas/provenance.schema.json)
├── twin.yaml                         # Twin metadata (name, SDK, port)
├── go.mod
├── go.sum
├── Makefile
├── .github/
│   └── workflows/
│       ├── ci.yml                    # PR checks: build, test, conformance
│       └── release.yml              # Tag-triggered: cross-compile + release + registry notify
├── workflows/
│   └── {name}.arazzo.json            # Arazzo 1.0.1 multi-step workflow descriptions
└── scenarios/
    └── basic.json                    # Starter scenario: health + CRUD (schemas/scenario.schema.json)
```

If the service has webhooks, add:

```
│   └── webhook/
│       └── signer.go                 # Service-specific webhook signature implementation
```

---

## Process

### Phase 0: Analyze

Before any code generation, gather and record all available sources. This phase produces the initial `provenance.json` and informs every subsequent phase.

**1. Discover the OpenAPI spec:**

- If the user provides an OpenAPI spec URL, fetch it and save to `specs/{name}-openapi.json` (or `.yaml`).
- If no URL is provided, check common locations: `https://api.{service}.com/openapi.json`, the service's developer docs, or GitHub repos.
- Record the result in provenance format (see below).

**2. Check DeepWiki for SDK analysis:**

- If the SDK repo has been indexed by DeepWiki, note the URL (e.g., `https://deepwiki.com/{org}/{sdk-repo}`) for retrieval during later phases.
- If DeepWiki has not indexed the repo, fall back to reading the SDK's README, examples, and test files directly. See [Fallback Guidance](#fallback-guidance) below.

**3. Identify the primary SDK:**

- Determine the SDK package name, programming language, and version.
- Locate the SDK repository URL and documentation URL.
- This information populates `sdk_target.primary` in `twin-manifest.json`.

**4. Record sources in provenance format:**

Create the initial `provenance.json` (validated against `schemas/provenance.schema.json`):

```json
{
  "twin": "{name}",
  "sdk_target": {
    "package": "{sdk_import_path}",
    "language": "go",
    "version": "{sdk_version}"
  },
  "generated_at": "{ISO 8601 timestamp}",
  "skill_version": "2.0",
  "sources": {
    "openapi": {
      "origin": "vendor_published",
      "url": "https://api.example.com/openapi.json",
      "retrieved_at": "{ISO 8601 timestamp}",
      "sha256": "{hash of spec content}",
      "api_version": "2024-01-01"
    },
    "sdk_analysis": {
      "method": "deepwiki",
      "repo": "https://github.com/{org}/{sdk-repo}",
      "repo_ref": "{tag or commit}",
      "retrieved_at": "{ISO 8601 timestamp}",
      "fallback_used": false
    }
  }
}
```

If no OpenAPI spec is available, omit the `openapi` field or set `origin` to `"derived"`. If DeepWiki was not available, set `method` to `"direct_repo"` and `fallback_used` to `true`.

**5. Create the initial `twin-manifest.json`:**

Populate the manifest shell (validated against `schemas/twin-manifest.schema.json`). Coverage fields will be completed as handlers are implemented:

```json
{
  "twin": "{name}",
  "display_name": "{Service}",
  "category": "{category}",
  "description": "Behavioral clone of the {Service} service for SDK-compatible local testing.",
  "sdk_target": {
    "primary": {
      "package": "{sdk_import_path}",
      "language": "go",
      "version": "{sdk_version}",
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
  },
  "generation": {
    "method": "wondertwin_skill",
    "skill_version": "2.0",
    "sources_used": {
      "deepwiki": true,
      "openapi": true,
      "manual_docs": false
    }
  }
}
```

---

### Phase 1: Project Setup

Before writing any twin code, set up the project from the template.

**1. Create the repository:**

```bash
# Public twin (community contribution)
gh repo create {org}/twin-{name} --template wondertwin-ai/twin-template --public

# Private twin (internal API)
gh repo create {org}/twin-{name} --template wondertwin-ai/twin-template --private
```

**2. Fill in `twin.yaml`** at the repo root:

```yaml
name: {name}
description: "Behavioral clone of the {Service} API"
category: {category}    # e.g., payments, messaging, auth, analytics, email

# SDK version this twin targets
sdk:
  package: "{sdk_import_path}"      # e.g., "github.com/stripe/stripe-go/v76"
  version: "{sdk_version}"          # e.g., "v76.0.0"

# Default port when run standalone
default_port: {port}                # Pick a unique port (e.g., 4111)
```

**3. Initialize the Go module:**

```bash
go mod init github.com/{org}/twin-{name}
go get github.com/wondertwin-ai/twinkit@latest
go get github.com/go-chi/chi/v5@latest
```

**4. The `go.mod` should look like:**

```
module github.com/{org}/twin-{name}

go 1.25.7

require (
    github.com/go-chi/chi/v5 v5.2.1
    github.com/wondertwin-ai/twinkit v0.1.0
)
```

No `replace` directives — twins depend on published `twinkit` versions.

### Phase 2: SDK and Service Analysis

Before writing any code, analyze the target SDK and service and produce a plan. Use the sources gathered in Phase 0 (OpenAPI spec, DeepWiki, SDK source). Document answers to ALL of the following:

**Authentication:**
- What scheme? (API key in header, Bearer token, Basic auth, OAuth)
- What header name? (e.g., `Authorization: Bearer`, `X-API-Key`, `Authorization: Basic`)
- For the twin: accept any value but validate that the header is present. Return 401 if missing.

**Request format:**
- Content type? (JSON body, form-encoded, multipart)
- Does it vary by endpoint?
- How does the SDK encode requests?

**Response format:**
- What is the success envelope? (bare object, `{"data": ...}`, `{"result": ...}`)
- What is the error envelope? (e.g., Stripe: `{"error":{"type":"...","code":"...","message":"..."}}`)
- What HTTP status codes are used for success/error cases?

**Pagination pattern:**
- Cursor-based with `has_more`? (Stripe pattern)
- Offset/limit with `count`/`next`?
- Page-based with `page`/`per_page`?
- What query parameters control pagination?

**Resources and relationships:**
- List all SDK resources (e.g., Users, Messages, Contacts)
- Map relationships (e.g., User has many Messages)
- Identify computed/derived state (e.g., balance = sum of transactions)
- Note any state machines (e.g., order status: pending → paid → shipped)

**URL patterns:**
- What is the base path? (e.g., `/v1`, `/api/v2`, bare `/`)
- How are resources nested? (e.g., `/accounts/{id}/external_accounts`)
- Are IDs in path or query params?

**ID format:**
- Prefixed? (e.g., `cus_`, `msg_`, `usr_`)
- UUID?
- Numeric/sequential?
- Random alphanumeric?

**Webhooks:**
- Does the service send webhooks?
- What is the signing scheme? (HMAC-SHA256, timestamp + signature, custom)
- What event types exist?
- What is the webhook payload format?

### Phase 3: Store Implementation

#### `internal/store/types.go`

Define domain types that mirror the API's response schemas.

**Rules:**
- Use the EXACT same JSON field names the real API returns
- Add `json` struct tags matching the API's casing (camelCase, snake_case, etc.)
- Use pointer types for optional/nullable fields
- Use `map[string]string` for metadata/custom fields
- Include all fields the SDK expects in responses, even rarely-used ones
- Match the real API's timestamp format (Unix epoch int64, ISO string, etc.)

```go
package store

// Match the real API's types exactly.
// Example for a service using snake_case JSON:
type Contact struct {
    ID        string            `json:"id"`
    Email     string            `json:"email"`
    FirstName *string           `json:"first_name,omitempty"`
    LastName  *string           `json:"last_name,omitempty"`
    Phone     *string           `json:"phone,omitempty"`
    Metadata  map[string]string `json:"metadata,omitempty"`
    CreatedAt int64             `json:"created_at"`
    UpdatedAt int64             `json:"updated_at"`
}
```

#### `internal/store/memory.go`

Implement `MemoryStore` using the generic `pkgstore.Store[T]`.

**Mandatory interface — every MemoryStore MUST implement `admin.StateStore`:**

```go
type StateStore interface {
    Snapshot() any
    LoadState(data []byte) error
    Reset()
}
```

**Implementation pattern:**

```go
package store

import (
    "encoding/json"
    pkgstore "github.com/wondertwin-ai/twinkit/store"
)

type MemoryStore struct {
    Contacts  *pkgstore.Store[Contact]
    Messages  *pkgstore.Store[Message]
    Clock     *pkgstore.Clock
}

func New() *MemoryStore {
    return &MemoryStore{
        Contacts: pkgstore.New[Contact]("con"),    // Prefix matches service's ID format
        Messages: pkgstore.New[Message]("msg"),
        Clock:    pkgstore.NewClock(),
    }
}

// --- admin.StateStore implementation ---

type stateSnapshot struct {
    Contacts map[string]Contact `json:"contacts"`
    Messages map[string]Message `json:"messages"`
}

func (s *MemoryStore) Snapshot() any {
    return stateSnapshot{
        Contacts: s.Contacts.Snapshot(),
        Messages: s.Messages.Snapshot(),
    }
}

func (s *MemoryStore) LoadState(data []byte) error {
    var snap stateSnapshot
    if err := json.Unmarshal(data, &snap); err != nil {
        return err
    }
    if snap.Contacts != nil {
        s.Contacts.LoadSnapshot(snap.Contacts)
    }
    if snap.Messages != nil {
        s.Messages.LoadSnapshot(snap.Messages)
    }
    return nil
}

func (s *MemoryStore) Reset() {
    s.Contacts.Reset()
    s.Messages.Reset()
    s.Clock.Reset()
}
```

**Rules:**
- One `pkgstore.Store[T]` per entity type
- Prefix passed to `pkgstore.New[T]()` should match the service's ID prefix format
- `stateSnapshot` struct field names become the JSON keys in seed data files
- Always nil-check snapshot fields in `LoadState()` to support partial seeding
- Always reset the Clock in `Reset()`
- Add domain-specific helper methods as needed (e.g., `GetBalance()`, `FindByEmail()`)

### Phase 4: Router and Handlers

#### `internal/api/router.go`

**Handler struct pattern:**

```go
package api

import (
    "github.com/go-chi/chi/v5"
    "github.com/wondertwin-ai/twinkit/twincore"
    "github.com/{org}/twin-{name}/internal/store"
)

type Handler struct {
    store *store.MemoryStore
    mw    *twincore.Middleware
    // Add dispatcher *webhook.Dispatcher if the service has webhooks
}

func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
    return &Handler{store: s, mw: mw}
}
```

**Routes() method — mount on chi.Router:**

```go
func (h *Handler) Routes(r chi.Router) {
    r.Route("/v1", func(r chi.Router) {     // Match the real API's base path
        r.Use(h.authMiddleware)              // Auth check
        r.Use(h.mw.FaultInjection)           // Enable fault injection

        // Resource: Contacts
        r.Get("/contacts", h.ListContacts)
        r.Post("/contacts", h.CreateContact)
        r.Get("/contacts/{id}", h.GetContact)
        r.Patch("/contacts/{id}", h.UpdateContact)
        r.Delete("/contacts/{id}", h.DeleteContact)

        // Resource: Messages
        r.Post("/messages", h.SendMessage)
        r.Get("/messages/{id}", h.GetMessage)
        r.Get("/messages", h.ListMessages)
    })
}
```

**Auth middleware — accept any valid-format credential:**

```go
func (h *Handler) authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Check that auth header is present, but accept any value
        auth := r.Header.Get("Authorization")
        if auth == "" {
            // Return error matching the real API's 401 format
            twincore.Error(w, http.StatusUnauthorized, "missing authorization")
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

**Rules:**
- Use `chi.Router` for routing (the shared library depends on chi)
- Match the real service's URL patterns EXACTLY as the SDK constructs them
- Include version prefixes if the real API uses them
- Apply `h.authMiddleware` and `h.mw.FaultInjection` inside the route group
- Group routes by resource, matching the order they appear in the API docs

#### `internal/api/handlers_{resource}.go`

One file per resource group. Each handler follows this pattern:

```go
// CREATE
func (h *Handler) CreateContact(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request body (JSON or form-encoded, matching the real API)
    var req struct {
        Email     string            `json:"email"`
        FirstName *string           `json:"first_name,omitempty"`
        LastName  *string           `json:"last_name,omitempty"`
        Metadata  map[string]string `json:"metadata,omitempty"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        twincore.Error(w, http.StatusBadRequest, "invalid request body")
        return
    }

    // 2. Validate required fields
    if req.Email == "" {
        // Use the real API's error format
        twincore.Error(w, http.StatusBadRequest, "email is required")
        return
    }

    // 3. Create entity with generated ID
    id := h.store.Contacts.NextID()
    now := h.store.Clock.Now()
    contact := store.Contact{
        ID:        id,
        Email:     req.Email,
        FirstName: req.FirstName,
        LastName:  req.LastName,
        Metadata:  req.Metadata,
        CreatedAt: now.Unix(),
        UpdatedAt: now.Unix(),
    }

    // 4. Persist
    h.store.Contacts.Set(id, contact)

    // 5. Emit webhook event if applicable
    // h.emitEvent("contact.created", contact)

    // 6. Respond with the real API's response format and status code
    twincore.JSON(w, http.StatusCreated, contact)
}

// GET
func (h *Handler) GetContact(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    contact, ok := h.store.Contacts.Get(id)
    if !ok {
        twincore.Error(w, http.StatusNotFound, "contact not found")
        return
    }
    twincore.JSON(w, http.StatusOK, contact)
}

// LIST with pagination
func (h *Handler) ListContacts(w http.ResponseWriter, r *http.Request) {
    cursor := r.URL.Query().Get("cursor")       // Match the real API's param name
    limitStr := r.URL.Query().Get("limit")
    limit := 25                                  // Match the real API's default
    if limitStr != "" {
        limit, _ = strconv.Atoi(limitStr)
    }

    page := h.store.Contacts.Paginate(cursor, limit)
    twincore.JSON(w, http.StatusOK, page)        // Page struct matches Stripe-style pagination
}

// UPDATE
func (h *Handler) UpdateContact(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    contact, ok := h.store.Contacts.Get(id)
    if !ok {
        twincore.Error(w, http.StatusNotFound, "contact not found")
        return
    }

    var req struct {
        Email     *string           `json:"email,omitempty"`
        FirstName *string           `json:"first_name,omitempty"`
        LastName  *string           `json:"last_name,omitempty"`
        Metadata  map[string]string `json:"metadata,omitempty"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        twincore.Error(w, http.StatusBadRequest, "invalid request body")
        return
    }

    // Apply partial updates
    if req.Email != nil {
        contact.Email = *req.Email
    }
    if req.FirstName != nil {
        contact.FirstName = req.FirstName
    }
    // ... remaining fields

    contact.UpdatedAt = h.store.Clock.Now().Unix()
    h.store.Contacts.Set(id, contact)

    twincore.JSON(w, http.StatusOK, contact)
}

// DELETE
func (h *Handler) DeleteContact(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    if !h.store.Contacts.Delete(id) {
        twincore.Error(w, http.StatusNotFound, "contact not found")
        return
    }
    twincore.JSON(w, http.StatusOK, map[string]any{
        "id":      id,
        "deleted": true,
    })
}
```

**Rules for handlers:**
- Parse requests in the EXACT format the SDK sends (JSON, form-encoded, etc.)
- For form-encoded APIs (like Stripe), use a `parseFormOrJSON()` helper
- Validate required fields and return errors in the real service's error format
- Use `h.store.{Resource}.NextID()` for ID generation
- Use `h.store.Clock.Now()` for timestamps (supports simulated time)
- Match the real service's HTTP status codes exactly (201 for create, 200 for update, etc.)
- Match the real service's response body format exactly
- Implement real business logic — not just CRUD:
  - Enforce constraints (e.g., can't send message without verified sender)
  - Update related entities (e.g., creating a transfer updates balance)
  - Transition state machines (e.g., order: pending → confirmed → shipped)

#### `internal/api/helpers.go` (optional but recommended)

Common helpers for the twin's handlers:

```go
package api

// For form-encoded APIs (like Stripe):
func parseFormOrJSON(r *http.Request) {
    if r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
        r.ParseForm()
    }
}

// For emitting webhook events:
func (h *Handler) emitEvent(eventType string, obj any) {
    if h.dispatcher != nil {
        payload := map[string]any{"object": obj}
        h.dispatcher.Enqueue(eventType, payload)
    }
}
```

### Phase 5: Webhook Support (if applicable)

Only implement if the target service sends webhooks.

#### `internal/webhook/signer.go`

Implement the `webhook.Signer` interface:

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

// Sign implements webhook.Signer.
// Return a map of header name → value that will be set on the webhook POST.
func (s *ServiceSigner) Sign(payload []byte, secret string) map[string]string {
    // Implement the real service's signing scheme.
    // Example: HMAC-SHA256 with timestamp
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

**Update the Handler struct** to include the dispatcher:

```go
type Handler struct {
    store      *store.MemoryStore
    dispatcher *webhook.Dispatcher
    mw         *twincore.Middleware
}
```

### Phase 6: Entry Point

#### `cmd/twin-{name}/main.go`

Follow this EXACT bootstrap pattern:

```go
package main

import (
    "os"

    // Shared WonderTwin packages (from twinkit)
    "github.com/wondertwin-ai/twinkit/admin"
    "github.com/wondertwin-ai/twinkit/twincore"

    // Twin-specific packages
    "github.com/{org}/twin-{name}/internal/api"
    "github.com/{org}/twin-{name}/internal/store"
)

func main() {
    // 1. Parse flags — provides --port, --verbose, --seed-file, --webhook-url, etc.
    cfg := twincore.ParseFlags("twin-{name}")
    if cfg.Port == 0 {
        cfg.Port = {default_port}   // Pick a unique default port
    }

    // 2. Create twin server (sets up middleware stack)
    twin := twincore.New(cfg)

    // 3. Create store
    memStore := store.New()

    // 4. Create API handler and register routes
    apiHandler := api.NewHandler(memStore, twin.Middleware())
    apiHandler.Routes(twin.Router)

    // 5. Create admin handler and register /admin/* routes
    //    This provides: /admin/health, /admin/reset, /admin/state,
    //    /admin/fault/*, /admin/time/*, /admin/webhooks/flush,
    //    /admin/config (GET/PUT), /admin/quirks (GET/PUT/DELETE)
    adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
    // Optionally wire in config and quirk providers:
    // adminHandler.SetConfigProvider(myConfigProvider)
    // adminHandler.SetQuirkStore(myQuirkStore)
    adminHandler.Routes(twin.Router)

    // 6. Load seed data if provided via --seed-file flag
    if cfg.SeedFile != "" {
        data, err := os.ReadFile(cfg.SeedFile)
        if err == nil {
            memStore.LoadState(data)
        }
    }

    // 7. Start server (blocks until SIGTERM)
    twin.Serve()
}
```

**For twins WITH webhooks, insert between steps 3 and 4:**

```go
    // 4a. Set up webhook dispatcher
    dispatcher := pkgwebhook.NewDispatcher(pkgwebhook.Config{
        URL:         cfg.WebhookURL,
        Secret:      "whsec_test_default",    // Default test secret
        Signer:      twinwebhook.NewServiceSigner(),
        Logger:      twin.Logger,
        EventPrefix: "{evt_prefix}",           // Match the service's event ID prefix
        AutoDeliver: cfg.WebhookURL != "",
    })

    // 4b. Create API handler WITH dispatcher
    apiHandler := api.NewHandler(memStore, dispatcher, twin.Middleware())
    apiHandler.Routes(twin.Router)

    // 5. Create admin handler with webhook flusher
    adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
    adminHandler.SetFlusher(dispatcher)        // Enables POST /admin/webhooks/flush
    adminHandler.Routes(twin.Router)
```

### Phase 7: Tests

#### `internal/api/handlers_test.go`

Write tests using the shared `twinkit/testutil` package:

```go
package api_test

import (
    "net/http/httptest"
    "testing"

    "github.com/wondertwin-ai/twinkit/admin"
    "github.com/wondertwin-ai/twinkit/testutil"
    "github.com/wondertwin-ai/twinkit/twincore"
    "github.com/{org}/twin-{name}/internal/api"
    "github.com/{org}/twin-{name}/internal/store"
)

func setupTestServer(t *testing.T) (*testutil.TwinClient, *testutil.AdminClient) {
    cfg := &twincore.Config{Name: "twin-{name}-test", Verbose: false}
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

func TestCreateContact(t *testing.T) {
    tc, ac := setupTestServer(t)

    // Reset state
    ac.Reset().AssertStatus(200)

    // Create
    resp := tc.Post("/v1/contacts", map[string]any{
        "email": "test@example.com",
        "first_name": "Test",
    })
    resp.AssertStatus(201)

    result := resp.JSONMap()
    if result["email"] != "test@example.com" {
        t.Errorf("expected email test@example.com, got %v", result["email"])
    }
    if result["id"] == nil || result["id"] == "" {
        t.Error("expected non-empty id")
    }
}

func TestListContacts_Pagination(t *testing.T) {
    tc, ac := setupTestServer(t)
    ac.Reset().AssertStatus(200)

    // Create multiple contacts
    for i := 0; i < 5; i++ {
        tc.Post("/v1/contacts", map[string]any{
            "email": fmt.Sprintf("user%d@example.com", i),
        }).AssertStatus(201)
    }

    // List with limit
    resp := tc.Get("/v1/contacts?limit=2")
    resp.AssertStatus(200)

    result := resp.JSONMap()
    data := result["data"].([]any)
    if len(data) != 2 {
        t.Errorf("expected 2 results, got %d", len(data))
    }
}

func TestAdminResetClearsState(t *testing.T) {
    tc, ac := setupTestServer(t)

    // Create data
    tc.Post("/v1/contacts", map[string]any{"email": "test@example.com"}).AssertStatus(201)

    // Reset
    ac.Reset().AssertStatus(200)

    // Verify empty
    resp := tc.Get("/v1/contacts")
    resp.AssertStatus(200)
    result := resp.JSONMap()
    data := result["data"].([]any)
    if len(data) != 0 {
        t.Errorf("expected 0 results after reset, got %d", len(data))
    }
}
```

**Test coverage requirements:**
- At minimum: create, get, list, update, delete for each resource
- Pagination test with multiple items
- Admin reset clears all state
- Admin seed loads fixtures correctly
- 404 for nonexistent resources
- 401 for missing auth header
- Required field validation returns appropriate errors

### Phase 8: Go Module

#### `go.mod`

```
module github.com/{org}/twin-{name}

go 1.25.7

require (
    github.com/go-chi/chi/v5 v5.2.1
    github.com/wondertwin-ai/twinkit v0.1.0
)
```

Twin repos depend on `twinkit` as a normal Go module — no `replace` directives.

### Phase 9: Arazzo Workflow Generation

After implementing handlers, review all endpoints and identify multi-step workflows that span multiple resources or require sequenced operations. Produce an Arazzo 1.0.1 JSON file at `workflows/{name}.arazzo.json`.

**When to produce workflows:**

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
            "payload": {
              "email": "$inputs.recipient_email"
            }
          },
          "successCriteria": [
            { "condition": "$statusCode == 201" }
          ],
          "outputs": {
            "contact_id": "$response.body.id"
          }
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
          "outputs": {
            "message_id": "$response.body.id"
          }
        }
      ]
    }
  ]
}
```

**Rules for Arazzo generation:**

- Each workflow MUST have a descriptive `workflowId` (kebab-case)
- Steps MUST reference `operationId` values that match the twin's handlers
- Use runtime expressions (`$response.body.*`, `$statusCode`, `$inputs.*`) to wire steps together
- Include `successCriteria` for every step (at minimum, the expected status code)
- If no OpenAPI spec exists, use `operationPath` with method and path instead of `operationId`

**Update provenance after generating:**

Add the `arazzo` source to `provenance.json`:

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

### Phase 10: Scenario Scaffolding

After Arazzo generation, produce a starter scenario JSON file at `scenarios/basic.json`. This file validates against `schemas/scenario.schema.json` and provides baseline test coverage for `wt test`.

**The starter scenario MUST include:**

1. A health check step (verifying `/admin/health`)
2. A state reset step (calling `/admin/reset`)
3. One complete CRUD cycle per primary resource (create, get, update, list, delete)
4. Variable capture between steps (e.g., capturing the created resource ID for subsequent get/update/delete)

**Starter scenario example:**

```json
{
  "name": "Basic CRUD - {Service}",
  "description": "Health check and CRUD cycle for primary {Service} resources.",
  "setup": {
    "reset": ["{name}"]
  },
  "variables": {
    "auth_token": "Bearer sk_test_123"
  },
  "steps": [
    {
      "name": "Health check",
      "request": {
        "method": "GET",
        "url": "{{base_url}}/admin/health"
      },
      "assert": {
        "status": 200,
        "body": {
          "$.status": "ok"
        }
      }
    },
    {
      "name": "Reset state",
      "request": {
        "method": "POST",
        "url": "{{base_url}}/admin/reset"
      },
      "assert": {
        "status": 200
      }
    },
    {
      "name": "Create contact",
      "request": {
        "method": "POST",
        "url": "{{base_url}}/v1/contacts",
        "headers": {
          "Authorization": "{{auth_token}}"
        },
        "body": {
          "email": "test@example.com",
          "first_name": "Test"
        }
      },
      "capture": {
        "contact_id": "$.id"
      },
      "assert": {
        "status": 201,
        "body": {
          "$.email": "test@example.com"
        }
      }
    },
    {
      "name": "Get contact",
      "request": {
        "method": "GET",
        "url": "{{base_url}}/v1/contacts/{{contact_id}}",
        "headers": {
          "Authorization": "{{auth_token}}"
        }
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
        "headers": {
          "Authorization": "{{auth_token}}"
        },
        "body": {
          "first_name": "Updated"
        }
      },
      "assert": {
        "status": 200,
        "body": {
          "$.first_name": "Updated"
        }
      }
    },
    {
      "name": "List contacts",
      "request": {
        "method": "GET",
        "url": "{{base_url}}/v1/contacts?limit=10",
        "headers": {
          "Authorization": "{{auth_token}}"
        }
      },
      "assert": {
        "status": 200
      }
    },
    {
      "name": "Delete contact",
      "request": {
        "method": "DELETE",
        "url": "{{base_url}}/v1/contacts/{{contact_id}}",
        "headers": {
          "Authorization": "{{auth_token}}"
        }
      },
      "assert": {
        "status": 200,
        "body": {
          "$.deleted": true
        }
      }
    }
  ]
}
```

**Rules for scenario scaffolding:**

- Use the v2 JSON format matching `schemas/scenario.schema.json`
- Always start with health check and reset steps
- Capture IDs from create responses and reuse them in subsequent steps via `{{variable}}` syntax
- Include at least one assertion per step (status code at minimum, body assertions preferred)
- If Arazzo workflows were generated, reference the workflow file in the `workflow` field for complex multi-step scenarios

### Phase 11: Local Testing with `wt`

Before publishing, validate the twin works end-to-end using the `wt` CLI. This is the primary development workflow — fully offline, no registry needed.

**1. Build the twin locally:**

```bash
go build -o ./bin/twin-{name} ./cmd/twin-{name}/
# or use the Makefile:
make build
```

**2. Add to your project's `wondertwin.yaml`:**

```yaml
twins:
  {name}:
    binary: ./path/to/twin-{name}/bin/twin-{name}
    port: {port}
    # seed: ./fixtures/seed.json    # optional seed data
```

The `binary:` field supports relative paths — they resolve against the `wondertwin.yaml` location.

**3. Run the full offline workflow:**

```bash
wt up        # Start the twin
wt status    # Verify it's healthy
wt test      # Run test scenarios against it
wt down      # Stop when done
```

**4. Run the starter scenario** generated in Phase 10:

```bash
wt test scenarios/basic.json
```

Or write additional scenario files in `scenarios/` using the JSON format described in Phase 10, validated against `schemas/scenario.schema.json`.

**5. Run conformance to validate admin API contract:**

```bash
wt conformance ./bin/twin-{name} --port 9999
```

This validates all 8 standard checks: health, reset, state POST/GET, fault injection, time advance, and clean shutdown.

**6. Iterate:**

```bash
# Make code changes, then:
make build && wt down && wt up && wt test
```

### Phase 12: Publish (Optional)

Once the twin passes local testing and conformance, publish it to make it installable via `wt install`.

**For public twins (community contribution):**

1. Push a version tag to trigger CI:
   ```bash
   git tag v0.1.0
   git push --tags
   ```
2. CI builds cross-platform binaries and creates a GitHub Release
3. Open a PR to `wondertwin-ai/registry` to add the twin entry
4. Registry CI runs conformance against the released binary
5. After merge, the twin is installable: `wt install {name}@latest`

**For private twins:**

1. Same tag-push workflow for building releases
2. Maintain your own `registry.yaml` in a private repo
3. Configure `wt registry add` to point to your private registry (Phase 2)
4. Or skip the registry entirely — use `binary:` paths in manifests

**Release workflow (`.github/workflows/release.yml`):**

The template includes a release workflow that:
1. Reads `twin.yaml` for metadata
2. Cross-compiles for 5 platforms (darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64)
3. Generates SHA256 checksums
4. Creates a GitHub Release with binaries
5. Sends a `repository_dispatch` to the registry repo with the release payload

---

## Development Workflow Summary

The recommended workflow emphasizes **offline-first local development**:

```
 1. Analyze sources and record provenance   (Phase 0)
 2. Set up project from template            (Phase 1)
 3. Analyze the target SDK and service      (Phase 2)
 4. Implement store, handlers, tests        (Phases 3-8)
 5. Generate Arazzo workflows               (Phase 9)
 6. Scaffold starter scenarios              (Phase 10)
 7. Build and test locally with wt          (Phase 11) ← Primary loop
 8. Publish to registry when ready          (Phase 12) ← Optional
```

For private/internal twins, Phase 12 is entirely optional. The `binary:` field in `wondertwin.yaml` supports any local path, so you can develop and use twins without ever publishing them.

---

## Checklist

Before considering a twin complete, verify:

**Source code and structure:**
- [ ] Follows exact directory structure: `cmd/`, `internal/api/`, `internal/store/`
- [ ] `twin.yaml` has correct name, description, category, SDK package/version, and default port
- [ ] `go.mod` uses `require github.com/wondertwin-ai/twinkit` (no `replace` directives)
- [ ] `MemoryStore` implements `admin.StateStore` (Snapshot, LoadState, Reset)
- [ ] All routes match the real service's URL patterns exactly
- [ ] Request parsing matches what the SDK sends (JSON vs form-encoded)
- [ ] Response format matches the real service's envelope and field names
- [ ] Error responses match the real service's error format
- [ ] ID generation matches the real service's ID format and prefix
- [ ] Timestamps use `store.Clock.Now()` (not `time.Now()`)
- [ ] Pagination matches the real service's pagination pattern
- [ ] Auth middleware validates header presence (accepts any value)
- [ ] `main.go` follows the standard bootstrap pattern
- [ ] Admin routes are registered via `admin.NewHandler().Routes()`
- [ ] Handler tests cover CRUD operations, pagination, reset, and auth
- [ ] No hardcoded ports (uses `twincore.ParseFlags()`)

**Pipeline artifacts:**
- [ ] `twin-manifest.json` is present and validates against `schemas/twin-manifest.schema.json`
- [ ] `provenance.json` is present and validates against `schemas/provenance.schema.json`
- [ ] `workflows/{name}.arazzo.json` is present with at least one workflow (if multi-step sequences exist)
- [ ] `scenarios/basic.json` is present and validates against `schemas/scenario.schema.json`
- [ ] Manifest `service_surface` fields are populated (auth pattern, webhook support, resource count)
- [ ] Manifest `coverage` fields reflect actual implementation status
- [ ] Provenance `sources` accurately records what was used during generation

**Webhooks (if applicable):**
- [ ] Signer implements `webhook.Signer`, dispatcher integrated
- [ ] `adminHandler.SetFlusher(dispatcher)` called

**Validation:**
- [ ] Passes `wt conformance` (all 8 checks)
- [ ] Local `wt up` + `wt test` workflow works end-to-end
- [ ] Starter scenario passes: `wt test scenarios/basic.json`

## Fallback Guidance

Not every service provides the same quality of documentation or tooling. Here is how to proceed when the ideal sources are unavailable.

### No OpenAPI spec available

When the service does not publish an OpenAPI spec:

1. **Derive the service surface from SDK analysis.** Read the SDK client source code to extract endpoints, request/response shapes, and authentication patterns. This is often more accurate than docs anyway, since the SDK is what users actually call.
2. **Set `service_surface.openapi_spec.available` to `false`** in the manifest.
3. **Set `sources.openapi.origin` to `"derived"`** in provenance, or omit the `openapi` source entirely.
4. **Use `operationPath` instead of `operationId`** in Arazzo workflows, since there is no spec to reference operation IDs from. Example: `"operationPath": "POST /v1/contacts"`.
5. **Note the limitation** in the manifest description so consumers know the twin was built from SDK analysis alone.

### DeepWiki has not indexed the SDK repo

When DeepWiki is unavailable for the target SDK:

1. **Clone the SDK repo directly** and read the source code, README, examples, and test files.
2. **Set `sources.sdk_analysis.method` to `"direct_repo"`** in provenance.
3. **Set `sources.sdk_analysis.fallback_used` to `true`** in provenance.
4. **Focus on test files** -- SDK integration tests are the best source of truth for expected request/response shapes and multi-step workflows.
5. **Check the SDK's changelog or release notes** for recent behavioral changes.

### Poorly documented SDK

When the SDK lacks documentation, has sparse examples, or has inconsistent behavior:

1. **Note coverage limitations honestly** in the manifest. Set `coverage.estimated_coverage_pct` to reflect what was actually verified, not what was guessed.
2. **Add entries to `coverage.resources_not_implemented`** for resources that could not be confidently implemented.
3. **Set `generation.sources_used.manual_docs` to `true`** if you relied on non-SDK documentation (blog posts, community guides, etc.).
4. **Document discovered quirks** using `schemas/quirk.schema.json` format and register them via `/admin/quirks` if the twin implements the `QuirkStore` interface. Quirks can be toggled at runtime via `PUT /admin/quirks/{quirk_id}` and `DELETE /admin/quirks/{quirk_id}`.
5. **Use runtime configuration** via `/admin/config` (`GET` to read, `PUT` to update) to allow consumers to adjust twin behavior for edge cases that may vary between SDK versions.

## Common Mistakes to Avoid

1. **Using `time.Now()` instead of `store.Clock.Now()`** — breaks simulated time
2. **Using `net/http` ServeMux instead of chi** — shared middleware depends on chi
3. **Forgetting FaultInjection middleware** — must be applied inside route group with `r.Use(h.mw.FaultInjection)`
4. **Returning wrong error format** — each service has its own error envelope, match it exactly
5. **Hardcoding the port** — always use `twincore.ParseFlags()` and allow `--port` override
6. **Forgetting to register admin routes** — every twin MUST mount `admin.Handler.Routes()`
7. **Using `http.StatusOK` for creates** — check what the real service returns (often 201)
8. **Skipping `omitempty` on optional JSON fields** — SDK clients may break on unexpected null fields
9. **Not nil-checking in `LoadState()`** — partial seed data should work
10. **Using `replace` directives in `go.mod`** — twins depend on published `twinkit` versions, not local paths
11. **Skipping `twin.yaml`** — required for release automation and registry metadata
12. **Not running `wt conformance`** — conformance pass is mandatory for registry listing
13. **Skipping `twin-manifest.json` or `provenance.json`** — both are required pipeline artifacts; validate against their schemas in `schemas/`
14. **Using `api_surface` instead of `service_surface`** — the manifest schema uses `service_surface`
15. **Forgetting to update provenance after Arazzo generation** — add the `arazzo` source entry with `origin` and `sha256`
16. **Writing scenarios in YAML instead of JSON** — the v2 scenario format uses JSON, validated against `schemas/scenario.schema.json`
17. **Omitting health check and reset steps from starter scenarios** — every scenario should begin with these steps
