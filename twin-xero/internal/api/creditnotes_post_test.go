package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func postCreditNotes(t *testing.T, h *Handler, query, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPost, "/api.xro/2.0/CreditNotes"+query, body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func creditNoteStatus(h *Handler, id string) string { return storedCreditNote(h, id).Status }

// storedNote makes a credit note of the given status and total for a contact and returns its ID.
func storedNote(t *testing.T, h *Handler, cid, status string, total float64) string {
	t.Helper()
	code, body := putCreditNotes(t, h, "", fmt.Sprintf(`{"Type":"ACCRECCREDIT","Status":%q,"Reference":"first","Contact":{"ContactID":%q},
		"LineItems":[{"AccountCode":"200","Quantity":1,"UnitAmount":%v}]}`, status, cid, total))
	if code != 200 {
		t.Fatalf("setup: %d %v", code, body)
	}
	return firstCreditNote(t, body)["CreditNoteID"].(string)
}

func TestPostCreatesACreditNoteWithAllocationsThroughThePutRules(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	inv := refundableInvoice(t, h, cid)
	got := subscribed(t, h)

	code, body := postCreditNotes(t, h, "", `{"CreditNotes":[`+creditNoteWithAllocations(cid, 50, allocJSON(inv, 30))+`]}`)
	cn := firstCreditNote(t, body)
	if code != 200 || cn["RemainingCredit"] != 20.0 || cn["AppliedAmount"] != 30.0 || storedInvoice(h, inv).AmountCredited != 30 {
		t.Fatalf("%d %v", code, body)
	}
	events := eventsOf(t, got, 2)
	wantEvent(t, events[0], "CREDITNOTE", "CREATE", cn["CreditNoteID"].(string))
	wantEvent(t, events[1], "INVOICE", "UPDATE", inv)
}

func TestPostAcceptsABareCreditNoteAndTheEnvelope(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	body := fmt.Sprintf(`{"Type":"ACCRECCREDIT","Status":"DRAFT","Contact":{"ContactID":%q},"LineItems":[{"AccountCode":"200","Quantity":1,"UnitAmount":5}]}`, cid)

	for name, b := range map[string]string{"bare": body, "envelope": `{"CreditNotes":[` + body + `]}`} {
		if code, out := postCreditNotes(t, h, "", b); code != 200 || len(out["CreditNotes"].([]any)) != 1 {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if n := len(h.store.CreditNotes.For(store.DefaultTenantID).List()); n != 2 {
		t.Errorf("want two credit notes, have %d", n)
	}
}

func TestPostUpdatesTheCreditNoteThatCarriesItsIDAndMovesItsStatus(t *testing.T) {
	h := newTestHandler(t)
	id := storedNote(t, h, newContactID(t, h, "Acme"), "DRAFT", 50)
	got := subscribed(t, h)

	code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Status":"AUTHORISED","Reference":"second"}`, id))
	cn := firstCreditNote(t, body)
	if code != 200 || cn["CreditNoteID"] != id || cn["Status"] != "AUTHORISED" || cn["Reference"] != "second" || cn["Total"] != 50.0 || cn["CreditNoteNumber"] != "CN-0001" {
		t.Fatalf("%d %v", code, body)
	}
	e := eventsOf(t, got, 1)[0]
	wantEvent(t, e, "CREDITNOTE", "UPDATE", id)
	if d, _ := e["data"].(map[string]any); d["Status"] != "AUTHORISED" || d["Type"] != "ACCRECCREDIT" {
		t.Errorf("data = %v, want the type and the status after the update", e["data"])
	}
}

func TestPostChangesOnlyTheFieldsSentAndStampsTheUpdate(t *testing.T) {
	h := newTestHandler(t)
	id := storedNote(t, h, newContactID(t, h, "Acme"), "AUTHORISED", 50)
	before := storedCreditNote(h, id)
	h.store.Clock.Advance(time.Hour)

	if code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Reference":"changed"}`, id)); code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	after := storedCreditNote(h, id)
	if after.Reference != "changed" || after.Total != before.Total || after.CreditNoteNumber != before.CreditNoteNumber || len(after.LineItems) != 1 || after.Contact.ContactID != before.Contact.ContactID {
		t.Errorf("an update changed more than it was sent: %+v", after)
	}
	if !after.UpdatedDateUTC.After(before.UpdatedDateUTC) {
		t.Errorf("UpdatedDateUTC %v did not move past %v", after.UpdatedDateUTC, before.UpdatedDateUTC)
	}
}

func TestPostRecomputesTheTotalsWhenALineIsSentOnAnUnspentCreditNote(t *testing.T) {
	h := newTestHandler(t)
	id := storedNote(t, h, newContactID(t, h, "Acme"), "AUTHORISED", 50)

	code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"LineItems":[{"AccountCode":"200","Quantity":2,"UnitAmount":15}]}`, id))
	cn := firstCreditNote(t, body)
	if code != 200 || cn["Total"] != 30.0 || cn["RemainingCredit"] != 30.0 || len(cn["LineItems"].([]any)) != 1 {
		t.Fatalf("%d %v", code, body)
	}
}

func TestPostVoidsAnUnspentCreditNoteAndDeletesADraft(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	authorised, draft := storedNote(t, h, cid, "AUTHORISED", 50), storedNote(t, h, cid, "DRAFT", 10)
	got := subscribed(t, h)

	for id, want := range map[string]string{authorised: "VOIDED", draft: "DELETED"} {
		if code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Status":%q}`, id, want)); code != 200 {
			t.Fatalf("%s: %d %v", want, code, body)
		}
		if got := creditNoteStatus(h, id); got != want || storedCreditNote(h, id).RemainingCredit != 0 {
			t.Errorf("status = %s, want %s with nothing remaining", got, want)
		}
	}
	eventsOf(t, got, 2)
	if code, _ := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Reference":"late"}`, authorised)); code != 400 {
		t.Errorf("updating a VOIDED credit note = %d, want 400", code)
	}
}

