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

	if op == "delete" || op == "void" {
		existing, ok := h.store.Bills.Get(bill.Id)
		if !ok {
			notFoundFault(w, "Bill", bill.Id)
			return
		}
		if err := ValidateSyncToken(existing.SyncToken, bill.SyncToken); err != nil {
			staleSyncTokenFault(w)
			return
		}
		if op == "void" {
			existing.Balance = 0
			existing.TotalAmt = 0
			for i := range existing.Line {
				existing.Line[i].Amount = 0
			}
		}
		existing.SyncToken = IncrementSyncToken(existing.SyncToken)
		existing.MetaData.LastUpdatedTime = h.store.Now()
		if op == "delete" {
			h.store.Bills.Delete(bill.Id)
			h.fireEvent("Bill", bill.Id, "Delete")
		} else {
			h.store.Bills.Set(bill.Id, existing)
			h.fireEvent("Bill", bill.Id, "Void")
		}
		qboJSON(w, http.StatusOK, entityResponse("Bill", existing))
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
		h.journalBillCreated(&bill)
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
	var subTotal float64
	for _, line := range bill.Line {
		subTotal += line.Amount
	}
	tax := 0.0
	if bill.TxnTaxDetail != nil {
		tax = bill.TxnTaxDetail.TotalTax
	}
	bill.TotalAmt = subTotal + tax
	if bill.ExchangeRate > 0 {
		bill.HomeTotalAmt = bill.TotalAmt * bill.ExchangeRate
	} else {
		bill.HomeTotalAmt = bill.TotalAmt
	}
}
