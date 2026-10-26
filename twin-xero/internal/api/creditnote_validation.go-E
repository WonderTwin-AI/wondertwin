package api

import (
	"fmt"
	"unicode/utf8"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// validateCreditNote returns the reasons a new credit note is refused: a type of
// ACCRECCREDIT or ACCPAYCREDIT, a status of DRAFT, SUBMITTED or AUTHORISED (the
// others come from later writes), a contact that exists, a unique number, and
// for one that is not a draft, line items that name accounts. An account code
// that is not in the chart is refused on any status. The wording is a guess at
// Xero's.
func (h *Handler) validateCreditNote(tenant string, in store.CreditNote, existing []store.CreditNote) []string {
	var msgs []string
	if in.Type != "ACCRECCREDIT" && in.Type != "ACCPAYCREDIT" {
		msgs = append(msgs, "Type must be ACCRECCREDIT or ACCPAYCREDIT.")
	}
	switch in.Status {
	case "", "DRAFT", "SUBMITTED", "AUTHORISED":
	default:
		msgs = append(msgs, fmt.Sprintf("A credit note cannot be created with the status %s.", in.Status))
	}
	if in.Contact == nil || in.Contact.ContactID == "" {
		msgs = append(msgs, "Contact must be specified, with its ContactID.")
	} else if _, ok := h.store.Contacts.For(tenant).Get(in.Contact.ContactID); !ok {
		msgs = append(msgs, fmt.Sprintf("Contact %s could not be found.", in.Contact.ContactID))
	}
	if utf8.RuneCountInString(in.CreditNoteNumber) > 255 {
		msgs = append(msgs, "CreditNoteNumber must be 255 characters or fewer.")
	}
	if in.CreditNoteNumber != "" {
		for _, e := range existing {
			if e.Type == in.Type && e.CreditNoteNumber == in.CreditNoteNumber && e.Status != "VOIDED" && e.Status != "DELETED" {
				msgs = append(msgs, "Credit Note # must be unique.")
				break
			}
		}
	}
	draft := in.Status == "" || in.Status == "DRAFT"
	if len(in.LineItems) == 0 && !draft {
		msgs = append(msgs, "A credit note that is not a draft must have at least one line item.")
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
