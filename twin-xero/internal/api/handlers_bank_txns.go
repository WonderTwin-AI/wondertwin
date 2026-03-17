package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func (h *Handler) CreateBankTransaction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BankTransactions []store.BankTransaction `json:"BankTransactions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}
	if len(req.BankTransactions) == 0 {
		xeroError(w, http.StatusBadRequest, "ValidationException", "At least one bank transaction is required")
		return
	}

	var result []store.BankTransaction
	for _, bt := range req.BankTransactions {
		if bt.BankTransactionID == "" {
			bt.BankTransactionID = newUUID()
		}
		if bt.Status == "" {
			bt.Status = "AUTHORISED"
		}

		var subTotal float64
		for i := range bt.LineItems {
			li := &bt.LineItems[i]
			li.LineAmount = li.Quantity * li.UnitAmount
			subTotal += li.LineAmount
		}
		bt.SubTotal = subTotal
		bt.Total = subTotal
		bt.UpdatedDateUTC = store.XeroDateNow()

		h.store.BankTxns.Set(bt.BankTransactionID, bt)
		result = append(result, bt)

		if h.dispatcher != nil {
			h.dispatcher.Enqueue("BANKTRANSACTION.CREATE", map[string]any{
				"resourceId": bt.BankTransactionID,
				"eventType":  "BANKTRANSACTION.CREATE",
			})
		}
	}

	xeroJSON(w, http.StatusOK, map[string]any{"BankTransactions": result})
}

func (h *Handler) ListBankTransactions(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	all := h.store.BankTxns.List()
	xeroJSON(w, http.StatusOK, map[string]any{"BankTransactions": paginate(all, page, pageSize)})
}

func (h *Handler) GetBankTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "BankTransactionID")
	bt, ok := h.store.BankTxns.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "BankTransaction not found")
		return
	}
	xeroJSON(w, http.StatusOK, map[string]any{"BankTransactions": []store.BankTransaction{bt}})
}
