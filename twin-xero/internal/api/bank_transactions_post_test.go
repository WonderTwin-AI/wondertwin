package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func postBankTransactions(t *testing.T, h *Handler, query, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPost, "/api.xro/2.0/BankTransactions"+query, body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func TestUpdateOrCreateBankTransactionsCreatesAndUpdatesInOneBatch(t *testing.T) {
	h := newTestHandler(t)
	lender := newContactID(t, h, "Lender Capital")
	id := createBankTransaction(t, h, settlement(lender))
	body := `{"BankTransactions":[{"BankTransactionID":"` + id + `","Reference":"ADV-UPDATED"},` + spend(lender, "FEE-2") + `]}`
	code, out := postBankTransactions(t, h, "", body)
	if code != 200 {
		t.Fatalf("status %d: %v", code, out)
	}
	list := out["BankTransactions"].([]any)
	first, second := list[0].(map[string]any), list[1].(map[string]any)
	if first["BankTransactionID"] != id || first["Reference"] != "ADV-UPDATED" || first["Total"] != 5000.0 {
		t.Errorf("an item with a stored BankTransactionID is a partial update: %v", first)
	}
	if second["BankTransactionID"] == id || second["Type"] != "SPEND" || second["Status"] != "AUTHORISED" {
		t.Errorf("an item with no BankTransactionID is created: %v", second)
	}
	if n := h.store.BankTransactions.For(store.DefaultTenantID).Count(); n != 2 {
		t.Errorf("want two stored bank transactions, got %d", n)
	}
}

func TestUpdateOrCreateBankTransactionsRefusesAnUnknownID(t *testing.T) {
	h := newTestHandler(t)
	lender := newContactID(t, h, "Lender Capital")
	body := `{"BankTransactions":[` + settlement(lender) + `,{"BankTransactionID":"00000000-0000-4000-8000-000000000000","Reference":"x"}]}`
	if code, _ := postBankTransactions(t, h, "", body); code != 404 {
		t.Errorf("an unknown BankTransactionID is a 404, got %d", code)
	}
	if n := h.store.BankTransactions.For(store.DefaultTenantID).Count(); n != 0 {
		t.Errorf("nothing is written when one ID is unknown, got %d", n)
	}
}

func TestUpdateOrCreateBankTransactionsAnswersAMixedBatchInPlace(t *testing.T) {
	h := newTestHandler(t)
	lender := newContactID(t, h, "Lender Capital")
	id := createBankTransaction(t, h, settlement(lender))
	body := `{"BankTransactions":[{"BankTransactionID":"` + id + `","Status":"VOIDED"},` + spend(lender, "FEE-3") + `]}`

	code, out := postBankTransactions(t, h, "", body)
	if code != 400 || !strings.Contains(bankTransactionJSON(out), "cannot change from AUTHORISED to VOIDED") {
		t.Fatalf("by default one invalid item fails the batch: %d %v", code, out)
	}
	if n := h.store.BankTransactions.For(store.DefaultTenantID).Count(); n != 1 {
		t.Fatalf("a failed batch writes nothing, got %d", n)
	}
	code, out = postBankTransactions(t, h, "?summarizeErrors=false", body)
	list := out["BankTransactions"].([]any)
	if code != 200 || list[0].(map[string]any)["StatusAttributeString"] != "ERROR" || list[1].(map[string]any)["StatusAttributeString"] != "OK" {
		t.Fatalf("with summarizeErrors=false each item is answered in place: %d %v", code, list)
	}
	if got, _ := h.store.BankTransactions.For(store.DefaultTenantID).Get(id); got.Status != "AUTHORISED" {
		t.Errorf("the refused update leaves the stored one as it was: %+v", got)
	}
}

func TestUpdateOrCreateBankTransactionsAppliesRepeatedIDsInOrder(t *testing.T) {
	h := newTestHandler(t)
	lender := newContactID(t, h, "Lender Capital")
	id := createBankTransaction(t, h, settlement(lender))
	body := `{"BankTransactions":[{"BankTransactionID":"` + id + `","Reference":"FIRST"},{"BankTransactionID":"` + id + `","Date":"2026-01-01"}]}`
	if code, out := postBankTransactions(t, h, "", body); code != 200 {
		t.Fatalf("status %d: %v", code, out)
	}
	got, _ := h.store.BankTransactions.For(store.DefaultTenantID).Get(id)
	if got.Reference != "FIRST" || got.Date == nil || got.Date.Format("2006-01-02") != "2026-01-01" {
		t.Errorf("a later item for the same ID builds on the earlier one: %+v", got)
	}
}
