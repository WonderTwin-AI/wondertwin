package api

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// DeletePayment serves POST /api.xro/2.0/Payments/{PaymentID}, the call
// xero-python's delete_payment makes with {"Status": "DELETED"}. Xero publishes
// no DELETE for payments, so a delete is a status write (crud-002). The payment
// stays in the tenant with Status DELETED, and its invoice goes back to what it
// was before the payment: AmountPaid down, AmountDue restored, and AUTHORISED
// again if the payment had made it PAID. The answer is the plural envelope.
// Which other fields a payment update may carry is not modelled: only Status is
// read. An unknown payment is a 404 whose body is a guess.
func (h *Handler) DeletePayment(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	var in struct{ Payments []json.RawMessage }
	if err == nil {
		err = json.Unmarshal(raw, &in)
	}
	one := json.RawMessage(raw) // a bare update
	if err == nil && in.Payments != nil {
		if len(in.Payments) == 0 {
			err = io.ErrUnexpectedEOF
		} else {
			one = in.Payments[0]
		}
	}
	var sent struct{ Status string }
	if err == nil {
		err = json.Unmarshal(one, &sent)
	}
	if err != nil {
		h.writeProblem(w, r, http.StatusBadRequest, "Bad Request", "The request body is not valid JSON")
		return
	}

	tenant := tenantID(r)
	payments := h.store.Payments.For(tenant)
	p, ok := payments.Get(chi.URLParam(r, "PaymentID"))
	if !ok {
		h.writeProblem(w, r, http.StatusNotFound, "Not Found", "The resource you have specified cannot be found")
		return
	}
	var msgs []string
	switch {
	case sent.Status != "DELETED":
		msgs = append(msgs, "The only update a payment accepts is Status DELETED.")
	case p.Status == "DELETED":
		msgs = append(msgs, "Payment has already been deleted.")
	case p.IsReconciled:
		msgs = append(msgs, "A reconciled payment cannot be deleted.")
	}
	if len(msgs) > 0 {
		writeValidationException(w, validationElement{Object: one, Messages: msgs})
		return
	}

	now := h.store.Clock.Now().UTC()
	p.Status, p.UpdatedDateUTC, p.UpdatedDateUTCString = "DELETED", now, now.Format("2006-01-02T15:04:05.000")
	payments.Set(p.PaymentID, p)

	if p.CreditNote != nil {
		cn, _ := h.store.CreditNotes.For(tenant).Get(p.CreditNote.CreditNoteID)
		cn.Payments = slices.DeleteFunc(slices.Clone(cn.Payments), func(e map[string]any) bool { return e["PaymentID"] == p.PaymentID })
		cn.RemainingCredit = max(round(cn.Total-cn.AppliedAmount-paymentsTotal(cn.Payments), 2), 0)
		if cn.Status == "PAID" && cn.RemainingCredit > 0 {
			cn.Status, cn.FullyPaidOnDate = "AUTHORISED", nil
		}
		cn.UpdatedDateUTC, cn.UpdatedDateUTCString = now, p.UpdatedDateUTCString
		h.store.CreditNotes.For(tenant).Set(cn.CreditNoteID, cn)
		h.writeEnvelope(w, http.StatusOK, "Payments", []payment{h.wireCreditNotePayment(tenant, p, cn)})
		return
	}
	var inv store.Invoice
	if p.Invoice != nil {
		invoices := h.store.Invoices.For(tenant)
		if inv, ok = invoices.Get(p.Invoice.InvoiceID); ok {
			inv.AmountPaid = round(inv.AmountPaid-p.Amount, 2)
			inv.AmountDue = round(inv.Total-inv.AmountPaid-inv.AmountCredited, 2)
			if inv.Status == "PAID" {
				inv.Status, inv.FullyPaidOnDate = "AUTHORISED", nil
			}
			inv.Payments = slices.DeleteFunc(slices.Clone(inv.Payments), func(e map[string]any) bool { return e["PaymentID"] == p.PaymentID })
			inv.UpdatedDateUTC, inv.UpdatedDateUTCString = now, p.UpdatedDateUTCString
			invoices.Set(inv.InvoiceID, inv)
		}
	}
	h.writeEnvelope(w, http.StatusOK, "Payments", []payment{h.wirePayment(tenant, p, inv)})
}
