package store

import (
	"strings"
	"testing"
)

func TestCustomUserConfigReadsTheModelledFields(t *testing.T) {
	cfg, err := ParseCustomUser(`{
		"version": "2", "seed": "acme", "roll_dates_forward": true, "mfa": {"type": "device"},
		"override_accounts": [{
			"type": "depository", "subtype": "checking", "starting_balance": 1250.5,
			"force_available_balance": 1200, "currency": "USD",
			"meta": {"name": "Operating", "official_name": "ACME Operating", "limit": 500, "mask": "4321", "apy": 4.25},
			"numbers": {"account": "9900001111", "ach_routing": "011401533", "ach_wire_routing": "021000021", "bacs_sort_code": "1"},
			"identity": {"names": ["ACME"]},
			"transactions": [{"date_transacted": "2026-08-01", "date_posted": "2026-08-02", "amount": -950, "description": "STRIPE PAYOUT", "currency": "USD"}]
		}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Seed != "acme" || !cfg.RollDatesForward || len(cfg.OverrideAccounts) != 1 {
		t.Fatalf("cfg = %+v", cfg)
	}
	a := cfg.OverrideAccounts[0]
	if a.Type != "depository" || a.Subtype != "checking" || *a.StartingBalance != 1250.5 || *a.ForceAvailableBalance != 1200 ||
		a.Meta.Name != "Operating" || a.Meta.OfficialName != "ACME Operating" || *a.Meta.Limit != 500 || *a.Meta.Mask != "4321" ||
		a.Numbers != (CustomAccountNumbers{Account: "9900001111", ACHRouting: "011401533", ACHWireRouting: "021000021"}) {
		t.Fatalf("account = %+v", a)
	}
	tx := a.Transactions[0]
	if tx.DateTransacted != "2026-08-01" || tx.DatePosted != "2026-08-02" || *tx.Amount != -950 || tx.Description != "STRIPE PAYOUT" || tx.Currency != "USD" {
		t.Fatalf("transaction = %+v", tx)
	}
}

func TestCustomUserConfigAllowsEveryTopLevelFieldToBeMissing(t *testing.T) {
	for _, in := range []string{`{}`, ` {"seed": "x"} `, `{"override_accounts": []}`,
		`{"override_accounts": [{"type": "loan", "subtype": "student", "meta": {"mask": ""}}]}`} {
		if _, err := ParseCustomUser(in); err != nil {
			t.Errorf("%s: %v", in, err)
		}
	}
}

func TestCustomUserConfigRefusals(t *testing.T) {
	account := func(extra string) string {
		return `{"override_accounts": [{"type": "depository", "subtype": "checking"` + extra + `}]}`
	}
	tx := func(fields string) string { return account(`, "transactions": [{` + fields + `}]`) }
	eleven := `{"override_accounts": [` + strings.TrimSuffix(strings.Repeat(`{"type": "depository", "subtype": "checking"},`, 11), ",") + `]}`
	for _, c := range []struct{ in, want string }{
		{``, "must be a JSON object"},
		{`not json`, "must be a JSON object"},
		{`[]`, "must be a JSON object"},
		{`{"seed": "a"} {}`, "must be a JSON object"},
		{`{"seed": 5}`, "seed must be a string"},
		{`{"override_accounts": {}}`, "override_accounts must be an array"},
		{eleven, "override_accounts has 11 accounts; at most 10 are supported"},
		{`{"override_accounts": [{"type": "bank", "subtype": "checking"}]}`, `override_accounts[0].type "bank" is not an account type`},
		{`{"override_accounts": [{"type": "depository"}]}`, `override_accounts[0].subtype "" is not an account subtype`},
		{account(`, "force_available_balance": 1, "has_null_available_balance": true`), "cannot set both force_available_balance and has_null_available_balance"},
		{account(`, "meta": {"mask": "12345"}`), "meta.mask must be empty or 2 to 4 letters and digits"},
		{account(`, "meta": {"mask": "1-2"}`), "meta.mask must be empty or 2 to 4 letters and digits"},
		{account(`, "starting_balance": "10"`), "starting_balance must be a number"},
		{tx(`"amount": 1`), "transactions[0] is missing date_transacted, date_posted, description"},
		{tx(`"date_transacted": "2026-08-01", "date_posted": "2026-08-01", "description": "x"`), "transactions[0] is missing amount"},
		{tx(`"date_transacted": "08/01/2026", "date_posted": "2026-08-01", "amount": 1, "description": "x"`), `date_transacted "08/01/2026" is not a YYYY-MM-DD date`},
		{tx(`"date_transacted": "2026-08-01", "date_posted": "2026-02-30", "amount": 1, "description": "x"`), `date_posted "2026-02-30" is not a YYYY-MM-DD date`},
	} {
		_, err := ParseCustomUser(c.in)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error = %v, want it to contain %q", c.in, err, c.want)
		}
	}
}

func TestPublicTokenCarriesCustomConfigThroughStateAndReset(t *testing.T) {
	s := seeded(1)
	cfg, err := ParseCustomUser(`{"override_accounts": [{"type": "depository", "subtype": "checking", "starting_balance": 42}]}`)
	if err != nil {
		t.Fatal(err)
	}
	pt := s.CreateCustomPublicToken("ins_1", []string{"transactions"}, "", cfg)

	raw, err := jsonMarshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"custom_user":{"override_accounts":[{"type":"depository","subtype":"checking","starting_balance":42`) {
		t.Fatalf("state does not carry the configuration: %s", raw)
	}
	loaded := seeded(2)
	if err := loaded.LoadState(raw); err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.PublicTokens.Get(pt.Token)
	if !ok || got.CustomUser == nil || *got.CustomUser.OverrideAccounts[0].StartingBalance != 42 {
		t.Fatalf("loaded token = %+v", got)
	}
	if plain := s.CreatePublicToken("ins_1", []string{"auth"}, ""); plain.CustomUser != nil {
		t.Fatal("a plain token carries a configuration")
	}

	s.Reset()
	if _, ok := s.PublicTokens.Get(pt.Token); ok {
		t.Fatal("Reset left the configured token behind")
	}
}
