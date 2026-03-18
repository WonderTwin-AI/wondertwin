package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

func (h *Handler) CreateOrUpdatePayment(w http.ResponseWriter, r *http.Request) {
	op := r.URL.Query().Get("operation")

	var pmt store.Payment
	if err := json.NewDecoder(r.Body).Decode(&pmt); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}

	if op == "void" {
		existing, ok := h.store.Payments.Get(pmt.Id)
		if !ok {
			notFoundFault(w, "Payment", pmt.Id)
			return
		}
		// Un-apply from linked invoices.
		for _, line := range existing.Line {
			for _, link := range line.LinkedTxn {
				if link.TxnType == "Invoice" {
					if inv, ok := h.store.Invoices.Get(link.TxnId); ok {
						inv.Balance += existing.TotalAmt
						inv.MetaData.LastUpdatedTime = h.store.Now()
						h.store.Invoices.Set(inv.Id, inv)
					}
				}
			}
		}
		existing.SyncToken = IncrementSyncToken(existing.SyncToken)
		existing.MetaData.LastUpdatedTime = h.store.Now()
		h.store.Payments.Set(existing.Id, existing)
		h.fireEvent("Payment", existing.Id, "Void")
		qboJSON(w, http.StatusOK, entityResponse("Payment", existing))
		return
	}

	if op == "delete" {
		h.store.Payments.Delete(pmt.Id)
		h.fireEvent("Payment", pmt.Id, "Delete")
		qboJSON(w, http.StatusOK, entityResponse("Payment", pmt))
		return
	}

	if pmt.Id != "" {
		existing, ok := h.store.Payments.Get(pmt.Id)
		if !ok {
			notFoundFault(w, "Payment", pmt.Id)
			return
		}
		if err := ValidateSyncToken(existing.SyncToken, pmt.SyncToken); err != nil {
			staleSyncTokenFault(w)
			return
		}
		pmt.SyncToken = IncrementSyncToken(existing.SyncToken)
		pmt.MetaData.CreateTime = existing.MetaData.CreateTime
		pmt.MetaData.LastUpdatedTime = h.store.Now()
		pmt.Domain = "QBO"
		h.store.Payments.Set(pmt.Id, pmt)
		h.fireEvent("Payment", pmt.Id, "Update")
	} else {
		pmt.Id = h.store.NextID()
		pmt.SyncToken = "0"
		pmt.Domain = "QBO"
		pmt.MetaData = h.store.NewMetaData()

		// Apply payment to linked invoices (reduce their Balance).
		var applied float64
		for _, line := range pmt.Line {
			for _, link := range line.LinkedTxn {
				if link.TxnType == "Invoice" {
					if inv, ok := h.store.Invoices.Get(link.TxnId); ok {
						inv.Balance -= line.Amount
						if inv.Balance < 0 {
							inv.Balance = 0
						}
						inv.MetaData.LastUpdatedTime = h.store.Now()
						h.store.Invoices.Set(inv.Id, inv)
						applied += line.Amount
					}
				}
			}
		}
		pmt.UnappliedAmt = pmt.TotalAmt - applied

		h.store.Payments.Set(pmt.Id, pmt)
		h.journalPaymentCreated(&pmt)
		h.fireEvent("Payment", pmt.Id, "Create")
	}

	qboJSON(w, http.StatusOK, entityResponse("Payment", pmt))
}

func (h *Handler) GetPayment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pmt, ok := h.store.Payments.Get(id)
	if !ok {
		notFoundFault(w, "Payment", id)
		return
	}
	qboJSON(w, http.StatusOK, entityResponse("Payment", pmt))
}

func (h *Handler) CreateOrUpdateBillPayment(w http.ResponseWriter, r *http.Request) {
	var bp store.BillPayment
	if err := json.NewDecoder(r.Body).Decode(&bp); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}

	if bp.Id != "" {
		existing, ok := h.store.BillPayments.Get(bp.Id)
		if !ok {
			notFoundFault(w, "BillPayment", bp.Id)
			return
		}
		if err := ValidateSyncToken(existing.SyncToken, bp.SyncToken); err != nil {
			staleSyncTokenFault(w)
			return
		}
		bp.SyncToken = IncrementSyncToken(existing.SyncToken)
		bp.MetaData.CreateTime = existing.MetaData.CreateTime
		bp.MetaData.LastUpdatedTime = h.store.Now()
		bp.Domain = "QBO"
		h.store.BillPayments.Set(bp.Id, bp)
	} else {
		bp.Id = h.store.NextID()
		bp.SyncToken = "0"
		bp.Domain = "QBO"
		bp.MetaData = h.store.NewMetaData()

		// Apply to linked bills.
		for _, line := range bp.Line {
			for _, link := range line.LinkedTxn {
				if link.TxnType == "Bill" {
					if bill, ok := h.store.Bills.Get(link.TxnId); ok {
						bill.Balance -= line.Amount
						if bill.Balance < 0 {
							bill.Balance = 0
						}
						bill.MetaData.LastUpdatedTime = h.store.Now()
						h.store.Bills.Set(bill.Id, bill)
					}
				}
			}
		}

		h.store.BillPayments.Set(bp.Id, bp)
		h.journalBillPaymentCreated(&bp)
		h.fireEvent("BillPayment", bp.Id, "Create")
	}

	qboJSON(w, http.StatusOK, entityResponse("BillPayment", bp))
}

func (h *Handler) GetBillPayment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	bp, ok := h.store.BillPayments.Get(id)
	if !ok {
		notFoundFault(w, "BillPayment", id)
		return
	}
	qboJSON(w, http.StatusOK, entityResponse("BillPayment", bp))
}
