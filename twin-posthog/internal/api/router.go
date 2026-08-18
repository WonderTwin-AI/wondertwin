// Package api implements the PostHog-compatible HTTP API handlers for the twin.
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/store"
	pkgevents "github.com/wondertwin-ai/wondertwin/twinkit/events"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// Handler holds all API handler state.
type Handler struct {
	store        *store.MemoryStore
	mw           *twincore.Middleware
	eventsEngine *pkgevents.Engine
}

// NewHandler creates a new API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware, ee *pkgevents.Engine) *Handler {
	return &Handler{store: s, mw: mw, eventsEngine: ee}
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
		r.Post("/e", h.CaptureEvent) // JS SDK alternative endpoint
		r.Post("/e/", h.CaptureEvent)
		r.Post("/decide/", h.Decide) // Feature flag evaluation
		r.Post("/decide", h.Decide)
		r.Post("/flags", h.Flags) // Feature flags v2
		r.Post("/flags/", h.Flags)
		r.Get("/api/feature_flag/local_evaluation", h.LocalEvaluation)
		r.Get("/api/feature_flag/local_evaluation/", h.LocalEvaluation)
	})

	// Feature Flag Management API
	r.Route("/api/projects/{project_id}/feature_flags", func(r chi.Router) {
		r.Use(h.mw.FaultInjection)

		r.Get("/", h.ListFeatureFlags)
		r.Post("/", h.CreateFeatureFlag)
		r.Get("/{flag_id}", h.GetFeatureFlag)
		r.Get("/{flag_id}/", h.GetFeatureFlag)
		r.Patch("/{flag_id}", h.UpdateFeatureFlag)
		r.Patch("/{flag_id}/", h.UpdateFeatureFlag)
		r.Delete("/{flag_id}", h.DeleteFeatureFlag)
		r.Delete("/{flag_id}/", h.DeleteFeatureFlag)
	})

	// Early Access Features
	r.Get("/api/early_access_features", h.ListEarlyAccessFeatures)
	r.Get("/api/early_access_features/", h.ListEarlyAccessFeatures)

	// Admin extras (no auth required)
	r.Get("/admin/events", h.AdminListEvents)
	r.Post("/admin/feature-flags", h.AdminSetFeatureFlags)
	r.Get("/admin/feature-flags", h.AdminGetFeatureFlags)
	r.Get("/admin/persons", h.AdminListPersons)
	r.Get("/admin/aliases", h.AdminListAliases)
	r.Get("/admin/groups", h.AdminListGroups)
}

// Note on API keys: this twin does not authenticate the capture/ingest path.
// (LocalEvaluation in handlers_flags.go is the exception — it rejects a missing
// Authorization header with 401, matching PostHog's personal-API-key rule.)
// PostHog itself accepts a project key from four places — the X-PostHog-Api-Key header, the
// Authorization header, an ?api_key query param, and properties.$token in the
// event body (the JS SDK path) — and answers an unknown key with 401
// authentication_error / invalid_api_key.
//
// Extraction helpers for all four existed here but were never wired to a
// rejection, so every request was accepted regardless. They were removed as
// dead code rather than switched on, because rejecting keyless requests is a
// breaking change for anyone already driving this twin without one. Wiring it
// up is tracked separately; this comment is the record of what real PostHog
// does so it does not have to be re-researched.
