package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func TestBankTransactionWireReadsDatesAndResolvesTheBankAccount(t *testing.T) {
	var w bankTransaction
	in := `{"Type":"RECEIVE","Date":"2026-11-03","Contact":{"ContactID":"c1","Name":"ignored"},"BankAccount":{"Code":"090"},` +
		`"LineItems":[{"Description":"Advance","LineAmount":5000,"AccountCode":"200"}]}`
	if err := json.Unmarshal([]byte(in), &w); err != nil {
		t.Fatal(err)
	}
	bt := inBankTransaction(w)
	if bt.Date == nil || !bt.Date.Equal(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date = %v", bt.Date)
	}
	if bt.Contact == nil || bt.Contact.ContactID != "c1" {
		t.Fatalf("the contact must be kept as a reference, got %+v", bt.Contact)
	}
	if bt.BankAccount == nil || bt.BankAccount.Code != "090" || len(bt.LineItems) != 1 {
		t.Fatalf("the bank account and lines must survive the way in: %+v", bt)
	}

	bt.UpdatedDateUTC = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	out, err := json.Marshal(newTestHandler(t).wireBankTransaction(store.DefaultTenantID, bt))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Date, UpdatedDateUTC string
		BankAccount          store.AccountRef
	}
	_ = json.Unmarshal(out, &m)
	for _, s := range []string{m.Date, m.UpdatedDateUTC} {
		if !msdateValue.MatchString(s) {
			t.Errorf("%q is not /Date(ms+0000)/ (time-001)", s)
		}
	}
	if m.BankAccount.AccountID == "" || m.BankAccount.Code != "090" || m.BankAccount.Name != "Business Bank Account" {
		t.Errorf("the bank account must carry its ID, code and name from the chart, got %+v", m.BankAccount)
	}

	empty, _ := json.Marshal(newTestHandler(t).wireBankTransaction(store.DefaultTenantID, store.BankTransaction{}))
	var loose map[string]any
	_ = json.Unmarshal(empty, &loose)
	if l, ok := loose["LineItems"].([]any); !ok || l == nil {
		t.Errorf("LineItems must be [], got %v", loose["LineItems"])
	}
	for _, k := range []string{"Date", "Contact", "BankAccount"} {
		if _, ok := loose[k]; ok {
			t.Errorf("%s is unset and must be omitted, not null", k)
		}
	}
}

func TestBankTransactionTotalsDefaultToInclusive(t *testing.T) {
	taxRates["OUTPUT"] = 0.25
	t.Cleanup(func() { delete(taxRates, "OUTPUT") })
	line := func() []store.LineItem {
		return []store.LineItem{{LineAmount: ptr(125), TaxType: "OUTPUT", AccountCode: "200"}}
	}

	bt := store.BankTransaction{LineItems: line()}
	if msgs := bankTransactionLines(&bt, 2); len(msgs) != 0 {
		t.Fatal(msgs)
	}
	if bt.LineAmountTypes != "Inclusive" || bt.SubTotal != 100 || bt.TotalTax != 25 || bt.Total != 125 {
		t.Errorf("no LineAmountTypes must be Inclusive (crud-018, operator decision 2026-11-03): %s %v %v %v",
			bt.LineAmountTypes, bt.SubTotal, bt.TotalTax, bt.Total)
	}

	for _, c := range []struct {
		types           string
		sub, tax, total float64
	}{{"Exclusive", 125, 31.25, 156.25}, {"NoTax", 125, 0, 125}} {
		bt := store.BankTransaction{LineAmountTypes: c.types, LineItems: line()}
		bankTransactionLines(&bt, 2)
		if bt.SubTotal != c.sub || bt.TotalTax != c.tax || bt.Total != c.total {
			t.Errorf("%s: totals = %v %v %v, want %v %v %v", c.types, bt.SubTotal, bt.TotalTax, bt.Total, c.sub, c.tax, c.total)
		}
	}

	bad := store.BankTransaction{LineAmountTypes: "Gross", LineItems: line()}
	if msgs := bankTransactionLines(&bad, 2); len(msgs) == 0 {
		t.Error("an unknown LineAmountTypes must be refused")
	}
}
