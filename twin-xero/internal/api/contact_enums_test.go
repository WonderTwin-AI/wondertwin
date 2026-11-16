package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// A value outside a Contact enum makes xero-python raise ValueError on every
// later read of the contact list (or of the paged list, which carries payment
// terms, groups and branding theme), so PUT and POST /Contacts refuse it.

func TestContactWritesRefuseOffSpecEnumValues(t *testing.T) {
	cases := []struct{ field, value, message string }{
		{"ContactStatus", `"BOGUS"`, "Contact Status 'BOGUS' is not valid."},
		{"TaxNumberType", `"BOGUS"`, "Tax Number Type 'BOGUS' is not valid."},
		{"SalesDefaultLineAmountType", `"Exclusive"`, "Sales Default Line Amount Type 'Exclusive' is not valid."},
		{"PurchasesDefaultLineAmountType", `"BOGUS"`, "Purchases Default Line Amount Type 'BOGUS' is not valid."},
		{"DefaultCurrency", `"ZZZ"`, "Default Currency 'ZZZ' is not valid."},
		{"Addresses", `[{"AddressType":"BOGUS","City":"Zurich"}]`, "Address Type 'BOGUS' is not valid."},
		{"Phones", `[{"PhoneType":"BOGUS","PhoneNumber":"1"}]`, "Phone Type 'BOGUS' is not valid."},
		{"ContactGroups", `[{"Name":"VIP","Status":"BOGUS"}]`, "Contact Group Status 'BOGUS' is not valid."},
		{"BrandingTheme", `{"Name":"Standard","Type":"BOGUS"}`, "Branding Theme Type 'BOGUS' is not valid."},
		{"PaymentTerms", `{"Bills":{"Day":5,"Type":"BOGUS"}}`, "Payment Terms Bills Type 'BOGUS' is not valid."},
		{"PaymentTerms", `{"Sales":{"Day":5}}`, "Payment Terms Sales Type '' is not valid."},
	}
	for _, c := range cases {
		t.Run(c.field+" "+c.value, func(t *testing.T) {
			h := newTestHandler(t)
			body := fmt.Sprintf(`{"Name":"Acme","%s":%s}`, c.field, c.value)
			refused := func(what string, code int, got map[string]any) {
				t.Helper()
				if code != 400 || got["Type"] != "ValidationException" || !strings.Contains(rawString(got), c.message) {
					t.Fatalf("%s: want a 400 ValidationException with %q, got %d %v", what, c.message, code, got)
				}
			}
			rec := do(h, http.MethodPut, "/api.xro/2.0/Contacts", body, accounting(t, h, store.DefaultTenantID))
			refused("PUT /Contacts", rec.Code, decode(t, rec))
			code, got := postContacts(t, h, body)
			refused("POST /Contacts (create)", code, got)
			if n := len(h.store.Contacts.For(store.DefaultTenantID).List()); n != 0 {
				t.Fatalf("a refused contact must not be stored: %d stored", n)
			}
			id := newContactID(t, h, "Acme")
			code, got = postContacts(t, h, fmt.Sprintf(`{"Contacts":[{"ContactID":%q,"%s":%s}]}`, id, c.field, c.value))
			refused("POST /Contacts (update)", code, got)
		})
	}
}

func TestContactWritesKeepSpecEnumValues(t *testing.T) {
	h := newTestHandler(t)
	got := putContacts(t, h, `{"Name":"Acme","ContactStatus":"ACTIVE","TaxNumberType":"EIN","SalesDefaultLineAmountType":"EXCLUSIVE",
		"PurchasesDefaultLineAmountType":"NONE","DefaultCurrency":"EUR","Addresses":[{"AddressType":"POBOX"},{"City":"Bern"}],
		"Phones":[{"PhoneType":"MOBILE"}],"ContactGroups":[{"Name":"VIP","Status":"ACTIVE"}],"BrandingTheme":{"Name":"Standard","Type":"INVOICE"},
		"PaymentTerms":{"Bills":{"Day":5,"Type":"OFCURRENTMONTH"},"Sales":{"Day":30,"Type":"DAYSAFTERBILLDATE"}}}`)
	c := got["Contacts"].([]any)[0].(map[string]any)
	if c["TaxNumberType"] != "EIN" || c["DefaultCurrency"] != "EUR" || c["SalesDefaultLineAmountType"] != "EXCLUSIVE" {
		t.Errorf("spec values must be stored as sent: %v", c)
	}
}
