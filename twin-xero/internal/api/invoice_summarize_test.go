package api

import (
	"fmt"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func mixedBatch(cid string) string {
	return fmt.Sprintf(`{"Invoices":[
		{"Type":"ACCREC","Contact":{"ContactID":%q},"Status":"AUTHORISED","LineItems":[{"AccountCode":"200","UnitAmount":10}]},
		{"Type":"ACCREC","Contact":{"ContactID":%q},"Status":"AUTHORISED","LineItems":[{"AccountCode":"999","UnitAmount":10}]}]}`, cid, cid)
}

func TestInvoiceSummarizeErrorsFalseCreatesTheValidOnesAndMarksTheRest(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	code, body := putInvoices(t, h, "?summarizeErrors=false", mixedBatch(cid))

	list, _ := body["Invoices"].([]any)
	if code != 200 || len(list) != 2 {
		t.Fatalf("status %d, body %v: want 200 with both invoices answered in place", code, body)
	}
	good, bad := list[0].(map[string]any), list[1].(map[string]any)
	if good["StatusAttributeString"] != "OK" || good["InvoiceID"] == nil || good["HasErrors"] != false {
		t.Errorf("the valid invoice = %v", good)
	}
	errs, _ := bad["ValidationErrors"].([]any)
	if bad["StatusAttributeString"] != "ERROR" || bad["HasErrors"] != true || len(errs) != 1 {
		t.Errorf("the invalid invoice = %v: want ERROR, HasErrors and one ValidationError", bad)
	}
	for _, k := range []string{"InvoiceID", "Total", "AmountDue", "UpdatedDateUTC"} {
		if _, ok := bad[k]; ok {
			t.Errorf("a refused invoice is answered as submitted: it must not carry a computed %s, got %v", k, bad[k])
		}
	}
	if n := h.store.Invoices.For(store.DefaultTenantID).Count(); n != 1 {
		t.Fatalf("%d invoices stored, want only the valid one", n)
	}
}

func TestInvoiceSummarizeErrorsDefaultFailsTheWholeRequest(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	for _, q := range []string{"", "?summarizeErrors=true"} {
		code, body := putInvoices(t, h, q, mixedBatch(cid))
		if code != 400 || body["Type"] != "ValidationException" || len(body["Elements"].([]any)) != 2 {
			t.Fatalf("%q: status %d, body %v: want 400 echoing both invoices", q, code, body)
		}
		if _, ok := body["Elements"].([]any)[0].(map[string]any)["Total"]; ok {
			t.Fatalf("%q: the echo must be the invoice as submitted, with no computed Total: %v", q, body["Elements"])
		}
	}
	if n := h.store.Invoices.For(store.DefaultTenantID).Count(); n != 0 {
		t.Fatalf("%d invoices stored, want none", n)
	}
}

func TestInvoiceSummarizeErrorsNumbersAreNotTakenByRefusedInvoices(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	_, body := putInvoices(t, h, "?summarizeErrors=false", mixedBatch(cid))
	_, next := putInvoices(t, h, "", fmt.Sprintf(`{"Type":"ACCREC","Contact":{"ContactID":%q}}`, cid))
	if first := body["Invoices"].([]any)[0].(map[string]any); first["InvoiceNumber"] != "INV-0001" || firstInvoice(t, next)["InvoiceNumber"] != "INV-0002" {
		t.Fatalf("numbers = %v then %v", first["InvoiceNumber"], firstInvoice(t, next)["InvoiceNumber"])
	}
}

func TestInvoiceSentToContactOnADraftIsAWarning(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	code, body := putInvoices(t, h, "", fmt.Sprintf(`{"Type":"ACCREC","Contact":{"ContactID":%q},"SentToContact":true}`, cid))
	inv := firstInvoice(t, body)
	warnings, _ := inv["Warnings"].([]any)
	if code != 200 || inv["StatusAttributeString"] != "WARNING" || len(warnings) != 1 || inv["SentToContact"] != false || inv["InvoiceID"] == nil {
		t.Fatalf("status %d, invoice %v: want created, WARNING, one warning and SentToContact false", code, inv)
	}
}
