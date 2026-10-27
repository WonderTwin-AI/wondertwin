package api_test

import "testing"

func TestDataRoutesRefuseAnItemLinkedWithoutTheirProduct(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	authOnly := linkedWithHook(tc, []string{"auth"}, "")
	txOnly := linkedWithHook(tc, []string{"transactions"}, "")

	for _, c := range []struct {
		route, token string
		body         map[string]any
	}{
		{"/transactions/sync", authOnly, nil},
		{"/transactions/get", authOnly, map[string]any{"start_date": "2020-01-01", "end_date": "2030-01-01"}},
		{"/transactions/refresh", authOnly, nil},
		{"/transactions/recurring/get", authOnly, nil},
		{"/identity/get", authOnly, nil},
		{"/identity/match", authOnly, map[string]any{"user": map[string]any{"legal_name": "A B"}}},
		{"/auth/get", txOnly, nil},
	} {
		body := map[string]any{"access_token": c.token}
		for k, v := range c.body {
			body[k] = v
		}
		m := tc.Post(c.route, creds(body)).AssertStatus(400).JSONMap()
		if m["error_code"] != "PRODUCT_NOT_ENABLED" || m["error_type"] != "ITEM_ERROR" {
			t.Errorf("%s: %v, want ITEM_ERROR PRODUCT_NOT_ENABLED", c.route, m)
		}
	}

	// Routes that need no product, and a product the Item has, are served.
	tc.Post("/accounts/get", creds(map[string]any{"access_token": authOnly})).AssertStatus(200)
	tc.Post("/accounts/balance/get", creds(map[string]any{"access_token": txOnly})).AssertStatus(200)
	tc.Post("/auth/get", creds(map[string]any{"access_token": authOnly})).AssertStatus(200)
	tc.Post("/transactions/sync", creds(map[string]any{"access_token": txOnly})).AssertStatus(200)
}

func TestSandboxPublicTokenCreateRefusesAnUnknownInstitution(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	m := tc.Post("/sandbox/public_token/create", creds(map[string]any{
		"institution_id": "ins_nope", "initial_products": []string{"auth"}})).AssertStatus(400).JSONMap()
	if m["error_code"] != "INVALID_INSTITUTION" || m["error_type"] != "INVALID_INPUT" {
		t.Fatalf("got %v, want INVALID_INPUT INVALID_INSTITUTION", m)
	}
}
