package api

import (
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ListBalanceTransactions handles GET /v1/balance_transactions.
// Supports optional ?type= query param to filter by transaction type.
func (h *Handler) ListBalanceTransactions(w http.ResponseWriter, r *http.Request) {
	filterType := r.URL.Query().Get("type")

	page := paginate(r, h.store.BalanceTransactions, parseLimit(r, 10), func(bt store.BalanceTransaction) bool {
		return filterType == "" || bt.Type == filterType
	})

	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/balance_transactions",
		"data":     page.Data,
		"has_more": page.HasMore,
	})
}

// GetBalanceTransaction handles GET /v1/balance_transactions/{id}.
func (h *Handler) GetBalanceTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	bt, ok := h.store.BalanceTransactions.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such balance transaction: '"+id+"'")
		return
	}

	twincore.JSON(w, http.StatusOK, bt)
}
