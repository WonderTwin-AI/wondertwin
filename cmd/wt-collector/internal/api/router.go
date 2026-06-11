package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-collector/internal/store"
)

// Handler holds API handler state.
type Handler struct {
	events    *store.EventStore
	ingestKey string
}

// NewHandler creates a new API handler.
func NewHandler(events *store.EventStore, ingestKey string) *Handler {
	return &Handler{events: events, ingestKey: ingestKey}
}

// Routes mounts all API routes. RequestID + Recoverer are applied by
// main; this keeps the router pure-routing and lets tests exercise the
// handlers without the slog-flavoured middleware stack.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/health", h.handleHealth)

	r.Route("/telemetry", func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Post("/v1/ingest", h.handleIngest)
		r.Get("/v1/stats", h.handleStats)
	})
}
