package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// seededCreditNotes creates three credit notes a minute apart so UpdatedDateUTC
// orders them: C1 and C2 for Acme, C3 for Bolt.
func seededCreditNotes(t *testing.T) (*Handler, map[string]string) {
	t.Helper()
	h, ids := newTestHandler(t), map[string]string{}
	acme, bolt := newContactID(t, h, "Acme"), newContactID(t, h, "Bolt")
	ids["acme"], ids["bolt"] = acme, bolt
	for _, c := range []struct {
		num, contact, status string
		amount               int
	}{
		{"C1", acme, "AUTHORISED", 10}, {"C2", acme, "DRAFT", 30}, {"C3", bolt, "AUTHORISED", 50},
	} {
		code, body := putCreditNotes(t, h, "", fmt.Sprintf(`{"Type":"ACCRECCREDIT","CreditNoteNumber":%q,"Contact":{"ContactID":%q},"Status":%q,
			"LineItems":[{"Description":"x","Quantity":1,"UnitAmount":%d,"AccountCode":"200"}]}`, c.num, c.contact, c.status, c.amount))
		if code != 200 {
			t.Fatalf("seed %s: %d %v", c.num, code, body)
		}
		h.store.Clock.Advance(time.Minute)
	}
	return h, ids
}

func getCreditNotes(t *testing.T, h *Handler, query string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodGet, "/api.xro/2.0/CreditNotes"+query, "", accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func creditNoteNumbers(body map[string]any) string {
	var out []string
	for _, c := range body["CreditNotes"].([]any) {
		out = append(out, c.(map[string]any)["CreditNoteNumber"].(string))
	}
	return strings.Join(out, ",")
}

func TestCreditNoteListWhereAndOrder(t *testing.T) {
	h, ids := seededCreditNotes(t)
	q := func(where string) string { return "?" + url.Values{"where": {where}}.Encode() }
	for _, tc := range []struct{ name, query, want string }{
		{"all, oldest first", "", "C1,C2,C3"},
		{"Status", q(`Status=="AUTHORISED"`), "C1,C3"},
		{"Contact.ContactID", q(`Contact.ContactID==Guid("` + ids["bolt"] + `")`), "C3"},
		{"a number comparison", q(`Total>20`), "C2,C3"},
		{"and, as finance-ops-style filters combine", q(`Type=="ACCRECCREDIT" AND Status=="DRAFT"`), "C2"},
		{"nothing matches", q(`Status=="VOIDED"`), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := getCreditNotes(t, h, tc.query)
			if code != 200 || creditNoteNumbers(body) != tc.want {
				t.Fatalf("status %d, got %q, want %q (%v)", code, creditNoteNumbers(body), tc.want, body)
			}
		})
	}
	if code, body := getCreditNotes(t, h, q(`Status==`)); code != 400 || body["Type"] != "ValidationException" {
		t.Errorf("a bad where: status %d, body %v", code, body)
	}
}

func TestCreditNoteListPagesInFullAndSummarisesWithoutPage(t *testing.T) {
	h, _ := seededCreditNotes(t)

	_, body := getCreditNotes(t, h, "?page=1")
	pg, _ := body["pagination"].(map[string]any)
	first := body["CreditNotes"].([]any)[0].(map[string]any)
	if pg["page"] != 1.0 || pg["pageSize"] != 100.0 || pg["itemCount"] != 3.0 || pg["pageCount"] != 1.0 {
		t.Errorf("pagination = %v", pg)
	}
	if lines, _ := first["LineItems"].([]any); len(lines) != 1 {
		t.Errorf("a page carries the line items: %v", first["LineItems"])
	}

	_, body = getCreditNotes(t, h, "")
	if _, ok := body["pagination"]; ok {
		t.Error("a list without page has no pagination object")
	}
	if lines, _ := body["CreditNotes"].([]any)[0].(map[string]any)["LineItems"].([]any); len(lines) != 0 {
		t.Errorf("a list without page is the summary form: %v", lines)
	}

	_, body = getCreditNotes(t, h, "?page=2&pageSize=2")
	if creditNoteNumbers(body) != "C3" {
		t.Errorf("page 2 of size 2 = %q, want C3", creditNoteNumbers(body))
	}
}
