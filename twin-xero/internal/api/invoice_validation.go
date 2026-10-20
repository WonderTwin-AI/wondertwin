package api

import (
	"fmt"
	"unicode/utf8"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// validateInvoice returns the reasons a new invoice is refused: a type of
// ACCREC or ACCPAY, a status of DRAFT, SUBMITTED or AUTHORISED (the others
// come from later writes), a contact that exists, a unique number, and for a
// document that is not a draft, line items that name accounts. An account code
// that is not in the chart is refused on any status.
// The wording is a guess at Xero's, except "Invoice # must be unique", which
// localstack/finance-ops recognises by those words.
func (h *Handler) validateInvoice(tenant string, in store.Invoice, existing []store.Invoice) []string {
	var msgs []string
	if in.Type != "ACCREC" && in.Type != "ACCPAY" {
		msgs = append(msgs, "Type must be ACCREC or ACCPAY.")
	}
	switch in.Status {
	case "", "DRAFT", "SUBMITTED", "AUTHORISED":
	default:
		msgs = append(msgs, fmt.Sprintf("An invoice cannot be created with the status %s.", in.Status))
	}
	if in.Contact == nil || in.Contact.ContactID == "" {
		msgs = append(msgs, "Contact must be specified, with its ContactID.")
	} else if _, ok := h.store.Contacts.For(tenant).Get(in.Contact.ContactID); !ok {
		msgs = append(msgs, fmt.Sprintf("Contact %s could not be found.", in.Contact.ContactID))
	}
	if utf8.RuneCountInString(in.InvoiceNumber) > 255 {
		msgs = append(msgs, "InvoiceNumber must be 255 characters or fewer.")
	}
	if in.InvoiceNumber != "" {
		for _, e := range existing {
			if e.Type == in.Type && e.InvoiceNumber == in.InvoiceNumber && e.Status != "VOIDED" && e.Status != "DELETED" {
				msgs = append(msgs, "Invoice # must be unique.")
				break
			}
		}
	}
	draft := in.Status == "" || in.Status == "DRAFT"
	if len(in.LineItems) == 0 && !draft {
		msgs = append(msgs, "An invoice that is not a draft must have at least one line item.")
	}
	chart := h.store.Accounts.For(tenant).List()
	for i, l := range in.LineItems {
		known := false
		for _, a := range chart {
			known = known || a.Code == l.AccountCode
		}
		switch {
		case l.AccountCode == "" && !draft:
			msgs = append(msgs, fmt.Sprintf("Line %d: an account code must be specified.", i+1))
		case l.AccountCode != "" && !known:
			msgs = append(msgs, fmt.Sprintf("Line %d: account code '%s' is not a valid code for this document.", i+1, l.AccountCode))
		}
	}
	return msgs
}
