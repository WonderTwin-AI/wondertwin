package api

import (
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// An item created through POST /BankTransactions is held to the rules of PUT
// (crud-022): each required field it leaves out refuses it with that field's
// message, and nothing is written.
func TestUpdateOrCreateBankTransactionsRefusesACreateMissingAField(t *testing.T) {
	line := `{"Description":"Fee","UnitAmount":12.5,"AccountCode":"404"}`
	for _, c := range []struct{ name, body, want string }{
		{"no Contact", `{"Type":"SPEND","BankAccount":{"Code":"090"},"LineItems":[` + line + `]}`,
			"The Contact must contain at least 1 of the following elements to identify the contact"},
		{"no BankAccount", `{"Type":"SPEND","Contact":{"ContactID":"CID"},"LineItems":[` + line + `]}`,
			"A BankAccount must be specified, by Code or AccountID."},
		{"no LineItems", `{"Type":"SPEND","Contact":{"ContactID":"CID"},"BankAccount":{"Code":"090"}}`,
			"At least one line item is required."},
		{"no line AccountCode", `{"Type":"SPEND","Contact":{"ContactID":"CID"},"BankAccount":{"Code":"090"},"LineItems":[{"Description":"Fee","UnitAmount":12.5}]}`,
			"Line 1: an account code must be specified."},
		{"no line Description", `{"Type":"SPEND","Contact":{"ContactID":"CID"},"BankAccount":{"Code":"090"},"LineItems":[{"UnitAmount":12.5,"AccountCode":"404"}]}`,
			"Line 1: a Description must be specified."},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newTestHandler(t)
			body := `{"BankTransactions":[` + strings.ReplaceAll(c.body, "CID", newContactID(t, h, "Bank")) + `]}`
			code, out := postBankTransactions(t, h, "", body)
			if code != 400 || out["Type"] != "ValidationException" || !strings.Contains(bankTransactionJSON(out), c.want) {
				t.Fatalf("want a 400 ValidationException with %q, got %d %v", c.want, code, out)
			}
			if n := h.store.BankTransactions.For(store.DefaultTenantID).Count(); n != 0 {
				t.Errorf("a refused bank transaction must not be stored, got %d", n)
			}
		})
	}
}

// An item created through POST /ManualJournals is held to the rules of PUT
// (crud-022): each required field it leaves out refuses it, and nothing is
// written.
func TestUpdateOrCreateManualJournalsRefusesACreateMissingAField(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{"no Narration", `{"JournalLines":[{"LineAmount":5,"AccountCode":"090"},{"LineAmount":-5,"AccountCode":"2100"}]}`,
			"Narration must be specified."},
		{"no JournalLines", `{"Narration":"n"}`,
			"A manual journal must have at least two journal lines."},
		{"one JournalLine", `{"Narration":"n","JournalLines":[{"LineAmount":0,"AccountCode":"090"}]}`,
			"A manual journal must have at least two journal lines."},
		{"no line account", `{"Narration":"n","JournalLines":[{"LineAmount":5},{"LineAmount":-5,"AccountCode":"2100"}]}`,
			"Journal line 1: an account code must be specified."},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newTestHandler(t)
			loanAccount(t, h)
			code, out := postManualJournals(t, h, "", `{"ManualJournals":[`+c.body+`]}`)
			if code != 400 || out["Type"] != "ValidationException" || !strings.Contains(bankTransactionJSON(out), c.want) {
				t.Fatalf("want a 400 ValidationException with %q, got %d %v", c.want, code, out)
			}
			if n := h.store.ManualJournals.For(store.DefaultTenantID).Count(); n != 0 {
				t.Errorf("a refused manual journal must not be stored, got %d", n)
			}
		})
	}
}
