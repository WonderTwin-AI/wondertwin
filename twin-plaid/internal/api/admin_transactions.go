package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// Plaid has no public call that changes or removes a transaction; its Sandbox
// does it on its own for dynamic users. So a test that needs /transactions/sync
// to report modified and removed rows drives these two admin routes, which sit
// beside the kit's /admin plane and take no Plaid credentials.

// adminTransactionModify is POST /admin/transactions/{transaction_id}/modify. The
// body may carry name, amount, date and pending; it replaces the stored row and
// logs it as modified, which sync then reports.
func (h *Handler) adminTransactionModify(w http.ResponseWriter, r *http.Request) {
	t, ok := h.store.Transactions.Get(chi.URLParam(r, "transaction_id"))
	if !ok {
		twincore.Error(w, http.StatusNotFound, "no such transaction")
		return
	}
	var in struct {
		Name    *string  `json:"name"`
		Amount  *float64 `json:"amount"`
		Date    *string  `json:"date"`
		Pending *bool    `json:"pending"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		twincore.Error(w, http.StatusBadRequest, "body must be a JSON object with name, amount, date or pending")
		return
	}
	if in.Name != nil {
		t.Body.Name = *in.Name
	}
	if in.Amount != nil {
		t.Amount, t.Body.Amount = *in.Amount, *in.Amount
	}
	if in.Date != nil {
		if _, err := time.Parse("2006-01-02", *in.Date); err != nil {
			twincore.Error(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
			return
		}
		t.Date, t.Body.Date = *in.Date, *in.Date
	}
	if in.Pending != nil {
		t.Pending, t.Body.Pending = *in.Pending, *in.Pending
	}
	h.store.ModifyTransaction(t)
	twincore.JSON(w, http.StatusOK, map[string]string{"status": "modified", "transaction_id": t.ID})
}

// adminTransactionRemove is DELETE /admin/transactions/{transaction_id}. Sync
// then reports the row as removed.
func (h *Handler) adminTransactionRemove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "transaction_id")
	if !h.store.RemoveTransaction(id) {
		twincore.Error(w, http.StatusNotFound, "no such transaction")
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]string{"status": "removed", "transaction_id": id})
}
