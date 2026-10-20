package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// refusal PUTs contacts and returns the ValidationException's elements.
func refusal(t *testing.T, h *Handler, body string) []map[string]any {
	t.Helper()
	rec := do(h, http.MethodPut, "/api.xro/2.0/Contacts", body, accounting(t, h, store.DefaultTenantID))
	var e struct {
		ErrorNumber int
		Type        string
		Elements    []map[string]any
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || rec.Code != 400 || e.Type != "ValidationException" || e.ErrorNumber != 10 {
		t.Fatalf("want a 400 ValidationException, got %d %s", rec.Code, rec.Body)
	}
	return e.Elements
}

func messages(el map[string]any) string {
	var out []string
	for _, v := range el["ValidationErrors"].([]any) {
		out = append(out, v.(map[string]any)["Message"].(string))
	}
	return strings.Join(out, " | ")
}

func TestContactValidationRequiresAName(t *testing.T) {
	for _, body := range []string{`{"Contacts":[{"AccountNumber":"x"}]}`, `{"Contacts":[{"Name":"   "}]}`} {
		if got := messages(refusal(t, newTestHandler(t), body)[0]); !strings.Contains(got, "contact name") {
			t.Fatalf("%s: messages = %q", body, got)
		}
	}
}

func TestContactValidationKeepsFieldsToTheirLengths(t *testing.T) {
	h := newTestHandler(t)
	for field, max := range map[string]int{"Name": 255, "FirstName": 255, "LastName": 255, "EmailAddress": 255,
		"ContactNumber": 50, "AccountNumber": 50, "CompanyNumber": 50, "TaxNumber": 50} {
		over := `{"Name":"n","` + field + `":"` + strings.Repeat("x", max+1) + `"}`
		if field == "Name" {
			over = `{"Name":"` + strings.Repeat("x", max+1) + `"}`
		}
		if got := messages(refusal(t, h, over)[0]); !strings.Contains(got, field+" must be") {
			t.Fatalf("%s over %d: messages = %q", field, max, got)
		}
	}
	putContacts(t, h, `{"Name":"`+strings.Repeat("é", 255)+`"}`) // characters, not bytes
	if got := messages(refusal(t, h, `{"Name":"`+strings.Repeat("é", 256)+`"}`)[0]); !strings.Contains(got, "Name must be") {
		t.Fatalf("256 multi-byte characters: messages = %q", got)
	}
	putContacts(t, h, `{"Name":"`+strings.Repeat("y", 255)+`","AccountNumber":"`+strings.Repeat("9", 50)+`"}`) // at the limit is fine
}

func TestContactValidationNameIsUniqueAmongActiveContacts(t *testing.T) {
	h := newTestHandler(t)
	putContacts(t, h, `{"Contacts":[{"Name":"Acme"},{"Name":"Gone","ContactStatus":"ARCHIVED"}]}`)

	got := messages(refusal(t, h, `{"Name":"ACME"}`)[0])
	// finance-ops recognises a name collision by these words (xero_client.py create_customer).
	if !strings.Contains(got, "contact name") || !strings.Contains(got, "unique") {
		t.Fatalf("a name collision (case-insensitive) must say the contact name must be unique: %q", got)
	}
	putContacts(t, h, `{"Name":"Gone"}`) // an archived contact's name is free again
}

func TestContactValidationAccountNumberIsUnique(t *testing.T) {
	h := newTestHandler(t)
	putContacts(t, h, `{"Name":"First","AccountNumber":"cus_1"}`)

	got := messages(refusal(t, h, `{"Name":"Second","AccountNumber":"cus_1"}`)[0])
	// finance-ops recognises a lost race by these words (xero_client.py create_customer).
	if !strings.Contains(strings.ToLower(got), "account number") || !strings.Contains(got, "already exists") {
		t.Fatalf("messages = %q", got)
	}
}

func TestContactValidationRefusesTheWholeRequestAndEchoesEveryContact(t *testing.T) {
	h := newTestHandler(t)
	els := refusal(t, h, `{"Contacts":[{"Name":"Fine"},{"Name":"Twin"},{"Name":"twin"},{"AccountNumber":"a"}]}`)

	if len(els) != 4 || els[0]["Name"] != "Fine" || messages(els[0]) != "" {
		t.Fatalf("every submitted contact is echoed, a valid one with no messages: %v", els)
	}
	if !strings.Contains(messages(els[2]), "unique") || !strings.Contains(messages(els[3]), "contact name") || messages(els[1]) != "" {
		t.Fatalf("a duplicate within the request is refused, the first of the pair is not: %v", els)
	}
	if n := h.store.Contacts.For(store.DefaultTenantID).Count(); n != 0 {
		t.Fatalf("nothing is created when any contact is invalid, but %d were", n)
	}
}
