// Package api implements the Xero API-compatible HTTP handlers for the twin.
package api

import (
	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// Handler holds the Xero API state.
type Handler struct {
	store *store.MemoryStore
	mw    *twincore.Middleware
}

// NewHandler creates a new Xero API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	return &Handler{store: s, mw: mw}
}

// Routes mounts the Xero API routes. The scaffold has none: the Accounting,
// connections and OAuth routes arrive with the slices that own them, inside
// this group so every one of them passes fault injection.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.mw.FaultInjection)
	})
}
