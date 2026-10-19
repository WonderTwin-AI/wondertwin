// Package api implements the Plaid API-compatible HTTP handlers for the emulator.
package api

import (
	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// Handler holds the Plaid API state.
type Handler struct {
	store *store.MemoryStore
	mw    *twincore.Middleware
}

// NewHandler creates a new Plaid API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	return &Handler{store: s, mw: mw}
}

// Routes mounts the Plaid API routes. The scaffold has none: each route
// arrives with the slice that owns it, inside this group so every one of them
// passes fault injection.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.mw.FaultInjection)
	})
}
