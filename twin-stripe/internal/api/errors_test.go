package api_test

import (
	"net/url"
	"testing"
)

// --- Stripe error shape tests ---

func errorOf(t *testing.T, r formResp) map[string]any {
	t.Helper()
	e, ok := r.json()["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected an error envelope, got %s", r.body)
	}
	return e
}

func TestError_ResourceMissingHasParamAndDocURL(t *testing.T) {
	_, tc := setupStripe(t)
	e := errorOf(t, stripeForm(t, tc, "GET", "/v1/customers/cus_nope", nil, nil).assertStatus(404))
	if e["code"] != "resource_missing" || e["param"] != "id" ||
		e["doc_url"] != "https://stripe.com/docs/error-codes/resource-missing" ||
		e["message"] != "No such customer: 'cus_nope'" {
		t.Fatalf("unexpected resource_missing error: %v", e)
	}
}

func TestError_ParameterMissingNamesTheParam(t *testing.T) {
	_, tc := setupStripe(t)
	e := errorOf(t, stripeForm(t, tc, "POST", "/v1/payment_intents", url.Values{"currency": {"usd"}}, nil).assertStatus(400))
	if e["code"] != "parameter_missing" || e["param"] != "amount" || e["message"] != "Missing required param: amount." {
		t.Fatalf("unexpected parameter_missing error: %v", e)
	}
}

func TestError_InvalidAPIKey(t *testing.T) {
	_, tc := setupStripe(t)
	r := stripeForm(t, tc, "GET", "/v1/customers", nil, map[string]string{"Authorization": "Bearer sk_live_invalid000"}).assertStatus(401)
	e := errorOf(t, r)
	if e["message"] != "Invalid API Key provided: sk_live_******d000" || e["type"] != "invalid_request_error" {
		t.Fatalf("unexpected invalid-key error: %v", e)
	}
	if _, hasCode := e["code"]; hasCode {
		t.Fatalf("expected no code on the invalid-key error, got %v", e)
	}
}

func TestError_BasicAuthKeyAccepted(t *testing.T) {
	_, tc := setupStripe(t)
	// curl -u sk_test_...: sends the key as the Basic user name.
	stripeForm(t, tc, "GET", "/v1/customers", nil, map[string]string{"Authorization": "Basic c2tfdGVzdF9hYmM6"}).assertStatus(200)
}

func TestError_UnrecognizedURLIsJSON(t *testing.T) {
	_, tc := setupStripe(t)
	e := errorOf(t, stripeForm(t, tc, "GET", "/v1/not_a_resource", nil, nil).assertStatus(404))
	want := "Unrecognized request URL (GET: /v1/not_a_resource). Please see https://stripe.com/docs or we can help at https://support.stripe.com/."
	if e["message"] != want || e["type"] != "invalid_request_error" {
		t.Fatalf("unexpected unrecognized-URL error: %v", e)
	}
}

// --- Stripe-Version tests ---

func TestVersion_DahliaAccepted(t *testing.T) {
	_, tc := setupStripe(t)
	for _, v := range []string{"2026-08-26.dahlia", "2026-03-25.dahlia"} {
		stripeForm(t, tc, "GET", "/v1/customers", nil, map[string]string{"Stripe-Version": v}).assertStatus(200)
	}
}

func TestVersion_OtherReleasesRefused(t *testing.T) {
	_, tc := setupStripe(t)
	for _, v := range []string{"2025-09-30.clover", "2024-06-20", "2026-08-26.preview", "2026-09-30.endive"} {
		e := errorOf(t, stripeForm(t, tc, "GET", "/v1/customers", nil, map[string]string{"Stripe-Version": v}).assertStatus(400))
		if e["type"] != "invalid_request_error" {
			t.Fatalf("%s: expected invalid_request_error, got %v", v, e)
		}
	}
}

func TestVersion_EventsLabelledWithAccountDefault(t *testing.T) {
	_, tc := setupStripe(t)
	stripeForm(t, tc, "POST", "/v1/customers", url.Values{"email": {"v@example.com"}}, nil).assertStatus(200)
	resp := stripeForm(t, tc, "GET", "/v1/events", url.Values{"type": {"customer.created"}}, nil).assertStatus(200).json()
	evt := resp["data"].([]any)[0].(map[string]any)
	if evt["api_version"] != "2026-08-26.dahlia" {
		t.Fatalf("expected event api_version 2026-08-26.dahlia, got %v", evt["api_version"])
	}
}

func TestVersion_WebhookEndpointLimitedToDahlia(t *testing.T) {
	_, tc := setupStripe(t)
	form := url.Values{"url": {"https://example.com/h"}, "enabled_events[0]": {"*"}, "api_version": {"2025-09-30.clover"}}
	e := errorOf(t, stripeForm(t, tc, "POST", "/v1/webhook_endpoints", form, nil).assertStatus(400))
	if e["param"] != "api_version" {
		t.Fatalf("expected param api_version, got %v", e)
	}
	form.Set("api_version", "2026-07-29.dahlia")
	we := stripeForm(t, tc, "POST", "/v1/webhook_endpoints", form, nil).assertStatus(200).json()
	if we["api_version"] != "2026-07-29.dahlia" {
		t.Fatalf("expected the endpoint to keep its dahlia api_version, got %v", we["api_version"])
	}
}

func TestVersion_V2NotServed(t *testing.T) {
	_, tc := setupStripe(t)
	stripeForm(t, tc, "GET", "/v2/core/event_destinations", nil, nil).assertStatus(404)
}
