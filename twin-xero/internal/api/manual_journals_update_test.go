package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func postManualJournal(t *testing.T, h *Handler, id, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPost, "/api.xro/2.0/ManualJournals/"+id, body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func updatedJournal(t *testing.T, h *Handler, id, body string) map[string]any {
	t.Helper()
	code, out := postManualJournal(t, h, id, body)
	if code != 200 {
		t.Fatalf("status %d: %v", code, out)
	}
	return out["ManualJournals"].([]any)[0].(map[string]any)
}

func TestUpdateManualJournalEditsADraftThenPostsIt(t *testing.T) {
	h := newTestHandler(t)
	loanAccount(t, h)
	id := createManualJournal(t, h, advanceJournal("DRAFT", 100, -10, "2100"))
	mj := updatedJournal(t, h, id, `{"ManualJournals":[{"Narration":"Advance, corrected","JournalLines":[`+
		`{"LineAmount":5000,"AccountCode":"090"},{"LineAmount":-5000,"AccountCode":"2100"}]}]}`)
	if mj["Narration"] != "Advance, corrected" || mj["Status"] != "DRAFT" || len(mj["JournalLines"].([]any)) != 2 {
		t.Fatalf("a draft's narration and lines change, lines replaced whole: %v", mj)
	}
	if mj = updatedJournal(t, h, id, `{"Status":"POSTED"}`); mj["Status"] != "POSTED" {
		t.Fatalf("a balanced draft can be posted: %v", mj)
	}
	if got, _ := h.store.ManualJournals.For(store.DefaultTenantID).Get(id); got.Status != "POSTED" || got.JournalLines[0].LineAmount != 5000 {
		t.Errorf("the update must reach the tenant's state: %+v", got)
	}
}

func TestUpdateManualJournalStatusMoves(t *testing.T) {
	h := newTestHandler(t)
	loanAccount(t, h)
	posted := createManualJournal(t, h, advanceJournal("POSTED", 5000, -5000, "2100"))
	if code, body := postManualJournal(t, h, posted, `{"Narration":"late edit"}`); code != 400 ||
		!strings.Contains(bankTransactionJSON(body), "Only a DRAFT manual journal can have its details changed.") {
		t.Errorf("a POSTED journal's details are fixed: %d %v", code, body)
	}
	if code, body := postManualJournal(t, h, posted, `{"Status":"DELETED"}`); code != 400 ||
		!strings.Contains(bankTransactionJSON(body), "cannot change from POSTED to DELETED") {
		t.Errorf("a POSTED journal is voided, not deleted: %d %v", code, body)
	}
	if mj := updatedJournal(t, h, posted, `{"Status":"VOIDED"}`); mj["Status"] != "VOIDED" {
		t.Errorf("a POSTED journal can be voided: %v", mj)
	}
	if code, body := postManualJournal(t, h, posted, `{"Status":"POSTED"}`); code != 400 ||
		!strings.Contains(bankTransactionJSON(body), "status VOIDED cannot be updated") {
		t.Errorf("a VOIDED journal has no moves: %d %v", code, body)
	}

	draft := createManualJournal(t, h, advanceJournal("DRAFT", 1, -1, "2100"))
	if mj := updatedJournal(t, h, draft, `{"Status":"DELETED"}`); mj["Status"] != "DELETED" {
		t.Errorf("a DRAFT can be deleted: %v", mj)
	}
}

func TestUpdateManualJournalChecksBalanceAndAccounts(t *testing.T) {
	h := newTestHandler(t)
	loanAccount(t, h)
	draft := createManualJournal(t, h, advanceJournal("DRAFT", 100, -10, "2100"))
	if code, body := postManualJournal(t, h, draft, `{"Status":"POSTED"}`); code != 400 ||
		!strings.Contains(bankTransactionJSON(body), "The total debits (100.00) must equal total credits (-10.00)") {
		t.Errorf("an unbalanced draft cannot be posted: %d %v", code, body)
	}
	if code, body := postManualJournal(t, h, draft, `{"JournalLines":[{"LineAmount":1,"AccountCode":"090"},{"LineAmount":-1,"AccountCode":"476"}]}`); code != 400 ||
		!strings.Contains(bankTransactionJSON(body), "Account code '476' does not match a recognised account.") {
		t.Errorf("an unknown account code is refused on update too: %d %v", code, body)
	}
	if got, _ := h.store.ManualJournals.For(store.DefaultTenantID).Get(draft); got.Status != "DRAFT" || got.JournalLines[0].LineAmount != 100 {
		t.Errorf("a refused update leaves the journal as it was: %+v", got)
	}
	if code, _ := postManualJournal(t, h, "00000000-0000-4000-8000-000000000000", `{"Narration":"x"}`); code != 404 {
		t.Errorf("an unknown ManualJournalID is a 404, got %d", code)
	}
}
