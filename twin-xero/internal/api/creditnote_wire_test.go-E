package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func TestCreditNoteWireReadsDatesInEveryFormAndWritesMsdates(t *testing.T) {
	var w creditNote
	in := `{"Type":"ACCRECCREDIT","Date":"2026-09-10","DueDate":"/Date(1790726400000+0000)/","Contact":{"ContactID":"c1","Name":"ignored"},` +
		`"Allocations":[{"Invoice":{"InvoiceID":"i1","InvoiceNumber":"ignored on the way in"},"Amount":20,"Date":"2026-09-16"}]}`
	if err := json.Unmarshal([]byte(in), &w); err != nil {
		t.Fatal(err)
	}
	cn := inCreditNote(w)
	if cn.Date == nil || !cn.Date.Equal(time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)) || cn.DueDate == nil {
		t.Fatalf("dates = %v %v", cn.Date, cn.DueDate)
	}
	if cn.Contact == nil || cn.Contact.ContactID != "c1" {
		t.Fatalf("the contact must be kept as a reference, got %+v", cn.Contact)
	}
	if len(cn.Allocations) != 1 || cn.Allocations[0].Amount != 20 || cn.Allocations[0].Invoice.InvoiceID != "i1" ||
		cn.Allocations[0].Date == nil || !cn.Allocations[0].Date.Equal(time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("the allocation must survive the round trip: %+v", cn.Allocations)
	}

	cn.UpdatedDateUTC = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out, err := json.Marshal(newTestHandler(t).wireCreditNote(store.DefaultTenantID, cn))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Date, DueDate, UpdatedDateUTC string
		Allocations                   []map[string]any
	}
	_ = json.Unmarshal(out, &m)
	for _, s := range []string{m.Date, m.DueDate, m.UpdatedDateUTC, m.Allocations[0]["Date"].(string)} {
		if !msdateValue.MatchString(s) {
			t.Errorf("%q is not /Date(ms+0000)/ (time-001)", s)
		}
	}
	var loose map[string]any
	_ = json.Unmarshal(out, &loose)
	if _, ok := loose["FullyPaidOnDate"]; ok {
		t.Error("FullyPaidOnDate is unset and must be omitted, not null")
	}
	for _, k := range []string{"LineItems", "Payments", "Allocations"} {
		if l, ok := loose[k].([]any); !ok || l == nil {
			t.Errorf("%s must be [], got %v", k, loose[k])
		}
	}
	empty, _ := json.Marshal(newTestHandler(t).wireCreditNote(store.DefaultTenantID, store.CreditNote{}))
	if !strings.Contains(string(empty), `"Allocations":[]`) {
		t.Errorf("a credit note with no allocations must write [], got %s", empty)
	}
}

func TestCreditNoteWireResolvesTheFullContact(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	out, _ := json.Marshal(h.wireCreditNote(store.DefaultTenantID, store.CreditNote{Contact: &store.ContactRef{ContactID: cid}}))
	if !strings.Contains(string(out), `"Name":"Acme"`) || !strings.Contains(string(out), cid) {
		t.Fatalf("the contact must be the full contact: %s", out)
	}
	gone, _ := json.Marshal(h.wireCreditNote(store.DefaultTenantID, store.CreditNote{Contact: &store.ContactRef{ContactID: "missing"}}))
	if strings.Contains(string(gone), `"Contact"`) {
		t.Fatalf("an unknown contact must not be invented: %s", gone)
	}
}
