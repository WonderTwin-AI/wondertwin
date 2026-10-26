package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// Audit finding F1: the lists an invoice and a credit note carry of the records
// that touch them are typed, and what they write is what the map values wrote.
func TestTheSummariesAnInvoiceAndACreditNoteListAreTypedAndKeepTheirKeys(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	id := refundableInvoice(t, h, cid)
	if code, body := postPayment(t, h, pay(id, 40)); code != 200 {
		t.Fatalf("payment: %d %v", code, body)
	}
	if code, body := putCreditNotes(t, h, "", creditNoteWithAllocations(cid, 20, allocJSON(id, 20))); code != 200 {
		t.Fatalf("credit note: %d %v", code, body)
	}

	inv := storedInvoice(h, id)
	if len(inv.Payments) != 1 || inv.Payments[0].Amount != 40 || !inv.Payments[0].HasAccount {
		t.Fatalf("payments = %+v", inv.Payments)
	}
	if len(inv.CreditNotes) != 1 || inv.CreditNotes[0].AppliedAmount != 20 || inv.CreditNotes[0].ID != inv.CreditNotes[0].CreditNoteID {
		t.Fatalf("credit notes = %+v", inv.CreditNotes)
	}

	rec := get(h, "/api.xro/2.0/Invoices/"+id, accounting(t, h, store.DefaultTenantID))
	var wire struct{ Invoices []map[string]json.RawMessage }
	if err := json.Unmarshal(rec.Body.Bytes(), &wire); err != nil || len(wire.Invoices) != 1 {
		t.Fatalf("%v %s", err, rec.Body)
	}
	keysOf := func(raw json.RawMessage) []string {
		var list []map[string]any
		if err := json.Unmarshal(raw, &list); err != nil || len(list) != 1 {
			t.Fatalf("list = %s (%v)", raw, err)
		}
		var keys []string
		for k := range list[0] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}
	want := map[string][]string{
		"Payments":    {"Amount", "Date", "HasAccount", "HasValidationErrors", "PaymentID", "Reference"},
		"CreditNotes": {"AppliedAmount", "CreditNoteID", "CreditNoteNumber", "Date", "HasErrors", "ID"},
	}
	for list, keys := range want {
		if got := keysOf(wire.Invoices[0][list]); !reflect.DeepEqual(got, keys) {
			t.Errorf("%s keys = %v, want %v", list, got, keys)
		}
	}
	for _, list := range []string{"Prepayments", "Overpayments", "Attachments"} {
		if string(wire.Invoices[0][list]) != "[]" {
			t.Errorf("%s = %s, want [] and not null", list, wire.Invoices[0][list])
		}
	}
}

func TestTheTypedListsSurviveAnExportAndAnImport(t *testing.T) {
	h := newTestHandler(t)
	id := creditNoteFor(t, h, false)
	if code, body := postPayment(t, h, payCreditNote(id, 7)); code != 200 {
		t.Fatalf("payment: %d %v", code, body)
	}
	before := storedCreditNote(h, id)
	if len(before.Payments) != 1 {
		t.Fatalf("setup: %+v", before.Payments)
	}

	state, err := json.Marshal(h.store.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	other := newTestHandler(t)
	if err := other.store.LoadState(state); err != nil {
		t.Fatal(err)
	}
	if after := storedCreditNote(other, id); !reflect.DeepEqual(after.Payments, before.Payments) {
		t.Fatalf("payments after import = %+v, want %+v", after.Payments, before.Payments)
	}
	if rec := get(other, "/api.xro/2.0/CreditNotes", accounting(t, other, store.DefaultTenantID)); rec.Code != http.StatusOK {
		t.Fatalf("list after import: %d", rec.Code)
	}
}
