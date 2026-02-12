# SKILL: WonderTwin Twin Generator

## Purpose

Generate a complete, production-ready WonderTwin behavioral API twin from a third-party service's public API documentation and/or SDK reference. The output is a self-contained Go module that behaviorally clones the target API — maintaining state, implementing business logic, and targeting compatibility with the service's official SDK client libraries.

## Prerequisites

Before using this skill, ensure you have access to:

1. The WonderTwin shared libraries (`pkg/twincore`, `pkg/store`, `pkg/admin`, `pkg/webhook`, `pkg/testutil`)
2. The target service's public API documentation or SDK reference
3. Optionally, the official SDK client library source code for compatibility verification

## Inputs

The user will provide:

- **Service name**: The SaaS service to twin (e.g., "Stripe", "SendGrid", "Auth0")
- **API documentation**: URL or document containing API reference
- **SDK target** (optional): Official SDK client library for compatibility (e.g., `github.com/stripe/stripe-go`)
- **Scope** (optional): Specific API resources to prioritize, or "full coverage"
- **Known quirks** (optional): Undocumented behaviors or edge cases to encode

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
│   │   ├── handlers_{resource}.go    # One file per API resource group
│   │   ├── helpers.go                # Request parsing, response formatting helpers
│   │   └── handlers_test.go          # Handler tests
│   └── store/
│       ├── types.go                  # Domain structs with JSON tags
│       └── memory.go                 # MemoryStore implementing admin.StateStore
├── go.mod
└── go.sum
```

If the service has webhooks, add:

```
│   └── webhook/
│       └── signer.go                 # Service-specific webhook signature implementation
```

---

## Process

### Phase 1: API Analysis

Before writing any code, analyze the target API and produce a plan. Document answers to ALL of the following:

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
- List all API resources (e.g., Users, Messages, Contacts)
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

### Phase 2: Store Implementation

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
    pkgstore "github.com/saltwyk/saltwyk-sim/pkg/store"  // Will become wondertwin pkg path
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

### Phase 3: Router and Handlers

#### `internal/api/router.go`

**Handler struct pattern:**

```go
package api

import (
    "github.com/go-chi/chi/v5"
    "github.com/saltwyk/saltwyk-sim/pkg/twincore"
    "github.com/saltwyk/saltwyk-sim/twin-{name}/internal/store"
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
- Match the real API's URL patterns EXACTLY as the SDK constructs them
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
- Validate required fields and return errors in the real API's error format
- Use `h.store.{Resource}.NextID()` for ID generation
- Use `h.store.Clock.Now()` for timestamps (supports simulated time)
- Match the real API's HTTP status codes exactly (201 for create, 200 for update, etc.)
- Match the real API's response body format exactly
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

### Phase 4: Webhook Support (if applicable)

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

### Phase 5: Entry Point

#### `cmd/twin-{name}/main.go`

Follow this EXACT bootstrap pattern:

```go
package main

import (
    "os"

    // Shared WonderTwin packages
    "github.com/saltwyk/saltwyk-sim/pkg/admin"
    "github.com/saltwyk/saltwyk-sim/pkg/twincore"

    // Twin-specific packages
    "github.com/saltwyk/saltwyk-sim/twin-{name}/internal/api"
    "github.com/saltwyk/saltwyk-sim/twin-{name}/internal/store"
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
    adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
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

### Phase 6: Tests

#### `internal/api/handlers_test.go`

Write tests using the shared `pkg/testutil` package:

```go
package api_test

import (
    "net/http/httptest"
    "testing"

    "github.com/saltwyk/saltwyk-sim/pkg/admin"
    "github.com/saltwyk/saltwyk-sim/pkg/testutil"
    "github.com/saltwyk/saltwyk-sim/pkg/twincore"
    "github.com/saltwyk/saltwyk-sim/twin-{name}/internal/api"
    "github.com/saltwyk/saltwyk-sim/twin-{name}/internal/store"
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

### Phase 7: Go Module

#### `go.mod`

```
module github.com/saltwyk/saltwyk-sim/twin-{name}

go 1.25.7

require (
    github.com/go-chi/chi/v5 v5.2.1
    github.com/saltwyk/saltwyk-sim/pkg v0.0.0
)
```

Note: The module path will change to `github.com/wondertwin-ai/wondertwin/twin-{name}` when twins are migrated from saltwyk-sim to the wondertwin repo. Use whatever module path matches the current repo structure.

---

## Checklist

Before considering a twin complete, verify:

- [ ] Follows exact directory structure: `cmd/`, `internal/api/`, `internal/store/`
- [ ] `MemoryStore` implements `admin.StateStore` (Snapshot, LoadState, Reset)
- [ ] All routes match the real API's URL patterns exactly
- [ ] Request parsing matches what the SDK sends (JSON vs form-encoded)
- [ ] Response format matches the real API's envelope and field names
- [ ] Error responses match the real API's error format
- [ ] ID generation matches the real API's ID format and prefix
- [ ] Timestamps use `store.Clock.Now()` (not `time.Now()`)
- [ ] Pagination matches the real API's pagination pattern
- [ ] Auth middleware validates header presence (accepts any value)
- [ ] `main.go` follows the standard bootstrap pattern
- [ ] Admin routes are registered via `admin.NewHandler().Routes()`
- [ ] Handler tests cover CRUD operations, pagination, reset, and auth
- [ ] `go.mod` uses correct module path for the repo
- [ ] No hardcoded ports (uses `twincore.ParseFlags()`)
- [ ] If webhooks: Signer implements `webhook.Signer`, dispatcher integrated
- [ ] If webhooks: `adminHandler.SetFlusher(dispatcher)` called

## Common Mistakes to Avoid

1. **Using `time.Now()` instead of `store.Clock.Now()`** — breaks simulated time
2. **Using `net/http` ServeMux instead of chi** — shared middleware depends on chi
3. **Forgetting FaultInjection middleware** — must be applied inside route group with `r.Use(h.mw.FaultInjection)`
4. **Returning wrong error format** — each service has its own error envelope, match it exactly
5. **Hardcoding the port** — always use `twincore.ParseFlags()` and allow `--port` override
6. **Forgetting to register admin routes** — every twin MUST mount `admin.Handler.Routes()`
7. **Using `http.StatusOK` for creates** — check what the real API returns (often 201)
8. **Skipping `omitempty` on optional JSON fields** — SDK clients may break on unexpected null fields
9. **Not nil-checking in `LoadState()`** — partial seed data should work
10. **Importing the wrong module path** — match whatever repo the twin lives in
