// Package api implements the Plaid API-compatible HTTP handlers for the emulator.
package api

import (
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/seed"
	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
)

// Handler holds the Plaid API state.
type Handler struct {
	store *store.MemoryStore
	mw    *twincore.Middleware

	// clientID and secret are each enforced when non-empty; see SetCredentials.
	clientID, secret string

	// keyOnce guards the webhook signing key, created on first use.
	keyOnce sync.Once
	sk      *signingKey

	// hooksOnce guards the webhook dispatcher, built on first use.
	hooksOnce sync.Once
	hooks     *webhook.Dispatcher
}

// NewHandler creates a new Plaid API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	s.SetDefaults(seed.LoadInstitutions)
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
		r.Post("/item/get", h.itemGet)
		r.Post("/accounts/get", h.accountsGet)
		r.Post("/accounts/balance/get", h.accountsBalanceGet)
		r.Post("/auth/get", h.authGet)
		r.Post("/processor/token/create", h.processorTokenCreate)
		r.Post("/processor/stripe/bank_account_token/create", h.processorStripeBankAccountTokenCreate)
		r.Post("/identity/get", h.identityGet)
		r.Post("/identity/match", h.identityMatch)
		r.Post("/item/remove", h.itemRemove)
		r.Post("/sandbox/item/fire_webhook", h.sandboxItemFireWebhook)
		r.Post("/item/webhook/update", h.itemWebhookUpdate)
		r.Post("/webhook_verification_key/get", h.webhookVerificationKeyGet)
		r.Post("/institutions/get", h.institutionsGet)
		r.Post("/institutions/search", h.institutionsSearch)
		r.Post("/institutions/get_by_id", h.institutionsGetByID)
		r.Post("/sandbox/item/reset_login", h.sandboxItemResetLogin)
		r.Post("/item/access_token/invalidate", h.itemAccessTokenInvalidate)
		r.Post("/link/token/create", h.linkTokenCreate)
		r.Post("/link/token/get", h.linkTokenGet)
	})
}
