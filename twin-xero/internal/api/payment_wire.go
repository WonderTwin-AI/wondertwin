package api

import "github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"

// payment is a Payment on the wire: the invoice is the full invoice with its
// amounts after the payment, the date is /Date(ms+0000)/ (time-001), and the
// account carries its ID and code.
type payment struct {
	store.Payment
	Invoice        *invoice              `json:"Invoice,omitempty"`
	Account        *store.PaymentAccount `json:"Account,omitempty"`
	Date           Msdate                `json:"Date"`
	UpdatedDateUTC Msdate                `json:"UpdatedDateUTC"`
}

// wirePayment readies a stored payment for the wire, with the invoice as it now
// stands: full, with its line items left out as Xero does on a payment.
func (h *Handler) wirePayment(tenant string, p store.Payment, inv store.Invoice) payment {
	out := payment{Payment: p, Date: Msdate(p.Date), UpdatedDateUTC: Msdate(p.UpdatedDateUTC), Account: p.Account}
	wi := h.wireInvoice(tenant, inv)
	wi.LineItems = []store.LineItem{}
	out.Invoice = &wi
	return out
}
