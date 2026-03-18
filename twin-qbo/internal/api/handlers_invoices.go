package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

func (h *Handler) CreateOrUpdateInvoice(w http.ResponseWriter, r *http.Request) {
	// Check for void/delete operations.
	op := r.URL.Query().Get("operation")

	var inv store.Invoice
	if err := json.NewDecoder(r.Body).Decode(&inv); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}

	if op == "delete" || op == "void" {
		existing, ok := h.store.Invoices.Get(inv.Id)
		if !ok {
			notFoundFault(w, "Invoice", inv.Id)
			return
		}
		if err := ValidateSyncToken(existing.SyncToken, inv.SyncToken); err != nil {
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
			h.store.Invoices.Delete(inv.Id)
			h.fireEvent("Invoice", inv.Id, "Delete")
		} else {
			h.store.Invoices.Set(inv.Id, existing)
			h.fireEvent("Invoice", inv.Id, "Void")
		}
		qboJSON(w, http.StatusOK, entityResponse("Invoice", existing))
		return
	}

	if inv.Id != "" {
		// Update
		existing, ok := h.store.Invoices.Get(inv.Id)
		if !ok {
			notFoundFault(w, "Invoice", inv.Id)
			return
		}
		if err := ValidateSyncToken(existing.SyncToken, inv.SyncToken); err != nil {
			staleSyncTokenFault(w)
			return
		}
		computeInvoiceTotals(&inv)
		// Preserve payment state.
		paid := existing.TotalAmt - existing.Balance
		inv.Balance = inv.TotalAmt - paid
		inv.SyncToken = IncrementSyncToken(existing.SyncToken)
		inv.MetaData.CreateTime = existing.MetaData.CreateTime
		inv.MetaData.LastUpdatedTime = h.store.Now()
		inv.Domain = "QBO"
		h.store.Invoices.Set(inv.Id, inv)
		h.fireEvent("Invoice", inv.Id, "Update")
	} else {
		// Create
		inv.Id = h.store.NextID()
		inv.SyncToken = "0"
		inv.Domain = "QBO"
		inv.MetaData = h.store.NewMetaData()
		computeInvoiceTotals(&inv)
		inv.Balance = inv.TotalAmt
		h.store.Invoices.Set(inv.Id, inv)
		h.journalInvoiceCreated(&inv)
		h.fireEvent("Invoice", inv.Id, "Create")
	}

	qboJSON(w, http.StatusOK, entityResponse("Invoice", inv))
}

func (h *Handler) GetInvoice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	inv, ok := h.store.Invoices.Get(id)
	if !ok {
		notFoundFault(w, "Invoice", id)
		return
	}
	qboJSON(w, http.StatusOK, entityResponse("Invoice", inv))
}

func computeInvoiceTotals(inv *store.Invoice) {
	var subTotal float64
	for i, line := range inv.Line {
		if line.DetailType == "SalesItemLineDetail" && line.SalesItemLineDetail != nil {
			inv.Line[i].Amount = line.SalesItemLineDetail.Qty * line.SalesItemLineDetail.UnitPrice
		}
		if line.DetailType != "SubTotalLineDetail" {
			subTotal += inv.Line[i].Amount
		}
	}
	tax := 0.0
	if inv.TxnTaxDetail != nil {
		tax = inv.TxnTaxDetail.TotalTax
	}
	inv.TotalAmt = subTotal + tax
	if inv.ExchangeRate > 0 {
		inv.HomeTotalAmt = inv.TotalAmt * inv.ExchangeRate
	} else {
		inv.HomeTotalAmt = inv.TotalAmt
	}
}
