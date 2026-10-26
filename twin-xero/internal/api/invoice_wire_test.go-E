package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func TestInvoiceWireReadsDatesInEveryFormAndWritesMsdates(t *testing.T) {
	var w invoice
	in := `{"Type":"ACCREC","Date":"2026-09-01","DueDate":"/Date(1790726400000+0000)/","ExpectedPaymentDate":"2026-10-05T00:00:00","Contact":{"ContactID":"c1","Name":"ignored"}}`
	if err := json.Unmarshal([]byte(in), &w); err != nil {
		t.Fatal(err)
	}
	inv := inInvoice(w)
	if inv.Date == nil || !inv.Date.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) || inv.DueDate == nil || inv.ExpectedPaymentDate == nil {
		t.Fatalf("dates = %v %v %v", inv.Date, inv.DueDate, inv.ExpectedPaymentDate)
	}
	if inv.Contact == nil || inv.Contact.ContactID != "c1" {
		t.Fatalf("the contact must be kept as a reference, got %+v", inv.Contact)
	}

	inv.UpdatedDateUTC = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out, err := json.Marshal(newTestHandler(t).wireInvoice(store.DefaultTenantID, inv))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	for _, k := range []string{"Date", "DueDate", "ExpectedPaymentDate", "UpdatedDateUTC"} {
		if s, _ := m[k].(string); !msdateValue.MatchString(s) {
			t.Errorf("%s = %v, want /Date(ms+0000)/ (time-001)", k, m[k])
		}
	}
	for _, k := range []string{"PlannedPaymentDate", "FullyPaidOnDate"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s is unset and must be omitted, not null", k)
		}
	}
	for _, k := range []string{"LineItems", "Payments", "Prepayments", "Overpayments", "CreditNotes", "Attachments"} {
		if l, ok := m[k].([]any); !ok || l == nil {
			t.Errorf("%s must be [], got %v", k, m[k])
		}
	}
}

func TestInvoiceWireResolvesTheFullContact(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	out, _ := json.Marshal(h.wireInvoice(store.DefaultTenantID, store.Invoice{Contact: &store.ContactRef{ContactID: cid}}))
	if !strings.Contains(string(out), `"Name":"Acme"`) || !strings.Contains(string(out), cid) {
		t.Fatalf("the contact must be the full contact: %s", out)
	}
	gone, _ := json.Marshal(h.wireInvoice(store.DefaultTenantID, store.Invoice{Contact: &store.ContactRef{ContactID: "missing"}}))
	if strings.Contains(string(gone), `"Contact"`) {
		t.Fatalf("an unknown contact must not be invented: %s", gone)
	}
}

func newContactID(t *testing.T, h *Handler, name string) string {
	t.Helper()
	c := putContacts(t, h, fmt.Sprintf(`{"Contacts":[{"Name":%q}]}`, name))["Contacts"].([]any)[0].(map[string]any)
	return c["ContactID"].(string)
}
