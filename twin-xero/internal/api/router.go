// Package api implements the Xero API-compatible HTTP handlers for the twin.
package api

import (
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	pkgwh "github.com/wondertwin-ai/wondertwin/twinkit/webhook"
)

// Handler holds the Xero API state.
type Handler struct {
	store *store.MemoryStore
	mw    *twincore.Middleware

	redeem sync.Mutex

	dispatcher *pkgwh.Dispatcher
	deliver    sync.Mutex // keeps events queued and flushed in order
}

// NewHandler creates a new Xero API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	h := &Handler{store: s, mw: mw, dispatcher: newDispatcher(s)}
	s.OnReset(h.dispatcher.Reset)
	return h
}

// Routes mounts the Xero API routes, inside one group so every one of them
// passes fault injection. The token endpoint answers without a bearer token,
// since it is how a client gets one; everything else sits behind bearer.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.mw.FaultInjection)
		r.Post("/connect/token", h.Token)

		r.Group(func(r chi.Router) {
			r.Use(h.bearer)
			r.Get("/connections", h.Connections)
			r.Get("/Connections", h.Connections)

			r.Route("/api.xro/2.0", func(r chi.Router) {
				r.Use(h.tenant)
				r.Get("/Organisation", h.Organisation)
				r.Get("/Accounts", h.Accounts)
			})
		})
	})
}
