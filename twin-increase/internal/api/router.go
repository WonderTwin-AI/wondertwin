// Package api implements the Increase-compatible HTTP API handlers for the twin.
package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
	"github.com/wondertwin-ai/wondertwin/twin-increase/internal/store"
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

// Routes mounts the Increase API routes.
func (h *Handler) Routes(r chi.Router) {
	// All Increase API routes require Bearer auth
	r.Use(h.bearerAuthMiddleware)
	r.Use(h.idempotencyMiddleware)
	r.Use(h.mw.FaultInjection)

	// Accounts
	r.Post("/accounts", h.CreateAccount)
	r.Get("/accounts/{account_id}", h.GetAccount)
	r.Patch("/accounts/{account_id}", h.UpdateAccount)
	r.Get("/accounts", h.ListAccounts)
	r.Post("/accounts/{account_id}/close", h.CloseAccount)

	// Account Numbers
	r.Post("/account_numbers", h.CreateAccountNumber)
	r.Get("/account_numbers/{account_number_id}", h.GetAccountNumber)
	r.Patch("/account_numbers/{account_number_id}", h.UpdateAccountNumber)
	r.Get("/account_numbers", h.ListAccountNumbers)

	// External Accounts
	r.Post("/external_accounts", h.CreateExternalAccount)
	r.Get("/external_accounts/{external_account_id}", h.GetExternalAccount)
	r.Patch("/external_accounts/{external_account_id}", h.UpdateExternalAccount)
	r.Get("/external_accounts", h.ListExternalAccounts)

	// ACH Transfers
	r.Post("/ach_transfers", h.CreateACHTransfer)
	r.Get("/ach_transfers/{ach_transfer_id}", h.GetACHTransfer)
	r.Get("/ach_transfers", h.ListACHTransfers)
	r.Post("/ach_transfers/{ach_transfer_id}/approve", h.ApproveACHTransfer)
	r.Post("/ach_transfers/{ach_transfer_id}/cancel", h.CancelACHTransfer)

	// Inbound ACH Transfers
	r.Get("/inbound_ach_transfers/{inbound_ach_transfer_id}", h.GetInboundACHTransfer)
	r.Get("/inbound_ach_transfers", h.ListInboundACHTransfers)
	r.Post("/inbound_ach_transfers/{inbound_ach_transfer_id}/decline", h.DeclineInboundACHTransfer)
	r.Post("/inbound_ach_transfers/{inbound_ach_transfer_id}/notification_of_change", h.NotificationOfChangeInboundACHTransfer)
	r.Post("/inbound_ach_transfers/{inbound_ach_transfer_id}/transfer_return", h.ReturnInboundACHTransfer)

	// Transactions
	r.Get("/transactions/{transaction_id}", h.GetTransaction)
	r.Get("/transactions", h.ListTransactions)

	// Pending Transactions
	r.Get("/pending_transactions/{pending_transaction_id}", h.GetPendingTransaction)
	r.Get("/pending_transactions", h.ListPendingTransactions)

	// Events
	r.Get("/events/{event_id}", h.GetEvent)
	r.Get("/events", h.ListEvents)

	// Event Subscriptions
	r.Post("/event_subscriptions", h.CreateEventSubscription)
	r.Get("/event_subscriptions/{event_subscription_id}", h.GetEventSubscription)
	r.Patch("/event_subscriptions/{event_subscription_id}", h.UpdateEventSubscription)
	r.Get("/event_subscriptions", h.ListEventSubscriptions)

	// Simulation API
	r.Post("/simulations/ach_transfers/{ach_transfer_id}/submit", h.SimulateACHTransferSubmit)
	r.Post("/simulations/ach_transfers/{ach_transfer_id}/acknowledge", h.SimulateACHTransferAcknowledge)
	r.Post("/simulations/ach_transfers/{ach_transfer_id}/settle", h.SimulateACHTransferSettle)
	r.Post("/simulations/ach_transfers/{ach_transfer_id}/return", h.SimulateACHTransferReturn)
	r.Post("/simulations/inbound_ach_transfers", h.SimulateInboundACHTransfer)
}

// bearerAuthMiddleware validates Increase-style Bearer token auth.
func (h *Handler) bearerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			increaseError(w, http.StatusUnauthorized, "unauthorized", "Missing authorization header")
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth || token == "" {
			increaseError(w, http.StatusUnauthorized, "unauthorized", "Invalid authorization header format. Use 'Authorization: Bearer <api_key>'")
			return
		}

		// In sim mode, accept any non-empty token
		next.ServeHTTP(w, r)
	})
}

// idempotencyMiddleware handles Idempotency-Key header for POST requests.
func (h *Handler) idempotencyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}

		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Check for cached response
		if status, body, ok := h.mw.Idempotent.Check(key); ok {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(status)
			w.Write(body)
			return
		}

		// Capture response for caching
		rec := &responseRecorder{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(rec, r)
		h.mw.Idempotent.Store(key, rec.statusCode, rec.body.Bytes())
	})
}

// increaseError writes an Increase-style error response.
func increaseError(w http.ResponseWriter, status int, errType, message string) {
	twincore.JSON(w, status, map[string]any{
		"type":   errType,
		"status": status,
		"title":  message,
	})
}
