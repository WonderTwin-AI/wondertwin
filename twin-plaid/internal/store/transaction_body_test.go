package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func marshalled(t *testing.T, b TransactionBody) map[string]any {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTransactionBodyKeepsEachPresenceRuleOfThePlaidModel(t *testing.T) {
	out := marshalled(t, TransactionBody{AccountID: "a", TransactionID: "t", Date: "2026-10-01", PaymentChannel: "other"})

	// Required and nullable: always present, null when empty.
	for _, k := range []string{"iso_currency_code", "unofficial_currency_code", "pending_transaction_id", "account_owner",
		"authorized_date", "authorized_datetime", "datetime", "transaction_code"} {
		if v, ok := out[k]; !ok || v != nil {
			t.Errorf("%s = %v, present %v; want present as null", k, v, ok)
		}
	}
	// Required objects: present with every key.
	if loc, _ := out["location"].(map[string]any); len(loc) != 8 {
		t.Errorf("location = %v, want its 8 keys as null", out["location"])
	}
	if pm, _ := out["payment_meta"].(map[string]any); len(pm) != 8 {
		t.Errorf("payment_meta = %v, want its 8 keys as null", out["payment_meta"])
	}
	// Optional: left out when unset, never null.
	for _, k := range []string{"category", "category_id", "check_number", "merchant_name", "original_description", "transaction_type",
		"logo_url", "website", "personal_finance_category", "personal_finance_category_icon_url", "counterparties",
		"merchant_entity_id", "merchant_category_code", "running_balance"} {
		if _, ok := out[k]; ok {
			t.Errorf("%s is present on a body that never set it", k)
		}
	}
}

func TestTransactionBodyOptionalFieldsSendNullOnlyWhereTheModelAllowsIt(t *testing.T) {
	b := TransactionBody{AccountID: "a", TransactionID: "t", Date: "2026-10-01", PaymentChannel: "other",
		MerchantName: Null[string](), RunningBalance: Value(12.5), Counterparties: Some([]Counterparty{}),
		TransactionType: Some("place")}
	out := marshalled(t, b)
	if v, ok := out["merchant_name"]; !ok || v != nil {
		t.Errorf("merchant_name = %v, present %v; want an explicit null", v, ok)
	}
	if out["running_balance"] != 12.5 || out["transaction_type"] != "place" {
		t.Errorf("values lost: %v", out)
	}
	if c, ok := out["counterparties"].([]any); !ok || len(c) != 0 {
		t.Errorf("counterparties = %v, want an empty list", out["counterparties"])
	}

	// A null read for a field the model does not allow to be null reads as unset.
	var back TransactionBody
	if err := json.Unmarshal([]byte(`{"transaction_type": null, "personal_finance_category": null}`), &back); err != nil {
		t.Fatal(err)
	}
	if back.TransactionType.Set || back.PersonalFinanceCategory.Set {
		t.Errorf("a null for a non-nullable field was kept: %+v", back)
	}
}

func TestLoadStateRefusesATransactionThatWouldBreakTheSDK(t *testing.T) {
	good := `{"account_id":"a","transaction_id":"t","date":"2026-10-01","payment_channel":"other"}`
	for name, body := range map[string]string{
		"missing id":      `{"account_id":"a","date":"2026-10-01","payment_channel":"other"}`,
		"bad date":        `{"account_id":"a","transaction_id":"t","date":"10/01/2026","payment_channel":"other"}`,
		"bad channel":     `{"account_id":"a","transaction_id":"t","date":"2026-10-01","payment_channel":"cash"}`,
		"bad type":        `{"account_id":"a","transaction_id":"t","date":"2026-10-01","payment_channel":"other","transaction_type":"x"}`,
		"missing channel": `{"account_id":"a","transaction_id":"t","date":"2026-10-01"}`,
	} {
		s := New()
		err := s.LoadState([]byte(`{"transactions":{"t":{"transaction_id":"t","item_id":"i","account_id":"a","date":"2026-10-01","body":` + body + `}}}`))
		if err == nil || !strings.Contains(err.Error(), "transaction") {
			t.Errorf("%s: LoadState = %v, want a refusal", name, err)
		}
		if _, ok := s.Transactions.Get("t"); ok {
			t.Errorf("%s: a refused snapshot was partly loaded", name)
		}
	}

	s := New()
	if err := s.LoadState([]byte(`{"transactions":{"t":{"transaction_id":"t","item_id":"i","account_id":"a","date":"2026-10-01","body":` + good + `}}}`)); err != nil {
		t.Fatalf("a complete row was refused: %v", err)
	}
}
