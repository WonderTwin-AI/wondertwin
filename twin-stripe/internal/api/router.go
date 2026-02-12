// Package api implements the Stripe-compatible HTTP API handlers for the twin.
package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/pkg/webhook"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// Handler holds all API handler state.
type Handler struct {
	store      *store.MemoryStore
	dispatcher *webhook.Dispatcher
	mw         *twincore.Middleware
}

// NewHandler creates a new API handler.
func NewHandler(s *store.MemoryStore, d *webhook.Dispatcher, mw *twincore.Middleware) *Handler {
	return &Handler{store: s, dispatcher: d, mw: mw}
}

// Routes mounts the Stripe v1 API routes.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/v1", func(r chi.Router) {
		// Auth middleware for all v1 routes
		r.Use(h.authMiddleware)
		// Fault injection for API routes (not admin)
		r.Use(h.mw.FaultInjection)

		// Accounts
		r.Post("/accounts", h.CreateAccount)
		r.Get("/accounts/{id}", h.GetAccount)
		r.Post("/accounts/{id}", h.UpdateAccount)
		r.Delete("/accounts/{id}", h.DeleteAccount)
		r.Get("/accounts", h.ListAccounts)

		// External Accounts
		r.Post("/accounts/{account_id}/external_accounts", h.CreateExternalAccount)
		r.Get("/accounts/{account_id}/external_accounts/{id}", h.GetExternalAccount)
		r.Post("/accounts/{account_id}/external_accounts/{id}", h.UpdateExternalAccount)
		r.Delete("/accounts/{account_id}/external_accounts/{id}", h.DeleteExternalAccount)

		// Transfers
		r.Post("/transfers", h.CreateTransfer)
		r.Get("/transfers/{id}", h.GetTransfer)
		r.Get("/transfers", h.ListTransfers)

		// Balance
		r.Get("/balance", h.GetBalance)

		// Payouts
		r.Post("/payouts", h.CreatePayout)
		r.Get("/payouts/{id}", h.GetPayout)
		r.Get("/payouts", h.ListPayouts)

		// Events
		r.Get("/events", h.ListEvents)
		r.Get("/events/{id}", h.GetEvent)
	})

	// Stripe-specific admin endpoints (outside /v1, no auth)
	r.Post("/admin/payouts/{id}/fail", h.AdminFailPayout)
}

// authMiddleware validates Stripe-style Bearer token authentication.
func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			twincore.StripeError(w, http.StatusUnauthorized,
				"invalid_request_error", "api_key_required",
				"You did not provide an API key.")
			return
		}

		// Accept "Bearer sk_test_*" or "Bearer sk_sim_*" or just "Bearer <anything>"
		key := strings.TrimPrefix(auth, "Bearer ")
		if key == auth {
			// No "Bearer " prefix, also check for raw key
			key = auth
		}
		if key == "" {
			twincore.StripeError(w, http.StatusUnauthorized,
				"invalid_request_error", "api_key_required",
				"You did not provide an API key.")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// stripeAccountFromRequest extracts the Stripe-Account header for connected account context.
func stripeAccountFromRequest(r *http.Request) string {
	return r.Header.Get("Stripe-Account")
}

// parseFormOrJSON extracts form values from either form-encoded or JSON body.
// Stripe's API uses form-encoded requests.
func parseFormOrJSON(r *http.Request) error {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/x-www-form-urlencoded") || strings.Contains(ct, "multipart/form-data") {
		return r.ParseForm()
	}
	// Also parse form for requests without explicit content type (Stripe SDK default)
	return r.ParseForm()
}
