package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// seededInvoices creates four invoices a minute apart, over two contacts, so
// UpdatedDateUTC orders them: A1 and A2 for Acme, B1 and B2 for Bolt.
func seededInvoices(t *testing.T) (*Handler, map[string]string) {
	t.Helper()
	h, ids := newTestHandler(t), map[string]string{}
	acme, bolt := newContactID(t, h, "Acme"), newContactID(t, h, "Bolt")
	ids["acme"], ids["bolt"] = acme, bolt
	for _, c := range []struct{ num, contact, status, date, qty string }{
		{"A1", acme, "AUTHORISED", "2026-09-01", "1"}, {"A2", acme, "DRAFT", "2026-09-10", "5"},
		{"B1", bolt, "AUTHORISED", "2026-09-20", "2"}, {"B2", bolt, "SUBMITTED", "2026-09-30", "3"},
	} {
		code, body := putInvoices(t, h, "", fmt.Sprintf(`{"Type":"ACCREC","InvoiceNumber":%q,"Reference":"ref-%s","Contact":{"ContactID":%q},"Date":%q,"Status":%q,
			"LineItems":[{"Description":"x","Quantity":%s,"UnitAmount":50,"AccountCode":"200"}]}`, c.num, c.num, c.contact, c.date, c.status, c.qty))
		if code != 200 {
			t.Fatalf("seed %s: %d %v", c.num, code, body)
		}
		ids[c.num] = firstInvoice(t, body)["InvoiceID"].(string)
		h.store.Clock.Advance(time.Minute)
	}
	return h, ids
}

func getInvoices(t *testing.T, h *Handler, path string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodGet, "/api.xro/2.0/"+path, "", accounting(t, h, store.DefaultTenantID))
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func numbers(body map[string]any) string {
	var out []string
	for _, i := range body["Invoices"].([]any) {
		out = append(out, i.(map[string]any)["InvoiceNumber"].(string))
	}
	return strings.Join(out, ",")
}

func TestInvoiceListFiltersAndWhere(t *testing.T) {
	h, ids := seededInvoices(t)
	q := func(k, v string) string { return "Invoices?" + url.Values{k: {v}}.Encode() }
	cases := []struct{ name, path, want string }{
		{"all, oldest first", "Invoices", "A1,A2,B1,B2"},
		{"ContactIDs, as finance-ops calls it", q("ContactIDs", ids["acme"]), "A1,A2"},
		{"ContactIDs in any case", q("ContactIDs", strings.ToUpper(ids["bolt"])), "B1,B2"},
		{"InvoiceNumbers", q("InvoiceNumbers", "A2,B1"), "A2,B1"},
		{"IDs", q("IDs", ids["B2"]), "B2"},
		{"Statuses", q("Statuses", "AUTHORISED,SUBMITTED"), "A1,B1,B2"},
		{"where on a status", q("where", `Status=="AUTHORISED"`), "A1,B1"},
		{"where with a single =", q("where", `InvoiceNumber="B2"`), "B2"},
		{"where on a nested contact", q("where", fmt.Sprintf(`Contact.ContactID==Guid(%q)`, ids["acme"])), "A1,A2"},
		{"where on a date", q("where", `Date >= DateTime(2026,9,15)`), "B1,B2"},
		{"where with logic and a number", q("where", `Total > 100 && Type=="ACCREC"`), "A2,B2"},
		{"where null on an unset date", q("where", `FullyPaidOnDate == null`), "A1,A2,B1,B2"},
		{"searchTerm on number and reference", q("searchTerm", "REF-b"), "B1,B2"},
		{"filters narrow together", "Invoices?" + url.Values{"ContactIDs": {ids["acme"]}, "Statuses": {"DRAFT"}}.Encode(), "A2"},
		{"nothing matches", q("InvoiceNumbers", "nope"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := getInvoices(t, h, tc.path)
			if code != 200 || numbers(body) != tc.want {
				t.Fatalf("%s: status %d, invoices %q, want %q", tc.path, code, numbers(body), tc.want)
			}
		})
	}
	if code, body := getInvoices(t, h, q("where", `Status ==`)); code != 400 || body["Type"] != "ValidationException" {
		t.Fatalf("a malformed where must be a 400 ValidationException, got %d %v", code, body)
	}
}

func TestInvoiceListPaginationAndSummaryForm(t *testing.T) {
	h, _ := seededInvoices(t)
	_, all := getInvoices(t, h, "Invoices")
	if _, has := all["pagination"]; has {
		t.Fatalf("without page there must be no pagination key: %v", all["pagination"])
	}
	if lines := all["Invoices"].([]any)[0].(map[string]any)["LineItems"].([]any); len(lines) != 0 {
		t.Fatalf("without page invoices come without line items (page-002), got %v", lines)
	}
	_, paged := getInvoices(t, h, "Invoices?page=2&pageSize=3")
	pg := paged["pagination"].(map[string]any)
	if numbers(paged) != "B2" || pg["page"] != 2.0 || pg["pageCount"] != 2.0 || pg["itemCount"] != 4.0 {
		t.Fatalf("page 2 of 3 per page: %q %v", numbers(paged), pg)
	}
	if lines := paged["Invoices"].([]any)[0].(map[string]any)["LineItems"].([]any); len(lines) != 1 {
		t.Fatalf("with page invoices carry their line items, got %v", lines)
	}
	if _, body := getInvoices(t, h, "Invoices?page=1&summaryOnly=true"); len(body["Invoices"].([]any)[0].(map[string]any)["LineItems"].([]any)) != 0 {
		t.Fatal("summaryOnly=true must leave out line items even with a page")
	}
}

func TestInvoiceReadByIDOrNumber(t *testing.T) {
	h, ids := seededInvoices(t)
	for _, key := range []string{ids["B1"], strings.ToUpper(ids["B1"]), "B1"} {
		code, body := getInvoices(t, h, "Invoices/"+key)
		if code != 200 || numbers(body) != "B1" || len(body["Invoices"].([]any)) != 1 || body["Status"] != "OK" {
			t.Fatalf("read by %q: %d %v", key, code, body)
		}
		if lines := body["Invoices"].([]any)[0].(map[string]any)["LineItems"].([]any); len(lines) != 1 {
			t.Fatalf("a single read carries line items, got %v", lines)
		}
	}
	for _, key := range []string{"00000000-0000-4000-8000-00000000dead", "NOPE-1"} {
		if code, body := getInvoices(t, h, "Invoices/"+key); code != 404 || body["Status"] != 404.0 {
			t.Fatalf("read of unknown %q must be a 404, got %d %v", key, code, body)
		}
	}
	// A tenant sees only its own invoices.
	rec := do(h, http.MethodGet, "/api.xro/2.0/Invoices/B1", "", accounting(t, h, "0f0f0f0f-0000-4000-8000-00000000dead"))
	if rec.Code != 404 {
		t.Fatalf("another tenant read this tenant's invoice: %d", rec.Code)
	}
}

func TestInvoiceListAcceptsTheDeferredParameters(t *testing.T) {
	h, _ := seededInvoices(t)
	_, plain := getInvoices(t, h, "Invoices")
	for _, p := range []string{"order=Total DESC", "includeArchived=true", "createdByMyApp=true", "unitdp=4"} {
		if code, body := getInvoices(t, h, "Invoices?"+strings.ReplaceAll(p, " ", "%20")); code != 200 || numbers(body) != numbers(plain) {
			t.Errorf("%s: %d %q, want it accepted and ignored (GAP-002)", p, code, numbers(body))
		}
	}
}
