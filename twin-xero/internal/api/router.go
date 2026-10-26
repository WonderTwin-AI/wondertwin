// Package api implements the Xero API-compatible HTTP handlers for the twin.
package api

import (
	"sync"
	"sync/atomic"

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
	deliverTo  atomic.Pointer[map[string]bool] // the subscriptions the event being delivered is for

	pend    sync.Mutex // guards the fields below; never held across a delivery
	pending []pendingEvent
	gen     uint64 // bumped by reset, so a delivery in flight can tell it is stale
	wake    chan struct{}
	stop    chan struct{}
	started bool
	closed  bool
	worker  sync.WaitGroup
}

// NewHandler creates a new Xero API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	h := &Handler{store: s, mw: mw, wake: make(chan struct{}, 1), stop: make(chan struct{})}
	h.dispatcher = newDispatcher(s, h.isDeliveryTarget)
	s.OnReset(h.resetWebhooks)
	return h
}

// Routes mounts the Xero API routes, inside one group so every one of them
// passes fault injection. The token endpoint answers without a bearer token,
// since it is how a client gets one; everything else sits behind bearer.
func (h *Handler) Routes(r chi.Router) {
	r.NotFound(h.NotFound)
	r.MethodNotAllowed(h.MethodNotAllowed)
	r.Group(func(r chi.Router) {
		r.Use(h.mw.FaultInjection)
		r.Post("/connect/token", h.Token)

		// Xero registers a webhook in its developer portal, so the emulator has an
		// admin route instead.
		r.Post("/admin/webhooks", h.RegisterWebhook)
		r.Get("/admin/webhooks", h.ListWebhooks)
		r.Get("/admin/webhooks/deliveries", h.WebhookDeliveries)
		r.Delete("/admin/webhooks/{id}", h.DeleteWebhook)
		r.Post("/admin/webhooks/{id}/verify", h.VerifyWebhook)

		// Strict mode checks the token endpoint's caller against these.
		r.Post("/admin/clients", h.RegisterClient)
		r.Get("/admin/clients", h.ListClients)
		r.Delete("/admin/clients/{id}", h.DeleteClient)
		r.Post("/admin/authorization-codes", h.IssueAuthorizationCode)

		r.Group(func(r chi.Router) {
			r.Use(h.bearer)
			r.Get("/connections", h.Connections)
			r.Get("/Connections", h.Connections)

			r.Route("/api.xro/2.0", func(r chi.Router) {
				r.Use(h.tenant)
				r.Get("/Organisation", h.Organisation)
				r.Get("/Accounts", h.Accounts)
				r.Get("/Contacts", h.Contacts)
				r.Put("/Contacts", h.CreateContacts)
				r.Post("/Contacts", h.UpdateOrCreateContacts)
				r.Get("/Invoices", h.Invoices)
				r.Get("/Invoices/{InvoiceID}", h.Invoice)
				r.Put("/Invoices", h.CreateInvoices)
				r.Post("/Invoices", h.UpdateOrCreateInvoices)
				r.Post("/Invoices/{InvoiceID}", h.UpdateInvoice)
				r.Post("/Payments", h.CreatePayment)
				r.Get("/CreditNotes", h.CreditNotes)
				r.Put("/CreditNotes", h.CreateCreditNotes)
				r.Post("/CreditNotes", h.UpdateOrCreateCreditNotes)
				r.Post("/Payments/{PaymentID}", h.DeletePayment)
			})
		})
	})
}
