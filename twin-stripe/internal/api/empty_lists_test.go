package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

// assertEmptyArray fails unless m[key] is present and an empty JSON array.
// Stripe returns [] for an empty non-nullable array, never null.
func assertEmptyArray(t *testing.T, what string, m map[string]any, key string) {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Errorf("%s: %s is omitted; Stripe returns []", what, key)
		return
	}
	arr, ok := v.([]any)
	if !ok {
		t.Errorf("%s: %s = %v, want []", what, key, v)
		return
	}
	if len(arr) != 0 {
		t.Errorf("%s: %s has %d entries, want []", what, key, len(arr))
	}
}

func invoiceLines(t *testing.T, what string, inv map[string]any) map[string]any {
	t.Helper()
	lines, ok := inv["lines"].(map[string]any)
	if !ok {
		t.Fatalf("%s: lines = %v, want a list object", what, inv["lines"])
	}
	return lines
}

func newCustomer(t *testing.T, tc *testutil.TwinClient) string {
	t.Helper()
	resp := stripePost(tc, "/v1/customers?email=empty@example.com", nil)
	resp.AssertStatus(200)
	return resp.JSONMap()["id"].(string)
}

func newMonthlyPrice(t *testing.T, tc *testutil.TwinClient, amount string) string {
	t.Helper()
	resp := stripePost(tc, "/v1/products?name=Plan", nil)
	resp.AssertStatus(200)
	prod := resp.JSONMap()["id"].(string)
	resp = stripePost(tc, "/v1/prices?currency=usd&unit_amount="+amount+"&recurring[interval]=month&product="+prod, nil)
	resp.AssertStatus(200)
	return resp.JSONMap()["id"].(string)
}

func TestEmptyLists_InvoiceWithoutItems(t *testing.T) {
	_, tc := setupStripe(t)
	cus := newCustomer(t, tc)

	resp := stripePost(tc, "/v1/invoices?customer="+cus, nil)
	resp.AssertStatus(200)
	inv := resp.JSONMap()
	assertEmptyArray(t, "POST /v1/invoices", inv, "discounts")
	assertEmptyArray(t, "POST /v1/invoices lines", invoiceLines(t, "POST /v1/invoices", inv), "data")

	resp = stripeGet(tc, "/v1/invoices/"+inv["id"].(string))
	resp.AssertStatus(200)
	got := resp.JSONMap()
	assertEmptyArray(t, "GET /v1/invoices/{id}", got, "discounts")
	assertEmptyArray(t, "GET /v1/invoices/{id} lines", invoiceLines(t, "GET /v1/invoices/{id}", got), "data")
}

func TestEmptyLists_SubscriptionInvoices(t *testing.T) {
	_, tc := setupStripe(t)
	cus := newCustomer(t, tc)
	stripePost(tc, "/v1/payment_methods/pm_card_visa/attach?customer="+cus, nil).AssertStatus(200)
	price := newMonthlyPrice(t, tc, "1000")

	resp := stripePost(tc, "/v1/subscriptions?customer="+cus+"&items[0][price]="+price+
		"&default_payment_method=pm_card_visa&expand[0]=latest_invoice", nil)
	resp.AssertStatus(200)
	sub := resp.JSONMap()
	first, ok := sub["latest_invoice"].(map[string]any)
	if !ok {
		t.Fatalf("latest_invoice = %v, want an expanded invoice", sub["latest_invoice"])
	}
	assertEmptyArray(t, "first subscription invoice", first, "discounts")

	items := sub["items"].(map[string]any)["data"].([]any)
	itemID := items[0].(map[string]any)["id"].(string)
	price2 := newMonthlyPrice(t, tc, "2000")
	resp = stripePost(tc, "/v1/subscriptions/"+sub["id"].(string)+"?items[0][id]="+itemID+
		"&items[0][price]="+price2+"&proration_behavior=always_invoice&expand[0]=latest_invoice", nil)
	resp.AssertStatus(200)
	proration, ok := resp.JSONMap()["latest_invoice"].(map[string]any)
	if !ok {
		t.Fatalf("latest_invoice after proration = %v, want an expanded invoice", resp.JSONMap()["latest_invoice"])
	}
	assertEmptyArray(t, "proration invoice", proration, "discounts")
}

// Webhook deliveries are encoded from the store types and never pass through
// the response fill, so the empty lines list must already be [] there.
func TestEmptyLists_InvoiceCreatedWebhook(t *testing.T) {
	receiver := &webhookReceiver{}
	_, tc := setupStripeWithWebhooks(t, receiver)
	cus := newCustomer(t, tc)
	stripePost(tc, "/v1/invoices?customer="+cus, nil).AssertStatus(200)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for i := 0; i < receiver.count(); i++ {
			ev, _ := receiver.get(i)
			if ev["type"] != "invoice.created" {
				continue
			}
			obj := ev["data"].(map[string]any)["object"].(map[string]any)
			assertEmptyArray(t, "invoice.created", obj, "discounts")
			assertEmptyArray(t, "invoice.created lines", invoiceLines(t, "invoice.created", obj), "data")
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no invoice.created delivery received")
}

// A list response is read the same way whatever it holds: an empty top-level
// list carries data [].
func TestEmptyLists_TopLevelLists(t *testing.T) {
	srv, _ := setupStripe(t)
	for _, path := range []string{"/v1/invoices", "/v1/subscriptions", "/v1/customers"} {
		req, _ := http.NewRequest("GET", srv.URL+path, strings.NewReader(""))
		req.Header.Set("Authorization", "Bearer sk_test_sim_123")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		assertEmptyArray(t, "GET "+path, m, "data")
	}
}
