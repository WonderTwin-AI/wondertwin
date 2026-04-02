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
	handler := api.NewHandler(memStore, twin.Middleware(), nil, nil)
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)
	return srv, tc
}

var logoHeaders = map[string]string{
	"Authorization": "Bearer pk_test_123",
}

func logoGet(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("GET", path, nil, logoHeaders)
}

// --- Auth Tests ---

func TestAuthTokenQueryParam(t *testing.T) {
	_, tc := setupLogodev(t)
	resp := tc.Get("/example.com?token=test_token_123")
	resp.AssertStatus(200)
}

func TestAuthBearerHeader(t *testing.T) {
	_, tc := setupLogodev(t)
	resp := logoGet(tc, "/example.com")
	resp.AssertStatus(200)
}

func TestAuthRequired(t *testing.T) {
	_, tc := setupLogodev(t)
	resp := tc.Get("/example.com")
	resp.AssertStatus(401)
	resp.AssertBodyContains("unauthorized")
	resp.AssertBodyContains("API token required")
}

// --- Logo Tests ---

func TestGetLogo(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/example.com")
	resp.AssertStatus(200)

	ct := resp.Headers.Get("Content-Type")
	if ct != "image/svg+xml" {
		t.Errorf("expected Content-Type=image/svg+xml, got %s", ct)
	}

	body := string(resp.Body)
	if !strings.Contains(body, "<svg") {
		t.Error("expected SVG content in body")
	}
	if !strings.Contains(body, "EX") {
		t.Error("expected initials 'EX' in SVG")
	}
}

func TestGetLogoCustomSize(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/stripe.com?size=64")
	resp.AssertStatus(200)

	body := string(resp.Body)
	if !strings.Contains(body, `width="64"`) {
		t.Errorf("expected width=64 in SVG, got %s", body)
	}
}

func TestGetLogoGreyscale(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/google.com?greyscale=true")
	resp.AssertStatus(200)
	if !strings.Contains(string(resp.Body), "<svg") {
		t.Error("expected SVG content")
	}
}

func TestGetLogoDeterministic(t *testing.T) {
	_, tc := setupLogodev(t)

	resp1 := logoGet(tc, "/deterministic.com")
	resp2 := logoGet(tc, "/deterministic.com")

	if string(resp1.Body) != string(resp2.Body) {
		t.Error("expected same SVG for same domain")
	}
}

func TestGetLogoDifferentDomains(t *testing.T) {
	_, tc := setupLogodev(t)

	resp1 := logoGet(tc, "/alpha.com")
	resp2 := logoGet(tc, "/beta.com")

	if string(resp1.Body) == string(resp2.Body) {
		t.Error("expected different SVGs for different domains")
	}
}

func TestGetLogoCacheHeaders(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/cached.com")
	resp.AssertStatus(200)

	cc := resp.Headers.Get("Cache-Control")
	if !strings.Contains(cc, "max-age=86400") {
		t.Errorf("expected Cache-Control with max-age=86400, got %s", cc)
	}
}

// --- Custom Logo Tests ---

func TestGetCustomLogoSVG(t *testing.T) {
	_, tc := setupLogodev(t)

	seed := json.RawMessage(`{"custom_logos":{"acme.com":{"content_type":"image/svg+xml","data":"PHN2Zz5hY21lPC9zdmc+"}}}`)
	tc.Post("/admin/state", seed).AssertStatus(200)

	resp := logoGet(tc, "/acme.com")
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

	seed := json.RawMessage(`{"custom_logos":{"img.com":{"content_type":"image/png","data":"iVBORw0KGgo="}}}`)
	tc.Post("/admin/state", seed).AssertStatus(200)

	resp := logoGet(tc, "/img.com")
	resp.AssertStatus(200)

	ct := resp.Headers.Get("Content-Type")
	if ct != "image/png" {
		t.Errorf("expected Content-Type=image/png, got %s", ct)
	}
}

func TestCustomLogoFallbackToPlaceholder(t *testing.T) {
	_, tc := setupLogodev(t)

	seed := json.RawMessage(`{"custom_logos":{"known.com":{"content_type":"image/svg+xml","data":"PHN2Zz48L3N2Zz4="}}}`)
	tc.Post("/admin/state", seed).AssertStatus(200)

	resp := logoGet(tc, "/unknown.com")
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

	resp := logoGet(tc, "/reset-test.com")
	resp.AssertStatus(200)
	if !strings.Contains(string(resp.Body), "RE") {
		t.Error("expected placeholder SVG with initials after reset")
	}
}

