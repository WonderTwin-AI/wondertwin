package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newTestRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/content/{spec}", handleGetContent)
	})
	return r
}

func validAuthHeader() string {
	return "Bearer " + makeTestKey("com", "acme", "abcdef")
}

func TestHandlerValidRequest(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/v1/content/stripe@0.1.0", nil)
	req.Header.Set("Authorization", validAuthHeader())
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var payload ContentPayload
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.Twin != "stripe" {
		t.Fatalf("expected twin=stripe, got %q", payload.Twin)
	}
	if len(payload.Quirks) == 0 {
		t.Fatal("expected at least one quirk")
	}
}

func TestHandlerNoAuth(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/v1/content/stripe@0.1.0", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandlerInvalidToken(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/v1/content/stripe@0.1.0", nil)
	req.Header.Set("Authorization", "Bearer invalid-key")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandlerNotFound(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/v1/content/nonexistent@9.9.9", nil)
	req.Header.Set("Authorization", validAuthHeader())
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerBadSpec(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/v1/content/no-version-here", nil)
	req.Header.Set("Authorization", validAuthHeader())
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestParseSpec(t *testing.T) {
	tests := []struct {
		spec    string
		twin    string
		version string
		ok      bool
	}{
		{"stripe@0.1.0", "stripe", "0.1.0", true},
		{"my-twin@1.2.3", "my-twin", "1.2.3", true},
		{"bad-spec", "", "", false},
		{"@0.1.0", "", "", false},
		{"twin@", "", "", false},
	}

	for _, tt := range tests {
		twin, version, ok := parseSpec(tt.spec)
		if ok != tt.ok || twin != tt.twin || version != tt.version {
			t.Errorf("parseSpec(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.spec, twin, version, ok, tt.twin, tt.version, tt.ok)
		}
	}
}
