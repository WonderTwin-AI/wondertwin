package api

import (
	"fmt"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// xero-python's LineAmountTypes enum has no "" value, so an update that sends
// "LineAmountTypes": "" takes the spec's default, Exclusive, as a create
// without one does, rather than storing "".

func TestInvoiceUpdateWithAnEmptyLineAmountTypesTakesTheDefault(t *testing.T) {
	h := newTestHandler(t)
	inv := newInvoice(t, h, `,"LineAmountTypes":"NoTax"`)
	id := inv["InvoiceID"].(string)
	if code, body := postInvoice(t, h, id, "", `{"LineAmountTypes":""}`); code != 200 {
		t.Fatalf("POST /Invoices/{id}: %d %v", code, body)
	}
	if got, _ := h.store.Invoices.For(store.DefaultTenantID).Get(id); got.LineAmountTypes != "Exclusive" {
		t.Errorf("POST /Invoices/{id}: want Exclusive stored, got %q", got.LineAmountTypes)
	}

	if code, body := postInvoice(t, h, id, "", `{"LineAmountTypes":"NoTax"}`); code != 200 {
		t.Fatalf("back to NoTax: %d %v", code, body)
	}
	if code, body := postInvoices(t, h, "", fmt.Sprintf(`{"Invoices":[{"InvoiceID":%q,"LineAmountTypes":""}]}`, id)); code != 200 {
		t.Fatalf("POST /Invoices: %d %v", code, body)
	}
	if got, _ := h.store.Invoices.For(store.DefaultTenantID).Get(id); got.LineAmountTypes != "Exclusive" {
		t.Errorf("POST /Invoices: want Exclusive stored, got %q", got.LineAmountTypes)
	}
}

func TestCreditNoteUpdateWithAnEmptyLineAmountTypesTakesTheDefault(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	code, body := putCreditNotes(t, h, "", fmt.Sprintf(`{"Type":"ACCRECCREDIT","Contact":{"ContactID":%q},"LineAmountTypes":"NoTax",
		"LineItems":[{"Description":"Refund","Quantity":1,"UnitAmount":10,"AccountCode":"200"}]}`, cid))
	if code != 200 {
		t.Fatalf("credit note setup: %d %v", code, body)
	}
	id := body["CreditNotes"].([]any)[0].(map[string]any)["CreditNoteID"].(string)
	if code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNotes":[{"CreditNoteID":%q,"LineAmountTypes":""}]}`, id)); code != 200 {
		t.Fatalf("POST /CreditNotes: %d %v", code, body)
	}
	if got, _ := h.store.CreditNotes.For(store.DefaultTenantID).Get(id); got.LineAmountTypes != "Exclusive" {
		t.Errorf("want Exclusive stored, got %q", got.LineAmountTypes)
	}
}
