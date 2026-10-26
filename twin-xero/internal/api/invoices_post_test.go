package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func postInvoices(t *testing.T, h *Handler, query, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPost, "/api.xro/2.0/Invoices"+query, body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func newInvoiceBody(cid string) string {
	return fmt.Sprintf(`{"Type":"ACCREC","Status":"AUTHORISED","Contact":{"ContactID":%q},"Reference":"first",
		"LineItems":[{"Description":"Work","Quantity":1,"UnitAmount":100,"AccountCode":"200"}]}`, cid)
}

func TestPostCreatesAnInvoiceWithNoInvoiceIDThroughThePutRules(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	got := subscribed(t, h)

	code, body := postInvoices(t, h, "", `{"Invoices":[`+newInvoiceBody(cid)+`]}`)
	inv := firstInvoice(t, body)
	if code != 200 || inv["InvoiceNumber"] != "INV-0001" || inv["Total"] != 100.0 || inv["AmountDue"] != 100.0 {
		t.Fatalf("%d %v", code, body)
	}
	wantEvent(t, eventsOf(t, got, 1)[0], "INVOICE", "CREATE", inv["InvoiceID"].(string))
}

func TestPostUpdatesTheInvoiceThatCarriesItsInvoiceIDAsTheItemRouteDoes(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	_, made := putInvoices(t, h, "", newInvoiceBody(cid))
	id := firstInvoice(t, made)["InvoiceID"].(string)
	_, other := putInvoices(t, h, "", newInvoiceBody(cid))
	twin := firstInvoice(t, other)["InvoiceID"].(string)
	got := subscribed(t, h)

	code, body := postInvoices(t, h, "", fmt.Sprintf(`{"Invoices":[{"InvoiceID":%q,"Reference":"second","Status":"VOIDED"}]}`, id))
	inv := firstInvoice(t, body)
	if code != 200 || inv["InvoiceID"] != id || inv["Reference"] != "second" || inv["Status"] != "VOIDED" || inv["InvoiceNumber"] != "INV-0001" {
		t.Fatalf("%d %v", code, body)
	}
	_, viaItem := postInvoice(t, h, twin, "", `{"Reference":"second","Status":"VOIDED"}`)
	if want := firstInvoice(t, viaItem); inv["Status"] != want["Status"] || inv["AmountDue"] != want["AmountDue"] || inv["Total"] != want["Total"] {
		t.Errorf("collection POST gave %v, the item route gave %v", inv, want)
	}
	wantEvent(t, eventsOf(t, got, 2)[0], "INVOICE", "UPDATE", id)
}

func TestPostDoesNotMatchAnInvoiceByNumber(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	putInvoices(t, h, "", newInvoiceBody(cid))

	code, body := postInvoices(t, h, "", fmt.Sprintf(`{"Invoices":[{"InvoiceNumber":"INV-0001","Type":"ACCREC","Contact":{"ContactID":%q},
		"LineItems":[{"Description":"Work","Quantity":1,"UnitAmount":5,"AccountCode":"200"}]}]}`, cid))
	if code != 400 || body["Type"] != "ValidationException" {
		t.Fatalf("a repeated number with no InvoiceID must be refused as PUT refuses it: %d %v", code, body)
	}
	if _, list := getInvoices(t, h, "Invoices"); numbers(list) != "INV-0001" {
		t.Fatalf("a refused request changed the store: %s", numbers(list))
	}
}

func TestPostWithAnUnknownInvoiceIDIsA404AndWritesNothing(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	got := subscribed(t, h)

	code, _ := postInvoices(t, h, "", `{"Invoices":[`+newInvoiceBody(cid)+`,{"InvoiceID":"00000000-0000-4000-8000-000000000009","Reference":"x"}]}`)
	if _, list := getInvoices(t, h, "Invoices"); code != 404 || len(list["Invoices"].([]any)) != 0 {
		t.Fatalf("status %d, store %v", code, list)
	}
	none(t, h, got)
}

func TestPostWithSummarizeErrorsFalseWritesTheValidOnesAndAnswersTheRestInPlace(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	_, made := putInvoices(t, h, "", newInvoiceBody(cid))
	id := firstInvoice(t, made)["InvoiceID"].(string)
	got := subscribed(t, h)

	code, body := postInvoices(t, h, "?summarizeErrors=false", fmt.Sprintf(`{"Invoices":[
		{"InvoiceID":%q,"Reference":"changed"},
		{"Type":"ACCREC","Contact":{"ContactID":"no-such"},"LineItems":[]}]}`, id))
	list := body["Invoices"].([]any)
	if code != 200 || len(list) != 2 || list[0].(map[string]any)["Reference"] != "changed" || list[1].(map[string]any)["HasErrors"] != true {
		t.Fatalf("%d %v", code, body)
	}
	wantEvent(t, eventsOf(t, got, 1)[0], "INVOICE", "UPDATE", id)
	none(t, h, got)
}
