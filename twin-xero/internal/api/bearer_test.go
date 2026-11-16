package api

import (
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// The auth scheme is a case-insensitive token (RFC 7235 section 2.1), so a
// client that sends "bearer" or "BEARER" is let in as one that sends "Bearer".
func TestBearerSchemeIsMatchedCaseInsensitively(t *testing.T) {
	for _, strict := range []bool{false, true} {
		h := newTestHandler(t)
		h.store.StrictAuth.Store(strict)
		token := tokenFor(t, h)
		for _, scheme := range []string{"Bearer", "bearer", "BEARER", "bEaReR"} {
			rec := get(h, "/api.xro/2.0/Organisation", map[string]string{
				"Authorization": scheme + " " + token, "xero-tenant-id": store.DefaultTenantID,
			})
			if rec.Code != 200 {
				t.Errorf("strict=%v %q: %d %s", strict, scheme, rec.Code, rec.Body)
			}
		}
	}
}

func TestBearerTokenNeedsTheBearerScheme(t *testing.T) {
	cases := []struct {
		header string
		token  string
		found  bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer abc", "abc", true},
		{"Bearer  abc", "abc", true},
		{"Bearer ", "", true},
		{"Bearer", "", false},
		{"Basic abc", "", false},
		{"Bearerabc", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		token, found := bearerToken(tc.header)
		if token != tc.token || found != tc.found {
			t.Errorf("bearerToken(%q) = %q, %v; want %q, %v", tc.header, token, found, tc.token, tc.found)
		}
	}
	h := newTestHandler(t)
	if rec := get(h, "/connections", map[string]string{"Authorization": "Basic " + strings.Repeat("a", 8)}); rec.Code != 401 || rec.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("a Basic credential: %d %v", rec.Code, rec.Header())
	}
}
