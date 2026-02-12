// Package api implements the PostHog-compatible HTTP API handlers for the twin.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/store"
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

// Routes mounts the PostHog API routes and admin extras.
func (h *Handler) Routes(r chi.Router) {
	// PostHog capture API routes (minimal auth - accept api_key in body/header)
	r.Group(func(r chi.Router) {
		r.Use(h.mw.FaultInjection)

		r.Post("/capture", h.CaptureEvent)
		r.Post("/capture/", h.CaptureEvent)
		r.Post("/batch", h.BatchCapture)
		r.Post("/batch/", h.BatchCapture)
		r.Post("/e", h.CaptureEvent)   // JS SDK alternative endpoint
		r.Post("/e/", h.CaptureEvent)
		r.Post("/decide/", h.Decide)    // Feature flag evaluation
		r.Post("/decide", h.Decide)
	})

	// Admin extras (no auth required)
	r.Get("/admin/events", h.AdminListEvents)
	r.Post("/admin/feature-flags", h.AdminSetFeatureFlags)
	r.Get("/admin/feature-flags", h.AdminGetFeatureFlags)
}

// apiKeyFromRequest extracts the PostHog api_key from the request.
// PostHog accepts it via header, query param, or request body.
// In sim mode we accept any non-empty key.
func apiKeyFromRequest(r *http.Request) string {
	// Check header
	if key := r.Header.Get("X-PostHog-Api-Key"); key != "" {
		return key
	}
	// Check Authorization header
	if auth := r.Header.Get("Authorization"); auth != "" {
		return auth
	}
	// Check query param
	if key := r.URL.Query().Get("api_key"); key != "" {
		return key
	}
	return ""
}

// noKeyError writes a PostHog-style auth error.
func noKeyError(w http.ResponseWriter) {
	twincore.JSON(w, http.StatusUnauthorized, map[string]any{
		"type":   "authentication_error",
		"code":   "invalid_api_key",
		"detail": "Project API key invalid. You can find your project API key in PostHog project settings.",
	})
}
