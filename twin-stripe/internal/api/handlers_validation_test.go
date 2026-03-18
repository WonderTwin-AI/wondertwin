package api_test

import (
	"testing"
)

func TestAttachPaymentMethodInvalidCustomer(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_methods/"+pmID+"/attach?customer=cus_nonexistent", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("No such customer")
}

func TestAttachPaymentMethodValidCustomer(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=Valid", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_methods/"+pmID+"/attach?customer="+custID, nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["customer"] != custID {
		t.Errorf("expected customer=%s, got %v", custID, resp.JSONMap()["customer"])
	}
}

func TestDeleteCustomerCancelsSubscriptions(t *testing.T) {
	_, tc := setupStripe(t)

	// Setup customer + subscription.
	resp := stripePost(tc, "/v1/customers?name=CascadeCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/products?name=CascProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=1000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID, nil)
	resp.AssertStatus(200)
	subID := resp.JSONMap()["id"].(string)

	// Delete customer.
	tc.DoWithHeaders("DELETE", "/v1/customers/"+custID, nil, map[string]string{
		"Authorization": "Bearer sk_test_sim_123",
	}).AssertStatus(200)

	// Subscription should be canceled.
	resp = stripeGet(tc, "/v1/subscriptions/"+subID)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "canceled" {
		t.Errorf("expected sub status=canceled after customer delete, got %v", resp.JSONMap()["status"])
	}
}

func TestDeleteCustomerDetachesPaymentMethods(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=DetachCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/payment_methods/"+pmID+"/attach?customer="+custID, nil).AssertStatus(200)

	// Delete customer.
	tc.DoWithHeaders("DELETE", "/v1/customers/"+custID, nil, map[string]string{
		"Authorization": "Bearer sk_test_sim_123",
	}).AssertStatus(200)

	// PM should be detached (no customer).
	resp = stripeGet(tc, "/v1/payment_methods/"+pmID)
	resp.AssertStatus(200)
	cust := resp.JSONMap()["customer"]
	if cust != nil && cust != "" {
		t.Errorf("expected PM customer to be empty after customer delete, got %v", cust)
	}
}

func TestCreateSubscriptionInvalidCustomer(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=ValProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=1000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer=cus_nonexistent&items[0][price]="+priceID, nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("No such customer")
}