func TestPostMarksAnAuthorisedCreditNoteSentButWarnsForADraft(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	authorised, draft := storedNote(t, h, cid, "AUTHORISED", 50), storedNote(t, h, cid, "DRAFT", 10)

	for id, want := range map[string]bool{authorised: true, draft: false} {
		if code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"SentToContact":true}`, id)); code != 200 {
			t.Fatalf("%d %v", code, body)
		}
		if cn := storedCreditNote(h, id); cn.SentToContact != want || (!want && len(cn.Warnings) == 0) {
			t.Errorf("SentToContact = %v with warnings %v, want sent=%v", cn.SentToContact, cn.Warnings, want)
		}
	}
	if code, _ := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"SentToContact":null}`, authorised)); code != 200 || !storedCreditNote(h, authorised).SentToContact {
		t.Errorf("a null SentToContact must leave it unchanged (null-001)")
	}
}

func TestPostVoidingASentCreditNoteLeavesSentToContactAlone(t *testing.T) {
	h := newTestHandler(t)
	id := storedNote(t, h, newContactID(t, h, "Acme"), "AUTHORISED", 50)
	if code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"SentToContact":true}`, id)); code != 200 {
		t.Fatalf("%d %v", code, body)
	}

	if code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Status":"VOIDED"}`, id)); code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	if cn := storedCreditNote(h, id); cn.Status != "VOIDED" || !cn.SentToContact || len(cn.Warnings) != 0 || cn.StatusAttributeString == "WARNING" {
		t.Errorf("voiding changed SentToContact it was not sent: %+v", cn)
	}
}

func TestPostAppliesSeveralItemsForOneCreditNoteInOrder(t *testing.T) {
	h := newTestHandler(t)
	id := storedNote(t, h, newContactID(t, h, "Acme"), "AUTHORISED", 50)

	code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNotes":[{"CreditNoteID":%q,"Reference":"one"},{"CreditNoteID":%q,"Reference":"two","Status":"VOIDED"}]}`, id, id))
	if cn := storedCreditNote(h, id); code != 200 || cn.Reference != "two" || cn.Status != "VOIDED" {
		t.Fatalf("%d %v, stored %+v", code, body, cn)
	}
}

func TestPostChangesTheContactToAnotherThatExistsAndRefusesOneThatDoesNot(t *testing.T) {
	h := newTestHandler(t)
	id := storedNote(t, h, newContactID(t, h, "Acme"), "DRAFT", 50)
	other := newContactID(t, h, "Beta")

	if code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Contact":{"ContactID":%q}}`, id, other)); code != 200 || storedCreditNote(h, id).Contact.ContactID != other {
		t.Fatalf("%d %v", code, body)
	}
	if code, _ := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Contact":{"ContactID":"no-such"}}`, id)); code != 400 || storedCreditNote(h, id).Contact.ContactID != other {
		t.Errorf("a contact that does not exist = %d, want 400 with the contact unchanged", code)
	}
}

