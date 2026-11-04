package api

import (
	"net/url"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func getManualJournals(t *testing.T, h *Handler, path string) (int, map[string]any) {
	t.Helper()
	rec := get(h, "/api.xro/2.0/ManualJournals"+path, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func createManualJournal(t *testing.T, h *Handler, body string) string {
	t.Helper()
	code, out := putManualJournals(t, h, "", body)
	if code != 200 {
		t.Fatalf("status %d: %v", code, out)
	}
	return out["ManualJournals"].([]any)[0].(map[string]any)["ManualJournalID"].(string)
}

func journals(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	list, ok := body["ManualJournals"].([]any)
	if !ok {
		t.Fatalf("no ManualJournals in %v", body)
	}
	out := make([]map[string]any, len(list))
	for i, m := range list {
		out[i] = m.(map[string]any)
	}
	return out
}

func TestManualJournalsListFiltersByWhereAndPages(t *testing.T) {
	h := newTestHandler(t)
	loanAccount(t, h)
	posted := createManualJournal(t, h, advanceJournal("POSTED", 5000, -5000, "2100"))
	createManualJournal(t, h, advanceJournal("DRAFT", 10, -10, "2100"))

	code, all := getManualJournals(t, h, "")
	if code != 200 || len(journals(t, all)) != 2 {
		t.Fatalf("want both journals, got %d %v", code, all)
	}
	if l := journals(t, all)[0]["JournalLines"].([]any); len(l) != 0 {
		t.Errorf("a list without page leaves the lines out: %v", l)
	}
	code, only := getManualJournals(t, h, "?where="+url.QueryEscape(`Status=="POSTED"`))
	if got := journals(t, only); code != 200 || len(got) != 1 || got[0]["ManualJournalID"] != posted {
		t.Errorf("where Status filters: %d %v", code, got)
	}
	code, paged := getManualJournals(t, h, "?page=1")
	pg, _ := paged["pagination"].(map[string]any)
	if code != 200 || pg["itemCount"] != 2.0 || len(journals(t, paged)[0]["JournalLines"].([]any)) != 2 {
		t.Errorf("a paged list carries pagination and the lines (page-004): %d %v", code, paged)
	}
	if code, _ := getManualJournals(t, h, "?where="+url.QueryEscape(`Status==`)); code != 400 {
		t.Errorf("a where that does not parse is a 400, got %d", code)
	}
}

func TestManualJournalGetReadsOneInFull(t *testing.T) {
	h := newTestHandler(t)
	loanAccount(t, h)
	id := createManualJournal(t, h, advanceJournal("POSTED", 5000, -5000, "2100"))
	code, body := getManualJournals(t, h, "/"+id)
	got := journals(t, body)
	if code != 200 || got[0]["ManualJournalID"] != id || len(got[0]["JournalLines"].([]any)) != 2 {
		t.Fatalf("the read returns the journal in full: %d %v", code, body)
	}
	if code, _ := getManualJournals(t, h, "/00000000-0000-4000-8000-000000000000"); code != 404 {
		t.Errorf("an unknown ManualJournalID is a 404, got %d", code)
	}
}
