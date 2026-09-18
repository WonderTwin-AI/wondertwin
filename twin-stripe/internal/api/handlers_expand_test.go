package api_test

import "testing"

// --- expand[] Tests ---
//
// The recursive path-walking algorithm itself is covered exhaustively by
// twinkit/expand's own unit tests. These tests exist to prove the
// middleware + resolver wiring works end-to-end through real HTTP
// requests: query parsing, the buffered response rewrite, and ID
// resolution against the live store.

func TestExpand_SingleLevelOnRetrieve(t *testing.T) {
	_, tc := setupStripe(t)

	cust := stripePostWithParams(tc, "/v1/customers", map[string]string{
		"email": "vlad@example.com",
	})
	cust.AssertStatus(200)
	custID := cust.JSONMap()["id"].(string)

	charge := stripePostWithParams(tc, "/v1/charges", map[string]string{
		"amount":   "2500",
		"currency": "usd",
		"customer": custID,
	})
	charge.AssertStatus(200)
	chargeID := charge.JSONMap()["id"].(string)

	// Without expand[], customer stays a plain ID string.
	resp := stripeGet(tc, "/v1/charges/"+chargeID)
	resp.AssertStatus(200)
	if _, isString := resp.JSONMap()["customer"].(string); !isString {
		t.Fatalf("expected unexpanded customer to be a string, got %T: %v", resp.JSONMap()["customer"], resp.JSONMap()["customer"])
	}

	// With expand[]=customer, it becomes the full object.
	resp = stripeGet(tc, "/v1/charges/"+chargeID+"?expand[]=customer")
	resp.AssertStatus(200)
	cust2, ok := resp.JSONMap()["customer"].(map[string]any)
	if !ok {
		t.Fatalf("expected expanded customer object, got %T: %v", resp.JSONMap()["customer"], resp.JSONMap()["customer"])
	}
	if cust2["id"] != custID {
		t.Errorf("expected expanded customer id=%s, got %v", custID, cust2["id"])
	}
	if cust2["email"] != "vlad@example.com" {
		t.Errorf("expected expanded customer email, got %v", cust2["email"])
	}
}

func TestExpand_ListDataPrefix(t *testing.T) {
	_, tc := setupStripe(t)

	cust := stripePostWithParams(tc, "/v1/customers", map[string]string{"email": "list@example.com"})
	cust.AssertStatus(200)
	custID := cust.JSONMap()["id"].(string)

	for range 2 {
		resp := stripePostWithParams(tc, "/v1/charges", map[string]string{
			"amount":   "1000",
			"currency": "usd",
			"customer": custID,
		})
		resp.AssertStatus(200)
	}

	resp := stripeGet(tc, "/v1/charges?expand[]=data.customer")
	resp.AssertStatus(200)

	data, ok := resp.JSONMap()["data"].([]any)
	if !ok || len(data) != 2 {
		t.Fatalf("expected 2 charges in data, got %T: %v", resp.JSONMap()["data"], resp.JSONMap()["data"])
	}
	for _, item := range data {
		ch, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("expected charge object in data, got %T", item)
		}
		expandedCust, ok := ch["customer"].(map[string]any)
		if !ok {
			t.Fatalf("expected expanded customer on list item, got %T: %v", ch["customer"], ch["customer"])
		}
		if expandedCust["id"] != custID {
			t.Errorf("expected expanded customer id=%s, got %v", custID, expandedCust["id"])
		}
	}
}

func TestExpand_DeepNestedThroughCheckoutSession(t *testing.T) {
	_, tc := setupStripe(t)

	cust := stripePostWithParams(tc, "/v1/customers", map[string]string{"email": "deep@example.com"})
	cust.AssertStatus(200)
	custID := cust.JSONMap()["id"].(string)

	cs := stripePostWithParams(tc, "/v1/checkout/sessions", map[string]string{
		"mode":         "payment",
		"amount_total": "5000",
		"currency":     "usd",
		"customer":     custID,
	})
	cs.AssertStatus(200)
	csID := cs.JSONMap()["id"].(string)

	complete := tc.Post("/admin/checkout/sessions/"+csID+"/complete", nil)
	complete.AssertStatus(200)

	resp := stripeGet(tc, "/v1/checkout/sessions/"+csID+"?expand[]=payment_intent.customer")
	resp.AssertStatus(200)

	pi, ok := resp.JSONMap()["payment_intent"].(map[string]any)
	if !ok {
		t.Fatalf("expected expanded payment_intent object, got %T: %v", resp.JSONMap()["payment_intent"], resp.JSONMap()["payment_intent"])
	}
	if pi["object"] != "payment_intent" {
		t.Errorf("expected payment_intent object type, got %v", pi["object"])
	}
	nestedCust, ok := pi["customer"].(map[string]any)
	if !ok {
		t.Fatalf("expected payment_intent.customer expanded, got %T: %v", pi["customer"], pi["customer"])
	}
	if nestedCust["id"] != custID {
		t.Errorf("expected nested customer id=%s, got %v", custID, nestedCust["id"])
	}
}

func TestExpand_UnknownPathLeftHarmless(t *testing.T) {
	_, tc := setupStripe(t)

	charge := stripePostWithParams(tc, "/v1/charges", map[string]string{
		"amount":   "500",
		"currency": "usd",
	})
	charge.AssertStatus(200)
	chargeID := charge.JSONMap()["id"].(string)

	// expand[] on a field the charge doesn't have set should not error.
	resp := stripeGet(tc, "/v1/charges/"+chargeID+"?expand[]=customer&expand[]=balance_transaction")
	resp.AssertStatus(200)
	if resp.JSONMap()["id"] != chargeID {
		t.Errorf("expected charge response unaffected by unresolvable expand paths, got %v", resp.JSONMap())
	}
}
