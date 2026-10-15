package api_test

import (
	"net/url"
	"strconv"
	"testing"
)

func fullMetadata() url.Values {
	v := url.Values{}
	for i := 0; i < 50; i++ {
		v.Set("metadata[k"+strconv.Itoa(i)+"]", "v")
	}
	return v
}

func TestRefusedAccountUpdateChangesNothing(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/accounts?"+fullMetadata().Encode(), nil)
	resp.AssertStatus(200)
	id := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/accounts/"+id+"?capabilities[transfers][requested]=true&business_profile[name]=Acme&metadata[new]=v", nil)
	resp.AssertStatus(400)

	acct := stripeGet(tc, "/v1/accounts/"+id).JSONMap()
	caps, _ := acct["capabilities"].(map[string]any)
	if _, ok := caps["transfers"]; ok {
		t.Errorf("refused update stored capability transfers: %v", caps)
	}
	profile, _ := acct["business_profile"].(map[string]any)
	if _, ok := profile["name"]; ok {
		t.Errorf("refused update stored business_profile name: %v", profile)
	}
	md := acct["metadata"].(map[string]any)
	if _, ok := md["new"]; ok || len(md) != 50 {
		t.Errorf("refused update changed metadata: %d keys, new=%v", len(md), md["new"])
	}
}

func TestRefusedExternalAccountUpdateChangesNothing(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/accounts", nil)
	resp.AssertStatus(200)
	acctID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/accounts/"+acctID+"/external_accounts", nil)
	resp.AssertStatus(200)
	eaID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/accounts/"+acctID+"/external_accounts/"+eaID+"?"+fullMetadata().Encode(), nil)
	resp.AssertStatus(200)

	resp = stripePost(tc, "/v1/accounts/"+acctID+"/external_accounts/"+eaID+"?metadata[new]=v", nil)
	resp.AssertStatus(400)

	md := stripeGet(tc, "/v1/accounts/"+acctID+"/external_accounts/"+eaID).JSONMap()["metadata"].(map[string]any)
	if _, ok := md["new"]; ok || len(md) != 50 {
		t.Errorf("refused update changed metadata: %d keys, new=%v", len(md), md["new"])
	}
}

func TestRefusedDisputeUpdateHasNoSideEffects(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/charges?amount=2000&currency=usd", nil)
	resp.AssertStatus(200)
	chargeID := resp.JSONMap()["id"].(string)

	resp = tc.Post("/admin/disputes?charge="+chargeID, nil)
	resp.AssertStatus(200)
	dpID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/disputes/"+dpID+"?"+fullMetadata().Encode(), nil).AssertStatus(200)

	updatedEvents := func() int {
		resp := stripeGet(tc, "/v1/events?type=charge.dispute.updated&limit=100")
		resp.AssertStatus(200)
		return len(resp.JSONMap()["data"].([]any))
	}
	before := updatedEvents()

	resp = stripePost(tc, "/v1/disputes/"+dpID+"?evidence[customer_name]=Jane&submit=true&metadata[new]=v", nil)
	resp.AssertStatus(400)

	dp := stripeGet(tc, "/v1/disputes/"+dpID).JSONMap()
	if dp["status"] != "needs_response" {
		t.Errorf("refused update changed status to %v", dp["status"])
	}
	if evidence, _ := dp["evidence"].(map[string]any); evidence["customer_name"] != nil {
		t.Errorf("refused update stored evidence: %v", evidence)
	}
	if after := updatedEvents(); after != before {
		t.Errorf("refused update emitted charge.dispute.updated: %d events before, %d after", before, after)
	}
}

func TestRefusedPaymentIntentUpdateCreatesNoPaymentMethod(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_intents?amount=2000&currency=usd&"+fullMetadata().Encode(), nil)
	resp.AssertStatus(200)
	id := resp.JSONMap()["id"].(string)

	paymentMethods := func() int {
		resp := stripeGet(tc, "/v1/payment_methods?limit=100")
		resp.AssertStatus(200)
		return len(resp.JSONMap()["data"].([]any))
	}
	before := paymentMethods()

	resp = stripePost(tc, "/v1/payment_intents/"+id+"?payment_method=pm_card_visa&metadata[new]=v", nil)
	resp.AssertStatus(400)

	if after := paymentMethods(); after != before {
		t.Errorf("refused update created payment methods: %d before, %d after", before, after)
	}
	if pi := stripeGet(tc, "/v1/payment_intents/"+id).JSONMap(); pi["payment_method"] != nil {
		t.Errorf("refused update set payment_method to %v", pi["payment_method"])
	}
}
