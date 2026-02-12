package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/pkg/admin"
	"github.com/wondertwin-ai/wondertwin/pkg/testutil"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/store"
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
