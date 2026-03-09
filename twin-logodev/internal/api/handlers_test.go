package api_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/store"
)

func setupLogodev(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-logodev-test"}
	twin := twincore.New(cfg)
	handler := api.NewHandler(memStore)
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)
	return srv, tc
}

// --- Logo Tests ---

func TestGetLogo(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := tc.Get("/example.com?token=test_token_123")
	resp.AssertStatus(200)

	// Should return SVG
	ct := resp.Headers.Get("Content-Type")
	if ct != "image/svg+xml" {
		t.Errorf("expected Content-Type=image/svg+xml, got %s", ct)
	}

	body := string(resp.Body)
	if !strings.Contains(body, "<svg") {
		t.Error("expected SVG content in body")
	}
	// Should contain initials from "example"
	if !strings.Contains(body, "EX") {
		t.Error("expected initials 'EX' in SVG")
	}
}

func TestGetLogoTokenRequired(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := tc.Get("/example.com")
	resp.AssertStatus(401)
	resp.AssertBodyContains("token required")
}

func TestGetLogoCustomSize(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := tc.Get("/stripe.com?token=test&size=64")
	resp.AssertStatus(200)

	body := string(resp.Body)
	if !strings.Contains(body, `width="64"`) {
		t.Errorf("expected width=64 in SVG, got %s", body)
	}
}

func TestGetLogoGreyscale(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := tc.Get("/google.com?token=test&greyscale=true")
	resp.AssertStatus(200)
	// Just verify it returns valid SVG (greyscale is visual)
	if !strings.Contains(string(resp.Body), "<svg") {
		t.Error("expected SVG content")
	}
}

func TestGetLogoDeterministic(t *testing.T) {
	_, tc := setupLogodev(t)

	resp1 := tc.Get("/deterministic.com?token=test")
	resp2 := tc.Get("/deterministic.com?token=test")

	if string(resp1.Body) != string(resp2.Body) {
		t.Error("expected same SVG for same domain")
	}
}

func TestGetLogoDifferentDomains(t *testing.T) {
	_, tc := setupLogodev(t)

	resp1 := tc.Get("/alpha.com?token=test")
	resp2 := tc.Get("/beta.com?token=test")

	// Different domains should produce different SVGs (different colors/initials)
	if string(resp1.Body) == string(resp2.Body) {
		t.Error("expected different SVGs for different domains")
	}
}

// --- Custom Logo Tests ---

func TestGetCustomLogoSVG(t *testing.T) {
	_, tc := setupLogodev(t)

	// Load a custom SVG logo via seed data
	seed := json.RawMessage(`{"custom_logos":{"acme.com":{"content_type":"image/svg+xml","data":"PHN2Zz5hY21lPC9zdmc+"}}}`)
	tc.Post("/admin/state", seed).AssertStatus(200)

	resp := tc.Get("/acme.com?token=test")
	resp.AssertStatus(200)

	ct := resp.Headers.Get("Content-Type")
	if ct != "image/svg+xml" {
		t.Errorf("expected Content-Type=image/svg+xml, got %s", ct)
	}
	if string(resp.Body) != "<svg>acme</svg>" {
		t.Errorf("expected custom SVG, got %s", string(resp.Body))
	}
}

func TestGetCustomLogoPNG(t *testing.T) {
	_, tc := setupLogodev(t)

	// Load a custom PNG logo (fake PNG data for test)
	seed := json.RawMessage(`{"custom_logos":{"img.com":{"content_type":"image/png","data":"iVBORw0KGgo="}}}`)
	tc.Post("/admin/state", seed).AssertStatus(200)

	resp := tc.Get("/img.com?token=test")
	resp.AssertStatus(200)

	ct := resp.Headers.Get("Content-Type")
	if ct != "image/png" {
		t.Errorf("expected Content-Type=image/png, got %s", ct)
	}
}

func TestCustomLogoFallbackToPlaceholder(t *testing.T) {
	_, tc := setupLogodev(t)

	// Load custom logo for one domain
	seed := json.RawMessage(`{"custom_logos":{"known.com":{"content_type":"image/svg+xml","data":"PHN2Zz48L3N2Zz4="}}}`)
	tc.Post("/admin/state", seed).AssertStatus(200)

	// Unknown domain should still get a placeholder
	resp := tc.Get("/unknown.com?token=test")
	resp.AssertStatus(200)
	if !strings.Contains(string(resp.Body), "<svg") {
		t.Error("expected placeholder SVG for unknown domain")
	}
}

func TestCustomLogosResetOnReset(t *testing.T) {
	_, tc := setupLogodev(t)

	seed := json.RawMessage(`{"custom_logos":{"reset-test.com":{"content_type":"image/svg+xml","data":"PHN2Zz48L3N2Zz4="}}}`)
	tc.Post("/admin/state", seed).AssertStatus(200)

	tc.Post("/admin/reset", nil).AssertStatus(200)

	// After reset, should get placeholder, not custom logo
	resp := tc.Get("/reset-test.com?token=test")
	resp.AssertStatus(200)
	// Placeholder SVGs contain initials — custom one was just "<svg></svg>"
	if !strings.Contains(string(resp.Body), "RE") {
		t.Error("expected placeholder SVG with initials after reset")
	}
}

// --- Admin Tests ---

func TestAdminListLogos(t *testing.T) {
	_, tc := setupLogodev(t)

	// Make some logo requests
	tc.Get("/stripe.com?token=test").AssertStatus(200)
	tc.Get("/stripe.com?token=test").AssertStatus(200)
	tc.Get("/google.com?token=test").AssertStatus(200)

	resp := tc.Get("/admin/logos")
	resp.AssertStatus(200)
	m := resp.JSONMap()

	domains, ok := m["domains"].(map[string]any)
	if !ok {
		t.Fatal("expected domains map")
	}
	if domains["stripe.com"] != float64(2) {
		t.Errorf("expected stripe.com=2, got %v", domains["stripe.com"])
	}
	if domains["google.com"] != float64(1) {
		t.Errorf("expected google.com=1, got %v", domains["google.com"])
	}

	total, _ := m["total_requests"].(float64)
	if total < 3 {
		t.Errorf("expected total_requests >= 3, got %v", total)
	}
}

func TestAdminReset(t *testing.T) {
	_, tc := setupLogodev(t)

	tc.Get("/test.com?token=test").AssertStatus(200)

	tc.Post("/admin/reset", nil).AssertStatus(200)

	resp := tc.Get("/admin/logos")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	total := m["total_requests"].(float64)
	if total != 0 {
		t.Errorf("expected 0 requests after reset, got %v", total)
	}
}

func TestAdminHealth(t *testing.T) {
	_, tc := setupLogodev(t)
	tc.Get("/admin/health").AssertStatus(200)
}
