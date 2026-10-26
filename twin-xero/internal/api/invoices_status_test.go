package api

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// documented is Xero's table of valid status changes (documentation, Invoice
// status changes), written out independently of statusChanges.
var documented = map[string]map[string]bool{
	"DRAFT":      {"DRAFT": true, "SUBMITTED": true, "AUTHORISED": true, "DELETED": true},
	"SUBMITTED":  {"SUBMITTED": true, "AUTHORISED": true, "DRAFT": true, "DELETED": true},
	"AUTHORISED": {"AUTHORISED": true, "VOIDED": true},
}

func TestInvoiceStatusChangesFollowTheDocumentedTable(t *testing.T) {
	h := newTestHandler(t)
	all := []string{"DRAFT", "SUBMITTED", "AUTHORISED", "PAID", "VOIDED", "DELETED"}
	for _, from := range all {
		for _, to := range all {
			stored := store.Invoice{Type: "ACCREC", Status: from, Contact: &store.ContactRef{ContactID: newContactID(t, h, from+to)},
				LineItems: []store.LineItem{{AccountCode: "200", LineItemID: "l"}}}
			keys := map[string]json.RawMessage{"Status": json.RawMessage(`"` + to + `"`)}
			got, msgs := h.applyInvoiceUpdate(store.DefaultTenantID, stored, store.Invoice{Status: to}, keys, nil, 2)
			// An update that leaves the status as it is is always valid on a live invoice, a paid one included.
			switch want := documented[from][to] || from == to; {
			case from == "VOIDED" || from == "DELETED":
				if len(msgs) != 1 {
					t.Errorf("%s -> %s: a %s invoice cannot be updated, got %v", from, to, from, msgs)
				}
			case want && (len(msgs) != 0 || got.Status != to):
				t.Errorf("%s -> %s should be valid, got %v %q", from, to, msgs, got.Status)
			case !want && len(msgs) == 0:
				t.Errorf("%s -> %s is not in the table and must be refused", from, to)
			}
		}
	}
}

func TestInvoiceVoidAndDeleteAsXeroDocumentsThem(t *testing.T) {
	h := newTestHandler(t)
	authorised := newInvoice(t, h, "")
	_, body := postInvoice(t, h, authorised["InvoiceNumber"].(string), "", fmt.Sprintf(`{"InvoiceNumber":%q,"Status":"VOIDED"}`, authorised["InvoiceNumber"]))
	voided := firstInvoice(t, body)
	if voided["Status"] != "VOIDED" || voided["AmountDue"] != 0.0 || voided["Total"] != 100.0 {
		t.Fatalf("a void keeps the total and zeroes what is due: %v", voided)
	}
	code, body := postInvoice(t, h, authorised["InvoiceID"].(string), "", `{"Reference":"too late"}`)
	if code != 400 || !bodyMentions(body, "cannot be updated") {
		t.Errorf("a voided invoice cannot be updated: %d %v", code, body)
	}
	// its number is free again
	if code, _ := putInvoices(t, h, "", fmt.Sprintf(`{"Type":"ACCREC","Contact":{"ContactID":%q},"InvoiceNumber":%q,"Status":"DRAFT"}`,
		authorised["Contact"].(map[string]any)["ContactID"], authorised["InvoiceNumber"])); code != 200 {
		t.Errorf("a voided invoice's number can be reused, got %d", code)
	}

	draft := newInvoice(t, h, `,"Status":"DRAFT"`)
	_, body = postInvoice(t, h, draft["InvoiceID"].(string), "", `{"Status":"DELETED"}`)
	if got := firstInvoice(t, body); got["Status"] != "DELETED" || got["AmountDue"] != 0.0 {
		t.Errorf("deleting a draft is a status write: %v", got)
	}
	if code, body := postInvoice(t, h, newInvoice(t, h, "")["InvoiceID"].(string), "", `{"Status":"DELETED"}`); code != 400 || !bodyMentions(body, "AUTHORISED to DELETED") {
		t.Errorf("an authorised invoice cannot be deleted, only voided: %d %v", code, body)
	}
}

func TestInvoiceVoidWorksWhenTheWholeInvoiceIsPostedBack(t *testing.T) {
	// localstack/finance-ops voids by posting the invoice it read, with Status VOIDED.
	h := newTestHandler(t)
	inv := newInvoice(t, h, "")
	inv["Status"] = "VOIDED"
	raw, _ := json.Marshal(map[string]any{"Invoices": []any{inv}})
	if code, body := postInvoice(t, h, inv["InvoiceID"].(string), "", string(raw)); code != 200 || firstInvoice(t, body)["Status"] != "VOIDED" {
		t.Fatalf("%d %v", code, body)
	}
}

func TestInvoiceWithPaymentsCannotBeVoidedAndPaidOnesKeepTheirAmounts(t *testing.T) {
	h := newTestHandler(t)
	inv := newInvoice(t, h, "")
	id := inv["InvoiceID"].(string)
	invoices := h.store.Invoices.For(store.DefaultTenantID)
	stored, _, _ := findInvoice(invoices.List(), id)
	stored.AmountPaid, stored.AmountDue = 100, 0
	stored.Payments = []store.PaymentSummary{{PaymentID: "p1"}}
	invoices.Set(id, stored) // s12 adds the payments themselves

	if code, body := postInvoice(t, h, id, "", `{"Status":"VOIDED"}`); code != 400 || !bodyMentions(body, "payments applied") {
		t.Errorf("an invoice with payments cannot be voided: %d %v", code, body)
	}
	if code, body := postInvoice(t, h, id, "", `{"Reference":"still fine","DueDate":"2026-12-01"}`); code != 200 || firstInvoice(t, body)["Reference"] != "still fine" {
		t.Errorf("a paid invoice keeps its Reference and DueDate editable: %d %v", code, body)
	}
	for _, sent := range []string{`{"Date":"2026-01-01"}`, `{"LineItems":[{"Quantity":1,"UnitAmount":5,"AccountCode":"200"}]}`, `{"LineAmountTypes":"NoTax"}`} {
		if code, body := postInvoice(t, h, id, "", sent); code != 400 || !bodyMentions(body, "has been paid") {
			t.Errorf("%s: a paid invoice's amounts cannot change: %d %v", sent, code, body)
		}
	}
}
