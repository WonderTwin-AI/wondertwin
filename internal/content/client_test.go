package content

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchContentSuccess(t *testing.T) {
	payload := ContentPayload{
		Twin:    "stripe",
		Version: "0.1.0",
		Quirks: []QuirkRecord{
			{ID: "STRIPE-Q-001", Service: "stripe", Resource: "customer", Type: "behavioral", Severity: "moderate", Summary: "test quirk"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/content/stripe@0.1.0" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	raw, got, err := client.FetchContent("stripe", "0.1.0")
	if err != nil {
		t.Fatalf("FetchContent: %v", err)
	}
	if raw == nil {
		t.Fatal("expected raw bytes")
	}
	if got.Twin != "stripe" || len(got.Quirks) != 1 {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestFetchContent401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "bad-token")
	_, _, err := client.FetchContent("stripe", "0.1.0")
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestFetchContent404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, _, err := client.FetchContent("nonexistent", "0.0.0")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestFetchContent500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, _, err := client.FetchContent("stripe", "0.1.0")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}
