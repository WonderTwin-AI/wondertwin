package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// controlAliases maps the control paths of the legacy xero-local emulator to the
// admin plane. Xero has no health or reset endpoint, so these are not Xero API:
// they exist so localstack/finance-ops, which polls /_xero/health and posts
// /_xero/reset before each test, can use this emulator unchanged (compat-001).
var controlAliases = []struct{ method, path, target string }{
	{http.MethodGet, "/_xero/health", "/admin/health"},
	{http.MethodPost, "/_xero/reset", "/admin/reset"},
}

// MountControlAliases serves each legacy control path through admin, the router
// that holds the admin plane, so an alias runs the very handler it stands for.
func MountControlAliases(r chi.Router, admin http.Handler) {
	for _, a := range controlAliases {
		target := a.target
		r.MethodFunc(a.method, a.path, func(w http.ResponseWriter, req *http.Request) {
			req.URL.Path = target
			admin.ServeHTTP(w, req)
		})
	}
}
