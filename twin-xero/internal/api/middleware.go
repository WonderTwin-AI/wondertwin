package api

import (
	"net/http"
	"strings"
)

// authMiddleware validates Bearer token presence (accepts any token).
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") == "" {
			xeroError(w, http.StatusUnauthorized, "AuthenticationUnsuccessful", "AuthorizationRequired")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tenantMiddleware validates Xero-Tenant-Id header presence (accepts any UUID).
func tenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := r.Header.Get("Xero-Tenant-Id")
		if tenant == "" {
			xeroError(w, http.StatusBadRequest, "ValidationException", "Xero-Tenant-Id header is required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
