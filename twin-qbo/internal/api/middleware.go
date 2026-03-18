package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

const minSupportedMinorVersion = 75

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

// minorVersionMiddleware validates the optional minorversion query parameter.
// Rejects deprecated versions (< 75). Accepts or defaults to 75.
func minorVersionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mvStr := r.URL.Query().Get("minorversion")
		if mvStr != "" {
			mv, err := strconv.Atoi(mvStr)
			if err != nil {
				qboFault(w, http.StatusBadRequest, "ValidationFault", "500",
					"Invalid minor version",
					"minorversion must be an integer.")
				return
			}
			if mv < minSupportedMinorVersion {
				qboFault(w, http.StatusBadRequest, "ValidationFault", "500",
					"Deprecated minor version",
					"Minor versions below 75 are deprecated. Use minorversion=75 or higher.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
