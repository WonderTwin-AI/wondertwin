package api

import (
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// creditNote is a CreditNote on the wire, in and out. Its dates are
// /Date(ms+0000)/ (xero-python raises on ISO here, time-001) but a client may
// send YYYY-MM-DD, and Contact is the full contact on the way out.
type creditNote struct {
	store.CreditNote
	Contact         *contact     `json:"Contact,omitempty"`
	Date            *Msdate      `json:"Date,omitempty"`
	DueDate         *Msdate      `json:"DueDate,omitempty"`
	FullyPaidOnDate *Msdate      `json:"FullyPaidOnDate,omitempty"`
	UpdatedDateUTC  Msdate       `json:"UpdatedDateUTC"`
	Allocations     []allocation `json:"Allocations"`
}

// allocation is an Allocation on the wire, with its date as an msdate.
type allocation struct {
	store.Allocation
	Date *Msdate `json:"Date,omitempty"`
}

// inCreditNote returns the submitted credit note whole, with the contact kept as
// a reference. The create handler owns resetting every computed field.
func inCreditNote(w creditNote) store.CreditNote {
	in := w.CreditNote
	if w.Contact != nil {
		in.Contact = &store.ContactRef{ContactID: w.Contact.ContactID}
	}
	in.Date, in.DueDate = tm(w.Date), tm(w.DueDate)
	in.Allocations = make([]store.Allocation, len(w.Allocations))
	for i, a := range w.Allocations {
		in.Allocations[i] = a.Allocation
		in.Allocations[i].Date = tm(a.Date)
	}
	return in
}

// wireCreditNote readies a stored credit note for the wire: the contact is
// resolved, the lists Xero always returns are [], and the dates are msdates.
func (h *Handler) wireCreditNote(tenant string, in store.CreditNote) creditNote {
	if in.Payments == nil {
		in.Payments = []store.PaymentSummary{}
	}
	if in.LineItems == nil {
		in.LineItems = []store.LineItem{}
	}
	out := creditNote{CreditNote: in, UpdatedDateUTC: Msdate(in.UpdatedDateUTC), Allocations: []allocation{}}
	for _, a := range in.Allocations {
		wa := allocation{Allocation: a, Date: msd(a.Date)}
		out.Allocations = append(out.Allocations, wa)
	}
	if in.Contact != nil {
		if c, ok := h.store.Contacts.For(tenant).Get(in.Contact.ContactID); ok {
			wc := wireContact(c)
			out.Contact = &wc
		}
	}
	out.Date, out.DueDate, out.FullyPaidOnDate = msd(in.Date), msd(in.DueDate), msd(in.FullyPaidOnDate)
	return out
}
