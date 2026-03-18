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
