package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func payCreditNote(id string, amount float64) string {
	return fmt.Sprintf(`{"CreditNote":{"CreditNoteID":%q},"Account":{"Code":"090"},"Date":"2026-09-15","Amount":%v,"PaymentType":"ARCREDITPAYMENT"}`, id, amount)
}

// creditNoteFor stores an AUTHORISED ACCRECCREDIT credit note of 20 for a contact, allocating
// the whole of it to a PAID invoice when allocate is set, as finance-ops does for a refund.
func creditNoteFor(t *testing.T, h *Handler, allocate bool) string {
	t.Helper()
	cid := newContactID(t, h, "Acme "+h.store.NewID())
	allocations := ""
	if allocate {
		allocations = allocJSON(paidInvoice(t, h, cid), 20)
	}
	code, body := putCreditNotes(t, h, "", creditNoteWithAllocations(cid, 20, allocations))
	if code != 200 {
		t.Fatalf("credit note setup: %d %v", code, body)
	}
	return firstCreditNote(t, body)["CreditNoteID"].(string)
}

func storedCreditNote(h *Handler, id string) store.CreditNote {
	cn, _ := h.store.CreditNotes.For(store.DefaultTenantID).Get(id)
	return cn
}

func TestCreditNotePaymentOfAFullyAllocatedRefundIsAccepted(t *testing.T) {
	// The finance-ops refund: the credit note allocated in full, then paid for its whole total.
	h := newTestHandler(t)
	id := creditNoteFor(t, h, true)

	code, body := postPayment(t, h, payCreditNote(id, 20))
	p := body["Payments"].([]any)[0].(map[string]any)
	if code != 200 || p["PaymentType"] != "ARCREDITPAYMENT" || p["Amount"] != 20.0 || p["Invoice"] != nil {
		t.Fatalf("status %d, payment %v", code, p)
	}
	if cn, _ := p["CreditNote"].(map[string]any); cn["CreditNoteID"] != id || cn["RemainingCredit"] != 0.0 || cn["Status"] != "PAID" {
		t.Errorf("CreditNote = %v", p["CreditNote"])
	}
	if cn := storedCreditNote(h, id); len(cn.Payments) != 1 || cn.FullyPaidOnDate == nil {
		t.Errorf("credit note = %d payments, FullyPaidOnDate %v", len(cn.Payments), cn.FullyPaidOnDate)
	}
}

func TestCreditNotePaymentsReduceTheRemainingCreditUntilPaid(t *testing.T) {
	h := newTestHandler(t)
	id := creditNoteFor(t, h, false)

	postPayment(t, h, payCreditNote(id, 5))
	if cn := storedCreditNote(h, id); cn.RemainingCredit != 15 || cn.Status != "AUTHORISED" {
		t.Fatalf("after 5: remaining %v, status %s", cn.RemainingCredit, cn.Status)
	}
	if code, _ := postPayment(t, h, `{"Payments":[`+payCreditNote(id, 10)+`,`+payCreditNote(id, 5)+`]}`); code != 200 {
		t.Fatalf("the rest in one request: %d", code)
	}
	if cn := storedCreditNote(h, id); cn.RemainingCredit != 0 || cn.Status != "PAID" || len(cn.Payments) != 3 {
		t.Errorf("after 20: remaining %v, status %s, %d payments", cn.RemainingCredit, cn.Status, len(cn.Payments))
	}
}

