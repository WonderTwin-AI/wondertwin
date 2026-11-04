package api

import (
	"net/url"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func getBankTransactions(t *testing.T, h *Handler, path string) (int, map[string]any) {
	t.Helper()
	rec := get(h, "/api.xro/2.0/BankTransactions"+path, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func createBankTransaction(t *testing.T, h *Handler, body string) string {
	t.Helper()
	code, out := putBankTransactions(t, h, "", body)
	if code != 200 {
		t.Fatalf("status %d: %v", code, out)
	}
	return out["BankTransactions"].([]any)[0].(map[string]any)["BankTransactionID"].(string)
}

func spend(contactID, ref string) string {
	return `{"Type":"SPEND","Contact":{"ContactID":"` + contactID + `"},"BankAccount":{"Code":"090"},"Reference":"` + ref + `",` +
		`"LineItems":[{"Description":"Fee","UnitAmount":12.5,"AccountCode":"404"}]}`
}

func bankTransactionIDs(t *testing.T, body map[string]any) []string {
	t.Helper()
	var out []string
	list, ok := body["BankTransactions"].([]any)
	if !ok {
		t.Fatalf("no BankTransactions in %v", body)
	}
	for _, b := range list {
		out = append(out, b.(map[string]any)["BankTransactionID"].(string))
	}
	return out
}

func TestBankTransactionsListFiltersByWhereAndReferences(t *testing.T) {
	h := newTestHandler(t)
	lender := newContactID(t, h, "Lender Capital")
	advance := createBankTransaction(t, h, settlement(lender))
	fee := createBankTransaction(t, h, spend(lender, "FEE-1"))

	code, all := getBankTransactions(t, h, "")
	if code != 200 || len(bankTransactionIDs(t, all)) != 2 {
		t.Fatalf("want both bank transactions, got %d %v", code, all)
	}
	if lines, _ := all["BankTransactions"].([]any)[0].(map[string]any)["LineItems"].([]any); len(lines) != 0 {
		t.Errorf("a list without page leaves the line items out: %v", lines)
	}
	if _, ok := all["pagination"]; ok {
		t.Error("a list without page has no pagination object")
	}

	for _, c := range []struct {
		query string
		want  []string
	}{
		{"?where=" + url.QueryEscape(`Type=="RECEIVE"`), []string{advance}},
		{"?where=" + url.QueryEscape(`Contact.ContactID==Guid("`+lender+`")`), []string{advance, fee}},
		{"?where=" + url.QueryEscape(`BankAccount.Code=="090" AND Type=="SPEND"`), []string{fee}},
		{"?References=ADV-1001", []string{advance}},
		{"?References=FEE-1,ADV-1001", []string{advance, fee}},
		{"?References=NONE", nil},
	} {
		code, body := getBankTransactions(t, h, c.query)
		got := bankTransactionIDs(t, body)
		if code != 200 || len(got) != len(c.want) {
			t.Errorf("%s: got %d %v, want %v", c.query, code, got, c.want)
			continue
		}
		for _, w := range c.want {
			found := false
			for _, g := range got {
				found = found || g == w
			}
			if !found {
				t.Errorf("%s: %s missing from %v", c.query, w, got)
			}
		}
	}
	if code, _ := getBankTransactions(t, h, "?where="+url.QueryEscape(`Type==`)); code != 400 {
		t.Errorf("a where that does not parse is a 400, got %d", code)
	}
}

func TestBankTransactionsListPagesWithLines(t *testing.T) {
	h := newTestHandler(t)
	lender := newContactID(t, h, "Lender Capital")
	createBankTransaction(t, h, settlement(lender))
	code, body := getBankTransactions(t, h, "?page=1")
	if code != 200 {
		t.Fatalf("status %d: %v", code, body)
	}
	pg, _ := body["pagination"].(map[string]any)
	if pg["page"] != 1.0 || pg["pageSize"] != 100.0 || pg["itemCount"] != 1.0 {
		t.Errorf("pagination = %v, want page 1 of 100 with one item", pg)
	}
	if lines, _ := body["BankTransactions"].([]any)[0].(map[string]any)["LineItems"].([]any); len(lines) != 1 {
		t.Errorf("a paged list carries the line items: %v", lines)
	}
}

func TestBankTransactionGetReadsOneInFull(t *testing.T) {
	h := newTestHandler(t)
	id := createBankTransaction(t, h, settlement(newContactID(t, h, "Lender Capital")))
	code, body := getBankTransactions(t, h, "/"+id)
	if code != 200 {
		t.Fatalf("status %d: %v", code, body)
	}
	bt := body["BankTransactions"].([]any)[0].(map[string]any)
	if bt["BankTransactionID"] != id || bt["Total"] != 5000.0 || len(bt["LineItems"].([]any)) != 1 {
		t.Errorf("the read returns the bank transaction in full: %v", bt)
	}
	if code, _ := getBankTransactions(t, h, "/00000000-0000-4000-8000-000000000000"); code != 404 {
		t.Errorf("an unknown BankTransactionID is a 404, got %d", code)
	}
}
