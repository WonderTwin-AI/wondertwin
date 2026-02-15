package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Helper: create a test server with typical endpoints
// ---------------------------------------------------------------------------

func newTestServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /items", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]string{{"id": "1"}, {"id": "2"}})
	})

	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"id": r.PathValue("id")})
	})

	mux.HandleFunc("POST /items", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		body["id"] = "new_1"
		json.NewEncoder(w).Encode(body)
	})

	mux.HandleFunc("PATCH /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"id": r.PathValue("id"), "updated": "true"})
	})

	mux.HandleFunc("DELETE /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /form", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := map[string]string{}
		for k, v := range r.Form {
			result[k] = v[0]
		}
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("GET /echo-headers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		headers := map[string]string{}
		for k := range r.Header {
			headers[k] = r.Header.Get(k)
		}
		json.NewEncoder(w).Encode(headers)
	})

	// Admin-like endpoints
	mux.HandleFunc("GET /admin/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /admin/reset", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "reset"})
	})

	mux.HandleFunc("GET /admin/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"items": []string{"a", "b"}})
	})

	mux.HandleFunc("POST /admin/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "loaded"})
	})

	mux.HandleFunc("POST /admin/fault/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "injected"})
	})

	mux.HandleFunc("DELETE /admin/fault/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
	})

	mux.HandleFunc("GET /admin/requests", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]string{{"method": "GET", "path": "/items"}})
	})

	mux.HandleFunc("POST /admin/webhooks/flush", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "flushed"})
	})

	mux.HandleFunc("POST /admin/time/advance", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "advanced"})
	})

	return httptest.NewServer(mux)
}

// ---------------------------------------------------------------------------
// TwinClient tests
// ---------------------------------------------------------------------------

func TestNewTwinClient(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	if tc.BaseURL != srv.URL {
		t.Errorf("expected BaseURL=%s, got %s", srv.URL, tc.BaseURL)
	}
}

func TestNewTwinClientURL(t *testing.T) {
	tc := NewTwinClientURL(t, "http://localhost:8080/")
	if tc.BaseURL != "http://localhost:8080" {
		t.Errorf("expected trailing slash trimmed, got %s", tc.BaseURL)
	}
}

func TestTwinClientGet(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	resp := tc.Get("/items")

	resp.AssertStatus(http.StatusOK)
	var items []map[string]string
	resp.JSON(&items)
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestTwinClientPost(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	resp := tc.Post("/items", map[string]string{"name": "test"})

	resp.AssertStatus(http.StatusCreated)
	m := resp.JSONMap()
	if m["id"] != "new_1" {
		t.Errorf("expected id=new_1, got %v", m["id"])
	}
}

func TestTwinClientPatch(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	resp := tc.Patch("/items/42", map[string]string{"name": "updated"})

	resp.AssertStatus(http.StatusOK)
	m := resp.JSONMap()
	if m["updated"] != "true" {
		t.Errorf("expected updated=true, got %v", m["updated"])
	}
}

func TestTwinClientDelete(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	resp := tc.Delete("/items/42")

	resp.AssertStatus(http.StatusNoContent)
}

func TestTwinClientPostForm(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	resp := tc.PostForm("/form", map[string]string{"key": "val"})

	resp.AssertStatus(http.StatusOK)
	m := resp.JSONMap()
	if m["key"] != "val" {
		t.Errorf("expected key=val, got %v", m["key"])
	}
}

func TestTwinClientDoWithHeaders(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	resp := tc.DoWithHeaders("GET", "/echo-headers", nil, map[string]string{
		"X-Custom": "header-value",
	})

	resp.AssertStatus(http.StatusOK)
	m := resp.JSONMap()
	if m["X-Custom"] != "header-value" {
		t.Errorf("expected X-Custom=header-value, got %v", m["X-Custom"])
	}
}

// ---------------------------------------------------------------------------
// Response helpers
// ---------------------------------------------------------------------------

func TestResponseJSON(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	resp := tc.Get("/items")

	var items []map[string]string
	resp.JSON(&items)
	if len(items) == 0 {
		t.Error("expected non-empty items")
	}
}

func TestResponseJSONMap(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	resp := tc.Get("/items/42")

	m := resp.JSONMap()
	if m["id"] != "42" {
		t.Errorf("expected id=42, got %v", m["id"])
	}
}

func TestResponseAssertStatus(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	resp := tc.Get("/items")

	// Should not fail.
	resp.AssertStatus(http.StatusOK)
}

func TestResponseAssertBodyContains(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	resp := tc.Get("/items")

	resp.AssertBodyContains(`"id"`)
}

func TestResponseAssertStatusChaining(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	resp := tc.Get("/items")

	// AssertStatus returns the same Response for chaining.
	chained := resp.AssertStatus(http.StatusOK)
	if chained != resp {
		t.Error("expected AssertStatus to return the same Response for chaining")
	}
}

func TestResponseAssertBodyContainsChaining(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	resp := tc.Get("/items")

	// AssertBodyContains returns the same Response for chaining.
	chained := resp.AssertBodyContains(`"id"`)
	if chained != resp {
		t.Error("expected AssertBodyContains to return the same Response for chaining")
	}
}

// ---------------------------------------------------------------------------
// AdminClient tests
// ---------------------------------------------------------------------------

func TestAdminClientHealth(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	ac := NewAdminClient(tc)

	resp := ac.Health()
	resp.AssertStatus(http.StatusOK)
	m := resp.JSONMap()
	if m["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", m["status"])
	}
}