func TestCreditNotePaymentRefusals(t *testing.T) {
	h := newTestHandler(t)
	id := creditNoteFor(t, h, false)
	draft := firstCreditNote(t, func() map[string]any {
		_, b := putCreditNotes(t, h, "", strings.Replace(creditNoteWithAllocations(newContactID(t, h, "D"), 20, ""), "AUTHORISED", "DRAFT", 1))
		return b
	}())["CreditNoteID"].(string)
	inv := refundableInvoice(t, h, newContactID(t, h, "I"))
	for name, tc := range map[string]struct{ body, want string }{
		"an unknown credit note":             {payCreditNote("00000000-0000-4000-8000-0000000000ff", 5), "Credit note could not be found"},
		"a draft credit note":                {payCreditNote(draft, 5), "not of valid status"},
		"more than the total":                {payCreditNote(id, 21), "exceeds the credit remaining"},
		"an amount of zero":                  {payCreditNote(id, 0), "greater than zero"},
		"no account":                         {strings.Replace(payCreditNote(id, 5), `"Code":"090"`, `"Code":"999"`, 1), "Account could not be found"},
		"an invoice and a credit note":       {strings.Replace(payCreditNote(id, 5), `{"CreditNote"`, fmt.Sprintf(`{"Invoice":{"InvoiceID":%q},"CreditNote"`, inv), 1), "not both"},
		"a credit note with no ID or number": {`{"CreditNote":{},"Account":{"Code":"090"},"Date":"2026-09-15","Amount":5}`, "CreditNote must be specified"},
	} {
		t.Run(name, func(t *testing.T) {
			code, body := postPayment(t, h, tc.body)
			if code != 400 || !strings.Contains(fmt.Sprint(body["Elements"]), tc.want) {
				t.Fatalf("status %d, body %v, want 400 with %q", code, body, tc.want)
			}
		})
	}
	if cn := storedCreditNote(h, id); len(cn.Payments) != 0 || cn.RemainingCredit != 20 {
		t.Errorf("a refused payment changed the credit note: %+v", cn)
	}
}

func TestCreditNotePaymentFindsTheCreditNoteByNumber(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	putCreditNotes(t, h, "", strings.Replace(creditNoteWithAllocations(cid, 20, ""), `"Type"`, `"CreditNoteNumber":"Stripe refund re_9","Type"`, 1))
	code, _ := postPayment(t, h, `{"CreditNote":{"CreditNoteNumber":"Stripe refund re_9"},"Account":{"Code":"090"},"Date":"2026-09-15","Amount":20}`)
	if cn := h.store.CreditNotes.For(store.DefaultTenantID).List()[0]; code != 200 || cn.Status != "PAID" {
		t.Fatalf("status %d, credit note %s", code, cn.Status)
	}
}

func TestCreditNotePaymentStrictNeedsAnAuthorisedCreditNoteWithCreditLeft(t *testing.T) {
	h := newTestHandler(t)
	h.store.StrictAuth.Store(true)
	cid := newContactID(t, h, "Acme")
	_, body := putCreditNotes(t, h, "", creditNoteWithAllocations(cid, 20, allocJSON(refundableInvoice(t, h, cid), 20)))
	allocated := firstCreditNote(t, body)["CreditNoteID"].(string)
	open := creditNoteFor(t, h, false)

	if code, body := postPayment(t, h, payCreditNote(allocated, 20)); code != 400 || !strings.Contains(fmt.Sprint(body["Elements"]), "not of valid status") {
		t.Errorf("a PAID credit note: status %d, body %v", code, body)
	}
	postPayment(t, h, payCreditNote(open, 5))
	if code, _ := postPayment(t, h, payCreditNote(open, 16)); code != 400 {
		t.Errorf("more than the RemainingCredit of 15: status %d", code)
	}
	if code, _ := postPayment(t, h, payCreditNote(open, 15)); code != 200 {
		t.Errorf("exactly the RemainingCredit: status %d", code)
	}
}

func TestDeletingACreditNotePaymentRestoresTheRemainingCredit(t *testing.T) {
	h := newTestHandler(t)
	id := creditNoteFor(t, h, false)
	_, body := postPayment(t, h, payCreditNote(id, 20))
	pid := body["Payments"].([]any)[0].(map[string]any)["PaymentID"].(string)

	rec := do(h, http.MethodPost, "/api.xro/2.0/Payments/"+pid, `{"Status":"DELETED"}`, accounting(t, h, store.DefaultTenantID))
	gone := decode(t, rec)["Payments"].([]any)[0].(map[string]any)
	cn := storedCreditNote(h, id)
	if rec.Code != 200 || gone["Status"] != "DELETED" || cn.Status != "AUTHORISED" || cn.RemainingCredit != 20 || len(cn.Payments) != 0 || cn.FullyPaidOnDate != nil {
		t.Fatalf("status %d, payment %v, credit note %s remaining %v with %d payments", rec.Code, gone["Status"], cn.Status, cn.RemainingCredit, len(cn.Payments))
	}
}
