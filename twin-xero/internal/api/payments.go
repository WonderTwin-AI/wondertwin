package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// paymentIn is a payment as a client sends it. Invoice may carry the whole
// invoice, as localstack/finance-ops sends it, so only its ID and number are read.
type paymentIn struct {
	store.Payment
	Invoice    *store.PaymentInvoice `json:"Invoice,omitempty"`
	Date       *Msdate               `json:"Date,omitempty"`
	CreditNote *store.CreditNoteRef  `json:"CreditNote,omitempty"`
	// Payments against these are not supported; a payment that names one is refused.
	Prepayment  json.RawMessage `json:"Prepayment,omitempty"`
	Overpayment json.RawMessage `json:"Overpayment,omitempty"`
}

// CreatePayment serves POST /api.xro/2.0/Payments, the call xero-python's
// create_payment makes: one payment sent bare, or {"Payments": [...]}. Each is
// recorded against an invoice, which then carries the paid amount, or against a
// credit note, which then carries it as a refund (creditnote_payment.go), and the
// answer is the plural envelope with status 200. If any payment is invalid none
// is recorded and the answer is a 400 ValidationException (crud-001, crud-004).
func (h *Handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	var in struct{ Payments []json.RawMessage }
	if err == nil {
		err = json.Unmarshal(raw, &in)
	}
	if err == nil && in.Payments == nil {
		in.Payments = []json.RawMessage{raw} // a bare payment
	}
	sent := make([]paymentIn, len(in.Payments))
	for i, item := range in.Payments {
		if err == nil {
			err = json.Unmarshal(item, &sent[i])
		}
	}
	if err != nil {
		h.writeProblem(w, r, http.StatusBadRequest, "Bad Request", "The request body is not valid JSON")
		return
	}

	tenant := tenantID(r)
	invoices := h.store.Invoices.For(tenant)
	working := map[string]store.Invoice{} // invoices as the payments so far leave them
	var elements []validationElement
	var made []store.Payment
	var touched []store.Invoice
	var notes []*store.CreditNote // the credit note a payment is for; nil for an invoice payment
	workingNotes := map[string]store.CreditNote{}
	invalid := false
	for i := range sent {
		var p store.Payment
		var inv store.Invoice
		var cn *store.CreditNote
		var msgs []string
		if sent[i].CreditNote != nil {
			p, cn, msgs = h.buildCreditNotePayment(tenant, sent[i], workingNotes)
		} else {
			p, inv, msgs = h.buildPayment(tenant, sent[i], working)
		}
		invalid = invalid || len(msgs) > 0
		elements = append(elements, validationElement{Object: in.Payments[i], Messages: msgs})
		if len(msgs) == 0 {
			if cn != nil {
				workingNotes[cn.CreditNoteID] = *cn
			} else {
				working[inv.InvoiceID] = inv
			}
			made, touched, notes = append(made, p), append(touched, inv), append(notes, cn)
		}
	}
	if invalid {
		writeValidationException(w, elements...)
		return
	}
	payments := h.store.Payments.For(tenant)
	out := make([]payment, len(made))
	updated := map[string]bool{} // an invoice or credit note several payments reach is reported once
	var noteOrder []string
	finalNotes := map[string]store.CreditNote{} // a credit note as its last payment leaves it
	for i, p := range made {
		payments.Set(p.PaymentID, p)
		if notes[i] != nil {
			h.store.CreditNotes.For(tenant).Set(notes[i].CreditNoteID, *notes[i])
			if _, seen := finalNotes[notes[i].CreditNoteID]; !seen {
				noteOrder = append(noteOrder, notes[i].CreditNoteID)
			}
			finalNotes[notes[i].CreditNoteID] = *notes[i]
			out[i] = h.wireCreditNotePayment(tenant, p, *notes[i])
			continue
		}
		invoices.Set(touched[i].InvoiceID, touched[i])
		if !updated[touched[i].InvoiceID] {
			updated[touched[i].InvoiceID] = true
			h.emit(r, tenant, "INVOICE", "UPDATE", touched[i].InvoiceID, nil)
		}
		out[i] = h.wirePayment(tenant, p, touched[i])
	}
	for _, id := range noteOrder {
		h.emitCreditNoteUpdate(r, tenant, finalNotes[id])
	}
	h.writeEnvelope(w, http.StatusOK, "Payments", out)
}

