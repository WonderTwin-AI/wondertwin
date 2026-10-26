package api

import (
	"net/http"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// Audit finding F2: a path or method nobody serves answers in Xero's error
// shape, after authentication (quirk auth-008).
func TestUnknownPathsAreAuthenticatedBeforeTheyAre404(t *testing.T) {
	paths := []string{"/nope", "/api.xro/3.0/Contacts", "/api.xro/2.0/Nothing", "/Api.xro/2.0/Contacts"}
	for _, strict := range []bool{false, true} {
		for _, p := range paths {
			h := newTestHandler(t)
			h.store.StrictAuth.Store(strict)

			rec := get(h, p, nil)
			if rec.Code != 401 || rec.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Errorf("strict=%v %s without a token: %d %v", strict, p, rec.Code, rec.Header())
			}
			rec = get(h, p, map[string]string{"Authorization": "Bearer made-up", "xero-tenant-id": store.DefaultTenantID})
			wantCode := 404
			if strict {
				wantCode = 401
			}
			if rec.Code != wantCode {
				t.Errorf("strict=%v %s with a made-up token: %d, want %d", strict, p, rec.Code, wantCode)
			}
			rec = get(h, p, accounting(t, h, store.DefaultTenantID))
			body := decode(t, rec)
			if rec.Code != 404 || body["Title"] != "Not Found" || body["Status"] != float64(404) {
				t.Errorf("strict=%v %s with an issued token: %d %v", strict, p, rec.Code, body)
			}
		}
	}
}

func TestAMethodNobodyServesIsA405ProblemAfterAuthentication(t *testing.T) {
	h := newTestHandler(t)
	h.store.StrictAuth.Store(true)

	if rec := do(h, http.MethodDelete, "/api.xro/2.0/Contacts", "", nil); rec.Code != 401 {
		t.Fatalf("DELETE without a token: %d, want 401", rec.Code)
	}
	rec := do(h, http.MethodDelete, "/api.xro/2.0/Contacts", "", accounting(t, h, store.DefaultTenantID))
	body := decode(t, rec)
	if rec.Code != 405 || body["Title"] != "Method Not Allowed" || body["Status"] != float64(405) {
		t.Fatalf("DELETE with a token: %d %v", rec.Code, body)
	}
}
