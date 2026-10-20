package api

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// maxLengths are the field limits in the Contact schema.
var maxLengths = []struct {
	field string
	max   int
	get   func(store.Contact) string
}{
	{"Name", 255, func(c store.Contact) string { return c.Name }},
	{"FirstName", 255, func(c store.Contact) string { return c.FirstName }},
	{"LastName", 255, func(c store.Contact) string { return c.LastName }},
	{"EmailAddress", 255, func(c store.Contact) string { return c.EmailAddress }},
	{"ContactNumber", 50, func(c store.Contact) string { return c.ContactNumber }},
	{"AccountNumber", 50, func(c store.Contact) string { return c.AccountNumber }},
	{"CompanyNumber", 50, func(c store.Contact) string { return c.CompanyNumber }},
	{"TaxNumber", 50, func(c store.Contact) string { return c.TaxNumber }},
}

// validateContact returns the reasons a new contact is refused: a name is
// required and unique among active contacts, an account number is unique, and
// fields keep to their lengths. The wording is a guess at Xero's, written so the
// duplicate-name and duplicate-account-number checks in finance-ops recognise it.
func validateContact(c store.Contact, existing []store.Contact) []string {
	var msgs []string
	if strings.TrimSpace(c.Name) == "" {
		msgs = append(msgs, "Please enter a contact name.")
	}
	for _, f := range maxLengths {
		if utf8.RuneCountInString(f.get(c)) > f.max {
			msgs = append(msgs, fmt.Sprintf("%s must be %d characters or fewer.", f.field, f.max))
		}
	}
	for _, e := range existing {
		if c.Name != "" && e.ContactStatus != "ARCHIVED" && strings.EqualFold(e.Name, c.Name) {
			msgs = append(msgs, fmt.Sprintf("The contact name %s is already assigned to another contact. The contact name must be unique across all active contacts.", c.Name))
			break
		}
	}
	for _, e := range existing {
		if c.AccountNumber != "" && e.AccountNumber == c.AccountNumber {
			msgs = append(msgs, fmt.Sprintf("Account number %s already exists. Please enter a different account number.", c.AccountNumber))
			break
		}
	}
	return msgs
}
