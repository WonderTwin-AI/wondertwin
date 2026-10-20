package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func newInvoice(t *testing.T, h *Handler, extra string) map[string]any {
	t.Helper()
	cid := newContactID(t, h, fmt.Sprintf("Customer %d", len(h.store.Contacts.For(store.DefaultTenantID).List())))
	_, body := putInvoices(t, h, "", fmt.Sprintf(`{"Invoices":[{"Type":"ACCREC","Contact":{"ContactID":%q},"Date":"2026-09-01","DueDate":"2026-09-30",
		"Status":"AUTHORISED","Reference":"r1","LineItems":[{"Description":"Consulting","Quantity":2,"UnitAmount":50,"AccountCode":"200"}]%s}]}`, cid, extra))
	return firstInvoice(t, body)
}

func postInvoice(t *testing.T, h *Handler, ref, query, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPost, "/api.xro/2.0/Invoices/"+ref+query, body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func TestInvoiceUpdateChangesOnlyTheFieldsSent(t *testing.T) {
	h := newTestHandler(t)
	inv := newInvoice(t, h, "")
	h.store.Clock.Advance(time.Hour)

	code, body := postInvoice(t, h, inv["InvoiceID"].(string), "", `{"Reference":"r2","DueDate":"2026-10-30"}`)
	got := firstInvoice(t, body)
	if code != 200 || body["Status"] != "OK" || got["Reference"] != "r2" {
		t.Fatalf("status %d, body %v", code, body)
	}
	for _, k := range []string{"InvoiceID", "InvoiceNumber", "Status", "SubTotal", "Total", "AmountDue"} {
		if got[k] != inv[k] {
			t.Errorf("%s changed from %v to %v although it was not sent", k, inv[k], got[k])
		}
	}
	if got["UpdatedDateUTC"] == inv["UpdatedDateUTC"] {
		t.Error("UpdatedDateUTC must move on an update")
	}
	if s, _ := got["DueDate"].(string); !msdateValue.MatchString(s) || s == inv["DueDate"] {
		t.Errorf("DueDate = %v, want a new /Date(ms+0000)/", got["DueDate"])
	}
	if len(got["LineItems"].([]any)) != 1 {
		t.Errorf("an update that sends no LineItems must keep them: %v", got["LineItems"])
	}
}

func TestInvoiceUpdateFindsTheInvoiceByNumberAndRefusesAnUnknownOne(t *testing.T) {
	h := newTestHandler(t)
	inv := newInvoice(t, h, "")
	if code, body := postInvoice(t, h, inv["InvoiceNumber"].(string), "", `{"Reference":"by number"}`); code != 200 || firstInvoice(t, body)["Reference"] != "by number" {
		t.Fatalf("by number: %d %v", code, body)
	}
	code, body := postInvoice(t, h, "INV-9999", "", `{"Reference":"x"}`)
	if code != 404 || body["Title"] != "Not Found" {
		t.Fatalf("unknown invoice: %d %v", code, body)
	}
}

func TestInvoiceUpdateTakesAWholeInvoiceBackAndIgnoresWhatXeroOwns(t *testing.T) {
	// localstack/finance-ops reads an invoice, changes one field, and posts the lot.
	h := newTestHandler(t)
	inv := newInvoice(t, h, "")
	inv["Reference"], inv["Total"], inv["AmountPaid"], inv["Payments"] = "echoed", 999.0, 50.0, []any{map[string]any{"PaymentID": "x"}}
	raw, _ := json.Marshal(map[string]any{"Invoices": []any{inv}})

	code, body := postInvoice(t, h, inv["InvoiceID"].(string), "", string(raw))
	got := firstInvoice(t, body)
	if code != 200 || got["Reference"] != "echoed" {
		t.Fatalf("status %d, body %v", code, body)
	}
	if got["Total"] != 100.0 || got["AmountPaid"] != 0.0 || len(got["Payments"].([]any)) != 0 {
		t.Errorf("Xero's own fields must be ignored: Total %v, AmountPaid %v, Payments %v", got["Total"], got["AmountPaid"], got["Payments"])
	}
}

func TestInvoiceUpdateSentToContact(t *testing.T) {
	h := newTestHandler(t)
	inv := newInvoice(t, h, "")
	id := inv["InvoiceID"].(string)
	if _, body := postInvoice(t, h, id, "", `{"SentToContact":"true"}`); firstInvoice(t, body)["SentToContact"] != true {
		t.Errorf("the string \"true\" Xero's own example sends must mark it sent: %v", body)
	}
	if _, body := postInvoice(t, h, id, "", `{"SentToContact":null,"Reference":"kept"}`); firstInvoice(t, body)["SentToContact"] != true {
		t.Errorf("null must leave SentToContact unchanged (null-001): %v", body)
	}
	if _, body := postInvoice(t, h, id, "", `{"SentToContact":false}`); firstInvoice(t, body)["SentToContact"] != false {
		t.Errorf("false must mark it unsent: %v", body)
	}
}

func TestInvoiceUpdateRefusesWhatCannotBeApplied(t *testing.T) {
	h := newTestHandler(t)
	first, second := newInvoice(t, h, ""), newInvoice(t, h, "")
	id := first["InvoiceID"].(string)
	cases := map[string]string{
		`{"Contact":{"ContactID":"00000000-0000-4000-8000-0000000000ff"}}`: "could not be found",
		fmt.Sprintf(`{"InvoiceNumber":%q}`, second["InvoiceNumber"]):       "Invoice # must be unique",
		`{"Type":"ACCPAY"}`: "Type of an invoice cannot be changed",
	}
	for sent, want := range cases {
		code, body := postInvoice(t, h, id, "", sent)
		if code != 400 || body["Type"] != "ValidationException" || !bodyMentions(body, want) {
			t.Errorf("%s: %d %v, want a ValidationException mentioning %q", sent, code, body, want)
		}
	}
	if _, body := postInvoice(t, h, id, "", fmt.Sprintf(`{"InvoiceNumber":%q}`, first["InvoiceNumber"])); body["Status"] != "OK" {
		t.Errorf("keeping its own number is not a duplicate: %v", body)
	}
	code, body := postInvoice(t, h, id, "?summarizeErrors=false", `{"Type":"ACCPAY"}`)
	if got := firstInvoice(t, body); code != 200 || got["StatusAttributeString"] != "ERROR" || got["HasErrors"] != true {
		t.Errorf("summarizeErrors=false answers the refused invoice in place: %d %v", code, body)
	}
	if code, _ := postInvoice(t, h, id, "", `{not json`); code != 400 {
		t.Errorf("a body that is not JSON must be 400, got %d", code)
	}
}

func bodyMentions(body map[string]any, want string) bool {
	raw, _ := json.Marshal(body)
	return strings.Contains(string(raw), want)
}
