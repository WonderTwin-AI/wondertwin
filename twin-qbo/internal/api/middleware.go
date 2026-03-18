package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// authMiddleware validates Bearer token presence (accepts any token).
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") == "" {
			qboFault(w, http.StatusUnauthorized, "AuthenticationFault", "100",
				"General Authentication Error",
				"AuthenticationFailed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// realmIDMiddleware validates the realmId path parameter is present.
func realmIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		realmID := chi.URLParam(r, "realmId")
		if realmID == "" {
			qboFault(w, http.StatusBadRequest, "ValidationFault", "500",
				"Invalid Company", "Company ID is required.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
