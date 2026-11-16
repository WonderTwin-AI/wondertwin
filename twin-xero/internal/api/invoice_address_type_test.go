package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// An InvoiceAddressType outside the spec enum makes xero-python raise
// ValueError when it reads the invoice or credit note back, so the creates that
// store InvoiceAddresses refuse it.

func refusedForInvoiceAddressType(t *testing.T, what string, code int, body map[string]any) {
	t.Helper()
	if code != 400 || body["Type"] != "ValidationException" || !strings.Contains(rawString(body), "Invoice Address Type 'BOGUS' is not valid.") {
		t.Fatalf("%s: want a 400 ValidationException naming the InvoiceAddressType, got %d %v", what, code, body)
	}
}

func TestInvoiceAndCreditNoteCreatesRefuseAnOffSpecInvoiceAddressType(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	addresses := func(typ string) string {
		return `"InvoiceAddresses":[{"InvoiceAddressType":"` + typ + `","City":"Zurich"}]`
	}
	inv := func(typ string) string {
		return strings.Replace(newInvoiceBody(cid), `"Reference":"first"`, addresses(typ), 1)
	}
	cn := func(typ string) string {
		return fmt.Sprintf(`{"Type":"ACCRECCREDIT","Contact":{"ContactID":%q},%s,
			"LineItems":[{"Description":"Refund","Quantity":1,"UnitAmount":10,"AccountCode":"200"}]}`, cid, addresses(typ))
	}
	code, body := putInvoices(t, h, "", inv("BOGUS"))
	refusedForInvoiceAddressType(t, "PUT /Invoices", code, body)
	code, body = postInvoices(t, h, "", `{"Invoices":[`+inv("BOGUS")+`]}`)
	refusedForInvoiceAddressType(t, "POST /Invoices", code, body)
	code, body = putCreditNotes(t, h, "", cn("BOGUS"))
	refusedForInvoiceAddressType(t, "PUT /CreditNotes", code, body)
	code, body = postCreditNotes(t, h, "", `{"CreditNotes":[`+cn("BOGUS")+`]}`)
	refusedForInvoiceAddressType(t, "POST /CreditNotes", code, body)
	if n := len(h.store.Invoices.For(store.DefaultTenantID).List()) + len(h.store.CreditNotes.For(store.DefaultTenantID).List()); n != 0 {
		t.Fatalf("a refused document must not be stored: %d stored", n)
	}

	if code, body := putInvoices(t, h, "", inv("TO")); code != 200 {
		t.Fatalf("TO is in the spec enum: %d %v", code, body)
	}
	if code, body := putCreditNotes(t, h, "", cn("FROM")); code != 200 {
		t.Fatalf("FROM is in the spec enum: %d %v", code, body)
	}
}
