package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func putInvoices(t *testing.T, h *Handler, query, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPut, "/api.xro/2.0/Invoices"+query, body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func firstInvoice(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	list, _ := body["Invoices"].([]any)
	if len(list) == 0 {
		t.Fatalf("no invoices in %v", body)
	}
	return list[0].(map[string]any)
}

func TestInvoiceCreateAnswersTheEnvelopeWithComputedFields(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	code, body := putInvoices(t, h, "", fmt.Sprintf(`{"Invoices":[{"Type":"ACCREC","Contact":{"ContactID":%q},"Date":"2026-09-01","DueDate":"2026-09-30",
		"Status":"AUTHORISED","LineItems":[{"Description":"Consulting","Quantity":2,"UnitAmount":50,"AccountCode":"200"}]}]}`, cid))

	inv := firstInvoice(t, body)
	if code != 200 || body["Status"] != "OK" {
		t.Fatalf("status %d, body %v", code, body)
	}
	if id, _ := inv["InvoiceID"].(string); !guidPattern.MatchString(id) || inv["InvoiceNumber"] != "INV-0001" || inv["Status"] != "AUTHORISED" || inv["CurrencyCode"] != "USD" {
		t.Fatalf("invoice = %v", inv)
	}
	for k, want := range map[string]float64{"SubTotal": 100, "TotalTax": 0, "Total": 100, "AmountDue": 100, "AmountPaid": 0, "AmountCredited": 0} {
		if inv[k] != want {
			t.Errorf("%s = %v, want %v", k, inv[k], want)
		}
	}
	for _, k := range []string{"Date", "DueDate", "UpdatedDateUTC"} {
		if s, _ := inv[k].(string); !msdateValue.MatchString(s) {
			t.Errorf("%s = %v, want /Date(ms+0000)/ (time-001)", k, inv[k])
		}
	}
	if c, _ := inv["Contact"].(map[string]any); c["Name"] != "Acme" || c["ContactID"] != cid {
		t.Errorf("Contact = %v, want the full contact", inv["Contact"])
	}
	line := inv["LineItems"].([]any)[0].(map[string]any)
	if id, _ := line["LineItemID"].(string); !guidPattern.MatchString(id) || line["LineAmount"] != 100.0 || line["AccountID"] == nil {
		t.Errorf("line = %v", line)
	}
	for _, k := range []string{"Payments", "Prepayments", "Overpayments", "CreditNotes", "Attachments"} {
		if l, ok := inv[k].([]any); !ok || l == nil {
			t.Errorf("%s must be [], got %v", k, inv[k])
		}
	}
}

func TestInvoiceCreateFillsInDefaultsAndNumbers(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	one := fmt.Sprintf(`{"Type":"ACCREC","Contact":{"ContactID":%q}}`, cid)
	_, a := putInvoices(t, h, "", one)
	_, b := putInvoices(t, h, "", one)
	_, bill := putInvoices(t, h, "", fmt.Sprintf(`{"Type":"ACCPAY","Contact":{"ContactID":%q}}`, cid))

	first := firstInvoice(t, a)
	if first["Status"] != "DRAFT" || first["LineAmountTypes"] != "Exclusive" || first["Date"] != first["DueDate"] {
		t.Errorf("defaults = %v", first)
	}
	if firstInvoice(t, b)["InvoiceNumber"] != "INV-0002" {
		t.Errorf("the second invoice number must follow the first: %v", firstInvoice(t, b)["InvoiceNumber"])
	}
	if n, ok := firstInvoice(t, bill)["InvoiceNumber"]; ok {
		t.Errorf("a bill gets no generated number, got %v", n)
	}
}

func TestInvoiceCreateIgnoresWhatXeroComputes(t *testing.T) {
	// localstack/finance-ops sends Total, AmountPaid and FullyPaidOnDate on create.
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	_, body := putInvoices(t, h, "", fmt.Sprintf(`{"Type":"ACCREC","Contact":{"ContactID":%q},"Status":"AUTHORISED","SentToContact":true,
		"InvoiceID":"11111111-1111-4111-8111-111111111111","Total":999,"AmountPaid":50,"AmountDue":1,"FullyPaidOnDate":"2026-09-02",
		"LineItems":[{"AccountCode":"201","TaxType":"TAX019","LineAmount":40}]}`, cid))

	inv := firstInvoice(t, body)
	if inv["InvoiceID"] == "11111111-1111-4111-8111-111111111111" || inv["Total"] != 40.0 || inv["AmountDue"] != 40.0 || inv["AmountPaid"] != 0.0 {
		t.Fatalf("computed fields must be ignored: %v", inv)
	}
	if _, ok := inv["FullyPaidOnDate"]; ok || inv["SentToContact"] != true {
		t.Errorf("FullyPaidOnDate must be dropped and an authorised invoice may be marked sent: %v", inv)
	}
	_, draft := putInvoices(t, h, "", fmt.Sprintf(`{"Type":"ACCREC","Contact":{"ContactID":%q},"SentToContact":true}`, cid))
	if firstInvoice(t, draft)["SentToContact"] != false {
		t.Error("only an authorised invoice can be marked sent (null-001)")
	}
}

func TestInvoiceCreateUnitdpKeepsFourDecimals(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	body := fmt.Sprintf(`{"Type":"ACCREC","Contact":{"ContactID":%q},"LineItems":[{"Quantity":1,"UnitAmount":0.12345}]}`, cid)
	for query, want := range map[string]float64{"": 0.12, "?unitdp=4": 0.1235} {
		_, got := putInvoices(t, h, query, body)
		if u := firstInvoice(t, got)["LineItems"].([]any)[0].(map[string]any)["UnitAmount"]; u != want {
			t.Errorf("unitdp %q: UnitAmount = %v, want %v", query, u, want)
		}
	}
}
