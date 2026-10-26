package api

import (
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

var creditPaymentType = map[string]string{"ACCRECCREDIT": "ARCREDITPAYMENT", "ACCPAYCREDIT": "APCREDITPAYMENT"}

// wireCreditNotePayment readies a payment made on a credit note, with the credit
// note as it now stands and its line items left out as Xero does on a payment.
func (h *Handler) wireCreditNotePayment(tenant string, p store.Payment, cn store.CreditNote) payment {
	out := payment{Payment: p, Date: Msdate(p.Date), UpdatedDateUTC: Msdate(p.UpdatedDateUTC), Account: p.Account}
	wc := h.wireCreditNote(tenant, cn)
	wc.LineItems = []store.LineItem{}
	out.CreditNote = &wc
	return out
}

// buildCreditNotePayment validates a payment made on a credit note, the refund
// localstack/finance-ops books with create_payment(credit_note, account)
// (ARCREDITPAYMENT), and returns it with the credit note as it leaves it: the
// payment listed, RemainingCredit down, and PAID with a FullyPaidOnDate when
// nothing is left. The rules are guesses; the research does not cover them.
//
// finance-ops allocates the whole credit note to an invoice and then pays its
// full total, so by default a credit note that is AUTHORISED or already PAID is
// accepted, and the payments on it may add up to its Total. Under StrictAuth the
// credit note must be AUTHORISED and the amount within its RemainingCredit.
func (h *Handler) buildCreditNotePayment(tenant string, in paymentIn, working map[string]store.CreditNote) (store.Payment, *store.CreditNote, []string) {
	var msgs []string
	ref := in.CreditNote
	cn, found := h.findCreditNote(tenant, ref, working)
	strict := h.store.StrictAuth.Load()
	switch {
	case in.Invoice != nil || in.Prepayment != nil || in.Overpayment != nil:
		msgs = append(msgs, "A payment is for an invoice or a credit note, not both or a prepayment or overpayment.")
	case ref.CreditNoteID == "" && ref.CreditNoteNumber == "":
		msgs = append(msgs, "CreditNote must be specified, with its CreditNoteID or CreditNoteNumber.")
	case !found:
		msgs = append(msgs, "Credit note could not be found.")
	case cn.Status != "AUTHORISED" && (strict || cn.Status != "PAID"):
		msgs = append(msgs, "Credit note not of valid status for creating a payment")
	}
	acct, fieldMsgs := h.checkPaymentFields(tenant, in)
	msgs = append(msgs, fieldMsgs...)
	paid := paymentsTotal(cn.Payments)
	room := cn.Total - paid
	if strict {
		room = cn.RemainingCredit
	}
	if in.Amount > 0 && found && in.Amount > room+0.005 {
		msgs = append(msgs, "Payment amount exceeds the credit remaining on this credit note.")
	}
	if len(msgs) > 0 {
		return store.Payment{}, nil, msgs
	}

	now := h.store.Clock.Now().UTC()
	p := in.Payment
	p.PaymentID, p.Status, p.HasAccount, p.HasValidationErrors = h.store.NewID(), "AUTHORISED", true, false
	p.Date, p.UpdatedDateUTC, p.UpdatedDateUTCString = in.Date.Time(), now, now.Format("2006-01-02T15:04:05.000")
	p.Invoice, p.CreditNote = nil, &store.CreditNoteRef{CreditNoteID: cn.CreditNoteID}
	p.Account, p.Code = &store.PaymentAccount{AccountID: acct.AccountID, Code: acct.Code}, ""
	p.PaymentType, p.BankAmount, p.StatusAttributeString = creditPaymentType[cn.Type], in.Amount, "OK"
	p.ValidationErrors, p.Warnings = nil, nil

	cn.Payments = append(append([]map[string]any(nil), cn.Payments...), map[string]any{
		"PaymentID": p.PaymentID, "Date": jsonString(Msdate(p.Date)), "Amount": p.Amount,
		"Reference": p.Reference, "HasAccount": true, "HasValidationErrors": false,
	})
	cn.RemainingCredit = max(round(cn.Total-cn.AppliedAmount-paid-p.Amount, 2), 0)
	if cn.RemainingCredit == 0 && cn.Status != "PAID" {
		cn.Status, cn.FullyPaidOnDate = "PAID", &p.Date
	}
	cn.UpdatedDateUTC, cn.UpdatedDateUTCString = now, p.UpdatedDateUTCString
	return p, &cn, nil
}

// paymentsTotal is what the payments listed on a credit note add up to.
func paymentsTotal(list []map[string]any) float64 {
	var sum float64
	for _, m := range list {
		sum += m["Amount"].(float64)
	}
	return sum
}

// findCreditNote finds the credit note a payment names by ID or by number,
// preferring the copy the earlier payments of this request have changed.
func (h *Handler) findCreditNote(tenant string, ref *store.CreditNoteRef, working map[string]store.CreditNote) (store.CreditNote, bool) {
	for _, cn := range h.store.CreditNotes.For(tenant).List() {
		if (ref.CreditNoteID != "" && strings.EqualFold(cn.CreditNoteID, ref.CreditNoteID)) ||
			(ref.CreditNoteID == "" && ref.CreditNoteNumber != "" && cn.CreditNoteNumber == ref.CreditNoteNumber) {
			if w, ok := working[cn.CreditNoteID]; ok {
				return w, true
			}
			return cn, true
		}
	}
	return store.CreditNote{}, false
}
