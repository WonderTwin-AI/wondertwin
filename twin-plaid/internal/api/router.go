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

	// clientID and secret are each enforced when non-empty; see SetCredentials.
	clientID, secret string
}

// NewHandler creates a new Plaid API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	return &Handler{store: s, mw: mw}
}

// Routes mounts the Plaid API routes. Each route arrives with the slice that
// owns it, inside this group so every one of them passes fault injection and
// the Plaid request checks.
func (h *Handler) Routes(r chi.Router) {
	r.NotFound(h.notFound)
	r.MethodNotAllowed(h.notFound)
	r.Group(func(r chi.Router) {
		r.Use(h.mw.FaultInjection)
		r.Use(h.checkRequest)

		r.Post("/sandbox/public_token/create", h.sandboxPublicTokenCreate)
		r.Post("/item/public_token/exchange", h.itemPublicTokenExchange)
	})
}
