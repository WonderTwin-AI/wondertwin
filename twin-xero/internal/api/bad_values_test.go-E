package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// Audit finding F3: a value of the wrong type, or a date that cannot be read, is
// a ValidationException that names the field.
func TestAnUnreadableValueNamesItsField(t *testing.T) {
	cases := []struct{ name, path, body, field string }{
		{"a number for a name", "/api.xro/2.0/Contacts", `{"Contacts":[{"Name":123}]}`, "Name"},
		{"a slash date", "/api.xro/2.0/Invoices", `{"Type":"ACCREC","Date":"10/01/2026"}`, "Date"},
		{"an impossible date", "/api.xro/2.0/Invoices", `{"Type":"ACCREC","DueDate":"2026-13-45"}`, "DueDate"},
		{"an empty date", "/api.xro/2.0/Invoices", `{"Type":"ACCREC","Date":""}`, "Date"},
		{"a number for a date", "/api.xro/2.0/Invoices", `{"Type":"ACCREC","Date":20260101}`, "Date"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t)
			rec := do(h, http.MethodPut, tc.path, tc.body, accounting(t, h, store.DefaultTenantID))
			body := decode(t, rec)
			if rec.Code != 400 || body["Type"] != "ValidationException" {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tc.field+" is ") || strings.Contains(rec.Body.String(), "not valid JSON") {
				t.Fatalf("the answer does not name %s: %s", tc.field, rec.Body)
			}
		})
	}
}

func TestBrokenJSONIsStillAPlain400(t *testing.T) {
	h := newTestHandler(t)
	rec := do(h, http.MethodPut, "/api.xro/2.0/Contacts", `{"Contacts":[`, accounting(t, h, store.DefaultTenantID))
	if rec.Code != 400 || decode(t, rec)["Detail"] != "The request body is not valid JSON" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
