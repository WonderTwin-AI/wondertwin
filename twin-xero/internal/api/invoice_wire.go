package api

import (
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// invoice is an Invoice on the wire, in and out. Its six dates are
// /Date(ms+0000)/ (xero-python raises on ISO here, time-001) but a client may
// send YYYY-MM-DD, and Contact is the full contact on the way out.
type invoice struct {
	store.Invoice
	Contact             *contact `json:"Contact,omitempty"`
	Date                *Msdate  `json:"Date,omitempty"`
	DueDate             *Msdate  `json:"DueDate,omitempty"`
	ExpectedPaymentDate *Msdate  `json:"ExpectedPaymentDate,omitempty"`
	PlannedPaymentDate  *Msdate  `json:"PlannedPaymentDate,omitempty"`
	FullyPaidOnDate     *Msdate  `json:"FullyPaidOnDate,omitempty"`
	UpdatedDateUTC      Msdate   `json:"UpdatedDateUTC"`
}

func tm(m *Msdate) *time.Time {
	if m == nil {
		return nil
	}
	t := m.Time()
	return &t
}

// inInvoice returns the submitted invoice whole, with the contact kept as a
// reference. It does not strip anything: the create handler (s39 buildInvoice)
// owns resetting every computed or read-only field.
func inInvoice(w invoice) store.Invoice {
	in := w.Invoice
	if w.Contact != nil {
		in.Contact = &store.ContactRef{ContactID: w.Contact.ContactID}
	}
	in.Date, in.DueDate, in.ExpectedPaymentDate, in.PlannedPaymentDate = tm(w.Date), tm(w.DueDate), tm(w.ExpectedPaymentDate), tm(w.PlannedPaymentDate)
	return in
}

// wireInvoice readies a stored invoice for the wire: the contact is resolved,
// the lists Xero always returns are [], and the dates are msdates.
func (h *Handler) wireInvoice(tenant string, in store.Invoice) invoice {
	if in.Payments == nil {
		in.Payments = []store.PaymentSummary{}
	}
	if in.Prepayments == nil {
		in.Prepayments = []store.PrepaymentSummary{}
	}
	if in.Overpayments == nil {
		in.Overpayments = []store.OverpaymentSummary{}
	}
	if in.CreditNotes == nil {
		in.CreditNotes = []store.CreditNoteSummary{}
	}
	if in.Attachments == nil {
		in.Attachments = []store.Attachment{}
	}
	if in.LineItems == nil {
		in.LineItems = []store.LineItem{}
	}
	out := invoice{Invoice: in, UpdatedDateUTC: Msdate(in.UpdatedDateUTC)}
	if in.Contact != nil {
		if c, ok := h.store.Contacts.For(tenant).Get(in.Contact.ContactID); ok {
			wc := wireContact(c)
			out.Contact = &wc
		}
	}
	out.Date, out.DueDate, out.ExpectedPaymentDate = msd(in.Date), msd(in.DueDate), msd(in.ExpectedPaymentDate)
	out.PlannedPaymentDate, out.FullyPaidOnDate = msd(in.PlannedPaymentDate), msd(in.FullyPaidOnDate)
	return out
}

func msd(t *time.Time) *Msdate {
	if t == nil {
		return nil
	}
	m := Msdate(*t)
	return &m
}
