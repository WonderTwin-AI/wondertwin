package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func makeInvoice(t *testing.T, h *Handler, typ, status string) string {
	t.Helper()
	cid := newContactID(t, h, "Party "+h.store.NewID())
	_, body := putInvoices(t, h, "", fmt.Sprintf(`{"Type":%q,"Contact":{"ContactID":%q},"Date":"2026-09-01","Status":%q,
		"LineItems":[{"Description":"Work","Quantity":1,"UnitAmount":100,"AccountCode":"200"}]}`, typ, cid, status))
	return firstInvoice(t, body)["InvoiceID"].(string)
}

func pay(id string, amount float64) string {
	return fmt.Sprintf(`{"Invoice":{"InvoiceID":%q},"Account":{"Code":"090"},"Date":"2026-09-15","Amount":%v}`, id, amount)
}

func storedInvoice(h *Handler, id string) store.Invoice {
	inv, _ := h.store.Invoices.For(store.DefaultTenantID).Get(id)
	return inv
}

func TestPaymentAppliesToTheInvoiceUntilItIsPaid(t *testing.T) {
	h := newTestHandler(t)
	id := makeInvoice(t, h, "ACCREC", "AUTHORISED")

	_, body := postPayment(t, h, pay(id, 40))
	inv := storedInvoice(h, id)
	if inv.AmountPaid != 40 || inv.AmountDue != 60 || inv.Status != "AUTHORISED" || inv.FullyPaidOnDate != nil || len(inv.Payments) != 1 {
		t.Fatalf("after 40: %+v", inv)
	}
	if e, _ := body["Payments"].([]any)[0].(map[string]any)["Invoice"].(map[string]any); e["AmountDue"] != 60.0 || e["Status"] != "AUTHORISED" {
		t.Errorf("the answer must carry the invoice as the payment leaves it, got %v", e)
	}

	_, body = postPayment(t, h, pay(id, 60))
	inv = storedInvoice(h, id)
	if inv.Status != "PAID" || inv.AmountDue != 0 || inv.AmountPaid != 100 || inv.FullyPaidOnDate == nil || len(inv.Payments) != 2 {
		t.Fatalf("after 100: %+v", inv)
	}
	e, _ := body["Payments"].([]any)[0].(map[string]any)["Invoice"].(map[string]any)
	if s, _ := e["FullyPaidOnDate"].(string); e["Status"] != "PAID" || !msdateValue.MatchString(s) {
		t.Errorf("a paid invoice answers PAID with an msdate FullyPaidOnDate, got %v", e)
	}
}

func TestPaymentsInOneRequestApplyInOrderAndFailTogether(t *testing.T) {
	h := newTestHandler(t)
	id := makeInvoice(t, h, "ACCREC", "AUTHORISED")
	if code, _ := postPayment(t, h, fmt.Sprintf(`{"Payments":[%s,%s]}`, pay(id, 40), pay(id, 60))); code != 200 || storedInvoice(h, id).Status != "PAID" {
		t.Fatalf("two payments that together clear the invoice: status %d, invoice %+v", code, storedInvoice(h, id))
	}

	other := makeInvoice(t, h, "ACCREC", "AUTHORISED")
	code, _ := postPayment(t, h, fmt.Sprintf(`{"Payments":[%s,%s]}`, pay(other, 70), pay(other, 70)))
	if inv := storedInvoice(h, other); code != 400 || inv.AmountPaid != 0 || len(inv.Payments) != 0 {
		t.Fatalf("the second 70 overpays what the first left, so nothing applies: status %d, invoice %+v", code, inv)
	}
}

func TestPaymentRefusals(t *testing.T) {
	h := newTestHandler(t)
	authorised, draft, paid := makeInvoice(t, h, "ACCREC", "AUTHORISED"), makeInvoice(t, h, "ACCREC", "DRAFT"), makeInvoice(t, h, "ACCREC", "AUTHORISED")
	postPayment(t, h, pay(paid, 100))
	for name, tc := range map[string]struct{ body, want string }{
		"a draft invoice":   {pay(draft, 10), "not of valid status"},
		"a paid invoice":    {pay(paid, 10), "not of valid status"},
		"more than is due":  {pay(authorised, 100.01), "exceeds the amount outstanding"},
		"a revenue account": {fmt.Sprintf(`{"Invoice":{"InvoiceID":%q},"Account":{"Code":"200"},"Date":"2026-09-15","Amount":10}`, authorised), "not a bank account"},
		"a prepayment":      {fmt.Sprintf(`{"Prepayment":{"PrepaymentID":"x"},"Account":{"Code":"090"},"Date":"2026-09-15","Amount":10,"Invoice":{"InvoiceID":%q}}`, authorised), "Only payments against an invoice"},
	} {
		code, resp := postPayment(t, h, tc.body)
		if code != 400 || !strings.Contains(fmt.Sprint(resp), tc.want) {
			t.Errorf("%s: status %d, body %v, want a 400 saying %q", name, code, resp, tc.want)
		}
	}
	if inv := storedInvoice(h, authorised); inv.AmountPaid != 0 {
		t.Errorf("a refused payment changed the invoice: %+v", inv)
	}
}

func TestPaymentTypeFollowsTheInvoiceType(t *testing.T) {
	h := newTestHandler(t)
	for typ, want := range map[string]string{"ACCREC": "ACCRECPAYMENT", "ACCPAY": "ACCPAYPAYMENT"} {
		_, body := postPayment(t, h, pay(makeInvoice(t, h, typ, "AUTHORISED"), 10))
		if got := body["Payments"].([]any)[0].(map[string]any)["PaymentType"]; got != want {
			t.Errorf("%s: PaymentType = %v, want %s", typ, got, want)
		}
	}
}
