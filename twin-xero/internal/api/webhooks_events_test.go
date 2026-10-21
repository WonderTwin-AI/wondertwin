package api

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// subscribed registers a receiver for every category and returns what it gets.
func subscribed(t *testing.T, h *Handler) chan received {
	t.Helper()
	srv, got := receiver(t)
	subscribe(h, "w1", srv.URL, "k", store.WebhookOK, "CONTACT", "INVOICE", "CREDITNOTE")
	return got
}

// eventsOf reads the next n deliveries, each of which carries one event.
func eventsOf(t *testing.T, got chan received, n int) []map[string]any {
	t.Helper()
	var out []map[string]any
	for range n {
		var p struct{ Events []map[string]any }
		if err := json.Unmarshal(next(t, got).body, &p); err != nil || len(p.Events) != 1 {
			t.Fatalf("delivery: %v %v", err, p)
		}
		out = append(out, p.Events[0])
	}
	return out
}

func wantEvent(t *testing.T, e map[string]any, category, eventType, id string) {
	t.Helper()
	if e["eventCategory"] != category || e["eventType"] != eventType || e["resourceId"] != id {
		t.Fatalf("event = %v, want %s %s %s", e, category, eventType, id)
	}
}

func TestWritesDeliverASignedEventForTheResourceChanged(t *testing.T) {
	h := newTestHandler(t)
	got := subscribed(t, h)

	cid := firstContactID(t, putContacts(t, h, `{"Contacts":[{"Name":"Acme"}]}`))
	wantEvent(t, eventsOf(t, got, 1)[0], "CONTACT", "CREATE", cid)

	_, body := putInvoices(t, h, "", fmt.Sprintf(`{"Type":"ACCREC","Status":"AUTHORISED","Contact":{"ContactID":%q},
		"LineItems":[{"AccountCode":"200","Quantity":1,"UnitAmount":100}]}`, cid))
	inv := firstInvoice(t, body)["InvoiceID"].(string)
	e := eventsOf(t, got, 1)[0]
	wantEvent(t, e, "INVOICE", "CREATE", inv)
	if e["data"] != nil {
		t.Errorf("an invoice event carries no data, got %v", e["data"])
	}

	_, body = putCreditNotes(t, h, "", fmt.Sprintf(`{"Type":"ACCRECCREDIT","Status":"AUTHORISED","Contact":{"ContactID":%q},
		"LineItems":[{"AccountCode":"200","Quantity":1,"UnitAmount":20}]}`, cid))
	cn := firstCreditNote(t, body)["CreditNoteID"].(string)
	e = eventsOf(t, got, 1)[0]
	wantEvent(t, e, "CREDITNOTE", "CREATE", cn)
	if d, _ := e["data"].(map[string]any); d["Type"] != "ACCRECCREDIT" || d["Status"] != "AUTHORISED" {
		t.Errorf("credit note data = %v, want Type and Status", e["data"])
	}
}

func TestInvoiceUpdatesAndPaymentsReportAnInvoiceUpdate(t *testing.T) {
	h := newTestHandler(t)
	voided, paid := makeInvoice(t, h, "ACCREC", "AUTHORISED"), makeInvoice(t, h, "ACCREC", "AUTHORISED")
	got := subscribed(t, h)

	if code, body := postInvoice(t, h, voided, "", `{"Status":"VOIDED"}`); code != 200 {
		t.Fatalf("void: %d %v", code, body)
	}
	wantEvent(t, eventsOf(t, got, 1)[0], "INVOICE", "UPDATE", voided)

	if code, body := postPayment(t, h, pay(paid, 100)); code != 200 {
		t.Fatalf("payment: %d %v", code, body)
	}
	wantEvent(t, eventsOf(t, got, 1)[0], "INVOICE", "UPDATE", paid)
}

func TestACreditNoteWithAllocationsReportsTheInvoicesItChanged(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	inv := refundableInvoice(t, h, cid)
	got := subscribed(t, h)

	_, body := putCreditNotes(t, h, "", creditNoteWithAllocations(cid, 20, allocJSON(inv, 20)))
	events := eventsOf(t, got, 2)
	wantEvent(t, events[0], "CREDITNOTE", "CREATE", firstCreditNote(t, body)["CreditNoteID"].(string))
	wantEvent(t, events[1], "INVOICE", "UPDATE", inv)
}

func TestDeletingAPaymentReportsTheInvoiceItRestores(t *testing.T) {
	h := newTestHandler(t)
	id := makeInvoice(t, h, "ACCREC", "AUTHORISED")
	_, body := postPayment(t, h, pay(id, 100))
	got := subscribed(t, h)

	if code, body := deletePayment(t, h, paymentIDOf(body), `{"Status":"DELETED"}`); code != 200 {
		t.Fatalf("delete: %d %v", code, body)
	}
	wantEvent(t, eventsOf(t, got, 1)[0], "INVOICE", "UPDATE", id)
}

