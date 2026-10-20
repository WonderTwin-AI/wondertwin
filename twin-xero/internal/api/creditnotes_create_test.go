package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func putCreditNotes(t *testing.T, h *Handler, query, body string) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodPut, "/api.xro/2.0/CreditNotes"+query, body, accounting(t, h, store.DefaultTenantID))
	return rec.Code, decode(t, rec)
}

func firstCreditNote(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	list, _ := body["CreditNotes"].([]any)
	if len(list) == 0 {
		t.Fatalf("no credit notes in %v", body)
	}
	return list[0].(map[string]any)
}

func TestCreditNoteCreateAnswersTheEnvelopeWithComputedFields(t *testing.T) {
	// The shape localstack/finance-ops sends for a refund: its own number, one line
	// with an empty LineItemID, and a client-side Total and SubTotal.
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	code, body := putCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNotes":[{"Type":"ACCRECCREDIT","Status":"AUTHORISED","Contact":{"ContactID":%q},
		"CreditNoteNumber":"Stripe refund re_1","Date":"2026-09-10","Reference":"in_1","Total":999,"SubTotal":999,
		"LineItems":[{"LineItemID":"","Description":"refund","Quantity":1,"UnitAmount":20,"LineAmount":20,"AccountCode":"200","TaxType":"NONE"}]}]}`, cid))

	cn := firstCreditNote(t, body)
	if code != 200 || body["Status"] != "OK" {
		t.Fatalf("status %d, body %v", code, body)
	}
	if id, _ := cn["CreditNoteID"].(string); !guidPattern.MatchString(id) || cn["CreditNoteNumber"] != "Stripe refund re_1" ||
		cn["Status"] != "AUTHORISED" || cn["CurrencyCode"] != "USD" || cn["Reference"] != "in_1" {
		t.Fatalf("credit note = %v", cn)
	}
	for k, want := range map[string]float64{"SubTotal": 20, "TotalTax": 0, "Total": 20, "RemainingCredit": 20, "AppliedAmount": 0} {
		if cn[k] != want {
			t.Errorf("%s = %v, want %v (the client's 999 is ignored)", k, cn[k], want)
		}
	}
	for _, k := range []string{"Date", "UpdatedDateUTC"} {
		if s, _ := cn[k].(string); !msdateValue.MatchString(s) {
			t.Errorf("%s = %v, want /Date(ms+0000)/ (time-001)", k, cn[k])
		}
	}
	if c, _ := cn["Contact"].(map[string]any); c["Name"] != "Acme" || c["ContactID"] != cid {
		t.Errorf("Contact = %v, want the full contact", cn["Contact"])
	}
	if line := cn["LineItems"].([]any)[0].(map[string]any); !guidPattern.MatchString(line["LineItemID"].(string)) || line["LineAmount"] != 20.0 {
		t.Errorf("an empty LineItemID must be replaced: %v", line)
	}
	for _, k := range []string{"Allocations", "Payments"} {
		if l, ok := cn[k].([]any); !ok || l == nil {
			t.Errorf("%s must be [], got %v", k, cn[k])
		}
	}
}

func TestCreditNoteCreateFillsInDefaultsAndNumbers(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	one := fmt.Sprintf(`{"Type":"ACCRECCREDIT","Contact":{"ContactID":%q}}`, cid)
	_, a := putCreditNotes(t, h, "", one)
	_, b := putCreditNotes(t, h, "", one)
	_, bill := putCreditNotes(t, h, "", fmt.Sprintf(`{"Type":"ACCPAYCREDIT","Contact":{"ContactID":%q}}`, cid))

	first := firstCreditNote(t, a)
	if first["Status"] != "DRAFT" || first["LineAmountTypes"] != "Exclusive" || first["CreditNoteNumber"] != "CN-0001" || first["Date"] == nil {
		t.Errorf("defaults = %v", first)
	}
	if firstCreditNote(t, b)["CreditNoteNumber"] != "CN-0002" {
		t.Errorf("the second number must follow the first: %v", firstCreditNote(t, b)["CreditNoteNumber"])
	}
	if n, ok := firstCreditNote(t, bill)["CreditNoteNumber"]; ok {
		t.Errorf("a supplier credit gets no generated number, got %v", n)
	}
}

func TestCreditNoteCreateIgnoresWhatXeroComputes(t *testing.T) {
	h := newTestHandler(t)
	cid := newContactID(t, h, "Acme")
	_, body := putCreditNotes(t, h, "", fmt.Sprintf(`{"CreditNotes":[{"Type":"ACCRECCREDIT","Contact":{"ContactID":%q},"Status":"AUTHORISED","SentToContact":true,
		"CreditNoteID":"11111111-1111-4111-8111-111111111111","RemainingCredit":1,"AppliedAmount":50,"FullyPaidOnDate":"2026-09-02",
		"Payments":[{"Amount":5}],"StatusAttributeString":"WARNING","LineItems":[{"AccountCode":"200","LineAmount":40}]}]}`, cid))

	cn := firstCreditNote(t, body)
	if cn["CreditNoteID"] == "11111111-1111-4111-8111-111111111111" || cn["RemainingCredit"] != 40.0 || cn["AppliedAmount"] != 0.0 || cn["StatusAttributeString"] != "OK" {
		t.Fatalf("computed fields must be ignored: %v", cn)
	}
	if p, _ := cn["Payments"].([]any); len(p) != 0 || cn["FullyPaidOnDate"] != nil || cn["SentToContact"] != true {
		t.Errorf("client-sent read-only fields must not be echoed: %v", cn)
	}
	_, draft := putCreditNotes(t, h, "", fmt.Sprintf(`{"Type":"ACCRECCREDIT","Contact":{"ContactID":%q},"SentToContact":true}`, cid))
	if firstCreditNote(t, draft)["SentToContact"] != false {
		t.Error("only an authorised credit note can be marked sent (null-001)")
	}
}
