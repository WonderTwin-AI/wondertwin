// Package api implements the Smile.io-compatible HTTP API handlers for the twin.
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-smile/internal/store"
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

// Routes mounts the Smile.io v1 API routes.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/v1", func(r chi.Router) {
		r.Use(h.mw.FaultInjection)

		// Customers
		r.Get("/customers/{id}", h.GetCustomer)

		// Points
		r.Post("/points/redeem", h.RedeemPoints)
		r.Post("/points/refund", h.RefundPoints)
	})
}
