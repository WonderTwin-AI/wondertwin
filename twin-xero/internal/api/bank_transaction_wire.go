package api

import (
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// bankTransaction is a BankTransaction on the wire, in and out. Its dates are
// /Date(ms+0000)/ (time-001) but a client may send YYYY-MM-DD, Contact is the
// full contact on the way out, and BankAccount carries its ID, code and name.
type bankTransaction struct {
	store.BankTransaction
	Contact        *contact `json:"Contact,omitempty"`
	Date           *Msdate  `json:"Date,omitempty"`
	UpdatedDateUTC Msdate   `json:"UpdatedDateUTC"`
}

// inBankTransaction returns the submitted bank transaction whole, with the
// contact kept as a reference. The create handler owns resetting every computed
// field.
func inBankTransaction(w bankTransaction) store.BankTransaction {
	in := w.BankTransaction
	in.Contact = nil
	if w.Contact != nil {
		in.Contact = &store.ContactRef{ContactID: w.Contact.ContactID}
	}
	in.Date = tm(w.Date)
	return in
}

// wireBankTransaction readies a stored bank transaction for the wire: the
// contact and bank account are resolved, the lists Xero always returns are [],
// and the dates are msdates.
func (h *Handler) wireBankTransaction(tenant string, in store.BankTransaction) bankTransaction {
	if in.LineItems == nil {
		in.LineItems = []store.LineItem{}
	}
	if in.BankAccount != nil {
		ref := store.PaymentAccount{AccountID: in.BankAccount.AccountID, Code: in.BankAccount.Code}
		if a, ok := h.findAccount(tenant, &ref, ""); ok {
			in.BankAccount = &store.AccountRef{AccountID: a.AccountID, Code: a.Code, Name: a.Name}
		}
	}
	out := bankTransaction{BankTransaction: in, UpdatedDateUTC: Msdate(in.UpdatedDateUTC), Date: msd(in.Date)}
	if in.Contact != nil {
		if c, ok := h.store.Contacts.For(tenant).Get(in.Contact.ContactID); ok {
			wc := wireContact(c)
			out.Contact = &wc
		}
	}
	return out
}

// bankTransactionLines computes a bank transaction's line amounts and totals.
// A bank transaction that names no LineAmountTypes is tax inclusive, as the
// BankTransactions page says; the spec's shared schema says exclusive
// (crud-018), and the operator chose inclusive on 2026-11-03.
func bankTransactionLines(bt *store.BankTransaction, unitdp int) []string {
	if bt.LineAmountTypes == "" {
		bt.LineAmountTypes = "Inclusive"
	}
	lines, t, msgs := computeLines(bt.LineAmountTypes, bt.LineItems, unitdp)
	bt.LineItems, bt.SubTotal, bt.TotalTax, bt.Total = lines, t.SubTotal, t.TotalTax, t.Total
	return msgs
}
