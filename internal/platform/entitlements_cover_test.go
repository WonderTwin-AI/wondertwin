package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestEntitlementsCover_HappyPath(t *testing.T) {
	t.Parallel()
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/org_t/entitlements/cover" {
			t.Errorf("path: got %s", r.URL.Path)
		}
		want := "stripe,hubspot"
		if got := r.URL.Query().Get("twins"); got != want {
			t.Errorf("twins query: want %q, got %q", want, got)
		}
		_ = json.NewEncoder(w).Encode(EntitlementsCoverResponse{
			Covered: []string{"hubspot", "stripe"},
			Missing: []MissingTwin{},
		})
	})

	got, err := client.EntitlementsCover(context.Background(), "org_t",
		[]string{"stripe", "hubspot"})
	if err != nil {
		t.Fatalf("EntitlementsCover: %v", err)
	}
	if len(got.Covered) != 2 || got.Covered[0] != "hubspot" {
		t.Errorf("covered: got %+v", got.Covered)
	}
}

func TestEntitlementsCover_MissingDecodesReasonCodes(t *testing.T) {
	t.Parallel()
	_, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(EntitlementsCoverResponse{
			Covered: []string{"stripe"},
			Missing: []MissingTwin{
				{TwinName: "hubspot", ReasonCode: "not_entitled"},
				{TwinName: "linear", ReasonCode: "entitlement_cancelled"},
			},
		})
	})
	got, err := client.EntitlementsCover(context.Background(), "org_t",
		[]string{"stripe", "hubspot", "linear"})
	if err != nil {
		t.Fatalf("EntitlementsCover: %v", err)
	}
	if len(got.Missing) != 2 {
		t.Fatalf("missing count: got %d", len(got.Missing))
	}
	if got.Missing[0].ReasonCode != "not_entitled" {
		t.Errorf("first reason: %s", got.Missing[0].ReasonCode)
	}
}

func TestEntitlementsCover_DedupesAndStripsEmpty(t *testing.T) {
	t.Parallel()
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("twins")
		// Order preserved, dedup'd, no empties.
		if got != "stripe,hubspot" {
			t.Errorf("twins query: got %q", got)
		}
		_ = json.NewEncoder(w).Encode(EntitlementsCoverResponse{
			Covered: []string{}, Missing: []MissingTwin{},
		})
	})
	_, err := client.EntitlementsCover(context.Background(), "org_t",
		[]string{"stripe", "", "stripe", " hubspot ", "  "})
	if err != nil {
		t.Fatalf("EntitlementsCover: %v", err)
	}
}

func TestEntitlementsCover_EmptyTwinsListSkipsServerCall(t *testing.T) {
	t.Parallel()
	_, client := newTestServer(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server should not be called for empty twins list")
	})
	got, err := client.EntitlementsCover(context.Background(), "org_t", nil)
	if err != nil {
		t.Fatalf("empty input: %v", err)
	}
	if len(got.Covered) != 0 || len(got.Missing) != 0 {
		t.Errorf("empty input: want empty response, got %+v", got)
	}
}

func TestEntitlementsCover_HTTP5xxBubbles(t *testing.T) {
	t.Parallel()
	_, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	_, err := client.EntitlementsCover(context.Background(), "org_t",
		[]string{"stripe"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("want HTTP 500 error, got %v", err)
	}
}

func TestEntitlementsCover_MissingAccountIDIsClientError(t *testing.T) {
	t.Parallel()
	client := New("http://unused", "key")
	_, err := client.EntitlementsCover(context.Background(), "",
		[]string{"stripe"})
	if err == nil || !strings.Contains(err.Error(), "accountID") {
		t.Errorf("want accountID error, got %v", err)
	}
}

func TestEntitlementsCover_AccountIDIsURLEscaped(t *testing.T) {
	t.Parallel()
	var seenPath string
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		_ = json.NewEncoder(w).Encode(EntitlementsCoverResponse{})
	})
	// account_id with a space (artificial; the real ones don't, but
	// the path escaping is defensive).
	_, err := client.EntitlementsCover(context.Background(),
		"org with space", []string{"stripe"})
	if err != nil {
		t.Fatalf("EntitlementsCover: %v", err)
	}
	if !strings.Contains(seenPath, url.PathEscape("org with space")) {
		t.Errorf("account_id not URL-escaped in path: %s", seenPath)
	}
}
