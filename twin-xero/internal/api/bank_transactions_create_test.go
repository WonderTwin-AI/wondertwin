package api

import (
	"net/http"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func putBankTransactions(t *testing.T, h *Handler, query, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPut, "/api.xro/2.0/BankTransactions"+query, body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func settlement(contactID string) string {
	return `{"Type":"RECEIVE","Contact":{"ContactID":"` + contactID + `"},"BankAccount":{"Code":"090"},"LineAmountTypes":"NoTax",` +
		`"Reference":"ADV-1001","LineItems":[{"Description":"Advance from lender","Quantity":1,"UnitAmount":5000,"AccountCode":"200"}]}`
}

func TestCreateBankTransactionsRecordsASettlementIntoTheBankAccount(t *testing.T) {
	h := newTestHandler(t)
	lender := newContactID(t, h, "Lender Capital")
	code, body := putBankTransactions(t, h, "", settlement(lender))
	if code != 200 || body["Status"] != "OK" {
		t.Fatalf("status %d: %v", code, body)
	}
	list, _ := body["BankTransactions"].([]any)
	if len(list) != 1 {
		t.Fatalf("want one bank transaction in the plural envelope, got %v", body["BankTransactions"])
	}
	bt := list[0].(map[string]any)
	id, _ := bt["BankTransactionID"].(string)
	if !guidPattern.MatchString(id) || bt["Status"] != "AUTHORISED" || bt["Type"] != "RECEIVE" || bt["Total"] != 5000.0 {
		t.Errorf("the emulator assigns the ID and AUTHORISED and computes the total: %v", bt)
	}
	if bank, _ := bt["BankAccount"].(map[string]any); bank["Code"] != "090" || bank["Name"] != "Business Bank Account" {
		t.Errorf("BankAccount = %v, want account 090 with its name", bt["BankAccount"])
	}
	if c, _ := bt["Contact"].(map[string]any); c["Name"] != "Lender Capital" {
		t.Errorf("Contact = %v, want the full contact", bt["Contact"])
	}
	if bt["CurrencyCode"] != "USD" || bt["IsReconciled"] != false || !msdateValue.MatchString(bt["Date"].(string)) {
		t.Errorf("currency, reconciled flag or date defaults are wrong: %v", bt)
	}
	lines, _ := bt["LineItems"].([]any)
	if l := lines[0].(map[string]any); !guidPattern.MatchString(l["LineItemID"].(string)) || l["AccountID"] == "" {
		t.Errorf("each line gets an ID and its account's ID: %v", l)
	}
	if got, ok := h.store.BankTransactions.For(store.DefaultTenantID).Get(id); !ok || got.Total != 5000 {
		t.Errorf("the bank transaction must reach the tenant's state: %+v %v", got, ok)
	}
}

func TestCreateBankTransactionsRefusesTransfersAndUnknownTypes(t *testing.T) {
	for _, typ := range []string{"RECEIVE-TRANSFER", "SPEND-TRANSFER", "DEPOSIT", ""} {
		h := newTestHandler(t)
		body := `{"Type":"` + typ + `","BankAccount":{"Code":"090"},"LineItems":[{"Description":"x","UnitAmount":1,"AccountCode":"200"}]}`
		code, out := putBankTransactions(t, h, "", body)
		if code != 400 || out["Type"] != "ValidationException" {
			t.Errorf("type %q: want a 400 ValidationException, got %d %v", typ, code, out)
		}
		if n := h.store.BankTransactions.For(store.DefaultTenantID).Count(); n != 0 {
			t.Errorf("type %q: nothing may be created, got %d", typ, n)
		}
	}
}

func TestCreateBankTransactionsInBulkWithSummarizeErrorsFalse(t *testing.T) {
	h := newTestHandler(t)
	lender := newContactID(t, h, "Lender Capital")
	bad := `{"Type":"RECEIVE-TRANSFER","BankAccount":{"Code":"090"},"LineItems":[{"Description":"x","UnitAmount":1,"AccountCode":"200"}]}`
	all := `{"BankTransactions":[` + settlement(lender) + `,` + bad + `]}`

	if code, _ := putBankTransactions(t, h, "", all); code != 400 || h.store.BankTransactions.For(store.DefaultTenantID).Count() != 0 {
		t.Fatalf("by default one invalid item fails the whole request: %d", code)
	}
	code, body := putBankTransactions(t, h, "?summarizeErrors=false", all)
	if code != 200 {
		t.Fatalf("status %d: %v", code, body)
	}
	list, _ := body["BankTransactions"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["StatusAttributeString"] != "OK" || list[1].(map[string]any)["StatusAttributeString"] != "ERROR" {
		t.Fatalf("each item is answered in place: %v", list)
	}
	if n := h.store.BankTransactions.For(store.DefaultTenantID).Count(); n != 1 {
		t.Errorf("only the valid item is created, got %d", n)
	}
}
