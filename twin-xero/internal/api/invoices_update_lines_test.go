package api

import (
	"fmt"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

const twoLines = `,"LineItems":[` +
	`{"Description":"A","Quantity":1,"UnitAmount":30,"AccountCode":"200"},` +
	`{"Description":"B","Quantity":2,"UnitAmount":20,"AccountCode":"201"}]`

func lineIDs(inv map[string]any) []string {
	var ids []string
	for _, l := range inv["LineItems"].([]any) {
		ids = append(ids, l.(map[string]any)["LineItemID"].(string))
	}
	return ids
}

func TestInvoiceUpdateLinesAreMatchedByIDAndTotalsRecomputed(t *testing.T) {
	h := newTestHandler(t)
	inv := newInvoice(t, h, "")
	_, body := putInvoices(t, h, "", fmt.Sprintf(`{"Invoices":[{"Type":"ACCREC","Contact":{"ContactID":%q},"Status":"AUTHORISED"%s}]}`,
		inv["Contact"].(map[string]any)["ContactID"], twoLines))
	two := firstInvoice(t, body)
	ids := lineIDs(two)
	if two["Total"] != 70.0 || len(ids) != 2 {
		t.Fatalf("setup: %v", two)
	}

	// Keep line A with a new quantity, drop B (left out), add a new line C.
	_, body = postInvoice(t, h, two["InvoiceID"].(string), "", fmt.Sprintf(`{"LineItems":[
		{"LineItemID":%q,"Description":"A","Quantity":3,"UnitAmount":30,"AccountCode":"200"},
		{"Description":"C","Quantity":1,"UnitAmount":5,"AccountCode":"200"}]}`, ids[0]))
	got := firstInvoice(t, body)
	gotIDs := lineIDs(got)
	if len(gotIDs) != 2 || gotIDs[0] != ids[0] || gotIDs[1] == ids[1] || !guidPattern.MatchString(gotIDs[1]) {
		t.Fatalf("A keeps its ID, B is gone, C is new: before %v, after %v", ids, gotIDs)
	}
	if got["SubTotal"] != 95.0 || got["Total"] != 95.0 || got["AmountDue"] != 95.0 {
		t.Errorf("totals not recomputed: %v %v %v", got["SubTotal"], got["Total"], got["AmountDue"])
	}
}

func TestInvoiceUpdateLinesWithoutIDsReplaceEveryLine(t *testing.T) {
	h := newTestHandler(t)
	inv := newInvoice(t, h, "")
	old := lineIDs(inv)
	_, body := postInvoice(t, h, inv["InvoiceID"].(string), "", `{"LineItems":[{"Description":"Consulting","Quantity":2,"UnitAmount":50,"AccountCode":"200"}]}`)
	if got := lineIDs(firstInvoice(t, body)); len(got) != 1 || got[0] == old[0] {
		t.Errorf("lines sent without IDs are deleted and recreated (crud-005): before %v, after %v", old, got)
	}
}

func TestInvoiceUpdateLineAmountTypesRecomputesTheTax(t *testing.T) {
	h := newTestHandler(t)
	inv := newInvoice(t, h, "")
	_, body := postInvoice(t, h, inv["InvoiceID"].(string), "", `{"LineItems":[{"Quantity":1,"UnitAmount":100,"TaxAmount":10,"AccountCode":"200"}]}`)
	if got := firstInvoice(t, body); got["TotalTax"] != 10.0 || got["Total"] != 110.0 {
		t.Fatalf("a supplied TaxAmount is kept: %v", got)
	}
	_, body = postInvoice(t, h, inv["InvoiceID"].(string), "", `{"LineAmountTypes":"NoTax"}`)
	if got := firstInvoice(t, body); got["TotalTax"] != 0.0 || got["Total"] != 100.0 || got["LineAmountTypes"] != "NoTax" {
		t.Errorf("NoTax carries no tax: %v", got)
	}
}

func TestInvoiceUpdateLineRefusalsAndAmountDueOfAPartlyPaidInvoice(t *testing.T) {
	h := newTestHandler(t)
	inv := newInvoice(t, h, "")
	id := inv["InvoiceID"].(string)
	for sent, want := range map[string]string{
		`{"LineItems":[{"Quantity":1,"UnitAmount":5,"AccountCode":"999"}]}`:                "not a valid code",
		`{"LineItems":[{"Quantity":1,"UnitAmount":5,"LineAmount":9,"AccountCode":"200"}]}`: "LineAmount does not equal",
		`{"LineAmountTypes":"Sideways"}`:                                                   "LineAmountTypes must be",
	} {
		if code, body := postInvoice(t, h, id, "", sent); code != 400 || !bodyMentions(body, want) {
			t.Errorf("%s: %d %v, want a refusal mentioning %q", sent, code, body, want)
		}
	}
	// A payment of 30 against the 100 invoice: a rename keeps the total, so 70 stays due.
	invoices := h.store.Invoices.For(store.DefaultTenantID)
	stored, _, _ := findInvoice(invoices.List(), id)
	stored.AmountPaid, stored.AmountDue = 30, 70
	invoices.Set(id, stored)
	_, body := postInvoice(t, h, id, "", `{"LineItems":[{"Description":"renamed","Quantity":2,"UnitAmount":50,"AccountCode":"200"}]}`)
	if got := firstInvoice(t, body); got["AmountDue"] != 70.0 || got["AmountPaid"] != 30.0 || got["Total"] != 100.0 {
		t.Errorf("AmountDue is Total less what is paid: %v", got)
	}
}