func TestAdminClientReset(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	ac := NewAdminClient(tc)

	resp := ac.Reset()
	resp.AssertStatus(http.StatusOK)
	resp.AssertBodyContains("reset")
}

func TestAdminClientGetState(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	ac := NewAdminClient(tc)

	resp := ac.GetState()
	resp.AssertStatus(http.StatusOK)
	resp.AssertBodyContains("items")
}

func TestAdminClientLoadState(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	ac := NewAdminClient(tc)

	resp := ac.LoadState(map[string]any{"key": "value"})
	resp.AssertStatus(http.StatusOK)
	resp.AssertBodyContains("loaded")
}

func TestAdminClientInjectFault(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	ac := NewAdminClient(tc)

	resp := ac.InjectFault("/v1/charges", map[string]any{"status_code": 503})
	resp.AssertStatus(http.StatusOK)
	resp.AssertBodyContains("injected")
}

func TestAdminClientRemoveFault(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	ac := NewAdminClient(tc)

	resp := ac.RemoveFault("/test")
	resp.AssertStatus(http.StatusOK)
	resp.AssertBodyContains("removed")
}

func TestAdminClientGetRequests(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	ac := NewAdminClient(tc)

	resp := ac.GetRequests()
	resp.AssertStatus(http.StatusOK)
	resp.AssertBodyContains("method")
}

func TestAdminClientFlushWebhooks(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	ac := NewAdminClient(tc)

	resp := ac.FlushWebhooks()
	resp.AssertStatus(http.StatusOK)
	resp.AssertBodyContains("flushed")
}

func TestAdminClientAdvanceTime(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	ac := NewAdminClient(tc)

	resp := ac.AdvanceTime("1h")
	resp.AssertStatus(http.StatusOK)
	resp.AssertBodyContains("advanced")
}

// ---------------------------------------------------------------------------
// AdminClient with leading slash handling
// ---------------------------------------------------------------------------

func TestAdminClientInjectFaultStripsLeadingSlash(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	ac := NewAdminClient(tc)

	// The InjectFault method should strip the leading "/" from the endpoint
	// so we end up with /admin/fault/v1/charges (not /admin/fault//v1/charges)
	resp := ac.InjectFault("/v1/charges", map[string]any{"status_code": 500})
	resp.AssertStatus(http.StatusOK)
}

func TestAdminClientRemoveFaultStripsLeadingSlash(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	tc := NewTwinClient(t, srv)
	ac := NewAdminClient(tc)

	resp := ac.RemoveFault("/test")
	resp.AssertStatus(http.StatusOK)
}
