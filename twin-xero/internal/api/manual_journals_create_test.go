package api

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func putManualJournals(t *testing.T, h *Handler, query, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPut, "/api.xro/2.0/ManualJournals"+query, body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

// loanAccount adds the demo's liability account through PUT /Accounts, as the
// demo's reset does, since the default chart has none.
func loanAccount(t *testing.T, h *Handler) {
	t.Helper()
	if code, body := putAccount(t, h, `{"Code":"2100","Name":"Merchant Advance Loan","Type":"TERMLIAB"}`); code != 200 {
		t.Fatalf("status %d: %v", code, body)
	}
}

func advanceJournal(status string, debit, credit float64, creditCode string) string {
	return `{"Narration":"Lender advance booked as a loan","Status":"` + status + `","Date":"2026-11-03","JournalLines":[` +
		`{"LineAmount":` + ftoa(debit) + `,"AccountCode":"090","Description":"Advance received"},` +
		`{"LineAmount":` + ftoa(credit) + `,"AccountCode":"` + creditCode + `","Description":"Loan from lender"}]}`
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func TestCreateManualJournalsBooksTheAdvanceAsALoan(t *testing.T) {
	h := newTestHandler(t)
	loanAccount(t, h)
	code, body := putManualJournals(t, h, "", advanceJournal("POSTED", 5000, -5000, "2100"))
	if code != 200 || body["Status"] != "OK" {
		t.Fatalf("status %d: %v", code, body)
	}
	mj := body["ManualJournals"].([]any)[0].(map[string]any)
	id, _ := mj["ManualJournalID"].(string)
	if !guidPattern.MatchString(id) || mj["Status"] != "POSTED" || mj["LineAmountTypes"] != "NoTax" || mj["ShowOnCashBasisReports"] != true {
		t.Errorf("the emulator assigns the ID and the defaults: %v", mj)
	}
	lines := mj["JournalLines"].([]any)
	if l := lines[1].(map[string]any); l["LineAmount"] != -5000.0 || l["AccountCode"] != "2100" || l["AccountID"] == "" {
		t.Errorf("the credit line keeps its sign and gets the account's ID: %v", l)
	}
	if got, ok := h.store.ManualJournals.For(store.DefaultTenantID).Get(id); !ok || got.Narration == "" {
		t.Errorf("the journal must reach the tenant's state: %+v %v", got, ok)
	}

	code, body = putManualJournals(t, h, "", `{"Narration":"Draft","JournalLines":[{"LineAmount":1,"AccountCode":"090"},{"LineAmount":-1,"AccountCode":"2100"}]}`)
	if code != 200 || body["ManualJournals"].([]any)[0].(map[string]any)["Status"] != "DRAFT" {
		t.Errorf("a journal sent with no status is a DRAFT: %d %v", code, body)
	}
}

func TestManualJournalBalanceIsRequiredWhenPosted(t *testing.T) {
	h := newTestHandler(t)
	loanAccount(t, h)
	code, body := putManualJournals(t, h, "", advanceJournal("POSTED", 100, -10, "2100"))
	if code != 400 || !strings.Contains(bankTransactionJSON(body), "The total debits (100.00) must equal total credits (-10.00)") {
		t.Fatalf("an unbalanced POSTED journal is refused with the spec's message: %d %v", code, body)
	}
	if code, body := putManualJournals(t, h, "", advanceJournal("DRAFT", 100, -10, "2100")); code != 200 {
		t.Errorf("whether a DRAFT must balance is not settled (crud-020); it is accepted: %d %v", code, body)
	}
}

func TestManualJournalUnknownAccountIsRefused(t *testing.T) {
	h := newTestHandler(t)
	code, body := putManualJournals(t, h, "", advanceJournal("POSTED", 5000, -5000, "476"))
	if code != 400 || !strings.Contains(bankTransactionJSON(body), "Account code '476' does not match a recognised account.") {
		t.Fatalf("an unknown account code is refused, naming the code (operator decision 2026-11-03): %d %v", code, body)
	}
	if n := h.store.ManualJournals.For(store.DefaultTenantID).Count(); n != 0 {
		t.Errorf("a refused journal must not be stored, got %d", n)
	}
}

func TestCreateManualJournalsRefusals(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{"no narration", `{"JournalLines":[{"LineAmount":1,"AccountCode":"090"},{"LineAmount":-1,"AccountCode":"404"}]}`, "Narration must be specified."},
		{"one line", `{"Narration":"x","JournalLines":[{"LineAmount":0,"AccountCode":"090"}]}`, "at least two journal lines"},
		{"line without account", `{"Narration":"x","JournalLines":[{"LineAmount":1},{"LineAmount":-1,"AccountCode":"404"}]}`, "Journal line 1: an account code must be specified."},
		{"status", `{"Narration":"x","Status":"VOIDED","JournalLines":[{"LineAmount":1,"AccountCode":"090"},{"LineAmount":-1,"AccountCode":"404"}]}`, "cannot be created with the status VOIDED"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newTestHandler(t)
			code, body := putManualJournals(t, h, "", c.body)
			if code != 400 || body["Type"] != "ValidationException" || !strings.Contains(bankTransactionJSON(body), c.want) {
				t.Fatalf("want a 400 with %q, got %d %v", c.want, code, body)
			}
		})
	}
}

func TestCreateManualJournalsWithSummarizeErrorsFalse(t *testing.T) {
	h := newTestHandler(t)
	loanAccount(t, h)
	all := `{"ManualJournals":[` + advanceJournal("POSTED", 5000, -5000, "2100") + `,` + advanceJournal("POSTED", 1, -2, "2100") + `]}`
	code, body := putManualJournals(t, h, "?summarizeErrors=false", all)
	list := body["ManualJournals"].([]any)
	if code != 200 || list[0].(map[string]any)["StatusAttributeString"] != "OK" || list[1].(map[string]any)["StatusAttributeString"] != "ERROR" {
		t.Fatalf("each journal is answered in place: %d %v", code, list)
	}
	if n := h.store.ManualJournals.For(store.DefaultTenantID).Count(); n != 1 {
		t.Errorf("only the balanced journal is created, got %d", n)
	}
}
