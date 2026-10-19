package api

import (
	"context"
	"net/http"
)

type tenantKey struct{}

// tenantID is the xero-tenant-id the call named, which the tenant middleware
// has checked.
func tenantID(r *http.Request) string {
	id, _ := r.Context().Value(tenantKey{}).(string)
	return id
}

// tenant requires the xero-tenant-id header on an Accounting call (auth-001). A
// missing header is 403. An unknown tenant is 403 under StrictAuth; otherwise it
// is accepted and sees an empty tenant. It runs after bearer, so an invalid
// token answers 401 whatever tenant it named (auth-007). The 403 body is
// Xero's problem-details shape, which is a guess: the real one is not observed.
func (h *Handler) tenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("xero-tenant-id")
		_, known := h.store.Organisations.Get(id)
		if id == "" || (!known && h.store.StrictAuth.Load()) {
			h.writeProblem(w, r, http.StatusForbidden, "Forbidden", "AuthenticationUnsuccessful")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tenantKey{}, id)))
	})
}
