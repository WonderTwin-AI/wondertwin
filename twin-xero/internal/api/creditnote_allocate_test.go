package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// refundableInvoice stores an AUTHORISED 100.00 ACCREC invoice and returns its ID.
func refundableInvoice(t *testing.T, h *Handler, cid string) string {
	t.Helper()
	code, body := putInvoices(t, h, "", fmt.Sprintf(`{"Type":"ACCREC","Status":"AUTHORISED","Contact":{"ContactID":%q},
		"LineItems":[{"AccountCode":"200","Quantity":1,"UnitAmount":100}]}`, cid))
	if code != 200 {
		t.Fatalf("invoice setup: %d %v", code, body)
	}
	return body["Invoices"].([]any)[0].(map[string]any)["InvoiceID"].(string)
}

func creditNoteWithAllocations(cid string, amount float64, allocations string) string {
	return fmt.Sprintf(`{"Type":"ACCRECCREDIT","Status":"AUTHORISED","Contact":{"ContactID":%q},
		"LineItems":[{"AccountCode":"200","Quantity":1,"UnitAmount":%v}],"Allocations":[%s]}`, cid, amount, allocations)
}

func allocJSON(invoiceID string, amount float64) string {
	return fmt.Sprintf(`{"Invoice":{"InvoiceID":%q},"Amount":%v,"Date":"2026-09-11"}`, invoiceID, amount)
}

func TestCreditNoteInlineAllocationAppliesToTheInvoice(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	inv := refundableInvoice(t, h, cid)

	code, body := putCreditNotes(t, h, "", creditNoteWithAllocations(cid, 50, allocJSON(inv, 30)))
	cn := firstCreditNote(t, body)
	if code != 200 {
		t.Fatalf("status %d, body %v", code, body)
	}
	if cn["AppliedAmount"] != 30.0 || cn["RemainingCredit"] != 20.0 || cn["Status"] != "AUTHORISED" {
		t.Errorf("credit note = %v, want 30 applied and 20 remaining", cn)
	}
	a := cn["Allocations"].([]any)[0].(map[string]any)
	if !guidPattern.MatchString(a["AllocationID"].(string)) || a["Amount"] != 30.0 || !msdateValue.MatchString(a["Date"].(string)) {
		t.Errorf("allocation = %v", a)
	}
	stored, _ := h.store.Invoices.For(store.DefaultTenantID).Get(inv)
	if stored.AmountCredited != 30 || stored.AmountDue != 70 || stored.Status != "AUTHORISED" || len(stored.CreditNotes) != 1 {
		t.Errorf("invoice = credited %v, due %v, status %s, %d credit notes", stored.AmountCredited, stored.AmountDue, stored.Status, len(stored.CreditNotes))
	}
}

func TestCreditNoteAllocationOfTheWholeInvoicePaysIt(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	inv := refundableInvoice(t, h, cid)

	code, body := putCreditNotes(t, h, "", creditNoteWithAllocations(cid, 100, allocJSON(inv, 40)+","+allocJSON(inv, 60)))
	cn := firstCreditNote(t, body)
	stored, _ := h.store.Invoices.For(store.DefaultTenantID).Get(inv)
	if code != 200 || stored.Status != "PAID" || stored.AmountDue != 0 || cn["RemainingCredit"] != 0.0 || len(cn["Allocations"].([]any)) != 2 {
		t.Fatalf("status %d, invoice %s due %v, credit note %v", code, stored.Status, stored.AmountDue, cn)
	}
}

func paidInvoice(t *testing.T, h *Handler, cid string) string {
	t.Helper()
	inv := refundableInvoice(t, h, cid)
	if code, body := putCreditNotes(t, h, "", creditNoteWithAllocations(cid, 100, allocJSON(inv, 100))); code != 200 {
		t.Fatalf("paid invoice setup: %d %v", code, body)
	}
	return inv
}

