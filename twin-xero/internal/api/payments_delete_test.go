package api

import (
	"net/http"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func deletePayment(t *testing.T, h *Handler, id, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPost, "/api.xro/2.0/Payments/"+id, body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func paymentIDOf(body map[string]any) string {
	return body["Payments"].([]any)[0].(map[string]any)["PaymentID"].(string)
}

func TestDeletingAPaymentRestoresTheInvoice(t *testing.T) {
	h := newTestHandler(t)
	id := makeInvoice(t, h, "ACCREC", "AUTHORISED")
	_, first := postPayment(t, h, pay(id, 40))
	_, second := postPayment(t, h, pay(id, 60))
	if inv := storedInvoice(h, id); inv.Status != "PAID" || inv.FullyPaidOnDate == nil {
		t.Fatalf("setup: %+v", inv)
	}

	code, body := deletePayment(t, h, paymentIDOf(second), `{"Status":"DELETED"}`)
	got := body["Payments"].([]any)[0].(map[string]any)
	if code != 200 || got["Status"] != "DELETED" || got["PaymentID"] != paymentIDOf(second) {
		t.Fatalf("status %d, body %v", code, body)
	}
	inv := storedInvoice(h, id)
	if inv.Status != "AUTHORISED" || inv.AmountPaid != 40 || inv.AmountDue != 60 || inv.FullyPaidOnDate != nil || len(inv.Payments) != 1 || inv.Payments[0]["PaymentID"] != paymentIDOf(first) {
		t.Errorf("deleting the payment that cleared the invoice must put it back as the first payment left it: %+v", inv)
	}
	if e, _ := got["Invoice"].(map[string]any); e["AmountDue"] != 60.0 || e["Status"] != "AUTHORISED" {
		t.Errorf("the answer must carry the restored invoice, got %v", e)
	}
	if p, _ := h.store.Payments.For(store.DefaultTenantID).Get(paymentIDOf(second)); p.Status != "DELETED" {
		t.Errorf("the payment stays, as DELETED: %+v", p)
	}

	if code, _ := postPayment(t, h, pay(id, 60)); code != 200 || storedInvoice(h, id).Status != "PAID" {
		t.Errorf("the freed amount can be paid again: %d %+v", code, storedInvoice(h, id))
	}
}

func TestDeletePaymentTakesTheEnvelopeAsWell(t *testing.T) {
	h := newTestHandler(t)
	id := makeInvoice(t, h, "ACCREC", "AUTHORISED")
	_, made := postPayment(t, h, pay(id, 100))
	if code, _ := deletePayment(t, h, paymentIDOf(made), `{"Payments":[{"Status":"DELETED"}]}`); code != 200 || storedInvoice(h, id).AmountDue != 100 {
		t.Errorf("status %d, invoice %+v", code, storedInvoice(h, id))
	}
}

func TestDeletePaymentRefusals(t *testing.T) {
	h := newTestHandler(t)
	id := makeInvoice(t, h, "ACCREC", "AUTHORISED")
	_, made := postPayment(t, h, pay(id, 30))
	pid := paymentIDOf(made)

	if code, _ := deletePayment(t, h, pid, `{"Status":"AUTHORISED"}`); code != 400 {
		t.Errorf("any other status is refused, got %d", code)
	}
	if code, _ := deletePayment(t, h, pid, `{`); code != 400 {
		t.Errorf("bad JSON is refused, got %d", code)
	}
	if code, _ := deletePayment(t, h, "no-such-payment", `{"Status":"DELETED"}`); code != 404 {
		t.Errorf("an unknown payment is a 404, got %d", code)
	}
	if storedInvoice(h, id).AmountPaid != 30 {
		t.Errorf("a refused delete changes nothing: %+v", storedInvoice(h, id))
	}

	if code, _ := deletePayment(t, h, pid, `{"Status":"DELETED"}`); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	code, _ := deletePayment(t, h, pid, `{"Status":"DELETED"}`)
	if code != 400 || storedInvoice(h, id).AmountPaid != 0 {
		t.Errorf("a second delete is refused and must not take the amount off twice: %d %+v", code, storedInvoice(h, id))
	}

	_, again := postPayment(t, h, pay(id, 30))
	payments := h.store.Payments.For(store.DefaultTenantID)
	p, _ := payments.Get(paymentIDOf(again))
	p.IsReconciled = true
	payments.Set(p.PaymentID, p)
	if code, _ := deletePayment(t, h, p.PaymentID, `{"Status":"DELETED"}`); code != 400 {
		t.Errorf("a reconciled payment cannot be deleted, got %d", code)
	}
}
