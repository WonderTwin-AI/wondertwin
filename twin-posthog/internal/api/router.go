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
