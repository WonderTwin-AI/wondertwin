package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
	"github.com/wondertwin-ai/wondertwin/twinkit/state/journal"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func (h *Handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payments []store.Payment `json:"Payments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}
	if len(req.Payments) == 0 {
		xeroError(w, http.StatusBadRequest, "ValidationException", "At least one payment is required")
		return
	}

	var result []store.Payment
	for _, pmt := range req.Payments {
		if pmt.PaymentID == "" {
			pmt.PaymentID = newUUID()
		}
		if pmt.Status == "" {
			pmt.Status = "AUTHORISED"
		}
		pmt.UpdatedDateUTC = store.XeroDateNow()

		// Determine which document this payment applies to.
		var docID string
		var docType accounting.DocumentType
		if pmt.Invoice != nil && pmt.Invoice.InvoiceID != "" {
			docID = pmt.Invoice.InvoiceID
			// Determine type from stored invoice.
			inv, ok := h.store.Invoices.Get(docID)
			if !ok {
				xeroError(w, http.StatusBadRequest, "ValidationException", "Invoice not found: "+docID)
				return
			}
			if inv.Type == "ACCPAY" {
				docType = accounting.DocTypeBill
			} else {
				docType = accounting.DocTypeInvoice
			}

			if pmt.PaymentType == "" {
				if inv.Type == "ACCPAY" {
					pmt.PaymentType = "ACCPAYPAYMENT"
				} else {
					pmt.PaymentType = "ACCRECPAYMENT"
				}
			}

			// Apply via engine.
			enginePmt := &accounting.Payment{
				ID:            pmt.PaymentID,
				DocumentID:    docID,
				DocumentType:  docType,
				BankAccountID: pmt.Account.AccountID,
				Amount:        int64(pmt.Amount * 100),
				Currency:      inv.CurrencyCode,
				Date:          parseXeroDate(pmt.Date),
			}

			// Build engine doc from stored invoice.
			doc := &accounting.Document{
				ID:         inv.InvoiceID,
				Type:       docType,
				Status:     accounting.DocumentStatus(inv.Status),
				Currency:   inv.CurrencyCode,
				Total:      int64(inv.Total * 100),
				AmountPaid: int64(inv.AmountPaid * 100),
				AmountDue:  int64(inv.AmountDue * 100),
			}

			if err := h.engine.ApplyPayment(r.Context(), enginePmt, doc); err != nil {
				xeroError(w, http.StatusBadRequest, "ValidationException", err.Error())
				return
			}

			// Update stored invoice from engine doc.
			inv.Status = string(doc.Status)
			inv.AmountPaid = float64(doc.AmountPaid) / 100
			inv.AmountDue = float64(doc.AmountDue) / 100
			inv.UpdatedDateUTC = store.XeroDateNow()
			h.store.Invoices.Set(inv.InvoiceID, inv)
			h.updateContactBalance(inv.Contact.ContactID)
		}

		h.store.Payments.Set(pmt.PaymentID, pmt)
		result = append(result, pmt)
	}

	xeroJSON(w, http.StatusOK, map[string]any{"Payments": result})
}

func (h *Handler) ListPayments(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	all := h.store.Payments.List()
	xeroJSON(w, http.StatusOK, map[string]any{"Payments": paginate(all, page, pageSize)})
}

func (h *Handler) GetPayment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "PaymentID")
	pmt, ok := h.store.Payments.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "Payment not found")
		return
	}
	xeroJSON(w, http.StatusOK, map[string]any{"Payments": []store.Payment{pmt}})
}

func (h *Handler) DeletePayment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "PaymentID")
	pmt, ok := h.store.Payments.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "Payment not found")
		return
	}
	if pmt.Status == "DELETED" {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Payment already deleted")
		return
	}

	// Reverse the payment on the linked invoice.
	if pmt.Invoice != nil && pmt.Invoice.InvoiceID != "" {
		inv, ok := h.store.Invoices.Get(pmt.Invoice.InvoiceID)
		if ok {
			inv.AmountPaid -= pmt.Amount
			inv.AmountDue += pmt.Amount
			if inv.AmountPaid < 0 {
				inv.AmountPaid = 0
			}
			if inv.AmountDue > inv.Total {
				inv.AmountDue = inv.Total
			}
			// Revert status from PAID to AUTHORISED if needed.
			if inv.Status == "PAID" && inv.AmountDue > 0 {
				inv.Status = "AUTHORISED"
			}
			inv.UpdatedDateUTC = store.XeroDateNow()
			h.store.Invoices.Set(inv.InvoiceID, inv)
		}

		// Reverse journal entries.
		amtCents := int64(pmt.Amount * 100)
		currency := "USD"
		if inv, ok := h.store.Invoices.Get(pmt.Invoice.InvoiceID); ok && inv.CurrencyCode != "" {
			currency = inv.CurrencyCode
		}
		bankAcctID := pmt.Account.AccountID
		if bankAcctID == "" {
			bankAcctID = pmt.Account.Code
		}

		// Determine AR/AP based on invoice type.
		arAcctID := h.resolveAccountCode("200") // fallback
		if inv, ok := h.store.Invoices.Get(pmt.Invoice.InvoiceID); ok && inv.Type == "ACCPAY" {
			// Reverse bill payment: Credit AP, Debit Bank
			entries := []journal.Entry{
				{AccountID: arAcctID, Type: journal.Credit, Amount: amtCents,
					Currency: currency, SourceType: "payment_reversal", SourceID: id},
				{AccountID: bankAcctID, Type: journal.Debit, Amount: amtCents,
					Currency: currency, SourceType: "payment_reversal", SourceID: id},
			}
			h.engine.Journal().Append(journal.Transaction{Entries: entries})
		} else {
			// Reverse invoice payment: Debit AR, Credit Bank
			entries := []journal.Entry{
				{AccountID: arAcctID, Type: journal.Debit, Amount: amtCents,
					Currency: currency, SourceType: "payment_reversal", SourceID: id},
				{AccountID: bankAcctID, Type: journal.Credit, Amount: amtCents,
					Currency: currency, SourceType: "payment_reversal", SourceID: id},
			}
			h.engine.Journal().Append(journal.Transaction{Entries: entries})
		}
	}

	pmt.Status = "DELETED"
	pmt.UpdatedDateUTC = store.XeroDateNow()
	h.store.Payments.Set(id, pmt)

	xeroJSON(w, http.StatusOK, map[string]any{"Payments": []store.Payment{pmt}})
}
