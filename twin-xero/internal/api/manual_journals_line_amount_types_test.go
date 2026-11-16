package api

import (
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// A LineAmountTypes outside the spec enum makes xero-python
// get_manual_journals raise ValueError, so every journal write refuses it with
// the message bank transactions give the same value.

func withLineAmountTypes(journal, lat string) string {
	return strings.Replace(journal, `{"Narration"`, `{"LineAmountTypes":"`+lat+`","Narration"`, 1)
}

func refusedForLineAmountTypes(t *testing.T, what string, code int, body map[string]any) {
	t.Helper()
	if code != 400 || body["Type"] != "ValidationException" || !strings.Contains(rawString(body), lineAmountTypesMessage) {
		t.Fatalf("%s: want a 400 ValidationException naming LineAmountTypes, got %d %v", what, code, body)
	}
}

func TestManualJournalLineAmountTypesMatchesBankTransactions(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Lender Capital")
	code, body := putBankTransactions(t, h, "", strings.Replace(settlement(cid), `"NoTax"`, `"Bogus"`, 1))
	refusedForLineAmountTypes(t, "PUT /BankTransactions", code, body)
}

func TestCreateManualJournalRefusesOffSpecLineAmountTypes(t *testing.T) {
	h := newTestHandler(t)
	loanAccount(t, h)
	for _, lat := range []string{"Bogus", "exclusive"} {
		code, body := putManualJournals(t, h, "", withLineAmountTypes(advanceJournal("DRAFT", 5000, -5000, "2100"), lat))
		refusedForLineAmountTypes(t, "PUT /ManualJournals "+lat, code, body)
		code, body = postManualJournals(t, h, "", withLineAmountTypes(advanceJournal("DRAFT", 5000, -5000, "2100"), lat))
		refusedForLineAmountTypes(t, "POST /ManualJournals "+lat, code, body)
	}
	if n := len(h.store.ManualJournals.For(store.DefaultTenantID).List()); n != 0 {
		t.Fatalf("a refused journal must not be stored: %d stored", n)
	}
	for _, lat := range []string{"Exclusive", "Inclusive", "NoTax"} {
		if code, body := putManualJournals(t, h, "", withLineAmountTypes(advanceJournal("DRAFT", 5000, -5000, "2100"), lat)); code != 200 {
			t.Errorf("%s is in the spec enum: %d %v", lat, code, body)
		}
	}
}

func TestUpdateManualJournalRefusesOffSpecLineAmountTypes(t *testing.T) {
	h := newTestHandler(t)
	loanAccount(t, h)
	id := createManualJournal(t, h, advanceJournal("DRAFT", 5000, -5000, "2100"))
	code, body := postManualJournal(t, h, id, `{"LineAmountTypes":"Bogus"}`)
	refusedForLineAmountTypes(t, "POST /ManualJournals/{id}", code, body)
	code, body = postManualJournal(t, h, id, `{"Status":"DELETED","LineAmountTypes":"Bogus"}`)
	refusedForLineAmountTypes(t, "POST /ManualJournals/{id} deleting", code, body)
	code, body = postManualJournals(t, h, "", `{"ManualJournalID":"`+id+`","LineAmountTypes":"Bogus"}`)
	refusedForLineAmountTypes(t, "POST /ManualJournals by ID", code, body)
	got, _ := h.store.ManualJournals.For(store.DefaultTenantID).Get(id)
	if got.LineAmountTypes != "NoTax" || got.Status != "DRAFT" {
		t.Errorf("a refused update must leave the journal alone: %+v", got)
	}
}
