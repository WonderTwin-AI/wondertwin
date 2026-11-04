package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func postManualJournals(t *testing.T, h *Handler, query, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPost, "/api.xro/2.0/ManualJournals"+query, body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func TestUpdateOrCreateManualJournalsCreatesAndUpdatesInOneBatch(t *testing.T) {
	h := newTestHandler(t)
	loanAccount(t, h)
	id := createManualJournal(t, h, advanceJournal("DRAFT", 5000, -5000, "2100"))
	body := `{"ManualJournals":[{"ManualJournalID":"` + id + `","Narration":"renamed"},{"ManualJournalID":"` + id + `","Status":"POSTED"},` +
		advanceJournal("POSTED", 750, -750, "2100") + `]}`
	code, out := postManualJournals(t, h, "", body)
	if code != 200 {
		t.Fatalf("status %d: %v", code, out)
	}
	if got, _ := h.store.ManualJournals.For(store.DefaultTenantID).Get(id); got.Narration != "renamed" || got.Status != "POSTED" {
		t.Errorf("a repeated ID applies its items in order, keeping both changes: %+v", got)
	}
	list := out["ManualJournals"].([]any)
	if fresh := list[2].(map[string]any); fresh["ManualJournalID"] == id || fresh["Status"] != "POSTED" {
		t.Errorf("an item with no ManualJournalID is created: %v", fresh)
	}
	if n := h.store.ManualJournals.For(store.DefaultTenantID).Count(); n != 2 {
		t.Errorf("want two stored journals, got %d", n)
	}
}

func TestUpdateOrCreateManualJournalsRefusals(t *testing.T) {
	h := newTestHandler(t)
	loanAccount(t, h)
	body := `{"ManualJournals":[` + advanceJournal("POSTED", 1, -1, "2100") + `,{"ManualJournalID":"00000000-0000-4000-8000-000000000000","Narration":"x"}]}`
	if code, _ := postManualJournals(t, h, "", body); code != 404 || h.store.ManualJournals.For(store.DefaultTenantID).Count() != 0 {
		t.Errorf("an unknown ManualJournalID is a 404 and nothing is written, got %d", code)
	}

	mixed := `{"ManualJournals":[` + advanceJournal("POSTED", 100, -10, "2100") + `,` + advanceJournal("POSTED", 5, -5, "2100") + `]}`
	if code, out := postManualJournals(t, h, "", mixed); code != 400 || !strings.Contains(bankTransactionJSON(out), "must equal total credits") {
		t.Fatalf("by default one invalid journal fails the batch: %d %v", code, out)
	}
	code, out := postManualJournals(t, h, "?summarizeErrors=false", mixed)
	list := out["ManualJournals"].([]any)
	if code != 200 || list[0].(map[string]any)["StatusAttributeString"] != "ERROR" || list[1].(map[string]any)["StatusAttributeString"] != "OK" {
		t.Fatalf("with summarizeErrors=false each journal is answered in place: %d %v", code, list)
	}
	if n := h.store.ManualJournals.For(store.DefaultTenantID).Count(); n != 1 {
		t.Errorf("only the balanced journal is written, got %d", n)
	}
}
