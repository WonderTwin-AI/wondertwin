package api

import (
	"encoding/json"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

type accountsBody struct {
	Status   string
	Accounts []map[string]any
}

func listAccounts(t *testing.T, h *Handler, tenant string) accountsBody {
	t.Helper()
	rec := get(h, "/api.xro/2.0/Accounts", accounting(t, h, tenant))
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var b accountsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	return b
}

func TestAccountsChartCarriesTheAccountsFinanceOpsPostsTo(t *testing.T) {
	h := newTestHandler(t)
	b := listAccounts(t, h, store.DefaultTenantID)
	if b.Status != "OK" {
		t.Fatalf("status = %q", b.Status)
	}
	byCode := map[string]map[string]any{}
	for _, a := range b.Accounts {
		byCode[a["Code"].(string)] = a
	}
	for code, typ := range map[string]string{"090": "BANK", "200": "REVENUE", "201": "REVENUE", "404": "EXPENSE", "610": "CURRENT", "800": "CURRLIAB"} {
		a, ok := byCode[code]
		if !ok || a["Type"] != typ || a["Status"] != "ACTIVE" {
			t.Fatalf("account %s = %v, want an active %s", code, a, typ)
		}
	}
	if byCode["610"]["SystemAccount"] != "DEBTORS" || byCode["800"]["SystemAccount"] != "CREDITORS" {
		t.Fatal("the receivable and payable accounts must be the system accounts Xero requires")
	}
	// xero-python parses UpdatedDateUTC as an ms-format date and raises on ISO (time-001).
	if s, _ := byCode["200"]["UpdatedDateUTC"].(string); !msdateValue.MatchString(s) {
		t.Fatalf("UpdatedDateUTC = %v, want /Date(ms+0000)/", byCode["200"]["UpdatedDateUTC"])
	}
	if _, ok := byCode["200"]["BankAccountNumber"]; ok {
		t.Fatal("an unset optional field must be omitted, not null")
	}
}

func TestAccountsAreKeptPerTenantAndStableAcrossReset(t *testing.T) {
	h := newTestHandler(t)
	if got := listAccounts(t, h, "0f0f0f0f-0000-4000-8000-00000000dead").Accounts; got == nil || len(got) != 0 {
		t.Fatalf("an unknown tenant must see an empty chart, got %v", got)
	}
	before := listAccounts(t, h, store.DefaultTenantID).Accounts[0]["AccountID"]
	h.store.Reset()
	if after := listAccounts(t, h, store.DefaultTenantID).Accounts[0]["AccountID"]; after != before {
		t.Fatalf("AccountID changed across a reset: %v then %v", before, after)
	}
}
