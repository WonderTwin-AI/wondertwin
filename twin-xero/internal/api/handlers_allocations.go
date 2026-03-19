package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/store/journal"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func (h *Handler) AllocateCreditNote(w http.ResponseWriter, r *http.Request) {
	cnID := chi.URLParam(r, "CreditNoteID")
	cn, ok := h.store.CreditNotes.Get(cnID)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "CreditNote not found")
		return
	}
	if cn.Status != "AUTHORISED" {
		xeroError(w, http.StatusBadRequest, "ValidationException", "CreditNote must be AUTHORISED to allocate")
		return
	}

	var req struct {
		Allocations []store.Allocation `json:"Allocations"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}

	for _, alloc := range req.Allocations {
		inv, ok := h.store.Invoices.Get(alloc.Invoice.InvoiceID)
		if !ok {
			xeroError(w, http.StatusBadRequest, "ValidationException", "Invoice not found: "+alloc.Invoice.InvoiceID)
			return
		}

		applyAmt := alloc.Amount
		if applyAmt > cn.RemainingCredit {
			applyAmt = cn.RemainingCredit
		}
		if applyAmt > inv.AmountDue {
			applyAmt = inv.AmountDue
		}
		if applyAmt <= 0 {
			continue
		}

		// Reduce invoice amount due.
		inv.AmountDue -= applyAmt
		inv.AmountPaid += applyAmt
		if inv.AmountDue <= 0 {
			inv.AmountDue = 0
			inv.Status = "PAID"
		}
		inv.UpdatedDateUTC = store.XeroDateNow()
		h.store.Invoices.Set(inv.InvoiceID, inv)

		// Reduce credit note remaining credit.
		cn.RemainingCredit -= applyAmt
		cn.Allocations = append(cn.Allocations, store.Allocation{
			Invoice: store.InvoiceRef{InvoiceID: inv.InvoiceID},
			Amount:  applyAmt,
			Date:    store.XeroDateNow(),
		})

		// Post offsetting journal entry.
		// Credit note application: Debit AR, Credit income (reverses the original credit note posting).
		amtCents := int64(applyAmt * 100)
		currency := inv.CurrencyCode
		if currency == "" {
			currency = "USD"
		}
		entries := []journal.Entry{
			{AccountID: h.resolveAccountCode("200"), Type: journal.Debit, Amount: amtCents,
				Currency: currency, SourceType: "creditnote_allocation", SourceID: cnID},
			{AccountID: h.resolveAccountCode("400"), Type: journal.Credit, Amount: amtCents,
				Currency: currency, SourceType: "creditnote_allocation", SourceID: cnID},
		}
		h.engine.Journal().Append(journal.Transaction{Entries: entries})
	}

	if cn.RemainingCredit <= 0 {
		cn.Status = "PAID"
	}
	cn.UpdatedDateUTC = store.XeroDateNow()
	h.store.CreditNotes.Set(cnID, cn)

	xeroJSON(w, http.StatusOK, map[string]any{"CreditNotes": []store.CreditNote{cn}})
}
