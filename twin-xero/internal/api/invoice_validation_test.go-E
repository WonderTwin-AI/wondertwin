package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func TestInvoiceValidationRefusesWhatXeroRefuses(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	ok := fmt.Sprintf(`"Type":"ACCREC","Contact":{"ContactID":%q},"Status":"AUTHORISED","InvoiceNumber":"INV-9"`, cid)
	line := `"LineItems":[{"AccountCode":"200","UnitAmount":10}]`
	if code, _ := putInvoices(t, h, "", `{`+ok+`,`+line+`}`); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	cases := map[string]struct{ body, want string }{
		"a type that is not ACCREC or ACCPAY": {`{"Type":"ACCRECCREDIT","Contact":{"ContactID":"` + cid + `"}}`, "Type must be ACCREC or ACCPAY"},
		"a status only a later write can set": {`{"Type":"ACCREC","Status":"PAID","Contact":{"ContactID":"` + cid + `"}}`, "cannot be created with the status PAID"},
		"no contact":                          {`{"Type":"ACCREC"}`, "Contact must be specified"},
		"an unknown contact":                  {`{"Type":"ACCREC","Contact":{"ContactID":"00000000-0000-4000-8000-0000000000ff"}}`, "could not be found"},
		"a duplicate invoice number":          {`{` + ok + `,` + line + `}`, "Invoice # must be unique"},
		"an unknown account code":             {`{"Type":"ACCREC","Contact":{"ContactID":"` + cid + `"},"LineItems":[{"AccountCode":"999","UnitAmount":1}]}`, "account code '999' is not a valid code"},
		"no account code on a live invoice":   {`{"Type":"ACCREC","Status":"AUTHORISED","Contact":{"ContactID":"` + cid + `"},"LineItems":[{"UnitAmount":1}]}`, "an account code must be specified"},
		"no lines on a live invoice":          {`{"Type":"ACCREC","Status":"SUBMITTED","Contact":{"ContactID":"` + cid + `"}}`, "at least one line item"},
		"a line amount that does not add up":  {`{"Type":"ACCREC","Contact":{"ContactID":"` + cid + `"},"LineItems":[{"AccountCode":"200","Quantity":2,"UnitAmount":5,"LineAmount":11}]}`, "does not equal Quantity x UnitAmount"},
	}
	before := h.store.Invoices.For(store.DefaultTenantID).Count()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			code, body := putInvoices(t, h, "", tc.body)
			if code != 400 || body["Type"] != "ValidationException" || !strings.Contains(fmt.Sprint(body["Elements"]), tc.want) {
				t.Fatalf("status %d, body %v, want 400 ValidationException with %q", code, body, tc.want)
			}
		})
	}
	if h.store.Invoices.For(store.DefaultTenantID).Count() != before {
		t.Fatal("a refused request must create nothing")
	}
}