func TestCreditNoteAllocationToAPaidInvoiceIsAcceptedByDefault(t *testing.T) {
	// localstack/finance-ops refunds an invoice that is already PAID; that has to run.
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	inv := paidInvoice(t, h, cid)

	code, body := putCreditNotes(t, h, "", creditNoteWithAllocations(cid, 20, allocJSON(inv, 20)))
	cn := firstCreditNote(t, body)
	stored, _ := h.store.Invoices.For(store.DefaultTenantID).Get(inv)
	if code != 200 || cn["AppliedAmount"] != 20.0 || cn["RemainingCredit"] != 0.0 {
		t.Fatalf("status %d, credit note %v", code, cn)
	}
	if stored.AmountCredited != 120 || stored.AmountDue != 0 || stored.Status != "PAID" || len(stored.CreditNotes) != 2 {
		t.Errorf("invoice = credited %v, due %v, status %s", stored.AmountCredited, stored.AmountDue, stored.Status)
	}
}

func TestCreditNoteAllocationDefaultIsBoundedOnlyByTheCredit(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	inv := refundableInvoice(t, h, cid)

	code, body := putCreditNotes(t, h, "", creditNoteWithAllocations(cid, 150, allocJSON(inv, 150)))
	stored, _ := h.store.Invoices.For(store.DefaultTenantID).Get(inv)
	if code != 200 || firstCreditNote(t, body)["Status"] != "PAID" || stored.AmountDue != 0 || stored.AmountCredited != 150 || stored.Status != "PAID" {
		t.Fatalf("status %d, invoice credited %v due %v %s", code, stored.AmountCredited, stored.AmountDue, stored.Status)
	}
}

func TestCreditNoteAllocationStrictRefusesWhatXeroWouldRefuse(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	other := newContactID(t, h, "Other")
	inv := refundableInvoice(t, h, cid)
	foreign := refundableInvoice(t, h, other)
	paid := paidInvoice(t, h, cid)
	missing := "00000000-0000-4000-8000-0000000000ff"
	h.store.StrictAuth.Store(true)
	cases := map[string]struct{ body, want string }{
		"an invoice that does not exist":       {creditNoteWithAllocations(cid, 30, allocJSON(missing, 30)), "the invoice could not be found"},
		"another contact's invoice":            {creditNoteWithAllocations(cid, 30, allocJSON(foreign, 30)), "same contact"},
		"more than the credit remaining":       {creditNoteWithAllocations(cid, 30, allocJSON(inv, 31)), "more than the credit remaining"},
		"more than the invoice has due":        {creditNoteWithAllocations(cid, 150, allocJSON(inv, 60)+","+allocJSON(inv, 50)), "more than the invoice's amount due"},
		"an invoice that is already paid":      {creditNoteWithAllocations(cid, 30, allocJSON(paid, 30)), "authorised invoice"},
		"an amount of zero":                    {creditNoteWithAllocations(cid, 30, allocJSON(inv, 0)), "above zero"},
		"no date":                              {creditNoteWithAllocations(cid, 30, fmt.Sprintf(`{"Invoice":{"InvoiceID":%q},"Amount":5}`, inv)), "Date is required"},
		"a credit note that is not authorised": {strings.Replace(creditNoteWithAllocations(cid, 30, allocJSON(inv, 30)), "AUTHORISED", "DRAFT", 1), "authorised credit note"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			code, body := putCreditNotes(t, h, "", tc.body)
			if code != 400 || !strings.Contains(fmt.Sprint(body["Elements"]), tc.want) {
				t.Fatalf("status %d, body %v, want 400 with %q", code, body, tc.want)
			}
		})
	}
	stored, _ := h.store.Invoices.For(store.DefaultTenantID).Get(inv)
	if stored.AmountDue != 100 || stored.AmountCredited != 0 || h.store.CreditNotes.For(store.DefaultTenantID).Count() != 1 {
		t.Fatalf("a refused request must change nothing (the one credit note is the setup): invoice %v, %d credit notes", stored.AmountDue, h.store.CreditNotes.For(store.DefaultTenantID).Count())
	}
}
