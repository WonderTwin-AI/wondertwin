package api

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func utc(y int, m time.Month, d, h, mi, s int) time.Time {
	return time.Date(y, m, d, h, mi, s, 0, time.UTC)
}

func TestWhereScanLiteralsAndOperators(t *testing.T) {
	cases := []struct {
		src  string
		want cmpExpr
	}{
		{`Type=="BANK"`, cmpExpr{"Type", "==", "BANK"}},
		{`  Status  !=  "PAID"  `, cmpExpr{"Status", "!=", "PAID"}},
		{`AmountDue > 5000`, cmpExpr{"AmountDue", ">", 5000.0}},
		{`AmountDue >= 12.5`, cmpExpr{"AmountDue", ">=", 12.5}},
		{`Total < -3`, cmpExpr{"Total", "<", -3.0}},
		{`Total <= 0`, cmpExpr{"Total", "<=", 0.0}},
		{`HasAttachments==true`, cmpExpr{"HasAttachments", "==", true}},
		{`IsDiscounted == false`, cmpExpr{"IsDiscounted", "==", false}},
		{`Reference == null`, cmpExpr{"Reference", "==", nil}},
		{`Contact.ContactID == Guid("AB12-cd")`, cmpExpr{"Contact.ContactID", "==", guid("AB12-cd")}},
		{`DueDate > DateTime(2015, 01, 01)`, cmpExpr{"DueDate", ">", utc(2015, 1, 1, 0, 0, 0)}},
		{`Date==DateTime(2026,10,19,9,30,5)`, cmpExpr{"Date", "==", utc(2026, 10, 19, 9, 30, 5)}},
		{`D == DateTime(2024,2,29)`, cmpExpr{"D", "==", utc(2024, 2, 29, 0, 0, 0)}},
		{`Name == ""`, cmpExpr{"Name", "==", ""}},
		{`Name == "a == b"`, cmpExpr{"Name", "==", "a == b"}},
		// A single = is an equality too: localstack/finance-ops looks a contact up with
		// where=AccountNumber="cus_1" against the real Xero.
		{`AccountNumber="cus_1"`, cmpExpr{"AccountNumber", "==", "cus_1"}},
		{`Type = "BANK"`, cmpExpr{"Type", "==", "BANK"}},
		{`Name="a=b"`, cmpExpr{"Name", "==", "a=b"}},
	}
	for _, tc := range cases {
		got, err := ParseWhere(tc.src)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %#v, %v; want %#v", tc.src, got, err, tc.want)
		}
	}
}

func TestWhereScanRefusesWhatXeroRefuses(t *testing.T) {
	cases := map[string]string{
		"":                               "expected a field name",
		`== 1`:                           "expected a field name",
		`Type`:                           "expected a comparison after Type",
		`Type == `:                       "expected a value",
		`Type == BANK`:                   "expected a value",
		`Type == "BANK`:                  "unterminated string",
		`Type == "BANK" extra`:           "unexpected",
		`Type == nullable`:               "expected a value",
		`Type == truely`:                 "expected a value",
		`D == DateTime(2015, 01)`:        "DateTime takes",
		`D == DateTime(2015, x, 01)`:     "DateTime needs numbers",
		`D == DateTime(2026,13,45)`:      "is not a date",
		`D == DateTime(2026,2,30)`:       "is not a date",
		`D == DateTime(2026,0,1)`:        "is not a date",
		`D == DateTime(2026,1,1,24,0,0)`: "is not a date",
		`D == DateTime(2026,1,1,0,60,0)`: "is not a date",
		`D == DateTime 2015`:             "expected ( after DateTime",
		`D == Guid(abc)`:                 "expected a string",
		`D == Guid("abc"`:                "missing )",
		`Type%3D%3D%2522BANK%2522`:       "expected a comparison after Type", // double-encoded: still encoded at the parser
		`Type==%22BANK%22`:               "expected a value",
	}
	for src, want := range cases {
		_, err := ParseWhere(src)
		var we *WhereError
		if !errors.As(err, &we) || !strings.Contains(we.Msg, want) {
			t.Errorf("%q: error %v, want a WhereError containing %q", src, err, want)
		}
	}
}
