package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func get(h *Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// tokenFor issues a real access token through the token endpoint.
func tokenFor(t *testing.T, h *Handler) string {
	t.Helper()
	body := decode(t, postToken(h, url.Values{"grant_type": {"client_credentials"}}, "app:secret"))
	return body["access_token"].(string)
}

var zonelessISO = regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{7}$`)

func TestConnectionsListTheDefaultTenantUnderBothCasings(t *testing.T) {
	h := newTestHandler()
	auth := map[string]string{"Authorization": "Bearer " + tokenFor(t, h)}

	var bodies []string
	for _, path := range []string{"/connections", "/Connections"} {
		rec := get(h, path, auth)
		if rec.Code != 200 {
			t.Fatalf("%s: status %d: %s", path, rec.Code, rec.Body)
		}
		bodies = append(bodies, rec.Body.String())
	}
	if bodies[0] != bodies[1] {
		t.Fatalf("the two casings must answer alike:\n%s\n%s", bodies[0], bodies[1])
	}

	var list []map[string]any
	if err := json.Unmarshal([]byte(bodies[1]), &list); err != nil || len(list) != 1 {
		t.Fatalf("want a bare array of one connection: %v %s", err, bodies[1])
	}
	c := list[0]
	if c["tenantId"] != store.DefaultTenantID || c["id"] != store.DefaultConnectionID || c["tenantType"] != "ORGANISATION" {
		t.Fatalf("connection = %v", c)
	}
	for _, k := range []string{"createdDateUtc", "updatedDateUtc"} {
		if s, _ := c[k].(string); !zonelessISO.MatchString(s) {
			t.Fatalf("%s = %v, want ISO with seven fractional digits and no zone", k, c[k])
		}
	}
}

func TestConnectionsNeedAValidUnexpiredBearerToken(t *testing.T) {
	h := newTestHandler()
	h.store.StrictAuth.Store(true)
	good := tokenFor(t, h)
	cases := []struct {
		name    string
		headers map[string]string
		wantErr bool // the challenge names invalid_token
	}{
		{"no header", nil, false},
		{"not a bearer scheme", map[string]string{"Authorization": "Basic abc"}, false},
		{"token never issued", map[string]string{"Authorization": "Bearer nope"}, true},
		{"the same invalid token with a tenant id", map[string]string{"Authorization": "Bearer nope", "xero-tenant-id": store.DefaultTenantID}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := get(h, "/connections", tc.headers)
			want := "Bearer"
			if tc.wantErr {
				want = `Bearer error="invalid_token"`
			}
			if rec.Code != 401 || rec.Header().Get("WWW-Authenticate") != want {
				t.Fatalf("status %d, challenge %q, want 401 %q", rec.Code, rec.Header().Get("WWW-Authenticate"), want)
			}
		})
	}

	h.store.Clock.Advance(2 * time.Hour)
	if rec := get(h, "/connections", map[string]string{"Authorization": "Bearer " + good}); rec.Code != 401 {
		t.Fatalf("a token past its 1800 s lifetime must be refused, got %d", rec.Code)
	}
	if !strings.Contains(get(h, "/Connections", nil).Body.String(), "AuthenticationUnsuccessful") {
		t.Fatal("the 401 must be Xero's problem-details body")
	}
}
