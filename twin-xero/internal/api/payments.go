package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// paymentIn is a payment as a client sends it. Invoice may carry the whole
// invoice, as localstack/finance-ops sends it, so only its ID and number are read.
type paymentIn struct {
	store.Payment
	Invoice *store.PaymentInvoice `json:"Invoice,omitempty"`
	Date    *Msdate               `json:"Date,omitempty"`
}

// CreatePayment serves POST /api.xro/2.0/Payments, the call xero-python's
// create_payment makes: one payment sent bare, or {"Payments": [...]}. Each is
// recorded against an invoice and the answer is the plural envelope with status
// 200. If any payment is invalid none is recorded and the answer is a 400
// ValidationException (crud-001).
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
	var elements []validationElement
	var made []store.Payment
	var against []store.Invoice
	invalid := false
	for i := range sent {
		p, inv, msgs := h.buildPayment(tenant, sent[i])
		invalid = invalid || len(msgs) > 0
		elements = append(elements, validationElement{Object: in.Payments[i], Messages: msgs})
		if len(msgs) == 0 {
			made, against = append(made, p), append(against, inv)
		}
	}
	if invalid {
		writeValidationException(w, elements...)
		return
	}
	payments := h.store.Payments.For(tenant)
	out := make([]payment, len(made))
	for i, p := range made {
		payments.Set(p.PaymentID, p)
		out[i] = h.wirePayment(tenant, p, against[i])
	}
	h.writeEnvelope(w, http.StatusOK, "Payments", out)
}

// buildPayment validates one payment and returns it with the invoice it is
// against: the invoice and account must exist, and the date and a positive
// amount are required. The wording of the refusals is a guess at Xero's.
func (h *Handler) buildPayment(tenant string, in paymentIn) (store.Payment, store.Invoice, []string) {
	var msgs []string
	inv, found := h.findInvoice(tenant, in.Invoice)
	switch {
	case in.Invoice == nil || (in.Invoice.InvoiceID == "" && in.Invoice.InvoiceNumber == ""):
		msgs = append(msgs, "Invoice must be specified, with its InvoiceID or InvoiceNumber.")
	case !found:
		msgs = append(msgs, "Invoice could not be found.")
	}
	acct, ok := h.findAccount(tenant, in.Account, in.Code)
	if !ok {
		msgs = append(msgs, "Account could not be found: name a bank account by its Code or AccountID.")
	}
	if in.Date == nil {
		msgs = append(msgs, "Payment date must be specified.")
	}
	if in.Amount <= 0 {
		msgs = append(msgs, "Payment amount must be greater than zero.")
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
	return p, inv, nil
}

// findInvoice finds the invoice a payment names, by ID or by number.
func (h *Handler) findInvoice(tenant string, ref *store.PaymentInvoice) (store.Invoice, bool) {
	if ref == nil {
		return store.Invoice{}, false
	}
	for _, inv := range h.store.Invoices.For(tenant).List() {
		if (ref.InvoiceID != "" && inv.InvoiceID == ref.InvoiceID) || (ref.InvoiceID == "" && ref.InvoiceNumber != "" && inv.InvoiceNumber == ref.InvoiceNumber) {
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
