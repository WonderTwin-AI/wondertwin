// Package api implements the Resend-compatible HTTP API handlers for the twin.
package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
)

// Handler holds all API handler state.
type Handler struct {
	store *store.MemoryStore
	mw    *twincore.Middleware
}

// NewHandler creates a new API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	return &Handler{store: s, mw: mw}
}

// Routes mounts the Resend API routes and admin extras.
func (h *Handler) Routes(r chi.Router) {
	// Resend API routes (Bearer token auth required)
	r.Route("/emails", func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Post("/", h.SendEmail)
		r.Get("/{id}", h.GetEmail)
		r.Post("/batch", h.SendBatch)
	})

	// Admin extras (no auth required)
	r.Get("/admin/emails", h.AdminListEmails)
}

// bearerAuthMiddleware validates Resend-style Bearer token auth.
// Accepts any token starting with "re_" in sim mode.
func (h *Handler) bearerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			twincore.JSON(w, http.StatusUnauthorized, map[string]any{
				"statusCode": 401,
				"message":    "Missing API key in the authorization header. Include the following header 'Authorization: Bearer re_123' in the request.",
				"name":       "missing_api_key",
			})
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth || token == "" {
			twincore.JSON(w, http.StatusUnauthorized, map[string]any{
				"statusCode": 401,
				"message":    "Invalid authorization header format. Use 'Authorization: Bearer re_123'.",
				"name":       "invalid_api_key",
			})
			return
		}

		// In sim mode, accept any non-empty token (preferably starting with "re_")
		next.ServeHTTP(w, r)
	})
}
