package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func setupLogodev(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	cfg := &twincore.Config{Name: "twin-logodev-test"}
	twin := twincore.New(cfg)
	memStore := store.New()
	handler := api.NewHandler(memStore, twin.Middleware())
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

func seedBrands(tc *testutil.TwinClient) {
	seed := json.RawMessage(`{"brands":{
		"brand_001":{"name":"Stripe","domain":"stripe.com","ticker":""},
		"brand_002":{"name":"Google","domain":"google.com","ticker":"GOOG","isin":"US02079K3059"},
		"brand_003":{"name":"Apple","domain":"apple.com","ticker":"AAPL","isin":"US0378331005"},
		"brand_004":{"name":"Stripe Atlas","domain":"atlas.stripe.com","ticker":""},
		"brand_005":{"name":"Bitcoin","domain":"bitcoin.org","crypto":"BTC"},
		"brand_006":{"name":"Ethereum","domain":"ethereum.org","crypto":"ETH"}
	}}`)
	tc.Post("/admin/state", seed).AssertStatus(200)
}

// --- Auth Tests ---

func TestAuthTokenQueryParam(t *testing.T) {
	_, tc := setupLogodev(t)
	resp := tc.Get("/example.com?token=pk_test_123")
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
	if !strings.Contains(string(resp.Body), "EX") {
		t.Error("expected initials 'EX' in SVG")
	}
}

func TestGetLogoCustomSize(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/stripe.com?size=64")
	resp.AssertStatus(200)
	if !strings.Contains(string(resp.Body), `width="64"`) {
		t.Error("expected width=64 in SVG")
	}
}

func TestGetLogoRetina(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/stripe.com?size=64&retina=true")
	resp.AssertStatus(200)
	// Retina doubles: 64 → 128
	if !strings.Contains(string(resp.Body), `width="128"`) {
		t.Error("expected width=128 (retina doubled) in SVG")
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

func TestGetLogoThemeDark(t *testing.T) {
	_, tc := setupLogodev(t)

	normal := logoGet(tc, "/test.com")
	dark := logoGet(tc, "/test.com?theme=dark")

	// Dark theme should produce a different SVG (inverted colors)
	if string(normal.Body) == string(dark.Body) {
		t.Error("expected different SVG for dark theme")
	}
}

func TestGetLogoFallback404(t *testing.T) {
	_, tc := setupLogodev(t)

	// Without fallback=404, unknown domains get a monogram placeholder
	resp := logoGet(tc, "/unknown.com")
	resp.AssertStatus(200)

	// With fallback=404, should get 404
	resp = logoGet(tc, "/unknown.com?fallback=404")
	resp.AssertStatus(404)
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
	if !strings.Contains(resp.Headers.Get("Cache-Control"), "max-age=86400") {
		t.Error("expected Cache-Control with max-age=86400")
	}
}

// --- Lookup by Name/Ticker ---

func TestGetLogoByName(t *testing.T) {
	_, tc := setupLogodev(t)
	seedBrands(tc)

	resp := logoGet(tc, "/name/Stripe")
	resp.AssertStatus(200)
	if !strings.Contains(string(resp.Body), "<svg") {
		t.Error("expected SVG content")
	}
}

func TestGetLogoByNameNotFound(t *testing.T) {
	_, tc := setupLogodev(t)

	// No brands seeded — falls back to name.com
	resp := logoGet(tc, "/name/Unknown")
	resp.AssertStatus(200)
	if !strings.Contains(string(resp.Body), "UN") {
		t.Error("expected initials 'UN' for unknown.com fallback")
	}
}

func TestGetLogoByTicker(t *testing.T) {
	_, tc := setupLogodev(t)
	seedBrands(tc)

	resp := logoGet(tc, "/ticker/GOOG")
	resp.AssertStatus(200)
	if !strings.Contains(string(resp.Body), "<svg") {
		t.Error("expected SVG content")
	}
}

func TestGetLogoByTickerNotFound404(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/ticker/ZZZZ?fallback=404")
	resp.AssertStatus(404)
}

// --- Crypto Lookup ---

func TestGetLogoByCrypto(t *testing.T) {
	_, tc := setupLogodev(t)
	seedBrands(tc)

	resp := logoGet(tc, "/crypto/BTC")
	resp.AssertStatus(200)
	if !strings.Contains(string(resp.Body), "<svg") {
		t.Error("expected SVG content for BTC")
	}
}

func TestGetLogoByCryptoNotFound(t *testing.T) {
	_, tc := setupLogodev(t)

	// No brands seeded — falls back to monogram
	resp := logoGet(tc, "/crypto/DOGE")
	resp.AssertStatus(200)
	if !strings.Contains(string(resp.Body), "DO") {
		t.Error("expected initials 'DO' for DOGE fallback")
	}
}

func TestGetLogoByCryptoNotFound404(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/crypto/DOGE?fallback=404")
	resp.AssertStatus(404)
}

// --- ISIN Lookup ---

func TestGetLogoByISIN(t *testing.T) {
	_, tc := setupLogodev(t)
	seedBrands(tc)

	resp := logoGet(tc, "/isin/US0378331005")
	resp.AssertStatus(200)
	if !strings.Contains(string(resp.Body), "<svg") {
		t.Error("expected SVG content for Apple ISIN")
	}
}

func TestGetLogoByISINNotFound404(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/isin/XX0000000000?fallback=404")
	resp.AssertStatus(404)
}

// --- Custom Logo Tests ---

func TestGetCustomLogoSVG(t *testing.T) {
	_, tc := setupLogodev(t)

	seed := json.RawMessage(`{"custom_logos":{"acme.com":{"content_type":"image/svg+xml","data":"PHN2Zz5hY21lPC9zdmc+"}}}`)
	tc.Post("/admin/state", seed).AssertStatus(200)

	resp := logoGet(tc, "/acme.com")
	resp.AssertStatus(200)
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
	if resp.Headers.Get("Content-Type") != "image/png" {
		t.Error("expected Content-Type=image/png")
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

// --- Custom Logo Upload Tests ---

// rawRequest issues an HTTP request with raw body bytes and a custom Content-Type.
// Used for upload tests where TwinClient's JSON marshaling would corrupt binary data.
func rawRequest(t *testing.T, srv *httptest.Server, method, path, contentType string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func TestPutCustomLogoSVG(t *testing.T) {
	srv, tc := setupLogodev(t)

	body := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><text>real</text></svg>`)
	resp := rawRequest(t, srv, "PUT", "/admin/logos/real.com", "image/svg+xml", body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("upload: got %d", resp.StatusCode)
	}

	got := logoGet(tc, "/real.com")
	got.AssertStatus(200)
	if string(got.Body) != string(body) {
		t.Errorf("expected uploaded SVG, got %s", string(got.Body))
	}
	if got.Headers.Get("Content-Type") != "image/svg+xml" {
		t.Errorf("expected image/svg+xml, got %s", got.Headers.Get("Content-Type"))
	}
}

func TestPutCustomLogoPNG(t *testing.T) {
	srv, tc := setupLogodev(t)

	pngBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x01, 0x02, 0x03}
	resp := rawRequest(t, srv, "PUT", "/admin/logos/png.com", "image/png", pngBytes)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("upload: got %d", resp.StatusCode)
	}

	got := logoGet(tc, "/png.com")
	got.AssertStatus(200)
	if !bytes.Equal(got.Body, pngBytes) {
		t.Errorf("expected exact PNG bytes back, got %d bytes", len(got.Body))
	}
	if got.Headers.Get("Content-Type") != "image/png" {
		t.Errorf("expected image/png, got %s", got.Headers.Get("Content-Type"))
	}
}

func TestPutCustomLogoFormats(t *testing.T) {
	// The twin must accept any content type the source service does.
	// Logo.dev returns SVG/PNG/JPG/WebP; we accept whatever the bridge sends.
	cases := []struct {
		name        string
		contentType string
		body        []byte
	}{
		{"webp", "image/webp", []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50}},
		{"jpeg", "image/jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}},
		{"avif", "image/avif", []byte{0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70, 0x61, 0x76, 0x69, 0x66}},
		{"gif", "image/gif", []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61}},
		{"ico", "image/x-icon", []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			srv, tc := setupLogodev(t)
			domain := tt.name + ".com"
			rawRequest(t, srv, "PUT", "/admin/logos/"+domain, tt.contentType, tt.body).Body.Close()

			got := logoGet(tc, "/"+domain)
			got.AssertStatus(200)
			if got.Headers.Get("Content-Type") != tt.contentType {
				t.Errorf("expected %s, got %s", tt.contentType, got.Headers.Get("Content-Type"))
			}
			if !bytes.Equal(got.Body, tt.body) {
				t.Errorf("body round-trip mismatch")
			}
		})
	}
}

func TestPutCustomLogoReplaces(t *testing.T) {
	srv, tc := setupLogodev(t)

	rawRequest(t, srv, "PUT", "/admin/logos/dup.com", "image/svg+xml", []byte("<svg>v1</svg>")).Body.Close()
	rawRequest(t, srv, "PUT", "/admin/logos/dup.com", "image/svg+xml", []byte("<svg>v2</svg>")).Body.Close()

	got := logoGet(tc, "/dup.com")
	if string(got.Body) != "<svg>v2</svg>" {
		t.Errorf("expected v2, got %s", string(got.Body))
	}
}

func TestPutCustomLogoMissingContentType(t *testing.T) {
	srv, _ := setupLogodev(t)
	resp := rawRequest(t, srv, "PUT", "/admin/logos/x.com", "", []byte("body"))
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPutCustomLogoEmptyBody(t *testing.T) {
	srv, _ := setupLogodev(t)
	resp := rawRequest(t, srv, "PUT", "/admin/logos/x.com", "image/svg+xml", nil)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPutCustomLogoTooLarge(t *testing.T) {
	srv, _ := setupLogodev(t)
	big := make([]byte, 5*1024*1024+1)
	resp := rawRequest(t, srv, "PUT", "/admin/logos/big.com", "image/png", big)
	resp.Body.Close()
	if resp.StatusCode != 413 {
		t.Errorf("expected 413, got %d", resp.StatusCode)
	}
}

func TestDeleteCustomLogo(t *testing.T) {
	srv, tc := setupLogodev(t)

	rawRequest(t, srv, "PUT", "/admin/logos/del.com", "image/svg+xml", []byte("<svg>x</svg>")).Body.Close()
	tc.Delete("/admin/logos/del.com").AssertStatus(204)

	got := logoGet(tc, "/del.com")
	got.AssertStatus(200)
	if !strings.Contains(string(got.Body), "<svg") {
		t.Error("expected fallback placeholder after delete")
	}
	if string(got.Body) == "<svg>x</svg>" {
		t.Error("expected delete to remove custom logo")
	}
}

func TestDeleteCustomLogoMissing(t *testing.T) {
	_, tc := setupLogodev(t)
	tc.Delete("/admin/logos/nonexistent.com").AssertStatus(204)
}

// --- Describe API Tests ---

func TestDescribeBrandSeeded(t *testing.T) {
	_, tc := setupLogodev(t)
	seedBrands(tc)

	resp := logoGet(tc, "/api/v1/describe/google.com")
	resp.AssertStatus(200)
	m := resp.JSONMap()

	if m["name"] != "Google" {
		t.Errorf("expected name=Google, got %v", m["name"])
	}
	if m["domain"] != "google.com" {
		t.Errorf("expected domain=google.com, got %v", m["domain"])
	}
	if m["ticker"] != "GOOG" {
		t.Errorf("expected ticker=GOOG, got %v", m["ticker"])
	}
	if m["colors"] == nil {
		t.Error("expected colors array")
	}
}

func TestDescribeBrandGenerated(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/api/v1/describe/example.com")
	resp.AssertStatus(200)
	m := resp.JSONMap()

	if m["domain"] != "example.com" {
		t.Errorf("expected domain=example.com, got %v", m["domain"])
	}
	// Generated name should be title-cased domain prefix
	if m["name"] != "Example" {
		t.Errorf("expected name=Example, got %v", m["name"])
	}
}

func TestDescribeAuthRequired(t *testing.T) {
	_, tc := setupLogodev(t)
	resp := tc.Get("/api/v1/describe/example.com")
	resp.AssertStatus(401)
}

// --- Search Tests ---

func TestSearchBrandsTypeahead(t *testing.T) {
	_, tc := setupLogodev(t)
	seedBrands(tc)

	// Default strategy is typeahead (prefix match)
	resp := logoGet(tc, "/api/v1/search?q=str")
	resp.AssertStatus(200)

	var results []map[string]any
	json.Unmarshal(resp.Body, &results)
	if len(results) != 2 {
		t.Fatalf("expected 2 results for prefix 'str', got %d", len(results))
	}
}

func TestSearchBrandsMatchStrategy(t *testing.T) {
	_, tc := setupLogodev(t)
	seedBrands(tc)

	// "match" strategy does substring contains
	resp := logoGet(tc, "/api/v1/search?q=ipe&strategy=match")
	resp.AssertStatus(200)

	var results []map[string]any
	json.Unmarshal(resp.Body, &results)
	// "Stripe" and "Stripe Atlas" contain "ipe"
	if len(results) != 2 {
		t.Fatalf("expected 2 results for substring 'ipe', got %d", len(results))
	}
}

func TestSearchBrandsByTicker(t *testing.T) {
	_, tc := setupLogodev(t)
	seedBrands(tc)

	resp := logoGet(tc, "/api/v1/search?q=goog&strategy=match")
	resp.AssertStatus(200)

	var results []map[string]any
	json.Unmarshal(resp.Body, &results)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for ticker 'goog', got %d", len(results))
	}
}

func TestSearchBrandsEmpty(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/api/v1/search?q=nonexistent")
	resp.AssertStatus(200)
}

func TestSearchBrandsMissingQuery(t *testing.T) {
	_, tc := setupLogodev(t)

	resp := logoGet(tc, "/api/v1/search")
	resp.AssertStatus(400)
	resp.AssertBodyContains("bad_request")
}

func TestSearchAuthRequired(t *testing.T) {
	_, tc := setupLogodev(t)
	resp := tc.Get("/api/v1/search?q=test")
	resp.AssertStatus(401)
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

	domains := m["domains"].(map[string]any)
	if domains["stripe.com"] != float64(2) {
		t.Errorf("expected stripe.com=2, got %v", domains["stripe.com"])
	}
}

func TestAdminListLogosFilterByDomain(t *testing.T) {
	_, tc := setupLogodev(t)

	logoGet(tc, "/stripe.com").AssertStatus(200)
	logoGet(tc, "/google.com").AssertStatus(200)

	resp := tc.Get("/admin/logos?domain=google")
	m := resp.JSONMap()
	domains := m["domains"].(map[string]any)

	if _, ok := domains["stripe.com"]; ok {
		t.Error("stripe.com should be filtered out")
	}
	if _, ok := domains["google.com"]; !ok {
		t.Error("google.com should match")
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
	if m["total"].(float64) != 2 {
		t.Errorf("expected 2 requests, got %v", m["total"])
	}
}

func TestAdminReset(t *testing.T) {
	_, tc := setupLogodev(t)

	logoGet(tc, "/test.com").AssertStatus(200)
	tc.Post("/admin/reset", nil).AssertStatus(200)

	resp := tc.Get("/admin/logos")
	m := resp.JSONMap()
	if m["total_requests"].(float64) != 0 {
		t.Error("expected 0 requests after reset")
	}
}

func TestAdminHealth(t *testing.T) {
	_, tc := setupLogodev(t)
	tc.Get("/admin/health").AssertStatus(200)
}
