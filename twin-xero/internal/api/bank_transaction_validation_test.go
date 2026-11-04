package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func TestBankTransactionValidationRefusesWhatXeroDocuments(t *testing.T) {
	line := `{"Description":"Advance","UnitAmount":5000,"AccountCode":"200"}`
	for _, c := range []struct{ name, body, want string }{
		{"no contact", `{"Type":"RECEIVE","BankAccount":{"Code":"090"},"LineItems":[` + line + `]}`,
			"The Contact must contain at least 1 of the following elements to identify the contact"},
		{"unknown contact", `{"Type":"RECEIVE","Contact":{"ContactID":"nope"},"BankAccount":{"Code":"090"},"LineItems":[` + line + `]}`,
			"Contact nope could not be found."},
		{"no bank account", `{"Type":"RECEIVE","Contact":{"ContactID":"CID"},"LineItems":[` + line + `]}`,
			"A BankAccount must be specified"},
		{"unknown bank account", `{"Type":"RECEIVE","Contact":{"ContactID":"CID"},"BankAccount":{"Code":"999"},"LineItems":[` + line + `]}`,
			"The BankAccount could not be found."},
		{"not a bank account", `{"Type":"RECEIVE","Contact":{"ContactID":"CID"},"BankAccount":{"Code":"200"},"LineItems":[` + line + `]}`,
			"Only accounts of Type BANK will be accepted."},
		{"no lines", `{"Type":"RECEIVE","Contact":{"ContactID":"CID"},"BankAccount":{"Code":"090"},"LineItems":[]}`,
			"At least one line item is required."},
		{"line without account", `{"Type":"RECEIVE","Contact":{"ContactID":"CID"},"BankAccount":{"Code":"090"},"LineItems":[{"Description":"x","UnitAmount":5}]}`,
			"Line 1: an account code must be specified."},
		{"line with unknown account", `{"Type":"RECEIVE","Contact":{"ContactID":"CID"},"BankAccount":{"Code":"090"},"LineItems":[{"Description":"x","UnitAmount":5,"AccountCode":"476"}]}`,
			"Line 1: account code '476' is not a valid code"},
		{"line on the bank account", `{"Type":"RECEIVE","Contact":{"ContactID":"CID"},"BankAccount":{"Code":"090"},"LineItems":[{"Description":"x","UnitAmount":5,"AccountCode":"090"}]}`,
			"is a bank account and cannot be used on a line"},
		{"line without description", `{"Type":"RECEIVE","Contact":{"ContactID":"CID"},"BankAccount":{"Code":"090"},"LineItems":[{"UnitAmount":5,"AccountCode":"200"}]}`,
			"Line 1: a Description must be specified."},
		{"zero quantity", `{"Type":"RECEIVE","Contact":{"ContactID":"CID"},"BankAccount":{"Code":"090"},"LineItems":[{"Description":"x","Quantity":0,"UnitAmount":5,"AccountCode":"200"}]}`,
			"the Quantity must be greater than zero"},
		{"zero unit amount", `{"Type":"RECEIVE","Contact":{"ContactID":"CID"},"BankAccount":{"Code":"090"},"LineItems":[{"Description":"x","UnitAmount":0,"AccountCode":"200"}]}`,
			"the UnitAmount must not be zero"},
		{"negative total", `{"Type":"RECEIVE","Contact":{"ContactID":"CID"},"BankAccount":{"Code":"090"},"LineItems":[{"Description":"x","UnitAmount":-5,"AccountCode":"200"}]}`,
			"The total value of the bank transaction must be positive."},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newTestHandler(t)
			body := strings.ReplaceAll(c.body, "CID", newContactID(t, h, "Lender Capital"))
			code, out := putBankTransactions(t, h, "", body)
			if code != 400 || out["Type"] != "ValidationException" || !strings.Contains(bankTransactionJSON(out), c.want) {
				t.Fatalf("want a 400 ValidationException with %q, got %d %v", c.want, code, out)
			}
			if n := h.store.BankTransactions.For(store.DefaultTenantID).Count(); n != 0 {
				t.Errorf("a refused bank transaction must not be stored, got %d", n)
			}
		})
	}
}

func TestBankTransactionValidationAcceptsTheBankAccountByID(t *testing.T) {
	h := newTestHandler(t)
	var bankID string
	for _, a := range h.store.Accounts.For(store.DefaultTenantID).List() {
		if a.Code == "090" {
			bankID = a.AccountID
		}
	}
	body := `{"Type":"SPEND","Contact":{"ContactID":"` + newContactID(t, h, "Bank") + `"},"BankAccount":{"AccountID":"` + bankID + `"},` +
		`"LineItems":[{"Description":"Fee","UnitAmount":12.5,"AccountCode":"404"}]}`
	if code, out := putBankTransactions(t, h, "", body); code != 200 {
		t.Fatalf("a bank account named by AccountID must be accepted: %d %v", code, out)
	}
}

func bankTransactionJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
