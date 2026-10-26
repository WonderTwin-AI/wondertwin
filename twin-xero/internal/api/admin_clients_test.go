package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Audit finding F10: the client registry the token endpoint checks under strict_auth.

func strictWithClient(t *testing.T) *Handler {
	t.Helper()
	h := newTestHandler(t)
	h.store.StrictAuth.Store(true)
	if rec := do(h, http.MethodPost, "/admin/clients", `{"client_id":"app","client_secret":"s3cret","redirect_uris":["https://app.test/cb"]}`, nil); rec.Code != 201 {
		t.Fatalf("register: %d %s", rec.Code, rec.Body)
	}
	return h
}

func issueCode(t *testing.T, h *Handler, client, redirect string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"client_id": client, "redirect_uri": redirect})
	rec := do(h, http.MethodPost, "/admin/authorization-codes", string(body), nil)
	if rec.Code != 201 {
		t.Fatalf("issue code: %d %s", rec.Code, rec.Body)
	}
	return decode(t, rec)["code"].(string)
}

func TestTheClientAdminRoutes(t *testing.T) {
	h := newTestHandler(t)
	for name, body := range map[string]string{"no secret": `{"client_id":"a"}`, "no id": `{"client_secret":"s"}`, "not JSON": `{`} {
		if rec := do(h, http.MethodPost, "/admin/clients", body, nil); rec.Code != 400 {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	do(h, http.MethodPost, "/admin/clients", `{"client_id":"a","client_secret":"s","redirect_uris":["https://a.test/cb"]}`, nil)
	rec := do(h, http.MethodGet, "/admin/clients", "", nil)
	if rec.Code != 200 || containsAny(rec.Body.String(), `"s"`, "client_secret") {
		t.Errorf("list: %d %s (the secret must not come back)", rec.Code, rec.Body)
	}
	if rec := do(h, http.MethodPost, "/admin/authorization-codes", `{"client_id":"a","redirect_uri":"https://other.test"}`, nil); rec.Code != 400 {
		t.Errorf("an unregistered redirect URI: %d", rec.Code)
	}
	if rec := do(h, http.MethodPost, "/admin/authorization-codes", `{"client_id":"zz","redirect_uri":"x"}`, nil); rec.Code != 404 {
		t.Errorf("an unknown client: %d", rec.Code)
	}
	if rec := do(h, http.MethodDelete, "/admin/clients/a", "", nil); rec.Code != 200 {
		t.Errorf("delete: %d", rec.Code)
	}
	if rec := do(h, http.MethodDelete, "/admin/clients/a", "", nil); rec.Code != 404 {
		t.Errorf("delete again: %d", rec.Code)
	}
}

func TestClientsAndCodesSurviveAnExportAndResetClearsThem(t *testing.T) {
	h := strictWithClient(t)
	issueCode(t, h, "app", "https://app.test/cb")
	state, err := json.Marshal(h.store.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	other := newTestHandler(t)
	if err := other.store.LoadState(state); err != nil {
		t.Fatal(err)
	}
	if _, ok := other.store.Clients.Get("app"); !ok || other.store.AuthCodes.Count() != 1 {
		t.Fatalf("after import: client %v, codes %d", ok, other.store.AuthCodes.Count())
	}
	h.store.Reset()
	if h.store.Clients.Count() != 0 || h.store.AuthCodes.Count() != 0 {
		t.Fatalf("after reset: clients %d, codes %d", h.store.Clients.Count(), h.store.AuthCodes.Count())
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
