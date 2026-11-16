package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// vendorRoute is one method and path the router serves for Xero's own API.
type vendorRoute struct{ method, path string }

// vendorRoutes walks the router for every route a Xero client calls, so a route
// added later is held to the auth rules without this test being edited. The
// token endpoint and the admin routes are the emulator's own and are left out.
func vendorRoutes(t *testing.T, h *Handler) []vendorRoute {
	t.Helper()
	r := chi.NewRouter()
	h.Routes(r)
	var out []vendorRoute
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if route == "/connect/token" || strings.HasPrefix(route, "/admin/") {
			return nil
		}
		path := route
		for strings.Contains(path, "{") {
			open, end := strings.Index(path, "{"), strings.Index(path, "}")
			path = path[:open] + "00000000-0000-4000-8000-000000000000" + path[end+1:]
		}
		out = append(out, vendorRoute{method, path})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 28 {
		t.Fatalf("walked %d vendor routes, want at least the 28 registered: %v", len(out), out)
	}
	return out
}

// Every vendor route answers 401 with the bare Bearer challenge when no token is
// sent, in either auth mode, before it reads the body or the tenant (auth-007).
func TestEveryVendorRouteRefusesACallWithoutAToken(t *testing.T) {
	for _, strict := range []bool{false, true} {
		h := newTestHandler(t)
		h.store.StrictAuth.Store(strict)
		for _, rt := range vendorRoutes(t, h) {
			rec := do(h, rt.method, rt.path, `{}`, map[string]string{"xero-tenant-id": store.DefaultTenantID})
			body := decode(t, rec)
			if rec.Code != 401 || rec.Header().Get("WWW-Authenticate") != "Bearer" || body["Title"] != "Unauthorized" {
				t.Errorf("strict=%v %s %s without a token: %d %v %v", strict, rt.method, rt.path, rec.Code, rec.Header(), body)
			}
		}
	}
}

// Every Accounting route answers 403 when the xero-tenant-id header is missing
// (auth-001), and the connections routes, which take no tenant, do not.
func TestEveryAccountingRouteRefusesACallWithoutATenant(t *testing.T) {
	for _, strict := range []bool{false, true} {
		h := newTestHandler(t)
		h.store.StrictAuth.Store(strict)
		auth := map[string]string{"Authorization": "Bearer " + tokenFor(t, h)}
		accountingRoutes := 0
		for _, rt := range vendorRoutes(t, h) {
			rec := do(h, rt.method, rt.path, `{}`, auth)
			if !strings.HasPrefix(rt.path, "/api.xro/2.0/") {
				if rec.Code != 200 {
					t.Errorf("strict=%v %s %s needs no tenant: %d %s", strict, rt.method, rt.path, rec.Code, rec.Body)
				}
				continue
			}
			accountingRoutes++
			body := decode(t, rec)
			if rec.Code != 403 || body["Title"] != "Forbidden" || body["Status"] != float64(403) {
				t.Errorf("strict=%v %s %s without a tenant: %d %v", strict, rt.method, rt.path, rec.Code, body)
			}
		}
		if accountingRoutes < 26 {
			t.Fatalf("checked %d Accounting routes, want at least 26", accountingRoutes)
		}
	}
}
