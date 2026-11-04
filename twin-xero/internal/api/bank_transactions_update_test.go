package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func postBankTransaction(t *testing.T, h *Handler, id, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPost, "/api.xro/2.0/BankTransactions/"+id, body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func updated(t *testing.T, h *Handler, id, update string) map[string]any {
	t.Helper()
	code, body := postBankTransaction(t, h, id, update)
	if code != 200 {
		t.Fatalf("status %d: %v", code, body)
	}
	return body["BankTransactions"].([]any)[0].(map[string]any)
}

func TestUpdateBankTransactionChangesOnlyTheFieldsSent(t *testing.T) {
	h := newTestHandler(t)
	id := createBankTransaction(t, h, settlement(newContactID(t, h, "Lender Capital")))
	bt := updated(t, h, id, `{"BankTransactions":[{"Reference":"ADV-2002","Date":"2026-11-04"}]}`)
	if bt["Reference"] != "ADV-2002" || bt["Total"] != 5000.0 || bt["Type"] != "RECEIVE" || len(bt["LineItems"].([]any)) != 1 {
		t.Errorf("only Reference and Date change: %v", bt)
	}
	if got, _ := h.store.BankTransactions.For(store.DefaultTenantID).Get(id); got.Reference != "ADV-2002" || got.Date.Day() != 4 {
		t.Errorf("the update must reach the tenant's state: %+v", got)
	}
}

func TestUpdateBankTransactionMatchesLinesByID(t *testing.T) {
	h := newTestHandler(t)
	id := createBankTransaction(t, h, settlement(newContactID(t, h, "Lender Capital")))
	stored, _ := h.store.BankTransactions.For(store.DefaultTenantID).Get(id)
	lineID := stored.LineItems[0].LineItemID
	body := `{"LineItems":[{"LineItemID":"` + lineID + `","Description":"Advance","UnitAmount":4000,"AccountCode":"200"},` +
		`{"Description":"Fee rebate","UnitAmount":250,"AccountCode":"201"}]}`
	bt := updated(t, h, id, body)
	lines := bt["LineItems"].([]any)
	if len(lines) != 2 || bt["Total"] != 4250.0 {
		t.Fatalf("want two lines totalling 4250, got %v", bt)
	}
	if lines[0].(map[string]any)["LineItemID"] != lineID || lines[1].(map[string]any)["LineItemID"] == lineID {
		t.Errorf("a line with a known ID keeps it and a new line gets its own: %v", lines)
	}
	bt = updated(t, h, id, `{"LineItems":[{"LineItemID":"`+lineID+`","Description":"Advance","UnitAmount":4000,"AccountCode":"200"}]}`)
	if len(bt["LineItems"].([]any)) != 1 || bt["Total"] != 4000.0 {
		t.Errorf("a stored line left out is removed: %v", bt)
	}
}

func TestUpdateBankTransactionDeletesByStatus(t *testing.T) {
	h := newTestHandler(t)
	id := createBankTransaction(t, h, settlement(newContactID(t, h, "Lender Capital")))
	if bt := updated(t, h, id, `{"Status":"DELETED"}`); bt["Status"] != "DELETED" {
		t.Fatalf("Status DELETED deletes: %v", bt)
	}
	code, body := postBankTransaction(t, h, id, `{"Reference":"again"}`)
	if code != 400 || !strings.Contains(bankTransactionJSON(body), "status DELETED cannot be updated") {
		t.Errorf("a deleted bank transaction cannot change: %d %v", code, body)
	}
}

func TestUpdateBankTransactionRefusals(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{"void", `{"Status":"VOIDED"}`, "cannot change from AUTHORISED to VOIDED"},
		{"type", `{"Type":"SPEND"}`, "The Type of a bank transaction cannot be changed."},
		{"not a bank account", `{"BankAccount":{"Code":"200"}}`, "Only accounts of Type BANK will be accepted."},
		{"no lines", `{"LineItems":[]}`, "At least one line item is required."},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newTestHandler(t)
			id := createBankTransaction(t, h, settlement(newContactID(t, h, "Lender Capital")))
			code, body := postBankTransaction(t, h, id, c.body)
			if code != 400 || body["Type"] != "ValidationException" || !strings.Contains(bankTransactionJSON(body), c.want) {
				t.Fatalf("want a 400 with %q, got %d %v", c.want, code, body)
			}
			if got, _ := h.store.BankTransactions.For(store.DefaultTenantID).Get(id); got.Status != "AUTHORISED" || got.Total != 5000 {
				t.Errorf("a refused update leaves the bank transaction as it was: %+v", got)
			}
		})
	}
}

func TestUpdateBankTransactionRefusesReconciledAndPrepayments(t *testing.T) {
	h := newTestHandler(t)
	lender := newContactID(t, h, "Lender Capital")
	id := createBankTransaction(t, h, settlement(lender))
	updated(t, h, id, `{"IsReconciled":true}`)
	if code, body := postBankTransaction(t, h, id, `{"Reference":"late"}`); code != 400 || !strings.Contains(bankTransactionJSON(body), "reconciled") {
		t.Errorf("once reconciled the app emulator refuses changes (crud-019): %d %v", code, body)
	}

	pre := strings.Replace(settlement(lender), `"Type":"RECEIVE"`, `"Type":"RECEIVE-PREPAYMENT"`, 1)
	pid := createBankTransaction(t, h, pre)
	if code, body := postBankTransaction(t, h, pid, `{"Status":"DELETED"}`); code != 400 || !strings.Contains(bankTransactionJSON(body), "cannot be updated or deleted") {
		t.Errorf("Xero documents no updates or deletes on prepayments: %d %v", code, body)
	}
	if code, _ := postBankTransaction(t, h, "00000000-0000-4000-8000-000000000000", `{"Reference":"x"}`); code != 404 {
		t.Errorf("an unknown BankTransactionID is a 404, got %d", code)
	}
}

func TestUpdateBankTransactionTakesTheNewBankAccountsCurrency(t *testing.T) {
	h := newTestHandler(t)
	if code, body := putAccount(t, h, `{"Code":"091","Name":"Euro Account","Type":"BANK","CurrencyCode":"EUR","BankAccountNumber":"9876543210"}`); code != 200 {
		t.Fatalf("status %d: %v", code, body)
	}
	id := createBankTransaction(t, h, settlement(newContactID(t, h, "Lender Capital")))
	bt := updated(t, h, id, `{"BankAccount":{"Code":"091"}}`)
	if bt["CurrencyCode"] != "EUR" || bt["BankAccount"].(map[string]any)["Code"] != "091" {
		t.Errorf("moving to another bank account takes that account's currency: %v", bt)
	}
	if got, _ := h.store.BankTransactions.For(store.DefaultTenantID).Get(id); got.CurrencyCode != "EUR" {
		t.Errorf("the new currency must reach the tenant's state: %+v", got)
	}
}
