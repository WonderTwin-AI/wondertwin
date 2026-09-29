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
