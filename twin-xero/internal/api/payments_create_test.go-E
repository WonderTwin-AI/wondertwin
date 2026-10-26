package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// authorisedInvoice creates an AUTHORISED sales invoice for 100 and returns its ID and number.
func authorisedInvoice(t *testing.T, h *Handler) (string, string) {
	t.Helper()
	cid := newContactID(t, h, "Payer")
	_, body := putInvoices(t, h, "", fmt.Sprintf(`{"Type":"ACCREC","Contact":{"ContactID":%q},"Date":"2026-09-01","Status":"AUTHORISED",
		"LineItems":[{"Description":"Work","Quantity":1,"UnitAmount":100,"AccountCode":"200"}]}`, cid))
	inv := firstInvoice(t, body)
	return inv["InvoiceID"].(string), inv["InvoiceNumber"].(string)
}

func postPayment(t *testing.T, h *Handler, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPost, "/api.xro/2.0/Payments", body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func TestPaymentCreateRecordsThePaymentInTheEnvelope(t *testing.T) {
	h := newTestHandler(t)
	id, _ := authorisedInvoice(t, h)
	// The shape localstack/finance-ops sends: the whole invoice nested, the account by code.
	code, body := postPayment(t, h, fmt.Sprintf(`{"Invoice":{"InvoiceID":%q,"Type":"ACCREC","Total":100},"Account":{"Code":"090"},
		"Date":"2026-09-15","Amount":100,"PaymentType":"ACCRECPAYMENT"}`, id))

	list, _ := body["Payments"].([]any)
	if code != 200 || body["Status"] != "OK" || len(list) != 1 {
		t.Fatalf("status %d, body %v", code, body)
	}
	p := list[0].(map[string]any)
	if pid, _ := p["PaymentID"].(string); !guidPattern.MatchString(pid) || p["Status"] != "AUTHORISED" || p["PaymentType"] != "ACCRECPAYMENT" || p["Amount"] != 100.0 {
		t.Fatalf("payment = %v", p)
	}
	if s, _ := p["Date"].(string); !msdateValue.MatchString(s) {
		t.Errorf("Date = %v, want /Date(ms+0000)/", p["Date"])
	}
	if a, _ := p["Account"].(map[string]any); a["Code"] != "090" || a["AccountID"] == nil {
		t.Errorf("Account = %v", p["Account"])
	}
	if inv, _ := p["Invoice"].(map[string]any); inv["InvoiceID"] != id {
		t.Errorf("Invoice = %v", p["Invoice"])
	}
	if n := h.store.Payments.For(store.DefaultTenantID).Count(); n != 1 {
		t.Errorf("%d payments recorded, want 1", n)
	}
}

func TestPaymentCreateFindsTheInvoiceAndAccountEveryWayItIsNamed(t *testing.T) {
	h := newTestHandler(t)
	id, number := authorisedInvoice(t, h)
	acct := h.store.Accounts.For(store.DefaultTenantID).List()[0]
	for name, body := range map[string]string{
		"invoice by number":   fmt.Sprintf(`{"Invoice":{"InvoiceNumber":%q},"Account":{"Code":"090"},"Date":"2026-09-15","Amount":10}`, number),
		"account by id":       fmt.Sprintf(`{"Invoice":{"InvoiceID":%q},"Account":{"AccountID":%q},"Date":"2026-09-15","Amount":10}`, id, acct.AccountID),
		"code beside account": fmt.Sprintf(`{"Invoice":{"InvoiceID":%q},"Code":"090","Date":"2026-09-15","Amount":10}`, id),
		"a Payments array":    fmt.Sprintf(`{"Payments":[{"Invoice":{"InvoiceID":%q},"Account":{"Code":"090"},"Date":"2026-09-15","Amount":10}]}`, id),
	} {
		if code, resp := postPayment(t, h, body); code != 200 {
			t.Errorf("%s: status %d, body %v", name, code, resp)
		}
	}
}

func TestPaymentCreateRefusesAndRecordsNothing(t *testing.T) {
	h := newTestHandler(t)
	id, _ := authorisedInvoice(t, h)
	good := fmt.Sprintf(`{"Invoice":{"InvoiceID":%q},"Account":{"Code":"090"},"Date":"2026-09-15","Amount":10}`, id)
	for name, body := range map[string]string{
		"no invoice":      `{"Account":{"Code":"090"},"Date":"2026-09-15","Amount":10}`,
		"unknown invoice": `{"Invoice":{"InvoiceID":"nope"},"Account":{"Code":"090"},"Date":"2026-09-15","Amount":10}`,
		"unknown account": fmt.Sprintf(`{"Invoice":{"InvoiceID":%q},"Account":{"Code":"999"},"Date":"2026-09-15","Amount":10}`, id),
		"no date":         fmt.Sprintf(`{"Invoice":{"InvoiceID":%q},"Account":{"Code":"090"},"Amount":10}`, id),
		"zero amount":     fmt.Sprintf(`{"Invoice":{"InvoiceID":%q},"Account":{"Code":"090"},"Date":"2026-09-15","Amount":0}`, id),
		"one bad in many": fmt.Sprintf(`{"Payments":[%s,{"Account":{"Code":"090"},"Date":"2026-09-15","Amount":5}]}`, good),
	} {
		code, resp := postPayment(t, h, body)
		if code != 400 || resp["Type"] != "ValidationException" {
			t.Errorf("%s: status %d, body %v, want a 400 ValidationException", name, code, resp)
		}
	}
	if n := h.store.Payments.For(store.DefaultTenantID).Count(); n != 0 {
		t.Errorf("%d payments recorded by refused requests, want 0", n)
	}
}
