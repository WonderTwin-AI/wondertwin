package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func TestPaymentWireWritesDatesAndTheInvoiceTheWayXeroDoes(t *testing.T) {
	h := newTestHandler(t)
	at := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	inv := store.Invoice{InvoiceID: "inv-1", Type: "ACCREC", Status: "PAID", Total: 100, AmountPaid: 100,
		LineItems: []store.LineItem{{Description: "x"}}, UpdatedDateUTC: at, FullyPaidOnDate: &at}
	p := store.Payment{PaymentID: "pay-1", Date: at, Amount: 100, Status: "AUTHORISED", PaymentType: "ACCRECPAYMENT", UpdatedDateUTC: at,
		Account: &store.PaymentAccount{AccountID: "acc-1", Code: "090"}, HasAccount: true}

	b, err := json.Marshal(h.wirePayment(store.DefaultTenantID, p, inv))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(b, &got)

	for _, k := range []string{"Date", "UpdatedDateUTC"} {
		if s, _ := got[k].(string); !msdateValue.MatchString(s) {
			t.Errorf("%s = %v, want /Date(ms+0000)/ (time-001)", k, got[k])
		}
	}
	if a, _ := got["Account"].(map[string]any); a["Code"] != "090" || a["AccountID"] != "acc-1" {
		t.Errorf("Account = %v", got["Account"])
	}
	wi, _ := got["Invoice"].(map[string]any)
	if wi["Status"] != "PAID" || wi["AmountPaid"] != 100.0 {
		t.Errorf("Invoice = %v, want the invoice as the payment leaves it", wi)
	}
	if lines, ok := wi["LineItems"].([]any); !ok || len(lines) != 0 {
		t.Errorf("a payment's invoice carries no line items, got %v", wi["LineItems"])
	}
	for _, k := range []string{"CreditNote", "Prepayment", "Overpayment", "BatchPaymentID", "Reference"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s is unset and must be omitted, not null", k)
		}
	}
}