func TestPostRefusesWhatAnUpdateMayNotDo(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	inv := refundableInvoice(t, h, cid)
	_, made := putCreditNotes(t, h, "", creditNoteWithAllocations(cid, 50, allocJSON(inv, 30)))
	id := firstCreditNote(t, made)["CreditNoteID"].(string)

	for name, change := range map[string]string{
		"a status move that is not allowed":         `"Status":"DRAFT"`,
		"voiding a credit note with credit applied": `"Status":"VOIDED"`,
		"new lines on a credit note with credit":    `"LineItems":[{"AccountCode":"200","Quantity":1,"UnitAmount":5}]`,
		"changing the Type":                         `"Type":"ACCPAYCREDIT"`,
		"a contact that does not exist":             `"Reference":"x","Contact":{"ContactID":"no-such"}`,
	} {
		if code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,%s}`, id, change)); code != 400 || body["Type"] != "ValidationException" {
			t.Errorf("%s = %d %v, want a 400 ValidationException", name, code, body)
		}
	}
	if after := storedCreditNote(h, id); after.Status != "AUTHORISED" || after.Total != 50 || after.Type != "ACCRECCREDIT" || after.Reference != "" || len(after.Allocations) != 1 {
		t.Errorf("a refused update changed the store: %+v", after)
	}
}

func TestPostDoesNotMatchACreditNoteByNumberAndKeepsNumbersUnique(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	first, second := storedNote(t, h, cid, "AUTHORISED", 50), storedNote(t, h, cid, "AUTHORISED", 20)

	code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteNumber":"CN-0001","Type":"ACCRECCREDIT","Contact":{"ContactID":%q},
		"LineItems":[{"AccountCode":"200","Quantity":1,"UnitAmount":5}]}`, cid))
	if code != 400 || body["Type"] != "ValidationException" || len(h.store.CreditNotes.For(store.DefaultTenantID).List()) != 2 {
		t.Fatalf("a repeated number with no CreditNoteID must be refused as PUT refuses it: %d %v", code, body)
	}
	if code, _ := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"CreditNoteNumber":"CN-0001"}`, second)); code != 400 {
		t.Errorf("renaming onto another credit note's number = %d, want 400", code)
	}
	if code, _ := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"CreditNoteNumber":"CN-0001","Reference":"same"}`, first)); code != 200 {
		t.Errorf("resending its own number = %d, want 200", code)
	}
}

func TestPostCreditNotesWithSummarizeErrorsFalseAnswersTheInvalidOnesInPlace(t *testing.T) {
	h := newTestHandler(t)
	id := storedNote(t, h, newContactID(t, h, "Acme"), "AUTHORISED", 50)
	got := subscribed(t, h)

	code, body := postCreditNotes(t, h, "?summarizeErrors=false", fmt.Sprintf(`{"CreditNotes":[{"CreditNoteID":%q,"Reference":"changed"},{"CreditNoteID":%q,"Status":"DRAFT"}]}`, id, id))
	list := body["CreditNotes"].([]any)
	if code != 200 || len(list) != 2 || list[0].(map[string]any)["Reference"] != "changed" || list[1].(map[string]any)["HasErrors"] != true {
		t.Fatalf("%d %v", code, body)
	}
	wantEvent(t, eventsOf(t, got, 1)[0], "CREDITNOTE", "UPDATE", id)
	none(t, h, got)
}

func TestPostWithAnUnknownCreditNoteIDIsA404AndWritesNothing(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	got := subscribed(t, h)

	code, _ := postCreditNotes(t, h, "", `{"CreditNotes":[`+creditNoteWithAllocations(cid, 50, "")+`,{"CreditNoteID":"00000000-0000-4000-8000-000000000009","Reference":"x"}]}`)
	if n := len(h.store.CreditNotes.For(store.DefaultTenantID).List()); code != 404 || n != 0 {
		t.Fatalf("status %d, %d credit notes", code, n)
	}
	none(t, h, got)
}