// buildPayment validates one payment and returns it with the invoice as the
// payment leaves it: AmountPaid up, AmountDue down, and PAID with a
// FullyPaidOnDate when nothing is due (crud-004). The wording of the refusals
// is a guess at Xero's, except the invoice status one, which is how Xero puts it.
func (h *Handler) buildPayment(tenant string, in paymentIn, working map[string]store.Invoice) (store.Payment, store.Invoice, []string) {
	var msgs []string
	inv, found := h.findInvoice(tenant, in.Invoice, working)
	switch {
	case in.Prepayment != nil || in.Overpayment != nil:
		msgs = append(msgs, "Only payments against an invoice or a credit note are supported.")
	case in.Invoice == nil || (in.Invoice.InvoiceID == "" && in.Invoice.InvoiceNumber == ""):
		msgs = append(msgs, "Invoice must be specified, with its InvoiceID or InvoiceNumber.")
	case !found:
		msgs = append(msgs, "Invoice could not be found.")
	case inv.Status != "AUTHORISED":
		msgs = append(msgs, "Invoice not of valid status for creating a payment")
	}
	acct, fieldMsgs := h.checkPaymentFields(tenant, in)
	msgs = append(msgs, fieldMsgs...)
	if in.Amount > 0 && found && inv.Status == "AUTHORISED" && in.Amount > inv.AmountDue+0.005 {
		msgs = append(msgs, "Payment amount exceeds the amount outstanding on this invoice.")
	}
	if len(msgs) > 0 {
		return store.Payment{}, store.Invoice{}, msgs
	}

	now := h.store.Clock.Now().UTC()
	p := in.Payment
	p.PaymentID, p.Status, p.HasAccount, p.HasValidationErrors = h.store.NewID(), "AUTHORISED", true, false
	p.Date, p.UpdatedDateUTC, p.UpdatedDateUTCString = in.Date.Time(), now, now.Format("2006-01-02T15:04:05.000")
	p.Invoice, p.Account, p.Code = &store.PaymentInvoice{InvoiceID: inv.InvoiceID}, &store.PaymentAccount{AccountID: acct.AccountID, Code: acct.Code}, ""
	p.PaymentType = map[string]string{"ACCREC": "ACCRECPAYMENT", "ACCPAY": "ACCPAYPAYMENT"}[inv.Type]
	p.BankAmount, p.StatusAttributeString = in.Amount, "OK"
	p.ValidationErrors, p.Warnings = nil, nil

	inv.AmountPaid = round(inv.AmountPaid+p.Amount, 2)
	inv.AmountDue = round(inv.Total-inv.AmountPaid-inv.AmountCredited, 2)
	if inv.AmountDue <= 0 {
		inv.Status, inv.FullyPaidOnDate = "PAID", &p.Date
	}
	inv.Payments = append(append([]map[string]any(nil), inv.Payments...), map[string]any{
		"PaymentID": p.PaymentID, "Date": jsonString(Msdate(p.Date)), "Amount": p.Amount,
		"Reference": p.Reference, "HasAccount": true, "HasValidationErrors": false,
	})
	inv.UpdatedDateUTC, inv.UpdatedDateUTCString = now, p.UpdatedDateUTCString
	return p, inv, nil
}

// checkPaymentFields checks what every payment needs, whatever it is for: a
// bank account that can take payments, a date and an amount above zero.
func (h *Handler) checkPaymentFields(tenant string, in paymentIn) (store.Account, []string) {
	var msgs []string
	acct, ok := h.findAccount(tenant, in.Account, in.Code)
	if !ok {
		msgs = append(msgs, "Account could not be found: name a bank account by its Code or AccountID.")
	} else if acct.Type != "BANK" && !acct.EnablePaymentsToAccount {
		msgs = append(msgs, fmt.Sprintf("Account %s is not a bank account and is not enabled for payments.", acct.Code))
	}
	if in.Date == nil {
		msgs = append(msgs, "Payment date must be specified.")
	}
	if in.Amount <= 0 {
		msgs = append(msgs, "Payment amount must be greater than zero.")
	}
	return acct, msgs
}

// findInvoice finds the invoice a payment names by ID or by number, preferring
// the copy the earlier payments of this request have already changed.
func (h *Handler) findInvoice(tenant string, ref *store.PaymentInvoice, working map[string]store.Invoice) (store.Invoice, bool) {
	if ref == nil {
		return store.Invoice{}, false
	}
	for _, inv := range h.store.Invoices.For(tenant).List() {
		if (ref.InvoiceID != "" && inv.InvoiceID == ref.InvoiceID) || (ref.InvoiceID == "" && ref.InvoiceNumber != "" && inv.InvoiceNumber == ref.InvoiceNumber) {
			if w, ok := working[inv.InvoiceID]; ok {
				return w, true
			}
			return inv, true
		}
	}
	return store.Invoice{}, false
}

// findAccount finds the account by ID or code, the code coming from Account or
// from the payment's own Code.
func (h *Handler) findAccount(tenant string, ref *store.PaymentAccount, code string) (store.Account, bool) {
	var id string
	if ref != nil {
		id, code = ref.AccountID, firstNonEmpty(ref.Code, code)
	}
	for _, a := range h.store.Accounts.For(tenant).List() {
		if (id != "" && a.AccountID == id) || (id == "" && code != "" && a.Code == code) {
			return a, true
		}
	}
	return store.Account{}, false
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// jsonString is the string a value writes as: Msdate writes /Date(ms+0000)/.
func jsonString(v json.Marshaler) string {
	b, _ := v.MarshalJSON()
	var s string
	_ = json.Unmarshal(b, &s)
	return s
}
