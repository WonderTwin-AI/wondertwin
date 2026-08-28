package api_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func setupPostHog(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-posthog-test"}
	twin := twincore.New(cfg)
	handler := api.NewHandler(memStore, twin.Middleware())
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)
	return srv, tc
}

// --- Capture Tests ---

func TestCaptureEvent(t *testing.T) {
	_, tc := setupPostHog(t)

	resp := tc.Post("/capture", map[string]any{
		"api_key":     "phc_test_key",
		"event":       "page_view",
		"distinct_id": "user_123",
		"properties": map[string]any{
			"$current_url": "https://example.com/home",
		},
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["status"] != float64(1) {
		t.Errorf("expected status=1, got %v", m["status"])
	}

	// Verify event was stored via admin
	resp = tc.Get("/admin/events?event=page_view")
	resp.AssertStatus(200)
	adminM := resp.JSONMap()
	events, ok := adminM["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected 1 event, got %v", adminM["events"])
	}
	evt := events[0].(map[string]any)
	if evt["event"] != "page_view" {
		t.Errorf("expected event=page_view, got %v", evt["event"])
	}
	if evt["distinct_id"] != "user_123" {
		t.Errorf("expected distinct_id=user_123, got %v", evt["distinct_id"])
	}
}

func TestCaptureEventMissingEvent(t *testing.T) {
	_, tc := setupPostHog(t)

	resp := tc.Post("/capture", map[string]any{
		"api_key":     "phc_test_key",
		"distinct_id": "user_123",
	})
	resp.AssertStatus(400)
	resp.AssertBodyContains("event field is required")
}

func TestBatchCapture(t *testing.T) {
	_, tc := setupPostHog(t)

	resp := tc.Post("/batch", map[string]any{
		"api_key": "phc_test_key",
		"batch": []map[string]any{
			{"event": "signup", "distinct_id": "user_1"},
			{"event": "login", "distinct_id": "user_2"},
			{"event": "purchase", "distinct_id": "user_1"},
		},
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["status"] != float64(1) {
		t.Errorf("expected status=1, got %v", m["status"])
	}

	// Verify all events stored
	resp = tc.Get("/admin/events")
	resp.AssertStatus(200)
	adminM := resp.JSONMap()
	events := adminM["events"].([]any)
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}
}

// --- Identify Tests ---

func TestIdentifyViaCapture(t *testing.T) {
	_, tc := setupPostHog(t)

	resp := tc.Post("/capture", map[string]any{
		"api_key":     "phc_test_key",
		"event":       "$identify",
		"distinct_id": "user_123",
		"properties": map[string]any{
			"$set": map[string]any{
				"email": "test@example.com",
				"name":  "Test User",
			},
		},
	})
	resp.AssertStatus(200)

	// Verify person was created
	resp = tc.Get("/admin/persons?distinct_id=user_123")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	persons := m["persons"].([]any)
	if len(persons) != 1 {
		t.Fatalf("expected 1 person, got %d", len(persons))
	}
	person := persons[0].(map[string]any)
	if person["distinct_id"] != "user_123" {
		t.Errorf("expected distinct_id=user_123, got %v", person["distinct_id"])
	}
	props := person["properties"].(map[string]any)
	if props["email"] != "test@example.com" {
		t.Errorf("expected email=test@example.com, got %v", props["email"])
	}

	// Verify event was also stored
	resp = tc.Get("/admin/events?event=$identify")
	resp.AssertStatus(200)
	evtM := resp.JSONMap()
	events := evtM["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("expected 1 $identify event, got %d", len(events))
	}
}

func TestIdentifyMergesProperties(t *testing.T) {
	_, tc := setupPostHog(t)

	// First identify
	tc.Post("/capture", map[string]any{
		"api_key": "phc_test_key", "event": "$identify", "distinct_id": "user_456",
		"properties": map[string]any{"$set": map[string]any{"email": "a@b.com", "plan": "free"}},
	}).AssertStatus(200)

	// Second identify with updated property
	tc.Post("/capture", map[string]any{
		"api_key": "phc_test_key", "event": "$identify", "distinct_id": "user_456",
		"properties": map[string]any{"$set": map[string]any{"plan": "pro"}},
	}).AssertStatus(200)

	resp := tc.Get("/admin/persons?distinct_id=user_456")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	persons := m["persons"].([]any)
	if len(persons) != 1 {
		t.Fatalf("expected 1 person, got %d", len(persons))
	}
	props := persons[0].(map[string]any)["properties"].(map[string]any)
	if props["plan"] != "pro" {
		t.Errorf("expected plan=pro, got %v", props["plan"])
	}
	if props["email"] != "a@b.com" {
		t.Errorf("expected email preserved, got %v", props["email"])
	}
}

// --- Alias Tests ---

func TestAliasViaCapture(t *testing.T) {
	_, tc := setupPostHog(t)

	resp := tc.Post("/capture", map[string]any{
		"api_key":     "phc_test_key",
		"event":       "$create_alias",
		"distinct_id": "user_123",
		"properties": map[string]any{
			"alias": "anon_456",
		},
	})
	resp.AssertStatus(200)

	// Verify alias was created
	resp = tc.Get("/admin/aliases")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	aliases := m["aliases"].([]any)
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}
	alias := aliases[0].(map[string]any)
	if alias["alias"] != "anon_456" {
		t.Errorf("expected alias=anon_456, got %v", alias["alias"])
	}
	if alias["distinct_id"] != "user_123" {
		t.Errorf("expected distinct_id=user_123, got %v", alias["distinct_id"])
	}
}

// --- Group Identify Tests ---

func TestGroupIdentifyViaCapture(t *testing.T) {
	_, tc := setupPostHog(t)

	resp := tc.Post("/capture", map[string]any{
		"api_key":     "phc_test_key",
		"event":       "$groupidentify",
		"distinct_id": "user_123",
		"properties": map[string]any{
			"$group_type": "company",
			"$group_key":  "acme_inc",
			"$group_set": map[string]any{
				"name":     "Acme Inc",
				"industry": "Technology",
			},
		},
	})
	resp.AssertStatus(200)

	// Verify group was created
	resp = tc.Get("/admin/groups?type=company")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	groups := m["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	group := groups[0].(map[string]any)
	if group["type"] != "company" {
		t.Errorf("expected type=company, got %v", group["type"])
	}
	if group["key"] != "acme_inc" {
		t.Errorf("expected key=acme_inc, got %v", group["key"])
	}
	props := group["properties"].(map[string]any)
	if props["name"] != "Acme Inc" {
		t.Errorf("expected name=Acme Inc, got %v", props["name"])
	}
}

// --- Exception Tests ---

func TestExceptionViaCapture(t *testing.T) {
	_, tc := setupPostHog(t)

	resp := tc.Post("/capture", map[string]any{
		"api_key":     "phc_test_key",
		"event":       "$exception",
		"distinct_id": "user_123",
		"properties": map[string]any{
			"$exception_message": "null pointer dereference",
			"$exception_type":    "NilPointerError",
		},
	})
	resp.AssertStatus(200)

	// Verify exception event was stored
	resp = tc.Get("/admin/events?event=$exception")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	events := m["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("expected 1 exception event, got %d", len(events))
	}
	evt := events[0].(map[string]any)
	props := evt["properties"].(map[string]any)
	if props["$exception_message"] != "null pointer dereference" {
		t.Errorf("expected exception message, got %v", props["$exception_message"])
	}
}

// --- Decide Tests ---

func TestDecideNoFlags(t *testing.T) {
	_, tc := setupPostHog(t)

	resp := tc.Post("/decide", map[string]any{
		"api_key":     "phc_test_key",
		"distinct_id": "user_123",
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	flags, ok := m["featureFlags"].(map[string]any)
	if !ok {
		t.Fatal("expected featureFlags in response")
	}
	if len(flags) != 0 {
		t.Errorf("expected 0 flags, got %d", len(flags))
	}
}

func TestDecideWithFlags(t *testing.T) {
	_, tc := setupPostHog(t)

	// Set feature flags via admin
	tc.Post("/admin/feature-flags", []map[string]any{
		{"key": "new-checkout", "enabled": true},
		{"key": "dark-mode", "enabled": false},
		{"key": "variant-test", "enabled": true, "variant": "control"},
	}).AssertStatus(200)

	resp := tc.Post("/decide", map[string]any{
		"api_key":     "phc_test_key",
		"distinct_id": "user_123",
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	flags := m["featureFlags"].(map[string]any)

	if flags["new-checkout"] != true {
		t.Errorf("expected new-checkout=true, got %v", flags["new-checkout"])
	}
	if flags["dark-mode"] != false {
		t.Errorf("expected dark-mode=false, got %v", flags["dark-mode"])
	}
	if flags["variant-test"] != "control" {
		t.Errorf("expected variant-test=control, got %v", flags["variant-test"])
	}
}

func TestDecideWithPayloads(t *testing.T) {
	_, tc := setupPostHog(t)

	tc.Post("/admin/feature-flags", []map[string]any{
		{"key": "banner", "enabled": true, "payload": `{"text":"Welcome!"}`},
		{"key": "no-payload", "enabled": true},
	}).AssertStatus(200)

	resp := tc.Post("/decide", map[string]any{
		"api_key":     "phc_test_key",
		"distinct_id": "user_123",
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()

	payloads := m["featureFlagPayloads"].(map[string]any)
	if payloads["banner"] != `{"text":"Welcome!"}` {
		t.Errorf("expected banner payload, got %v", payloads["banner"])
	}
	if payloads["no-payload"] != nil {
		t.Errorf("expected nil payload for no-payload flag, got %v", payloads["no-payload"])
	}

	// Verify additional response fields
	if m["requestId"] == nil {
		t.Error("expected requestId in response")
	}
	if m["errorsWhileComputingFlags"] != false {
		t.Errorf("expected errorsWhileComputingFlags=false, got %v", m["errorsWhileComputingFlags"])
	}
}

// --- Flags v2 Tests ---

func TestFlagsEndpoint(t *testing.T) {
	_, tc := setupPostHog(t)

	tc.Post("/admin/feature-flags", []map[string]any{
		{"key": "beta-feature", "enabled": true, "payload": `{"color":"blue"}`},
		{"key": "disabled-flag", "enabled": false},
	}).AssertStatus(200)

	resp := tc.Post("/flags", map[string]any{
		"token":       "phc_test_key",
		"distinct_id": "user_123",
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()

	flags := m["featureFlags"].(map[string]any)
	if flags["beta-feature"] != true {
		t.Errorf("expected beta-feature=true, got %v", flags["beta-feature"])
	}
	if flags["disabled-flag"] != false {
		t.Errorf("expected disabled-flag=false, got %v", flags["disabled-flag"])
	}

	payloads := m["featureFlagPayloads"].(map[string]any)
	if payloads["beta-feature"] != `{"color":"blue"}` {
		t.Errorf("expected payload for beta-feature, got %v", payloads["beta-feature"])
	}
}

// --- Local Evaluation Tests ---

func TestLocalEvaluation(t *testing.T) {
	_, tc := setupPostHog(t)

	tc.Post("/admin/feature-flags", []map[string]any{
		{"key": "local-flag", "enabled": true},
		{"key": "variant-flag", "enabled": true, "variant": "test"},
	}).AssertStatus(200)

	resp := tc.DoWithHeaders("GET", "/api/feature_flag/local_evaluation", nil, map[string]string{
		"Authorization": "Bearer phx_personal_api_key",
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()

	flagDefs := m["flags"].([]any)
	if len(flagDefs) != 2 {
		t.Fatalf("expected 2 flag definitions, got %d", len(flagDefs))
	}

	// Check that flags have expected structure
	for _, f := range flagDefs {
		def := f.(map[string]any)
		if def["key"] == nil {
			t.Error("expected key field in flag definition")
		}
		if def["filters"] == nil {
			t.Error("expected filters field in flag definition")
		}
	}
}

func TestLocalEvaluationNoAuth(t *testing.T) {
	_, tc := setupPostHog(t)

	resp := tc.Get("/api/feature_flag/local_evaluation")
	resp.AssertStatus(401)
}

// --- Admin Tests ---

func TestAdminListEventsFilter(t *testing.T) {
	_, tc := setupPostHog(t)

	tc.Post("/capture", map[string]any{
		"api_key": "phc_test_key", "event": "click", "distinct_id": "u1",
	}).AssertStatus(200)
	tc.Post("/capture", map[string]any{
		"api_key": "phc_test_key", "event": "view", "distinct_id": "u2",
	}).AssertStatus(200)

	resp := tc.Get("/admin/events?distinct_id=u1")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	events := m["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("expected 1 event for u1, got %d", len(events))
	}
}

func TestAdminSetAndGetFeatureFlags(t *testing.T) {
	_, tc := setupPostHog(t)

	tc.Post("/admin/feature-flags", []map[string]any{
		{"key": "beta", "enabled": true},
	}).AssertStatus(200)

	resp := tc.Get("/admin/feature-flags")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	beta, ok := m["beta"].(map[string]any)
	if !ok {
		t.Fatal("expected 'beta' flag in response")
	}
	if beta["enabled"] != true {
		t.Errorf("expected beta enabled=true, got %v", beta["enabled"])
	}
}

func TestAdminReset(t *testing.T) {
	_, tc := setupPostHog(t)

	tc.Post("/capture", map[string]any{
		"api_key": "phc_test_key", "event": "test", "distinct_id": "u1",
	}).AssertStatus(200)

	tc.Post("/admin/reset", nil).AssertStatus(200)

	resp := tc.Get("/admin/events")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	events := m["events"]
	if events != nil {
		if arr, ok := events.([]any); ok && len(arr) > 0 {
			t.Errorf("expected 0 events after reset, got %d", len(arr))
		}
	}
}

func TestAdminHealth(t *testing.T) {
	_, tc := setupPostHog(t)
	tc.Get("/admin/health").AssertStatus(200)
}

func TestAdminListPersons(t *testing.T) {
	_, tc := setupPostHog(t)

	// Create a person via identify
	tc.Post("/capture", map[string]any{
		"api_key": "phc_test_key", "event": "$identify", "distinct_id": "person_1",
		"properties": map[string]any{"$set": map[string]any{"name": "Alice"}},
	}).AssertStatus(200)

	resp := tc.Get("/admin/persons")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["total"] != float64(1) {
		t.Errorf("expected total=1, got %v", m["total"])
	}
}

func TestAdminListGroups(t *testing.T) {
	_, tc := setupPostHog(t)

	tc.Post("/capture", map[string]any{
		"api_key": "phc_test_key", "event": "$groupidentify", "distinct_id": "u1",
		"properties": map[string]any{
			"$group_type": "org", "$group_key": "org_1",
			"$group_set": map[string]any{"name": "Org One"},
		},
	}).AssertStatus(200)

	resp := tc.Get("/admin/groups?type=org")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["total"] != float64(1) {
		t.Errorf("expected total=1, got %v", m["total"])
	}
}

// --- Project-key enforcement ---

// TestIngestRequiresProjectKey covers the ingest endpoints that PostHog gates
// on a project key. Real PostHog answers a missing key with 401
// authentication_error / invalid_api_key; this twin accepts any non-empty key,
// having no project registry to validate against.
func TestIngestRequiresProjectKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		body map[string]any
	}{
		{"capture", "/capture", map[string]any{"event": "page_view", "distinct_id": "u1"}},
		{"e", "/e", map[string]any{"event": "page_view", "distinct_id": "u1"}},
		{"batch", "/batch", map[string]any{"batch": []any{
			map[string]any{"event": "page_view", "distinct_id": "u1"},
		}}},
		{"decide", "/decide", map[string]any{"distinct_id": "u1"}},
		{"flags", "/flags", map[string]any{"distinct_id": "u1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, c := setupPostHog(t)
			resp := c.Post(tc.path, tc.body)
			resp.AssertStatus(401)

			m := resp.JSONMap()
			if m["type"] != "authentication_error" {
				t.Errorf("type = %v, want authentication_error", m["type"])
			}
			if m["code"] != "invalid_api_key" {
				t.Errorf("code = %v, want invalid_api_key", m["code"])
			}
		})
	}
}

// TestIngestRejectsOversizedBody covers the ceiling on the ingest read path.
// The body is consumed before any project-key check, so an unauthenticated
// caller must not be able to make the twin allocate without bound.
func TestIngestRejectsOversizedBody(t *testing.T) {
	_, c := setupPostHog(t)
	huge := strings.Repeat("a", (5<<20)+1024)
	resp := c.Post("/capture", map[string]any{
		"api_key": "phc_k", "event": "e", "distinct_id": "u1", "properties": map[string]any{"pad": huge},
	})
	resp.AssertStatus(400)
}

// TestDecideAcceptsFormWrappedBody pins /decide to the same body handling as
// /capture and /flags — posthog-js sends it form-wrapped as well.
func TestDecideAcceptsFormWrappedBody(t *testing.T) {
	_, c := setupPostHog(t)
	c.PostForm("/decide", map[string]string{
		"data": `{"distinct_id":"u1","token":"phc_wrapped"}`,
	}).AssertStatus(200)
}

// TestIngestAcceptsKeyFromEverySource pins the four places PostHog takes the
// project key. Each must satisfy the check on its own.
func TestIngestAcceptsKeyFromEverySource(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{"event": "page_view", "distinct_id": "u1"}
	}

	t.Run("body api_key", func(t *testing.T) {
		_, c := setupPostHog(t)
		b := base()
		b["api_key"] = "phc_body"
		c.Post("/capture", b).AssertStatus(200)
	})

	t.Run("properties $token", func(t *testing.T) {
		_, c := setupPostHog(t)
		b := base()
		b["properties"] = map[string]any{"$token": "phc_js_sdk"}
		c.Post("/capture", b).AssertStatus(200)
	})

	for _, h := range []struct{ name, header string }{
		{"X-PostHog-Api-Key header", "X-PostHog-Api-Key"},
		{"Authorization header", "Authorization"},
	} {
		t.Run(h.name, func(t *testing.T) {
			_, c := setupPostHog(t)
			c.DoWithHeaders("POST", "/capture", base(), map[string]string{h.header: "phc_hdr"}).AssertStatus(200)
		})
	}

	t.Run("batch per-event api_key", func(t *testing.T) {
		// PostHog accepts a batch whose key rides on the first event rather than
		// the envelope, so the admission check scans the events too.
		_, c := setupPostHog(t)
		c.Post("/batch", map[string]any{"batch": []any{
			map[string]any{"api_key": "phc_per_event", "event": "page_view", "distinct_id": "u1"},
		}}).AssertStatus(200)
	})

	t.Run("batch key on a later event", func(t *testing.T) {
		// The key may ride on any event in the batch, not only the first.
		_, c := setupPostHog(t)
		c.Post("/batch", map[string]any{"batch": []any{
			map[string]any{"event": "page_view", "distinct_id": "u1"},
			map[string]any{"api_key": "phc_later", "event": "click", "distinct_id": "u2"},
		}}).AssertStatus(200)
	})

	t.Run("flags accepts a token", func(t *testing.T) {
		_, c := setupPostHog(t)
		c.Post("/flags", map[string]any{"distinct_id": "u1", "token": "phc_flags"}).AssertStatus(200)
	})

	t.Run("form-encoded api_key sibling of data", func(t *testing.T) {
		// posthog-js posts data=<json> with api_key as a sibling form field.
		_, c := setupPostHog(t)
		c.PostForm("/capture", map[string]string{
			"api_key": "phc_form",
			"data":    `{"event":"page_view","distinct_id":"u1"}`,
		}).AssertStatus(200)
	})

	t.Run("flags with a form-wrapped body", func(t *testing.T) {
		// posthog-js posts /flags form-wrapped too; a plain JSON decode would
		// see nothing and 401 a request that carried a token.
		_, c := setupPostHog(t)
		c.PostForm("/flags", map[string]string{
			"data": `{"distinct_id":"u1","token":"phc_wrapped"}`,
		}).AssertStatus(200)
	})

	t.Run("api_key query param", func(t *testing.T) {
		_, c := setupPostHog(t)
		c.Post("/capture?api_key=phc_query", base()).AssertStatus(200)
	})
}
