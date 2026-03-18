package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

func (h *Handler) CreateOrUpdateBill(w http.ResponseWriter, r *http.Request) {
	op := r.URL.Query().Get("operation")

	var bill store.Bill
	if err := json.NewDecoder(r.Body).Decode(&bill); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}

	if op == "delete" {
		if _, ok := h.store.Bills.Get(bill.Id); !ok {
			notFoundFault(w, "Bill", bill.Id)
			return
		}
		h.store.Bills.Delete(bill.Id)
		h.fireEvent("Bill", bill.Id, "Delete")
		qboJSON(w, http.StatusOK, entityResponse("Bill", bill))
		return
	}

	if bill.Id != "" {
		existing, ok := h.store.Bills.Get(bill.Id)
		if !ok {
			notFoundFault(w, "Bill", bill.Id)
			return
		}
		if err := ValidateSyncToken(existing.SyncToken, bill.SyncToken); err != nil {
			staleSyncTokenFault(w)
			return
		}
		computeBillTotals(&bill)
		paid := existing.TotalAmt - existing.Balance
		bill.Balance = bill.TotalAmt - paid
		bill.SyncToken = IncrementSyncToken(existing.SyncToken)
		bill.MetaData.CreateTime = existing.MetaData.CreateTime
		bill.MetaData.LastUpdatedTime = h.store.Now()
		bill.Domain = "QBO"
		h.store.Bills.Set(bill.Id, bill)
		h.fireEvent("Bill", bill.Id, "Update")
	} else {
		bill.Id = h.store.NextID()
		bill.SyncToken = "0"
		bill.Domain = "QBO"
		bill.MetaData = h.store.NewMetaData()
		computeBillTotals(&bill)
		bill.Balance = bill.TotalAmt
		h.store.Bills.Set(bill.Id, bill)
		h.fireEvent("Bill", bill.Id, "Create")
	}

	qboJSON(w, http.StatusOK, entityResponse("Bill", bill))
}

func (h *Handler) GetBill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	bill, ok := h.store.Bills.Get(id)
	if !ok {
		notFoundFault(w, "Bill", id)
		return
	}
	qboJSON(w, http.StatusOK, entityResponse("Bill", bill))
}

func computeBillTotals(bill *store.Bill) {
	var total float64
	for _, line := range bill.Line {
		total += line.Amount
	}
	bill.TotalAmt = total
}
