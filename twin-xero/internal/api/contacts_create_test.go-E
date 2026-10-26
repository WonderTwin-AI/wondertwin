package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func do(h *Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func putContacts(t *testing.T, h *Handler, body string) map[string]any {
	t.Helper()
	rec := do(h, http.MethodPut, "/api.xro/2.0/Contacts", body, accounting(t, h, store.DefaultTenantID))
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	return decode(t, rec)
}

var guidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestContactCreateAnswersThePluralEnvelopeWithTheFieldsXeroAssigns(t *testing.T) {
	h := newTestHandler(t)
	body := putContacts(t, h, `{"Contacts":[{"Name":"Acme","AccountNumber":"cus_1","EmailAddress":"a@acme.test","IsCustomer":true,
		"Addresses":[{"AddressType":"STREET","City":"Zurich"}],"Phones":[{"PhoneType":"DEFAULT","PhoneNumber":"555"}]}]}`)

	if body["Status"] != "OK" {
		t.Fatalf("body = %v", body)
	}
	list, _ := body["Contacts"].([]any)
	if len(list) != 1 {
		t.Fatalf("want one contact, got %v", body["Contacts"])
	}
	c := list[0].(map[string]any)
	if id, _ := c["ContactID"].(string); !guidPattern.MatchString(id) || c["ContactStatus"] != "ACTIVE" || c["Name"] != "Acme" {
		t.Fatalf("contact = %v", c)
	}
	if s, _ := c["UpdatedDateUTC"].(string); !msdateValue.MatchString(s) {
		t.Fatalf("UpdatedDateUTC = %v, want /Date(ms+0000)/ (time-001)", c["UpdatedDateUTC"])
	}
	for _, k := range []string{"ContactPersons", "ContactGroups", "SalesTrackingCategories", "PurchasesTrackingCategories"} {
		if l, ok := c[k].([]any); !ok || l == nil {
			t.Fatalf("%s must be an array, never null: %v", k, c[k])
		}
	}
	for _, k := range []string{"BankAccountDetails", "TaxNumber", "MergedToContactID", "Balances"} {
		if _, ok := c[k]; ok {
			t.Fatalf("%s is unset and must be omitted, not null", k)
		}
	}
	if got := h.store.Contacts.For(store.DefaultTenantID).Count(); got != 1 {
		t.Fatalf("the tenant holds %d contacts, want 1", got)
	}
}

func TestContactCreateTakesABareContactOrAnArrayAndAssignsItsOwnID(t *testing.T) {
	h := newTestHandler(t)
	one := putContacts(t, h, `{"Name":"Solo","ContactID":"11111111-1111-4111-8111-111111111111"}`)["Contacts"].([]any)
	if len(one) != 1 || one[0].(map[string]any)["ContactID"] == "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("a bare contact is created, and its ContactID is assigned here, not taken: %v", one)
	}
	two := putContacts(t, h, `{"Contacts":[{"Name":"A"},{"Name":"B","ContactStatus":"ARCHIVED"}]}`)["Contacts"].([]any)
	if len(two) != 2 || two[1].(map[string]any)["ContactStatus"] != "ARCHIVED" {
		t.Fatalf("an array creates each contact and keeps a status it names: %v", two)
	}
	// A client that reads a contact and sends it back sends UpdatedDateUTC as /Date()/.
	putContacts(t, h, `{"Name":"RoundTrip","UpdatedDateUTC":"/Date(1792143000000+0000)/"}`)
}

func TestContactCreateKeepsTenantsApart(t *testing.T) {
	h := newTestHandler(t)
	putContacts(t, h, `{"Name":"Mine"}`)
	if n := h.store.Contacts.For("0f0f0f0f-0000-4000-8000-00000000dead").Count(); n != 0 {
		t.Fatalf("another tenant holds %d contacts, want 0", n)
	}
}

func TestContactCreateRefusesBodiesThatAreNotJSON(t *testing.T) {
	h := newTestHandler(t)
	for _, body := range []string{`{not json`, ``, `[1,2]`} {
		rec := do(h, http.MethodPut, "/api.xro/2.0/Contacts", body, accounting(t, h, store.DefaultTenantID))
		if rec.Code != 400 || !json.Valid(rec.Body.Bytes()) {
			t.Fatalf("%q: want a 400 JSON body, got %d %s", body, rec.Code, rec.Body)
		}
	}
}
