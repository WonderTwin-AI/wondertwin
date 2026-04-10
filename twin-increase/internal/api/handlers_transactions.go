package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// GetTransaction handles GET /transactions/{transaction_id}.
func (h *Handler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "transaction_id")
	txn, ok := h.store.Transactions.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No transaction found for %s", id))
		return
	}
	twincore.JSON(w, http.StatusOK, txn)
}

// ListTransactions handles GET /transactions.
func (h *Handler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	cursor, limit := paginationParams(r)
	accountID := r.URL.Query().Get("account_id")

	page := h.store.Transactions.Paginate(cursor, limit)

	// Filter by account_id if provided
	if accountID != "" {
		filtered := make([]any, 0)
		for _, txn := range page.Data {
			if txn.AccountID == accountID {
				filtered = append(filtered, txn)
			}
		}
		resp := listResponse(filtered, page.Cursor)
		twincore.JSON(w, http.StatusOK, resp)
		return
	}

	data := make([]any, len(page.Data))
	for i, txn := range page.Data {
		data[i] = txn
	}

	resp := listResponse(data, page.Cursor)
	twincore.JSON(w, http.StatusOK, resp)
}

// GetPendingTransaction handles GET /pending_transactions/{pending_transaction_id}.
func (h *Handler) GetPendingTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "pending_transaction_id")
	pending, ok := h.store.PendingTransactions.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No pending transaction found for %s", id))
		return
	}
	twincore.JSON(w, http.StatusOK, pending)
}

// ListPendingTransactions handles GET /pending_transactions.
func (h *Handler) ListPendingTransactions(w http.ResponseWriter, r *http.Request) {
	cursor, limit := paginationParams(r)
	accountID := r.URL.Query().Get("account_id")

	page := h.store.PendingTransactions.Paginate(cursor, limit)

	// Filter by account_id if provided
	if accountID != "" {
		filtered := make([]any, 0)
		for _, pending := range page.Data {
			if pending.AccountID == accountID {
				filtered = append(filtered, pending)
			}
		}
		resp := listResponse(filtered, page.Cursor)
		twincore.JSON(w, http.StatusOK, resp)
		return
	}

	data := make([]any, len(page.Data))
	for i, pending := range page.Data {
		data[i] = pending
	}

	resp := listResponse(data, page.Cursor)
	twincore.JSON(w, http.StatusOK, resp)
}
