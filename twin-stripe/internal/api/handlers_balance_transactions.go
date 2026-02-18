package api

import (
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ListBalanceTransactions handles GET /v1/balance_transactions.
// Supports optional ?type= query param to filter by transaction type.
func (h *Handler) ListBalanceTransactions(w http.ResponseWriter, r *http.Request) {
	filterType := r.URL.Query().Get("type")

	all := h.store.BalanceTransactions.List()

	// Sort by created descending (most recent first)
	sort.Slice(all, func(i, j int) bool {
		return all[i].Created > all[j].Created
	})

	// Filter by type if specified
	if filterType != "" {
		filtered := all[:0]
		for _, bt := range all {
			if bt.Type == filterType {
				filtered = append(filtered, bt)
			}
		}
		all = filtered
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/balance_transactions",
		"data":     all,
		"has_more": false,
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
