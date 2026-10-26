package api

import (
	"errors"
	"strings"
	"testing"
)

func TestWhereLogicGroupsAndBindsTighterThanOr(t *testing.T) {
	cases := map[string]string{
		`A==1 and B==2`:           `(A==1 and B==2)`,
		`A==1 AND B==2`:           `(A==1 and B==2)`,
		`A==1 && B==2`:            `(A==1 and B==2)`,
		`A==1 or B==2`:            `(A==1 or B==2)`,
		`A==1 OR B==2`:            `(A==1 or B==2)`,
		`A==1||B==2`:              `(A==1 or B==2)`,
		`A==1 or B==2 and C==3`:   `(A==1 or (B==2 and C==3))`,
		`A==1 and B==2 or C==3`:   `((A==1 and B==2) or C==3)`,
		`A==1 and B==2 and C==3`:  `((A==1 and B==2) and C==3)`,
		`(A==1 or B==2) and C==3`: `((A==1 or B==2) and C==3)`,
		`((A==1))`:                `A==1`,
		`Type=="ACCREC" && (Status=="PAID" || Status=="AUTHORISED")`: `(Type=="ACCREC" and (Status=="PAID" or Status=="AUTHORISED"))`,
		`AmountDue > 5000 and DueDate > DateTime(2015, 01, 01)`:      `(AmountDue>5000 and DueDate>DateTime(2015,1,1,0,0,0))`,
		`Contact.ContactID == Guid("AB") or Reference == null`:       `(Contact.ContactID==Guid("AB") or Reference==null)`,
	}
	for src, want := range cases {
		got, err := ParseWhere(src)
		if err != nil || got.String() != want {
			t.Errorf("%s: got %v, %v; want %s", src, got, err, want)
		}
	}
}

func TestWhereLogicParsesStringMethods(t *testing.T) {
	cases := map[string]string{
		`Name.Contains("ac")`:                   `Name.Contains("ac")`,
		`Name.StartsWith("A")`:                  `Name.StartsWith("A")`,
		`Contact.Name.EndsWith("Ltd")`:          `Contact.Name.EndsWith("Ltd")`,
		`Name.Contains("a") and Status=="PAID"`: `(Name.Contains("a") and Status=="PAID")`,
	}
	for src, want := range cases {
		got, err := ParseWhere(src)
		if err != nil || got.String() != want {
			t.Errorf("%s: got %v, %v; want %s", src, got, err, want)
		}
	}
}

func TestWhereLogicRefusesWhatItCannotGroup(t *testing.T) {
	cases := map[string]string{
		`A==1 and`:                     "expected a field name",
		`or A==1`:                      "expected a comparison after or",
		`()`:                           "expected a field name",
		`(A==1`:                        "missing )",
		`A==1)`:                        "unexpected",
		`A==1 B==2`:                    "unexpected",
		`A==1 order==2`:                "unexpected", // "or" must end where a word ends
		`A==1 android==2`:              "unexpected",
		`Name.Length("x")`:             "unknown method Length",
		`Name.Contains(5)`:             "takes one string",
		`Name.Contains("x"`:            "missing )",
		`Name.Contains`:                "expected a comparison after Name.Contains",
		`Name.Contains("a") == "b"`:    "unexpected",
		`A==1 && (B==2 or C=="x" && )`: "expected a field name",
	}
	for src, want := range cases {
		_, err := ParseWhere(src)
		var we *WhereError
		if !errors.As(err, &we) || !strings.Contains(we.Msg, want) {
			t.Errorf("%q: error %v, want a WhereError containing %q", src, err, want)
		}
	}
}
