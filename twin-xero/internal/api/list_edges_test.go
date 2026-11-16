package api

import (
	"net/http"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// The bank transaction and manual journal lists answer an empty tenant and a
// page past the last with an empty array, never null. PUT or POST with an empty
// array writes nothing and answers an empty array; whether Xero refuses an empty
// batch instead is not settled by the references, so this pins what the
// emulator does.
func TestBankTransactionAndJournalListsAtTheirEdges(t *testing.T) {
	routes := []struct {
		plural, id string
		create     func(t *testing.T, h *Handler)
	}{
		{"BankTransactions", "BankTransactionID", func(t *testing.T, h *Handler) {
			createBankTransaction(t, h, spend(newContactID(t, h, "Bank"), "FEE-1"))
		}},
		{"ManualJournals", "ManualJournalID", func(t *testing.T, h *Handler) {
			loanAccount(t, h)
			createManualJournal(t, h, advanceJournal("POSTED", 10, -10, "2100"))
		}},
	}
	for _, rt := range routes {
		t.Run(rt.plural, func(t *testing.T) {
			h := newTestHandler(t)
			list := func(query string) (int, map[string]any, []any) {
				t.Helper()
				rec := get(h, "/api.xro/2.0/"+rt.plural+query, accounting(t, h, store.DefaultTenantID))
				body := decode(t, rec)
				items, ok := body[rt.plural].([]any)
				if !ok {
					t.Fatalf("%s%s: %s is not an array: %s", rt.plural, query, rt.plural, rec.Body)
				}
				return rec.Code, body, items
			}

			if code, _, items := list(""); code != 200 || len(items) != 0 {
				t.Errorf("empty store: %d %v", code, items)
			}
			code, body, items := list("?page=1")
			if pg, _ := body["pagination"].(map[string]any); code != 200 || len(items) != 0 || pg["page"] != 1.0 || pg["itemCount"] != 0.0 {
				t.Errorf("empty store, page 1: %d %v", code, body)
			}

			for _, method := range []string{http.MethodPut, http.MethodPost} {
				rec := do(h, method, "/api.xro/2.0/"+rt.plural, `{"`+rt.plural+`":[]}`, accounting(t, h, store.DefaultTenantID))
				out, _ := decode(t, rec)[rt.plural].([]any)
				if rec.Code != 200 || out == nil || len(out) != 0 {
					t.Errorf("%s with an empty array: %d %s", method, rec.Code, rec.Body)
				}
			}
			if _, _, items := list(""); len(items) != 0 {
				t.Fatalf("an empty array wrote %v", items)
			}

			rt.create(t, h)
			code, body, items = list("?page=2")
			if pg, _ := body["pagination"].(map[string]any); code != 200 || len(items) != 0 || pg["page"] != 2.0 || pg["itemCount"] != 1.0 || pg["pageCount"] != 1.0 {
				t.Errorf("a page past the last: %d %v", code, body)
			}
			code, _, items = list("?page=2&pageSize=1")
			if code != 200 || len(items) != 0 {
				t.Errorf("a page past the last with pageSize 1: %d %v", code, items)
			}
			if code, _, items = list("?page=1&pageSize=1"); code != 200 || len(items) != 1 {
				t.Errorf("the last page itself: %d %v", code, items)
			}
		})
	}
}
