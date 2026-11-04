package api

import (
	"fmt"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// validateBankTransaction returns the reasons a new bank transaction is
// refused, run after its lines and totals are computed. The BankTransactions
// page documents the rules; it publishes the wording of only the Contact
// message, so the rest are guesses (err-005). A Contact is required, as the page
// says and the spec does not (crud-017; the operator chose to require it on
// 2026-11-03). The bank account must be an account of Type BANK, every line
// needs an account code from the chart that is not itself a bank account, and
// the total must be positive.
func (h *Handler) validateBankTransaction(tenant string, in store.BankTransaction) []string {
	var msgs []string
	if in.Contact == nil || in.Contact.ContactID == "" {
		msgs = append(msgs, "The Contact must contain at least 1 of the following elements to identify the contact: Name, ContactID, ContactNumber")
	} else if _, ok := h.store.Contacts.For(tenant).Get(in.Contact.ContactID); !ok {
		msgs = append(msgs, fmt.Sprintf("Contact %s could not be found.", in.Contact.ContactID))
	}

	switch {
	case in.BankAccount == nil || (in.BankAccount.AccountID == "" && in.BankAccount.Code == ""):
		msgs = append(msgs, "A BankAccount must be specified, by Code or AccountID.")
	default:
		ref := store.PaymentAccount{AccountID: in.BankAccount.AccountID, Code: in.BankAccount.Code}
		bank, ok := h.findAccount(tenant, &ref, "")
		switch {
		case !ok:
			msgs = append(msgs, "The BankAccount could not be found.")
		case bank.Type != "BANK":
			msgs = append(msgs, fmt.Sprintf("Account '%s' is not a bank account. Only accounts of Type BANK will be accepted.", bank.Code))
		}
	}

	if len(in.LineItems) == 0 {
		msgs = append(msgs, "At least one line item is required.")
	}
	chart := map[string]store.Account{}
	for _, a := range h.store.Accounts.For(tenant).List() {
		chart[a.Code] = a
	}
	for i, l := range in.LineItems {
		a, known := chart[l.AccountCode]
		switch {
		case l.AccountCode == "":
			msgs = append(msgs, fmt.Sprintf("Line %d: an account code must be specified.", i+1))
		case !known:
			msgs = append(msgs, fmt.Sprintf("Line %d: account code '%s' is not a valid code for this document.", i+1, l.AccountCode))
		case a.Type == "BANK":
			msgs = append(msgs, fmt.Sprintf("Line %d: account code '%s' is a bank account and cannot be used on a line.", i+1, l.AccountCode))
		}
		if strings.TrimSpace(l.Description) == "" && l.ItemCode == "" {
			msgs = append(msgs, fmt.Sprintf("Line %d: a Description must be specified.", i+1))
		}
		if l.Quantity != nil && *l.Quantity <= 0 {
			msgs = append(msgs, fmt.Sprintf("Line %d: the Quantity must be greater than zero.", i+1))
		}
		if l.UnitAmount != nil && *l.UnitAmount == 0 {
			msgs = append(msgs, fmt.Sprintf("Line %d: the UnitAmount must not be zero.", i+1))
		}
	}
	if len(in.LineItems) > 0 && in.Total <= 0 {
		msgs = append(msgs, "The total value of the bank transaction must be positive.")
	}
	return msgs
}
