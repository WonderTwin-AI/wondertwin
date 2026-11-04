package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func putAccount(t *testing.T, h *Handler, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPut, "/api.xro/2.0/Accounts", body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func chartHas(t *testing.T, h *Handler, code string) bool {
	t.Helper()
	for _, a := range listAccounts(t, h, store.DefaultTenantID).Accounts {
		if a["Code"] == code {
			return true
		}
	}
	return false
}

func TestCreateAccountAddsALiabilityToTheChart(t *testing.T) {
	h := newTestHandler(t)
	code, body := putAccount(t, h, `{"Code":"2100","Name":"Merchant Advance Loan","Type":"TERMLIAB","Class":"ASSET"}`)
	if code != 200 || body["Status"] != "OK" {
		t.Fatalf("status %d: %v", code, body)
	}
	list, _ := body["Accounts"].([]any)
	if len(list) != 1 {
		t.Fatalf("want one account in the plural envelope, got %v", body["Accounts"])
	}
	a := list[0].(map[string]any)
	if !guidPattern.MatchString(a["AccountID"].(string)) || a["Class"] != "LIABILITY" || a["Status"] != "ACTIVE" || a["TaxType"] != "NONE" {
		t.Errorf("the emulator assigns the ID and Status and derives Class from Type, ignoring the Class sent: %v", a)
	}
	if !msdateValue.MatchString(a["UpdatedDateUTC"].(string)) {
		t.Errorf("UpdatedDateUTC = %v, want /Date(ms+0000)/", a["UpdatedDateUTC"])
	}
	if !chartHas(t, h, "2100") {
		t.Error("GET /Accounts must list the new account straight away")
	}
}

func TestCreateAccountCountsLengthLimitsInCharacters(t *testing.T) {
	h := newTestHandler(t)
	body := `{"Code":"` + strings.Repeat("é", 10) + `","Name":"` + strings.Repeat("é", 150) + `","Type":"TERMLIAB"}`
	if code, b := putAccount(t, h, body); code != 200 {
		t.Fatalf("10 and 150 multi-byte characters are within the limits: %d %v", code, b)
	}
}

func TestCreateAccountRefusesWhatXeroRefuses(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{"no code", `{"Name":"Loan","Type":"TERMLIAB"}`, "Please enter a Code."},
		{"long code", `{"Code":"12345678901","Name":"Loan","Type":"TERMLIAB"}`, "not be more than 10"},
		{"no name", `{"Code":"2100","Type":"TERMLIAB"}`, "Please enter a Name."},
		{"long name", `{"Code":"2100","Name":"` + strings.Repeat("n", 151) + `","Type":"TERMLIAB"}`, "not be more than 150"},
		{"no type", `{"Code":"2100","Name":"Loan"}`, "Account Type '' is not valid."},
		{"bad type", `{"Code":"2100","Name":"Loan","Type":"LOAN"}`, "Account Type 'LOAN' is not valid."},
		{"duplicate code", `{"Code":"090","Name":"Second Bank","Type":"BANK"}`, "Please enter a unique Code."},
		{"duplicate name", `{"Code":"2100","Name":"bank fees","Type":"EXPENSE"}`, "Please enter a unique Name."},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newTestHandler(t)
			code, body := putAccount(t, h, c.body)
			if code != 400 || body["Type"] != "ValidationException" || !strings.Contains(rawString(body), c.want) {
				t.Fatalf("want a 400 ValidationException with %q, got %d %v", c.want, code, body)
			}
			if n := len(listAccounts(t, h, store.DefaultTenantID).Accounts); n != 8 {
				t.Errorf("a refused account must not reach the chart: %d accounts", n)
			}
		})
	}
}

func TestCreateAccountIsDroppedByResetAndCanBeMadeAgain(t *testing.T) {
	h := newTestHandler(t)
	body := `{"Code":"2100","Name":"Merchant Advance Loan","Type":"TERMLIAB"}`
	if code, b := putAccount(t, h, body); code != 200 {
		t.Fatalf("status %d: %v", code, b)
	}
	h.store.Reset()
	if chartHas(t, h, "2100") || !chartHas(t, h, "090") {
		t.Fatal("reset must drop an added account and restore the default chart")
	}
	if code, b := putAccount(t, h, body); code != 200 {
		t.Fatalf("after reset the account must be creatable again: %d %v", code, b)
	}
}

func rawString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