// --- Admin Tests ---

func TestAdminListLogos(t *testing.T) {
	_, tc := setupLogodev(t)

	logoGet(tc, "/stripe.com").AssertStatus(200)
	logoGet(tc, "/stripe.com").AssertStatus(200)
	logoGet(tc, "/google.com").AssertStatus(200)

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

func TestAdminListLogosFilterByDomain(t *testing.T) {
	_, tc := setupLogodev(t)

	logoGet(tc, "/stripe.com").AssertStatus(200)
	logoGet(tc, "/google.com").AssertStatus(200)
	logoGet(tc, "/github.com").AssertStatus(200)

	resp := tc.Get("/admin/logos?domain=g")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	domains := m["domains"].(map[string]any)

	if _, ok := domains["stripe.com"]; ok {
		t.Error("stripe.com should be filtered out")
	}
	if _, ok := domains["google.com"]; !ok {
		t.Error("google.com should match filter")
	}
	if _, ok := domains["github.com"]; !ok {
		t.Error("github.com should match filter")
	}
}

func TestAdminGetLogo(t *testing.T) {
	_, tc := setupLogodev(t)

	logoGet(tc, "/test.com").AssertStatus(200)
	logoGet(tc, "/test.com?size=64").AssertStatus(200)

	resp := tc.Get("/admin/logos/test.com")
	resp.AssertStatus(200)
	m := resp.JSONMap()

	if m["domain"] != "test.com" {
		t.Errorf("expected domain=test.com, got %v", m["domain"])
	}
	total, _ := m["total"].(float64)
	if total != 2 {
		t.Errorf("expected 2 requests, got %v", total)
	}
	if m["has_custom"] != false {
		t.Error("expected has_custom=false")
	}
}

func TestAdminGetLogoWithCustom(t *testing.T) {
	_, tc := setupLogodev(t)

	seed := json.RawMessage(`{"custom_logos":{"custom.com":{"content_type":"image/svg+xml","data":"PHN2Zz48L3N2Zz4="}}}`)
	tc.Post("/admin/state", seed).AssertStatus(200)

	resp := tc.Get("/admin/logos/custom.com")
	resp.AssertStatus(200)
	m := resp.JSONMap()

	if m["has_custom"] != true {
		t.Error("expected has_custom=true")
	}
}

// --- Search Tests ---

func TestSearchBrands(t *testing.T) {
	_, tc := setupLogodev(t)

	seed := json.RawMessage(`{"brands":{"brand_001":{"name":"Stripe","domain":"stripe.com","ticker":""},"brand_002":{"name":"Google","domain":"google.com","ticker":"GOOG"},"brand_003":{"name":"Stripe Atlas","domain":"atlas.stripe.com","ticker":""}}}`)
	tc.Post("/admin/state", seed).AssertStatus(200)

	resp := logoGet(tc, "/api/v1/search?q=stripe")
	resp.AssertStatus(200)

	var results []map[string]any
	json.Unmarshal(resp.Body, &results)
	if len(results) != 2 {
		t.Fatalf("expected 2 results for 'stripe', got %d", len(results))
	}
}

func TestSearchBrandsByTicker(t *testing.T) {
	_, tc := setupLogodev(t)

	seed := json.RawMessage(`{"brands":{"brand_001":{"name":"Alphabet","domain":"google.com","ticker":"GOOG"},"brand_002":{"name":"Apple","domain":"apple.com","ticker":"AAPL"}}}`)
	tc.Post("/admin/state", seed).AssertStatus(200)

	resp := logoGet(tc, "/api/v1/search?q=goog")
	resp.AssertStatus(200)

	var results []map[string]any
	json.Unmarshal(resp.Body, &results)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for ticker 'goog', got %d", len(results))
	}
	if results[0]["name"] != "Alphabet" {
		t.Errorf("expected Alphabet, got %v", results[0]["name"])
	}
}

func TestSearchBrandsEmpty(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/api/v1/search?q=nonexistent")
	resp.AssertStatus(200)

	var results []map[string]any
	json.Unmarshal(resp.Body, &results)
	if results != nil {
		t.Errorf("expected null/empty results, got %v", results)
	}
}

func TestSearchBrandsMissingQuery(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/api/v1/search")
	resp.AssertStatus(400)
	resp.AssertBodyContains("bad_request")
}

func TestAdminReset(t *testing.T) {
	_, tc := setupLogodev(t)

	logoGet(tc, "/test.com").AssertStatus(200)

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
