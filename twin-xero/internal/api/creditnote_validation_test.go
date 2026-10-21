package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func TestCreditNoteValidationRefusesWhatXeroRefuses(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	ok := fmt.Sprintf(`"Type":"ACCRECCREDIT","Contact":{"ContactID":%q},"Status":"AUTHORISED","CreditNoteNumber":"CN-9"`, cid)
	line := `"LineItems":[{"AccountCode":"200","UnitAmount":10}]`
	if code, _ := putCreditNotes(t, h, "", `{`+ok+`,`+line+`}`); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	cases := map[string]struct{ body, want string }{
		"a type that is not a credit type":    {`{"Type":"ACCREC","Contact":{"ContactID":"` + cid + `"}}`, "Type must be ACCRECCREDIT or ACCPAYCREDIT"},
		"a status only a later write can set": {`{"Type":"ACCRECCREDIT","Status":"PAID","Contact":{"ContactID":"` + cid + `"}}`, "cannot be created with the status PAID"},
		"no contact":                          {`{"Type":"ACCRECCREDIT"}`, "Contact must be specified"},
		"an unknown contact":                  {`{"Type":"ACCRECCREDIT","Contact":{"ContactID":"00000000-0000-4000-8000-0000000000ff"}}`, "could not be found"},
		"a duplicate credit note number":      {`{` + ok + `,` + line + `}`, "Credit Note # must be unique"},
		"an unknown account code":             {`{"Type":"ACCRECCREDIT","Contact":{"ContactID":"` + cid + `"},"LineItems":[{"AccountCode":"999","UnitAmount":1}]}`, "account code '999' is not a valid code"},
		"no account code on a live credit":    {`{"Type":"ACCRECCREDIT","Status":"AUTHORISED","Contact":{"ContactID":"` + cid + `"},"LineItems":[{"UnitAmount":1}]}`, "an account code must be specified"},
		"no lines on a live credit":           {`{"Type":"ACCRECCREDIT","Status":"SUBMITTED","Contact":{"ContactID":"` + cid + `"}}`, "at least one line item"},
		"a line amount that does not add up":  {`{"Type":"ACCRECCREDIT","Contact":{"ContactID":"` + cid + `"},"LineItems":[{"AccountCode":"200","Quantity":2,"UnitAmount":5,"LineAmount":11}]}`, "does not equal Quantity x UnitAmount"},
	}
	before := h.store.CreditNotes.For(store.DefaultTenantID).Count()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			code, body := putCreditNotes(t, h, "", tc.body)
			if code != 400 || body["Type"] != "ValidationException" || !strings.Contains(fmt.Sprint(body["Elements"]), tc.want) {
				t.Fatalf("status %d, body %v, want 400 ValidationException with %q", code, body, tc.want)
			}
		})
	}
	if h.store.CreditNotes.For(store.DefaultTenantID).Count() != before {
		t.Fatal("a refused request must create nothing")
	}
}

func TestCreditNoteSummarizeErrorsFalseCreatesTheValidOnesAndMarksTheRest(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	code, body := putCreditNotes(t, h, "?summarizeErrors=false", fmt.Sprintf(`{"CreditNotes":[
		{"Type":"ACCRECCREDIT","Contact":{"ContactID":%q},"Status":"AUTHORISED","LineItems":[{"AccountCode":"200","UnitAmount":10}]},
		{"Type":"ACCRECCREDIT","Contact":{"ContactID":%q},"Status":"AUTHORISED","LineItems":[{"AccountCode":"999","UnitAmount":10}]}]}`, cid, cid))

	list, _ := body["CreditNotes"].([]any)
	if code != 200 || len(list) != 2 {
		t.Fatalf("status %d, body %v: want 200 with both answered in place", code, body)
	}
	good, bad := list[0].(map[string]any), list[1].(map[string]any)
	errs, _ := bad["ValidationErrors"].([]any)
	if good["StatusAttributeString"] != "OK" || good["CreditNoteID"] == nil || bad["StatusAttributeString"] != "ERROR" || bad["HasErrors"] != true || len(errs) != 1 {
		t.Errorf("good = %v, bad = %v", good, bad)
	}
	if _, ok := bad["CreditNoteID"]; ok {
		t.Error("a refused credit note is answered as submitted, with no computed ID")
	}
	if n := h.store.CreditNotes.For(store.DefaultTenantID).Count(); n != 1 {
		t.Fatalf("%d credit notes stored, want only the valid one", n)
	}
}
