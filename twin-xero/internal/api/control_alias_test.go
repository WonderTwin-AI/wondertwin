package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
)

// controlPlane serves the admin plane and the legacy aliases the way main wires them.
func controlPlane(h *Handler) http.Handler {
	r := chi.NewRouter()
	a := admin.NewHandler(h.store, h.mw, h.store.Clock)
	a.Routes(r)
	plane := chi.NewRouter()
	a.Routes(plane)
	MountControlAliases(r, plane)
	return r
}

func callPlane(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestLegacyHealthPathAnswersWhatTheAdminHealthPathAnswers(t *testing.T) {
	h := newTestHandler(t)
	plane := controlPlane(h)

	legacy, native := callPlane(plane, http.MethodGet, "/_xero/health"), callPlane(plane, http.MethodGet, "/admin/health")
	if legacy.Code != 200 || legacy.Body.String() != native.Body.String() || decode(t, legacy)["status"] != "ok" {
		t.Fatalf("legacy = %d %s, admin = %d %s", legacy.Code, legacy.Body, native.Code, native.Body)
	}
}

func TestLegacyResetPathClearsTheStoreAsTheAdminResetPathDoes(t *testing.T) {
	for _, path := range []string{"/_xero/reset", "/admin/reset"} {
		h := newTestHandler(t)
		putContacts(t, h, `{"Contacts":[{"Name":"Acme"}]}`)
		if _, body := listContacts(t, h, url.Values{}); len(body["Contacts"].([]any)) != 1 {
			t.Fatalf("%s: setup wrote %v", path, body)
		}

		if rec := callPlane(controlPlane(h), http.MethodPost, path); rec.Code != 200 || decode(t, rec)["status"] != "reset" {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
		}
		if _, body := listContacts(t, h, url.Values{}); len(body["Contacts"].([]any)) != 0 {
			t.Errorf("%s left the contacts in place: %v", path, body["Contacts"])
		}
	}
}

func TestOnlyHealthAndResetAreAliased(t *testing.T) {
	plane := controlPlane(newTestHandler(t))
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/_xero/state"}, {http.MethodPost, "/_xero/health"}, {http.MethodGet, "/_xero/reset"},
	} {
		if rec := callPlane(plane, c.method, c.path); rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d, want it unserved", c.method, c.path, rec.Code)
		}
	}
}
