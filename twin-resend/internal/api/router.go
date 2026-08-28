// Package api implements the Resend-compatible HTTP API handlers for the twin.
package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
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

	// Domains
	r.Route("/domains", func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Get("/", h.ListDomains)
		r.Post("/", h.CreateDomain)
		r.Get("/{id}", h.GetDomain)
		r.Patch("/{id}", h.UpdateDomain)
		r.Delete("/{id}", h.DeleteDomain)
		r.Post("/{id}/verify", h.VerifyDomain)
	})

	// API Keys
	r.Route("/api-keys", func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Get("/", h.ListAPIKeys)
		r.Post("/", h.CreateAPIKey)
		r.Delete("/{id}", h.DeleteAPIKey)
	})

	// Audiences + Contacts
	r.Route("/audiences", func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Get("/", h.ListAudiences)
		r.Post("/", h.CreateAudience)
		r.Get("/{id}", h.GetAudience)
		r.Delete("/{id}", h.DeleteAudience)

		r.Get("/{audience_id}/contacts", h.ListContacts)
		r.Post("/{audience_id}/contacts", h.CreateContact)
		r.Get("/{audience_id}/contacts/{id}", h.GetContact)
		r.Patch("/{audience_id}/contacts/{id}", h.UpdateContact)
		r.Delete("/{audience_id}/contacts/{id}", h.DeleteContact)
	})

	// Broadcasts
	r.Route("/broadcasts", func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Get("/", h.ListBroadcasts)
		r.Post("/", h.CreateBroadcast)
		r.Get("/{id}", h.GetBroadcast)
		r.Post("/{id}/send", h.SendBroadcast)
		r.Delete("/{id}", h.DeleteBroadcast)
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
