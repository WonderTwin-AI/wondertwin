package api_test

import (
	"testing"
)

// assertNullFields fails for each of nulls that is absent from m or not null.
func assertNullFields(t *testing.T, what string, m map[string]any, nulls ...string) {
	t.Helper()
	for _, k := range nulls {
		v, ok := m[k]
		if !ok {
			t.Errorf("%s: %s is omitted; Stripe returns it as null", what, k)
		} else if v != nil {
			t.Errorf("%s: %s = %v, want null", what, k, v)
		}
	}
}

func assertEmptyMetadata(t *testing.T, what string, m map[string]any) {
	t.Helper()
	md, ok := m["metadata"].(map[string]any)
	if !ok || len(md) != 0 {
		t.Errorf("%s: metadata = %v, want {}", what, m["metadata"])
	}
}

func TestUnsetFieldsAreReturnedAsStripeReturnsThem(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers", nil)
	resp.AssertStatus(200)
	cus := resp.JSONMap()
	assertNullFields(t, "customer", cus, "email", "name", "phone", "description",
		"address", "shipping", "currency", "default_source", "discount", "test_clock")
	assertEmptyMetadata(t, "customer", cus)
	settings, ok := cus["invoice_settings"].(map[string]any)
	if !ok {
		t.Fatalf("customer: invoice_settings = %v, want an object", cus["invoice_settings"])
	}
	assertNullFields(t, "customer.invoice_settings", settings,
		"custom_fields", "default_payment_method", "footer", "rendering_options")
	for _, k := range []string{"sources", "subscriptions", "tax_ids", "cash_balance"} {
		if _, ok := cus[k]; ok {
			t.Errorf("customer: includable %s is returned without expand", k)
		}
	}

	resp = stripePost(tc, "/v1/payment_intents?amount=500&currency=usd", nil)
	resp.AssertStatus(200)
	pi := resp.JSONMap()
	assertNullFields(t, "payment_intent", pi, "customer", "payment_method",
		"last_payment_error", "latest_charge", "description", "next_action", "shipping")
	assertEmptyMetadata(t, "payment_intent", pi)

	// Read back, and in a list, the same way.
	resp = stripeGet(tc, "/v1/payment_intents/"+pi["id"].(string))
	resp.AssertStatus(200)
	assertNullFields(t, "retrieved payment_intent", resp.JSONMap(), "customer", "latest_charge")
	resp = stripeGet(tc, "/v1/customers")
	resp.AssertStatus(200)
	first := resp.JSONMap()["data"].([]any)[0].(map[string]any)
	assertNullFields(t, "listed customer", first, "email", "address")

	resp = stripePost(tc, "/v1/products?name=Widget", nil)
	resp.AssertStatus(200)
	prod := resp.JSONMap()
	assertEmptyMetadata(t, "product", prod)
	if imgs, ok := prod["images"].([]any); !ok || len(imgs) != 0 {
		t.Errorf("product: images = %v, want []", prod["images"])
	}
}

func TestEventObjectsCarryUnsetFields(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers", nil)
	resp.AssertStatus(200)

	resp = stripeGet(tc, "/v1/events?type=customer.created")
	resp.AssertStatus(200)
	evt := resp.JSONMap()["data"].([]any)[0].(map[string]any)
	obj := evt["data"].(map[string]any)["object"].(map[string]any)
	assertNullFields(t, "event customer", obj, "email", "name", "address")
	assertEmptyMetadata(t, "event customer", obj)
}

func TestSetFieldsAreKept(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?email=a@example.com&metadata[k]=v", nil)
	resp.AssertStatus(200)
	cus := resp.JSONMap()
	if cus["email"] != "a@example.com" {
		t.Errorf("email = %v", cus["email"])
	}
	if md := cus["metadata"].(map[string]any); md["k"] != "v" {
		t.Errorf("metadata = %v", md)
	}
}
