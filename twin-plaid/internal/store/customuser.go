package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
)

// CustomUser is the configuration a Sandbox client passes as override_password
// with override_username user_custom: Plaid's UserCustomPassword (spec 1.740.1)
// plus roll_dates_forward (docs only, 2026-09-24). Only what the app emulator
// models is kept. The other documented fields (mfa, recaptcha, force_error,
// identity, holdings, liability, income and the rest) are accepted and ignored,
// and declared in divergences.json. Every top-level field is optional, as the
// docs say, although the spec marks some required.
type CustomUser struct {
	Seed             string          `json:"seed,omitempty"`
	RollDatesForward bool            `json:"roll_dates_forward,omitempty"`
	OverrideAccounts []CustomAccount `json:"override_accounts,omitempty"`
}

// CustomAccount is one OverrideAccounts entry.
type CustomAccount struct {
	Type                    string                  `json:"type"`
	Subtype                 string                  `json:"subtype"`
	StartingBalance         *float64                `json:"starting_balance,omitempty"`
	ForceAvailableBalance   *float64                `json:"force_available_balance,omitempty"`
	HasNullAvailableBalance bool                    `json:"has_null_available_balance,omitempty"`
	Currency                string                  `json:"currency,omitempty"`
	Meta                    CustomAccountMeta       `json:"meta"`
	Numbers                 CustomAccountNumbers    `json:"numbers"`
	Transactions            []CustomUserTransaction `json:"transactions,omitempty"`
}

// CustomAccountMeta is the modelled part of Meta; apy is ignored.
type CustomAccountMeta struct {
	Name         string   `json:"name,omitempty"`
	OfficialName string   `json:"official_name,omitempty"`
	Limit        *float64 `json:"limit,omitempty"`
	Mask         *string  `json:"mask,omitempty"`
}

// CustomAccountNumbers is the US part of Numbers; the eft, international and
// bacs numbers are ignored.
type CustomAccountNumbers struct {
	Account        string `json:"account,omitempty"`
	ACHRouting     string `json:"ach_routing,omitempty"`
	ACHWireRouting string `json:"ach_wire_routing,omitempty"`
}

// CustomUserTransaction is a TransactionOverride. The four fields other than
// currency are required, and the dates are YYYY-MM-DD.
type CustomUserTransaction struct {
	DateTransacted string   `json:"date_transacted"`
	DatePosted     string   `json:"date_posted"`
	Amount         *float64 `json:"amount"`
	Description    string   `json:"description"`
	Currency       string   `json:"currency,omitempty"`
}

// maxCustomAccounts is the docs' limit: more than ten accounts fails Link.
const maxCustomAccounts = 10

var maskRE = regexp.MustCompile(`^$|^[A-Za-z0-9]{2,4}$`)

// Account types and subtypes from the OverrideAccounts enums (spec 1.740.1).
var (
	customAccountTypes    = []string{"investment", "credit", "depository", "loan", "payroll", "other"}
	customAccountSubtypes = []string{"401a", "401k", "403B", "457b", "529", "auto", "brokerage", "business",
		"cash isa", "cash management", "cd", "checking", "commercial", "commercial line of credit", "construction",
		"consumer", "credit card", "crypto exchange", "ebt", "education savings account", "fhsa", "fixed annuity",
		"gic", "health reimbursement arrangement", "home equity", "home equity loan", "hsa", "isa", "ira", "keogh",
		"lif", "life insurance", "limited purpose checking", "line of credit", "lira", "loan", "lrif", "lrsp",
		"money market", "mortgage", "mutual fund", "non-custodial wallet", "non-taxable brokerage account", "other",
		"other insurance", "other annuity", "overdraft", "paypal", "payroll", "pension", "prediction market",
		"prepaid", "prif", "profit sharing plan", "qshr", "rdsp", "resp", "retirement", "rlif", "roth", "roth 401k",
		"roth 403B", "roth 457b", "roth pension", "roth profit sharing plan", "roth thrift savings plan", "rrif",
		"rrsp", "sarsep", "savings", "sep ira", "simple ira", "sipp", "stock plan", "student",
		"thrift savings plan", "tfsa", "trust", "ugma", "utma", "variable annuity"}
)

// ParseCustomUser decodes and checks a user_custom configuration. The error
// names the problem; Plaid's answer to a bad configuration is not documented
// (quirk LIFECYCLE-008), so the caller's refusal is a labelled guess.
func ParseCustomUser(password string) (*CustomUser, error) {
	notObject := errors.New("must be a JSON object")
	raw := bytes.TrimSpace([]byte(password))
	if !bytes.HasPrefix(raw, []byte("{")) {
		return nil, notObject
	}
	var cfg CustomUser
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&cfg); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return nil, fmt.Errorf("%s must be %s", typeErr.Field, jsonKind(typeErr.Type.Kind()))
		}
		return nil, notObject
	}
	if dec.More() {
		return nil, notObject
	}
	if len(cfg.OverrideAccounts) > maxCustomAccounts {
		return nil, fmt.Errorf("override_accounts has %d accounts; at most %d are supported", len(cfg.OverrideAccounts), maxCustomAccounts)
	}
	for i, a := range cfg.OverrideAccounts {
		at := fmt.Sprintf("override_accounts[%d]", i)
		if !slices.Contains(customAccountTypes, a.Type) {
			return nil, fmt.Errorf("%s.type %q is not an account type", at, a.Type)
		}
		if !slices.Contains(customAccountSubtypes, a.Subtype) {
			return nil, fmt.Errorf("%s.subtype %q is not an account subtype", at, a.Subtype)
		}
		if a.ForceAvailableBalance != nil && a.HasNullAvailableBalance {
			return nil, fmt.Errorf("%s cannot set both force_available_balance and has_null_available_balance", at)
		}
		if a.Meta.Mask != nil && !maskRE.MatchString(*a.Meta.Mask) {
			return nil, fmt.Errorf("%s.meta.mask must be empty or 2 to 4 letters and digits", at)
		}
		for j, t := range a.Transactions {
			tt := fmt.Sprintf("%s.transactions[%d]", at, j)
			var missing []string
			for _, f := range []struct {
				name string
				set  bool
			}{{"date_transacted", t.DateTransacted != ""}, {"date_posted", t.DatePosted != ""},
				{"amount", t.Amount != nil}, {"description", t.Description != ""}} {
				if !f.set {
					missing = append(missing, f.name)
				}
			}
			if len(missing) > 0 {
				return nil, fmt.Errorf("%s is missing %s", tt, strings.Join(missing, ", "))
			}
			for _, d := range []struct{ name, v string }{{"date_transacted", t.DateTransacted}, {"date_posted", t.DatePosted}} {
				if _, err := time.Parse("2006-01-02", d.v); err != nil {
					return nil, fmt.Errorf("%s.%s %q is not a YYYY-MM-DD date", tt, d.name, d.v)
				}
			}
		}
	}
	return &cfg, nil
}

// jsonKind names a Go kind the way a JSON client would read it.
func jsonKind(k reflect.Kind) string {
	switch k {
	case reflect.Float64, reflect.Int:
		return "a number"
	case reflect.Bool:
		return "a boolean"
	case reflect.Slice:
		return "an array"
	case reflect.Struct:
		return "an object"
	default:
		return "a string"
	}
}