func TestCreditNotePaymentsReportTheCreditNoteTheyChange(t *testing.T) {
	h := newTestHandler(t)
	id := creditNoteFor(t, h, false)
	got := subscribed(t, h)

	_, body := postPayment(t, h, payCreditNote(id, 20))
	e := eventsOf(t, got, 1)[0]
	wantEvent(t, e, "CREDITNOTE", "UPDATE", id)
	if d, _ := e["data"].(map[string]any); d["Type"] != "ACCRECCREDIT" || d["Status"] != "PAID" {
		t.Errorf("data = %v, want the credit note's type and its status after the payment", e["data"])
	}

	if code, del := deletePayment(t, h, paymentIDOf(body), `{"Status":"DELETED"}`); code != 200 {
		t.Fatalf("delete: %d %v", code, del)
	}
	e = eventsOf(t, got, 1)[0]
	wantEvent(t, e, "CREDITNOTE", "UPDATE", id)
	if d, _ := e["data"].(map[string]any); d["Status"] != "AUTHORISED" {
		t.Errorf("data = %v, deleting the payment puts the credit note back to AUTHORISED", e["data"])
	}
	none(t, got) // no invoice is touched by a credit note payment
}

func TestACreditNoteReachedTwiceInOneRequestIsReportedOnceWithItsFinalStatus(t *testing.T) {
	h := newTestHandler(t)
	id := creditNoteFor(t, h, false)
	got := subscribed(t, h)

	if code, body := postPayment(t, h, fmt.Sprintf(`{"Payments":[%s,%s]}`, payCreditNote(id, 10), payCreditNote(id, 10))); code != 200 {
		t.Fatalf("payments: %d %v", code, body)
	}
	e := eventsOf(t, got, 1)[0]
	wantEvent(t, e, "CREDITNOTE", "UPDATE", id)
	if d, _ := e["data"].(map[string]any); d["Status"] != "PAID" {
		t.Errorf("data = %v, want the status after the last payment", e["data"])
	}
	none(t, got)
}

func TestAnInvoiceReachedTwiceInOneRequestIsReportedOnce(t *testing.T) {
	h := newTestHandler(t)
	id := makeInvoice(t, h, "ACCREC", "AUTHORISED")
	got := subscribed(t, h)

	if code, body := postPayment(t, h, fmt.Sprintf(`{"Payments":[%s,%s]}`, pay(id, 40), pay(id, 60))); code != 200 {
		t.Fatalf("payments: %d %v", code, body)
	}
	wantEvent(t, eventsOf(t, got, 1)[0], "INVOICE", "UPDATE", id)
	none(t, got)
}

func TestARefusedWriteReportsNothing(t *testing.T) {
	h := newTestHandler(t)
	got := subscribed(t, h)

	if code, _ := putInvoices(t, h, "", `{"Type":"ACCREC","Contact":{"ContactID":"no-such"},"LineItems":[]}`); code == 200 {
		t.Fatal("the invalid invoice was accepted")
	}
	if rec := do(h, "POST", "/api.xro/2.0/Invoices/no-such", `{"Status":"VOIDED"}`, accounting(t, h, store.DefaultTenantID)); rec.Code != 404 {
		t.Fatalf("unknown invoice answered %d", rec.Code)
	}
	none(t, got)
}

func TestOnlyTheValidInvoicesOfAPartialBatchAreReported(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	got := subscribed(t, h)

	_, body := putInvoices(t, h, "?summarizeErrors=false", fmt.Sprintf(`{"Invoices":[
		{"Type":"ACCREC","Status":"AUTHORISED","Contact":{"ContactID":%q},"LineItems":[{"AccountCode":"200","Quantity":1,"UnitAmount":10}]},
		{"Type":"ACCREC","Contact":{"ContactID":"no-such"},"LineItems":[]}]}`, cid))
	good := body["Invoices"].([]any)[0].(map[string]any)["InvoiceID"].(string)
	wantEvent(t, eventsOf(t, got, 1)[0], "INVOICE", "CREATE", good)
	none(t, got)
}

func TestEventsReachOnlyTheCategoriesSubscribedTo(t *testing.T) {
	h := newTestHandler(t)
	srv, got := receiver(t)
	subscribe(h, "w1", srv.URL, "k", store.WebhookOK, "CONTACT")

	cid := firstContactID(t, putContacts(t, h, `{"Contacts":[{"Name":"Acme"}]}`))
	wantEvent(t, eventsOf(t, got, 1)[0], "CONTACT", "CREATE", cid)
	putInvoices(t, h, "", fmt.Sprintf(`{"Type":"ACCREC","Status":"AUTHORISED","Contact":{"ContactID":%q},
		"LineItems":[{"AccountCode":"200","Quantity":1,"UnitAmount":10}]}`, cid))
	none(t, got)
}

func TestEventSequencesIncreaseInWriteOrder(t *testing.T) {
	h := newTestHandler(t)
	got := subscribed(t, h)
	putContacts(t, h, `{"Contacts":[{"Name":"A"},{"Name":"B"}]}`)

	var last float64
	for range 2 {
		var p struct{ FirstEventSequence, LastEventSequence float64 }
		if err := json.Unmarshal(next(t, got).body, &p); err != nil || p.FirstEventSequence <= last || p.LastEventSequence != p.FirstEventSequence {
			t.Fatalf("sequences %+v after %v: %v", p, last, err)
		}
		last = p.LastEventSequence
	}
}

func firstContactID(t *testing.T, body map[string]any) string {
	t.Helper()
	list, _ := body["Contacts"].([]any)
	if len(list) == 0 {
		t.Fatalf("no contacts in %v", body)
	}
	return list[0].(map[string]any)["ContactID"].(string)
}
