package api

import (
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func postContacts(t *testing.T, h *Handler, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPost, "/api.xro/2.0/Contacts", body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func contactCount(t *testing.T, h *Handler) int {
	t.Helper()
	_, body := listContacts(t, h, url.Values{})
	return len(body["Contacts"].([]any))
}

func TestPostCreatesAContactWithNoContactIDAndSendsAContactCreate(t *testing.T) {
	h := newTestHandler(t)
	got := subscribed(t, h)

	code, body := postContacts(t, h, `{"Contacts":[{"Name":"Acme","AccountNumber":"cus_1"}]}`)
	if code != 200 || body["Status"] != "OK" {
		t.Fatalf("%d %v", code, body)
	}
	id := firstContactID(t, body)
	if c := body["Contacts"].([]any)[0].(map[string]any); !guidPattern.MatchString(id) || c["ContactStatus"] != "ACTIVE" {
		t.Fatalf("contact = %v", c)
	}
	wantEvent(t, eventsOf(t, got, 1)[0], "CONTACT", "CREATE", id)
}

func TestPostUpdatesTheContactThatCarriesItsContactIDAndKeepsTheOtherFields(t *testing.T) {
	h := newTestHandler(t)
	id := firstContactID(t, putContacts(t, h, `{"Contacts":[{"Name":"Acme","AccountNumber":"cus_1","EmailAddress":"a@acme.test"}]}`))
	got := subscribed(t, h)

	code, body := postContacts(t, h, `{"ContactID":"`+id+`","Name":"Acme Ltd"}`) // a bare contact, as PUT takes
	c := body["Contacts"].([]any)[0].(map[string]any)
	if code != 200 || c["ContactID"] != id || c["Name"] != "Acme Ltd" || c["AccountNumber"] != "cus_1" || c["EmailAddress"] != "a@acme.test" {
		t.Fatalf("%d %v", code, body)
	}
	if n := contactCount(t, h); n != 1 {
		t.Fatalf("an update added a contact: %d", n)
	}
	wantEvent(t, eventsOf(t, got, 1)[0], "CONTACT", "UPDATE", id)
}

func TestPostNeverUpsertsByName(t *testing.T) {
	h := newTestHandler(t)
	putContacts(t, h, `{"Contacts":[{"Name":"Acme"}]}`)

	code, body := postContacts(t, h, `{"Contacts":[{"Name":"acme"}]}`)
	if code != 400 || body["Type"] != "ValidationException" || contactCount(t, h) != 1 {
		t.Fatalf("a repeated Name with no ContactID must be refused as PUT refuses it: %d %v", code, body)
	}
}

func TestPostRefusesARenameToAnotherContactsNameButAllowsKeepingItsOwn(t *testing.T) {
	h := newTestHandler(t)
	putContacts(t, h, `{"Contacts":[{"Name":"Acme"},{"Name":"Beta"}]}`)
	_, list := listContacts(t, h, url.Values{"where": {`Name="Beta"`}})
	beta := firstContactID(t, list)

	if code, _ := postContacts(t, h, `{"ContactID":"`+beta+`","Name":"Acme"}`); code != 400 {
		t.Errorf("rename onto another contact's name = %d, want 400", code)
	}
	if code, body := postContacts(t, h, `{"ContactID":"`+beta+`","Name":"Beta","EmailAddress":"b@beta.test"}`); code != 200 {
		t.Errorf("resending its own name = %d %v", code, body)
	}
}

func TestPostWithAnUnknownContactIDIsA404AndWritesNothing(t *testing.T) {
	h := newTestHandler(t)
	got := subscribed(t, h)

	code, _ := postContacts(t, h, `{"Contacts":[{"Name":"New"},{"ContactID":"00000000-0000-4000-8000-000000000009","Name":"Ghost"}]}`)
	if code != 404 || contactCount(t, h) != 0 {
		t.Fatalf("status %d, %d contacts", code, contactCount(t, h))
	}
	none(t, got)
}

func TestPostWithOneInvalidContactWritesNoneAndSendsNoEvent(t *testing.T) {
	h := newTestHandler(t)
	id := firstContactID(t, putContacts(t, h, `{"Contacts":[{"Name":"Acme"}]}`))
	got := subscribed(t, h)

	code, body := postContacts(t, h, `{"Contacts":[{"ContactID":"`+id+`","Name":"Acme 2"},{"Name":""},{"Name":"Same"},{"Name":"same"}]}`)
	if code != 400 || body["Type"] != "ValidationException" {
		t.Fatalf("%d %v", code, body)
	}
	if _, list := listContacts(t, h, url.Values{}); names(list)[0] != "Acme" || contactCount(t, h) != 1 {
		t.Fatalf("a refused request changed the store: %v", names(list))
	}
	none(t, got)
}

func TestPostRefusedForAnInvalidNameLeavesTheStoredAddressesAndPhonesUnchanged(t *testing.T) {
	h := newTestHandler(t)
	id := firstContactID(t, putContacts(t, h, `{"Contacts":[{"Name":"Acme","Addresses":[{"AddressType":"STREET","City":"Dublin"},{"AddressType":"POBOX","City":"Cork"}],"Phones":[{"PhoneType":"DEFAULT","PhoneNumber":"1234"}]}]}`))
	_, before := listContacts(t, h, url.Values{})

	code, _ := postContacts(t, h, `{"ContactID":"`+id+`","Name":"","Addresses":[{"AddressType":"STREET","City":"Galway"}],"Phones":[{"PhoneType":"MOBILE","PhoneNumber":"9999"}]}`)
	if code != 400 {
		t.Fatalf("status %d, want 400", code)
	}
	_, after := listContacts(t, h, url.Values{})
	if !reflect.DeepEqual(before["Contacts"], after["Contacts"]) {
		t.Fatalf("a refused request changed the stored contact:\nbefore %v\nafter  %v", before["Contacts"], after["Contacts"])
	}
}
