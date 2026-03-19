package api_test

import (
	"testing"
)

func TestCreateDisputeViaAdmin(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a charge.
	resp := stripePost(tc, "/v1/charges?amount=5000&currency=usd", nil)
	resp.AssertStatus(200)
	chargeID := resp.JSONMap()["id"].(string)

	// Create dispute via admin.
	resp = tc.Post("/admin/disputes?charge="+chargeID+"&reason=fraudulent", nil)
	resp.AssertStatus(200)
	dp := resp.JSONMap()

	if dp["object"] != "dispute" {
		t.Errorf("expected object=dispute, got %v", dp["object"])
	}
	if dp["status"] != "needs_response" {
		t.Errorf("expected status=needs_response, got %v", dp["status"])
	}
	if dp["amount"].(float64) != 5000 {
		t.Errorf("expected amount=5000, got %v", dp["amount"])
	}
	if dp["charge"] != chargeID {
		t.Errorf("expected charge=%s, got %v", chargeID, dp["charge"])
	}
	if dp["reason"] != "fraudulent" {
		t.Errorf("expected reason=fraudulent, got %v", dp["reason"])
	}

	// Charge should be marked disputed.
	resp = stripeGet(tc, "/v1/charges/"+chargeID)
	resp.AssertStatus(200)
	if resp.JSONMap()["disputed"] != true {
		t.Error("expected charge disputed=true")
	}
}

func TestDisputeDebitsBalance(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a charge to fund the balance.
	stripePost(tc, "/v1/charges?amount=8000&currency=usd", nil).AssertStatus(200)

	resp := stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	before := getAvailableBalance(t, resp.JSONMap(), "usd")

	// Create another charge then dispute it.
	resp = stripePost(tc, "/v1/charges?amount=3000&currency=usd", nil)
	resp.AssertStatus(200)
	chargeID := resp.JSONMap()["id"].(string)

	tc.Post("/admin/disputes?charge="+chargeID, nil).AssertStatus(200)

	resp = stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	after := getAvailableBalance(t, resp.JSONMap(), "usd")

	// Balance should have increased by 3000 (charge) then decreased by 3000 (dispute).
	if after != before+3000-3000 {
		t.Errorf("expected balance=%v, got %v (before=%v)", before, after, before)
	}
}

func TestSubmitDisputeEvidence(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/charges?amount=2000&currency=usd", nil)
	resp.AssertStatus(200)
	chargeID := resp.JSONMap()["id"].(string)

	resp = tc.Post("/admin/disputes?charge="+chargeID, nil)
	resp.AssertStatus(200)
	dpID := resp.JSONMap()["id"].(string)

	// Submit evidence.
	resp = stripePost(tc, "/v1/disputes/"+dpID+"?evidence[uncategorized_text]=Customer+received+goods&submit=true", nil)
	resp.AssertStatus(200)
	dp := resp.JSONMap()

	if dp["status"] != "under_review" {
		t.Errorf("expected status=under_review after submit, got %v", dp["status"])
	}
	evidence, ok := dp["evidence"].(map[string]any)
	if !ok {
		t.Fatal("expected evidence object")
	}
	if evidence["uncategorized_text"] != "Customer received goods" {
		t.Errorf("expected evidence text, got %v", evidence["uncategorized_text"])
	}
}

func TestCloseDisputeAcceptsLoss(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/charges?amount=1000&currency=usd", nil)
	resp.AssertStatus(200)
	chargeID := resp.JSONMap()["id"].(string)

	resp = tc.Post("/admin/disputes?charge="+chargeID, nil)
	resp.AssertStatus(200)
	dpID := resp.JSONMap()["id"].(string)

	// Close (accept loss).
	resp = stripePost(tc, "/v1/disputes/"+dpID+"/close", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "lost" {
		t.Errorf("expected status=lost, got %v", resp.JSONMap()["status"])
	}

	// Closing again should fail.
	resp = stripePost(tc, "/v1/disputes/"+dpID+"/close", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("dispute_already_closed")
}

func TestResolveDisputeWonRecreditsBalance(t *testing.T) {
	_, tc := setupStripe(t)

	// Fund balance first.
	stripePost(tc, "/v1/charges?amount=10000&currency=usd", nil).AssertStatus(200)

	resp := stripePost(tc, "/v1/charges?amount=4000&currency=usd", nil)
	resp.AssertStatus(200)
	chargeID := resp.JSONMap()["id"].(string)

	tc.Post("/admin/disputes?charge="+chargeID, nil).AssertStatus(200)

	resp = stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	afterDispute := getAvailableBalance(t, resp.JSONMap(), "usd")

	// List disputes to find the ID.
	resp = stripeGet(tc, "/v1/disputes")
	resp.AssertStatus(200)
	disputes := resp.JSONMap()["data"].([]any)
	dpID := disputes[0].(map[string]any)["id"].(string)

	// Resolve as won.
	resp = tc.Post("/admin/disputes/"+dpID+"/resolve?outcome=won", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "won" {
		t.Errorf("expected status=won, got %v", resp.JSONMap()["status"])
	}

	resp = stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	afterWon := getAvailableBalance(t, resp.JSONMap(), "usd")

	if afterWon-afterDispute != 4000 {
		t.Errorf("expected balance re-credited by 4000, got %v", afterWon-afterDispute)
	}
}

func TestUpdateClosedDisputeFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/charges?amount=1000&currency=usd", nil)
	resp.AssertStatus(200)
	chargeID := resp.JSONMap()["id"].(string)

	resp = tc.Post("/admin/disputes?charge="+chargeID, nil)
	resp.AssertStatus(200)
	dpID := resp.JSONMap()["id"].(string)

	// Close it.
	stripePost(tc, "/v1/disputes/"+dpID+"/close", nil).AssertStatus(200)

	// Updating a closed dispute should fail.
	resp = stripePost(tc, "/v1/disputes/"+dpID+"?evidence[uncategorized_text]=too+late", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("dispute_not_updatable")
}
