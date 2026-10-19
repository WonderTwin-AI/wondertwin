package api

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func recordOf(m map[string]any) Getter {
	return func(path string) (any, bool) { v, ok := m[path]; return v, ok }
}

func matches(t *testing.T, src string, rec map[string]any) (bool, error) {
	t.Helper()
	w, err := ParseWhere(src)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return w.Match(recordOf(rec))
}

func TestWhereEvalComparesTypedValues(t *testing.T) {
	inv := map[string]any{
		"Type": "ACCREC", "Status": "PAID", "Total": 5000.5, "Count": 3, "Big": int64(1000000000001),
		"HasAttachments": true, "Contact.ContactID": "AB12-CD", "Reference": nil,
		"Date": time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC),
	}
	cases := map[string]bool{
		`Type=="ACCREC"`: true, `Type=="ACCPAY"`: false, `Type!="ACCPAY"`: true, `Status < "Q"`: true, `Status >= "PAID"`: true,
		`Total > 5000`: true, `Total <= 5000`: false, `Total == 5000.5`: true, `Count == 3`: true, `Count >= 4`: false,
		`Big > 1000000000000.5`: true, `Big == 1000000000001`: true, // beyond what a formatted float keeps apart
		`HasAttachments==true`: true, `HasAttachments==false`: false,
		`Contact.ContactID == Guid("ab12-cd")`: true, `Contact.ContactID != Guid("ab12-cd")`: false,
		`Date == DateTime(2026,10,19)`: true, `Date > DateTime(2026,10,18)`: true, `Date < DateTime(2026,10,19,0,0,1)`: true,
		`Date >= DateTime(2026,10,20)`: false,
	}
	for src, want := range cases {
		if got, err := matches(t, src, inv); err != nil || got != want {
			t.Errorf("%s: got %v, %v; want %v", src, got, err, want)
		}
	}
}

func TestWhereEvalNullAndMissingFields(t *testing.T) {
	rec := map[string]any{"Name": "Acme", "Reference": nil}
	cases := map[string]bool{
		`Reference == null`: true, `Missing == null`: true, `Name == null`: false, `Name != null`: true,
		`Missing != "x"`: true, `Missing == "x"`: false, `Missing > 1`: false, `Reference <= "a"`: false,
		`Missing.Contains("x")`: false, `Name.Contains("cm")`: true, `Name.StartsWith("Ac")`: true,
		`Name.EndsWith("me")`: true, `Name.StartsWith("cme")`: false,
	}
	for src, want := range cases {
		if got, err := matches(t, src, rec); err != nil || got != want {
			t.Errorf("%s: got %v, %v; want %v", src, got, err, want)
		}
	}
}

func TestWhereEvalLogicShortCircuits(t *testing.T) {
	rec := map[string]any{"A": 1, "B": 2.0, "Name": "x"}
	cases := map[string]bool{
		`A==1 and B==2`: true, `A==1 and B==3`: false, `A==2 or B==2`: true, `A==2 or B==3`: false,
		`A==1 or B > "x"`:              true,  // the type error on the right is never reached
		`A==2 and B > "x"`:             false, // nor here
		`(A==2 or A==1) and B==2`:      true,
		`A==2 or A==3 or B==2`:         true,
		`Name.Contains("x") and A < 2`: true,
	}
	for src, want := range cases {
		if got, err := matches(t, src, rec); err != nil || got != want {
			t.Errorf("%s: got %v, %v; want %v", src, got, err, want)
		}
	}
}

func TestWhereEvalReportsTypeMismatches(t *testing.T) {
	rec := map[string]any{"Total": 5.0, "Name": "x", "Date": time.Now(), "Flag": true}
	for _, src := range []string{`Total == "5"`, `Name > 1`, `Date == "2026-10-19"`, `Flag == 1`, `Name == DateTime(2026,1,1)`, `A==1 or Total == "5"`} {
		_, err := matches(t, src, rec)
		var we *WhereError
		if !errors.As(err, &we) || !strings.Contains(we.Msg, "cannot compare") {
			t.Errorf("%s: error %v, want a WhereError saying cannot compare", src, err)
		}
	}
}
