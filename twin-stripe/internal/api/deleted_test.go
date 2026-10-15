package api_test

import (
	"testing"
)

func TestRetrieveDeletedCustomerReturnsStub(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?email=a@example.com", nil)
	resp.AssertStatus(200)
	id := resp.JSONMap()["id"].(string)

	resp = tc.DoWithHeaders("DELETE", "/v1/customers/"+id, nil, map[string]string{"Authorization": "Bearer sk_test_sim_123"})
	resp.AssertStatus(200)

	resp = stripeGet(tc, "/v1/customers/"+id)
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["id"] != id || m["object"] != "customer" || m["deleted"] != true || len(m) != 3 {
		t.Errorf("retrieve after delete = %v, want {id, object: customer, deleted: true}", m)
	}

	// A deleted customer is gone from lists and cannot be deleted again.
	resp = stripeGet(tc, "/v1/customers")
	resp.AssertStatus(200)
	if data := resp.JSONMap()["data"].([]any); len(data) != 0 {
		t.Errorf("list after delete = %v, want empty", data)
	}
	resp = tc.DoWithHeaders("DELETE", "/v1/customers/"+id, nil, map[string]string{"Authorization": "Bearer sk_test_sim_123"})
	resp.AssertStatus(404)

	// A customer that never existed is still a 404.
	stripeGet(tc, "/v1/customers/cus_missing").AssertStatus(404)
}

func TestDeletedCustomerSurvivesSnapshot(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers", nil)
	resp.AssertStatus(200)
	id := resp.JSONMap()["id"].(string)
	tc.DoWithHeaders("DELETE", "/v1/customers/"+id, nil, map[string]string{"Authorization": "Bearer sk_test_sim_123"}).AssertStatus(200)

	state := tc.Get("/admin/state")
	state.AssertStatus(200)
	tc.Post("/admin/reset", nil).AssertStatus(200)
	stripeGet(tc, "/v1/customers/"+id).AssertStatus(404)
	tc.Post("/admin/state", state.JSONMap()).AssertStatus(200)
	resp = stripeGet(tc, "/v1/customers/"+id)
	resp.AssertStatus(200)
	if resp.JSONMap()["deleted"] != true {
		t.Errorf("deleted stub lost across a state round-trip: %s", resp.Body)
	}
}