func TestPostAddsAllocationsToAnAuthorisedCreditNoteUpToWhatIsLeft(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	inv := refundableInvoice(t, h, cid)
	id := storedNote(t, h, cid, "AUTHORISED", 50)
	got := subscribed(t, h)

	code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Allocations":[%s]}`, id, allocJSON(inv, 30)))
	cn := firstCreditNote(t, body)
	if code != 200 || cn["RemainingCredit"] != 20.0 || cn["AppliedAmount"] != 30.0 || len(cn["Allocations"].([]any)) != 1 || storedInvoice(h, inv).AmountCredited != 30 {
		t.Fatalf("%d %v", code, body)
	}
	events := eventsOf(t, got, 2)
	wantEvent(t, events[0], "CREDITNOTE", "UPDATE", id)
	wantEvent(t, events[1], "INVOICE", "UPDATE", inv)

	if code, _ := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Allocations":[%s]}`, id, allocJSON(inv, 30))); code != 400 {
		t.Errorf("an allocation above the credit left = %d, want 400", code)
	}
	if after := storedCreditNote(h, id); after.RemainingCredit != 20 || len(after.Allocations) != 1 || storedInvoice(h, inv).AmountCredited != 30 {
		t.Errorf("a refused allocation changed the store: %+v", after)
	}
}

func TestPostSpendingTheLastOfTheCreditPaysTheCreditNote(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	inv := refundableInvoice(t, h, cid)
	id := storedNote(t, h, cid, "AUTHORISED", 50)

	postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Allocations":[%s]}`, id, allocJSON(inv, 20)))
	code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Allocations":[%s]}`, id, allocJSON(inv, 30)))
	cn := firstCreditNote(t, body)
	if code != 200 || cn["Status"] != "PAID" || cn["RemainingCredit"] != 0.0 || cn["AppliedAmount"] != 50.0 || len(cn["Allocations"].([]any)) != 2 {
		t.Fatalf("%d %v", code, body)
	}
	if got := storedInvoice(h, inv); got.AmountCredited != 50 || got.AmountDue != 50 {
		t.Errorf("invoice = credited %v due %v, want 50 and 50", got.AmountCredited, got.AmountDue)
	}
}

func TestPostIgnoresAllocationsItAlreadyHasWhenACreditNoteIsSentBack(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	inv := refundableInvoice(t, h, cid)
	_, made := putCreditNotes(t, h, "", creditNoteWithAllocations(cid, 50, allocJSON(inv, 30)))
	cn := firstCreditNote(t, made)
	known, _ := json.Marshal(cn["Allocations"])

	code, _ := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Reference":"again","Allocations":%s}`, cn["CreditNoteID"], known))
	if after := storedCreditNote(h, cn["CreditNoteID"].(string)); code != 200 || after.AppliedAmount != 30 || len(after.Allocations) != 1 || storedInvoice(h, inv).AmountCredited != 30 {
		t.Errorf("%d: a credit note read and sent back was allocated again: %+v", code, after)
	}
}

func TestPostRefusesAnAllocationThatTheCreateRulesRefuse(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	inv := refundableInvoice(t, h, cid)
	authorised, draft := storedNote(t, h, cid, "AUTHORISED", 50), storedNote(t, h, cid, "DRAFT", 50)

	for name, c := range map[string]struct{ id, alloc string }{
		"a credit note that is not authorised": {draft, allocJSON(inv, 10)},
		"an invoice that does not exist":       {authorised, allocJSON("no-such", 10)},
		"an invoice of another contact":        {authorised, allocJSON(refundableInvoice(t, h, newContactID(t, h, "Beta")), 10)},
		"an amount of zero":                    {authorised, allocJSON(inv, 0)},
		"no date":                              {authorised, fmt.Sprintf(`{"Invoice":{"InvoiceID":%q},"Amount":10}`, inv)},
	} {
		if code, body := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Allocations":[%s]}`, c.id, c.alloc)); code != 400 || body["Type"] != "ValidationException" {
			t.Errorf("%s = %d %v, want a 400 ValidationException", name, code, body)
		}
	}
	if after := storedCreditNote(h, authorised); after.AppliedAmount != 0 || storedInvoice(h, inv).AmountCredited != 0 {
		t.Errorf("a refused allocation changed the store: %+v", after)
	}
}

func TestPostUnderStrictAuthKeepsAllocationsWithinTheInvoicesAmountDue(t *testing.T) {
	h := newTestHandler(t)
	h.store.StrictAuth.Store(true)
	cid := newContactID(t, h, "Acme")
	inv := refundableInvoice(t, h, cid) // 100 due
	id := storedNote(t, h, cid, "AUTHORISED", 150)

	if code, _ := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Allocations":[%s,%s]}`, id, allocJSON(inv, 60), allocJSON(inv, 60))); code != 400 {
		t.Errorf("two allocations that together pass the amount due = %d, want 400 under strict_auth", code)
	}
	if code, _ := postCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNoteID":%q,"Allocations":[%s]}`, id, allocJSON(inv, 100))); code != 200 {
		t.Errorf("an allocation of the whole amount due = %d, want 200", code)
	}
}
