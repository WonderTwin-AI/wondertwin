package api_test

import (
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

func TestRequestValuesAreValidated(t *testing.T) {
	_, tc := setupStripe(t)

	for _, c := range []struct {
		name, method, path, code, param string
	}{
		{"non-integer amount", "POST", "/v1/payment_intents?amount=abc&currency=usd", "parameter_invalid_integer", "amount"},
		{"negative amount", "POST", "/v1/payment_intents?amount=-5&currency=usd", "parameter_invalid_integer", "amount"},
		{"unknown parameter", "POST", "/v1/customers?bogus=1", "parameter_unknown", "bogus"},
		{"unknown nested parameter's top level", "POST", "/v1/customers?bogus[a]=1", "parameter_unknown", "bogus"},
		{"non-integer nested", "POST", "/v1/subscriptions?customer=cus_x&items[0][price]=p&items[0][quantity]=two", "parameter_invalid_integer", "items[0][quantity]"},
		{"non-integer limit", "GET", "/v1/customers?limit=abc", "parameter_invalid_integer", "limit"},
		{"limit above 100", "GET", "/v1/customers?limit=1000", "parameter_invalid_integer", "limit"},
		{"limit below 1", "GET", "/v1/customers?limit=0", "parameter_invalid_integer", "limit"},
		{"unknown list parameter", "GET", "/v1/customers?bogus=1", "parameter_unknown", "bogus"},
	} {
		var resp *testutil.Response
		if c.method == "GET" {
			resp = stripeGet(tc, c.path)
		} else {
			resp = stripePost(tc, c.path, nil)
		}
		resp.AssertStatus(400)
		m := resp.JSONMap()
		e, _ := m["error"].(map[string]any)
		if e == nil || e["type"] != "invalid_request_error" || e["code"] != c.code || e["param"] != c.param {
			t.Errorf("%s: got %v, want invalid_request_error %s on %s", c.name, m, c.code, c.param)
		}
	}
}

func TestValidRequestsStillPass(t *testing.T) {
	_, tc := setupStripe(t)

	stripePost(tc, "/v1/payment_intents?amount=500&currency=usd&expand[0]=customer", nil).AssertStatus(200)
	stripeGet(tc, "/v1/customers?limit=100&expand[]=data.default_source").AssertStatus(200)
	stripeGet(tc, "/v1/customers?limit=1").AssertStatus(200)
	// An empty value unsets an optional integer.
	stripePost(tc, "/v1/customers?balance=", nil).AssertStatus(200)
	// Nested parameters the spec documents pass.
	stripePost(tc, "/v1/payment_methods?type=card&card[number]=4242424242424242&card[exp_month]=12&card[exp_year]=2030", nil).AssertStatus(200)
}
