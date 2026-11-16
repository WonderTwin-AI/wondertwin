package api

import (
	"net/http"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// A body that is not a JSON object, or an envelope whose records are not, has
// no field to name; it used to be answered " is not valid".

func TestNonObjectBodiesAreNamedOnEveryWriteRoute(t *testing.T) {
	h := newTestHandler(t)
	routes := []struct{ method, path, envelope string }{
		{http.MethodPut, "/Contacts", "Contacts"}, {http.MethodPost, "/Contacts", "Contacts"},
		{http.MethodPut, "/Invoices", "Invoices"}, {http.MethodPost, "/Invoices", "Invoices"},
		{http.MethodPost, "/Invoices/INV-0001", "Invoices"},
		{http.MethodPut, "/CreditNotes", "CreditNotes"}, {http.MethodPost, "/CreditNotes", "CreditNotes"},
		{http.MethodPost, "/Payments", "Payments"}, {http.MethodPost, "/Payments/00000000-0000-4000-8000-000000000000", "Payments"},
		{http.MethodPut, "/BankTransactions", "BankTransactions"}, {http.MethodPost, "/BankTransactions", "BankTransactions"},
		{http.MethodPost, "/BankTransactions/00000000-0000-4000-8000-000000000000", "BankTransactions"},
		{http.MethodPut, "/ManualJournals", "ManualJournals"}, {http.MethodPost, "/ManualJournals", "ManualJournals"},
		{http.MethodPost, "/ManualJournals/00000000-0000-4000-8000-000000000000", "ManualJournals"},
	}
	for _, rt := range routes {
		for _, body := range []string{`[]`, `[{}]`, `"x"`, `7`, `{"` + rt.envelope + `":[1]}`} {
			rec := do(h, rt.method, "/api.xro/2.0"+rt.path, body, accounting(t, h, store.DefaultTenantID))
			got := decode(t, rec)
			want := nonObjectBodyMessage
			if rt.method == http.MethodPut && rt.path == "/Contacts" && body[0] == '{' {
				want = "Contacts is not valid" // PUT /Contacts decodes the envelope whole, so the field is named
			}
			if rec.Code != 400 || got["Type"] != "ValidationException" || !containsMessage(got, want) {
				t.Errorf("%s %s %s: want a 400 ValidationException saying %q, got %d %v", rt.method, rt.path, body, want, rec.Code, got)
			}
		}
	}
}

func containsMessage(body map[string]any, msg string) bool {
	elements, _ := body["Elements"].([]any)
	for _, e := range elements {
		el, _ := e.(map[string]any)
		errs, _ := el["ValidationErrors"].([]any)
		for _, v := range errs {
			if m, _ := v.(map[string]any); m["Message"] == msg {
				return true
			}
		}
	}
	return false
}
