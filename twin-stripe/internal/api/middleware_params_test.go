package api_test

import (
	"net/url"
	"testing"
)

// --- Indexed array parameter tests ---

func TestArrayParams_IndexedExpandOnRetrieve(t *testing.T) {
	_, tc := setupStripe(t)

	cust := stripePostWithParams(tc, "/v1/customers", map[string]string{"email": "idx@example.com"})
	cust.AssertStatus(200)
	custID := cust.JSONMap()["id"].(string)
	ch := stripePostWithParams(tc, "/v1/charges", map[string]string{"amount": "100", "currency": "usd", "customer": custID})
	ch.AssertStatus(200)
	chID := ch.JSONMap()["id"].(string)

	resp := stripeGet(tc, "/v1/charges/"+chID+"?expand%5B0%5D=customer")
	resp.AssertStatus(200)
	obj, ok := resp.JSONMap()["customer"].(map[string]any)
	if !ok || obj["id"] != custID {
		t.Fatalf("expand[0]=customer not applied: %v", resp.JSONMap()["customer"])
	}
}

func TestArrayParams_IndexedEnabledEventsInBody(t *testing.T) {
	_, tc := setupStripe(t)

	form := url.Values{}
	form.Set("url", "https://example.com/hook")
	form.Set("enabled_events[1]", "customer.updated")
	form.Set("enabled_events[0]", "customer.created")
	resp := stripeForm(t, tc, "POST", "/v1/webhook_endpoints", form, nil).assertStatus(200)
	events, _ := resp.json()["enabled_events"].([]any)
	if len(events) != 2 || events[0] != "customer.created" || events[1] != "customer.updated" {
		t.Fatalf("expected enabled_events in index order, got %v", events)
	}
}

func TestArrayParams_BracketFormStillWorks(t *testing.T) {
	_, tc := setupStripe(t)

	form := url.Values{}
	form.Set("url", "https://example.com/hook")
	form.Add("enabled_events[]", "charge.succeeded")
	resp := stripeForm(t, tc, "POST", "/v1/webhook_endpoints", form, nil).assertStatus(200)
	events, _ := resp.json()["enabled_events"].([]any)
	if len(events) != 1 || events[0] != "charge.succeeded" {
		t.Fatalf("expected enabled_events [charge.succeeded], got %v", events)
	}
}
