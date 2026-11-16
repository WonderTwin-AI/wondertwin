package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// Taxability outside the spec enum makes xero-python raise ValueError when it
// reads the line back, so every write of line items refuses it.

func refusedForTaxability(t *testing.T, what string, code int, body map[string]any) {
	t.Helper()
	if code != 400 || body["Type"] != "ValidationException" || !strings.Contains(rawString(body), "Line 1: Taxability 'BOGUS' is not valid.") {
		t.Fatalf("%s: want a 400 ValidationException naming the Taxability, got %d %v", what, code, body)
	}
}

func TestLineTaxabilityIsCheckedOnBankTransactions(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Lender Capital")
	line := func(tax string) string {
		return `{"Description":"Advance","Quantity":1,"UnitAmount":50,"AccountCode":"200","Taxability":"` + tax + `"}`
	}
	bt := func(tax string) string {
		return `{"Type":"RECEIVE","Contact":{"ContactID":"` + cid + `"},"BankAccount":{"Code":"090"},"LineItems":[` + line(tax) + `]}`
	}
	code, body := putBankTransactions(t, h, "", bt("BOGUS"))
	refusedForTaxability(t, "PUT /BankTransactions", code, body)
	if n := len(h.store.BankTransactions.For(store.DefaultTenantID).List()); n != 0 {
		t.Fatalf("a refused bank transaction must not be stored: %d stored", n)
	}

	code, body = putBankTransactions(t, h, "", bt("EXEMPT"))
	if code != 200 {
		t.Fatalf("EXEMPT is in the spec enum: %d %v", code, body)
	}
	made := body["BankTransactions"].([]any)[0].(map[string]any)
	if l := made["LineItems"].([]any)[0].(map[string]any); l["Taxability"] != "EXEMPT" {
		t.Errorf("Taxability EXEMPT must be stored: %v", l)
	}
	id := made["BankTransactionID"].(string)
	rec := do(h, http.MethodPost, "/api.xro/2.0/BankTransactions/"+id, `{"LineItems":[`+line("BOGUS")+`]}`, accounting(t, h, store.DefaultTenantID))
	refusedForTaxability(t, "POST /BankTransactions/{id}", rec.Code, decode(t, rec))
	got, _ := h.store.BankTransactions.For(store.DefaultTenantID).Get(id)
	if got.LineItems[0].Taxability != "EXEMPT" {
		t.Errorf("a refused update must leave the lines alone: %+v", got.LineItems)
	}
}

func TestLineTaxabilityIsCheckedOnInvoices(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	inv := func(tax string) string {
		return fmt.Sprintf(`{"Type":"ACCREC","Contact":{"ContactID":%q},"LineItems":[{"Description":"Consulting","Quantity":1,"UnitAmount":50,"AccountCode":"200","Taxability":%q}]}`, cid, tax)
	}
	code, body := putInvoices(t, h, "", inv("BOGUS"))
	refusedForTaxability(t, "PUT /Invoices", code, body)

	code, body = putInvoices(t, h, "", inv("TAXABLE"))
	if code != 200 {
		t.Fatalf("TAXABLE is in the spec enum: %d %v", code, body)
	}
	id := firstInvoice(t, body)["InvoiceID"].(string)
	rec := do(h, http.MethodPost, "/api.xro/2.0/Invoices/"+id,
		`{"LineItems":[{"Description":"Consulting","Quantity":1,"UnitAmount":50,"AccountCode":"200","Taxability":"BOGUS"}]}`, accounting(t, h, store.DefaultTenantID))
	refusedForTaxability(t, "POST /Invoices/{id}", rec.Code, decode(t, rec))
	got, _ := h.store.Invoices.For(store.DefaultTenantID).Get(id)
	if got.LineItems[0].Taxability != "TAXABLE" {
		t.Errorf("a refused update must leave the lines alone: %+v", got.LineItems)
	}
}
