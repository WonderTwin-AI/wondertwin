package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// A CurrencyCode outside the spec enum makes xero-python raise ValueError when
// it reads the record back, so every write that can store one refuses it with
// the message PUT /Accounts gives.

func refusedForCurrency(t *testing.T, what string, code int, body map[string]any) {
	t.Helper()
	if code != 400 || body["Type"] != "ValidationException" || !strings.Contains(rawString(body), "Currency Code 'ZZZ' is not valid.") {
		t.Fatalf("%s: want a 400 ValidationException naming the CurrencyCode, got %d %v", what, code, body)
	}
}

func TestInvoiceWritesRefuseAnOffSpecCurrencyCode(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	withCurrency := func(ccy string) string {
		return strings.Replace(newInvoiceBody(cid), `"Reference":"first"`, `"CurrencyCode":"`+ccy+`"`, 1)
	}
	code, body := putInvoices(t, h, "", withCurrency("ZZZ"))
	refusedForCurrency(t, "PUT /Invoices", code, body)
	code, body = postInvoices(t, h, "", `{"Invoices":[`+withCurrency("ZZZ")+`]}`)
	refusedForCurrency(t, "POST /Invoices (create)", code, body)
	if n := len(h.store.Invoices.For(store.DefaultTenantID).List()); n != 0 {
		t.Fatalf("a refused invoice must not be stored: %d stored", n)
	}

	code, body = putInvoices(t, h, "", withCurrency("EUR"))
	if code != 200 || firstInvoice(t, body)["CurrencyCode"] != "EUR" {
		t.Fatalf("EUR is in the spec enum: %d %v", code, body)
	}
	id := firstInvoice(t, body)["InvoiceID"].(string)
	code, body = postInvoice(t, h, id, "", `{"CurrencyCode":"ZZZ"}`)
	refusedForCurrency(t, "POST /Invoices/{id}", code, body)
	code, body = postInvoices(t, h, "", fmt.Sprintf(`{"Invoices":[{"InvoiceID":%q,"CurrencyCode":"ZZZ"}]}`, id))
	refusedForCurrency(t, "POST /Invoices (update)", code, body)
	if got, _ := h.store.Invoices.For(store.DefaultTenantID).Get(id); got.CurrencyCode != "EUR" {
		t.Errorf("a refused update must leave the currency alone: %q", got.CurrencyCode)
	}
}

func TestCreditNoteWritesRefuseAnOffSpecCurrencyCode(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	cn := func(ccy string) string {
		return fmt.Sprintf(`{"Type":"ACCRECCREDIT","Contact":{"ContactID":%q},"CurrencyCode":%q,
			"LineItems":[{"Description":"Refund","Quantity":1,"UnitAmount":10,"AccountCode":"200"}]}`, cid, ccy)
	}
	code, body := putCreditNotes(t, h, "", cn("ZZZ"))
	refusedForCurrency(t, "PUT /CreditNotes", code, body)
	code, body = postCreditNotes(t, h, "", `{"CreditNotes":[`+cn("ZZZ")+`]}`)
	refusedForCurrency(t, "POST /CreditNotes (create)", code, body)
	if n := len(h.store.CreditNotes.For(store.DefaultTenantID).List()); n != 0 {
		t.Fatalf("a refused credit note must not be stored: %d stored", n)
	}

	code, body = putCreditNotes(t, h, "", cn("GBP"))
	if code != 200 {
		t.Fatalf("GBP is in the spec enum: %d %v", code, body)
	}
	id := body["CreditNotes"].([]any)[0].(map[string]any)["CreditNoteID"].(string)
	code, body = postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNotes":[{"CreditNoteID":%q,"CurrencyCode":"ZZZ"}]}`, id))
	refusedForCurrency(t, "POST /CreditNotes (update)", code, body)
	if got, _ := h.store.CreditNotes.For(store.DefaultTenantID).Get(id); got.CurrencyCode != "GBP" {
		t.Errorf("a refused update must leave the currency alone: %q", got.CurrencyCode)
	}
}

func TestBankTransactionWritesRefuseAnOffSpecCurrencyCode(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Lender Capital")
	if code, body := putAccount(t, h, `{"Code":"095","Name":"No currency bank","Type":"BANK","BankAccountNumber":"9"}`); code != 200 {
		t.Fatalf("account setup: %d %v", code, body)
	}
	bt := func(bank, ccy string) string {
		return strings.Replace(strings.Replace(settlement(cid), `"Code":"090"`, `"Code":"`+bank+`"`, 1),
			`"Reference"`, `"CurrencyCode":"`+ccy+`","Reference"`, 1)
	}
	// 095 has no currency, so the sent one would be stored; 090 has one, which
	// would replace it, but an off-spec value is refused either way.
	for _, bank := range []string{"095", "090"} {
		code, body := putBankTransactions(t, h, "", bt(bank, "ZZZ"))
		refusedForCurrency(t, "PUT /BankTransactions on "+bank, code, body)
		code, body = postBankTransactions(t, h, "", `{"BankTransactions":[`+bt(bank, "ZZZ")+`]}`)
		refusedForCurrency(t, "POST /BankTransactions on "+bank, code, body)
	}
	if n := len(h.store.BankTransactions.For(store.DefaultTenantID).List()); n != 0 {
		t.Fatalf("a refused bank transaction must not be stored: %d stored", n)
	}
	code, body := putBankTransactions(t, h, "", bt("095", "GBP"))
	if code != 200 || body["BankTransactions"].([]any)[0].(map[string]any)["CurrencyCode"] != "GBP" {
		t.Fatalf("GBP is in the spec enum and 095 has no currency to replace it: %d %v", code, body)
	}
}
