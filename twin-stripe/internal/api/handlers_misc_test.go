package api_test

import (
	"testing"
)

func TestGetReview404(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripeGet(tc, "/v1/reviews/prv_nonexistent")
	resp.AssertStatus(404)
	resp.AssertBodyContains("resource_missing")
}

func TestApproveReview404(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/reviews/prv_nonexistent/approve", nil)
	resp.AssertStatus(404)
	resp.AssertBodyContains("resource_missing")
}

func TestApproveReview(t *testing.T) {
	_, tc := setupStripe(t)

	// Seed a review via admin state load
	stateBody := map[string]any{
		"reviews": map[string]any{
			"prv_test1": map[string]any{
				"id":            "prv_test1",
				"object":        "review",
				"charge":        "ch_123",
				"status":        "open",
				"opened_reason": "rule",
				"created":       1700000000,
			},
		},
	}
	resp := tc.Post("/admin/state", stateBody)
	resp.AssertStatus(200)

	// Verify the review exists
	resp = stripeGet(tc, "/v1/reviews/prv_test1")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["status"] != "open" {
		t.Errorf("expected status=open, got %v", m["status"])
	}

	// Approve the review
	resp = stripePost(tc, "/v1/reviews/prv_test1/approve", nil)
	resp.AssertStatus(200)
	m = resp.JSONMap()
	if m["status"] != "closed" {
		t.Errorf("expected status=closed, got %v", m["status"])
	}
	if m["closed_reason"] != "approved" {
		t.Errorf("expected closed_reason=approved, got %v", m["closed_reason"])
	}
}

func TestListReviews(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripeGet(tc, "/v1/reviews")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["object"] != "list" {
		t.Errorf("expected object=list, got %v", m["object"])
	}
}

func TestCreateAndGetTaxID(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a customer first
	resp := stripePost(tc, "/v1/customers", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create tax ID using query params
	resp = stripePostWithParams(tc, "/v1/customers/"+custID+"/tax_ids", map[string]string{
		"type":  "eu_vat",
		"value": "DE123456789",
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	taxIDID := m["id"].(string)
	if m["object"] != "tax_id" {
		t.Errorf("expected object=tax_id, got %v", m["object"])
	}
	if m["type"] != "eu_vat" {
		t.Errorf("expected type=eu_vat, got %v", m["type"])
	}
	if m["value"] != "DE123456789" {
		t.Errorf("expected value=DE123456789, got %v", m["value"])
	}
	if m["customer"] != custID {
		t.Errorf("expected customer=%s, got %v", custID, m["customer"])
	}
	verification, ok := m["verification"].(map[string]any)
	if !ok {
		t.Fatal("expected verification object")
	}
	if verification["status"] != "pending" {
		t.Errorf("expected verification.status=pending, got %v", verification["status"])
	}

	// Get tax ID via customer-scoped endpoint
	resp = stripeGet(tc, "/v1/customers/"+custID+"/tax_ids/"+taxIDID)
	resp.AssertStatus(200)
	m = resp.JSONMap()
	if m["id"] != taxIDID {
		t.Errorf("expected id=%s, got %v", taxIDID, m["id"])
	}

	// Get tax ID via top-level endpoint
	resp = stripeGet(tc, "/v1/tax_ids/"+taxIDID)
	resp.AssertStatus(200)
	m = resp.JSONMap()
	if m["id"] != taxIDID {
		t.Errorf("expected id=%s, got %v", taxIDID, m["id"])
	}
}

func TestDeleteTaxID(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a customer and tax ID
	resp := stripePost(tc, "/v1/customers", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePostWithParams(tc, "/v1/customers/"+custID+"/tax_ids", map[string]string{
		"type":  "us_ein",
		"value": "12-3456789",
	})
	resp.AssertStatus(200)
	taxIDID := resp.JSONMap()["id"].(string)

	// Delete tax ID
	resp = tc.DoWithHeaders("DELETE", "/v1/customers/"+custID+"/tax_ids/"+taxIDID, nil, map[string]string{
		"Authorization": "Bearer sk_test_sim_123",
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["deleted"] != true {
		t.Errorf("expected deleted=true, got %v", m["deleted"])
	}

	// Verify it's gone
	resp = stripeGet(tc, "/v1/tax_ids/"+taxIDID)
	resp.AssertStatus(404)
}

func TestListCustomerTaxIDs(t *testing.T) {
	_, tc := setupStripe(t)

	// Create two customers
	resp := stripePost(tc, "/v1/customers", nil)
	resp.AssertStatus(200)
	custID1 := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers", nil)
	resp.AssertStatus(200)
	custID2 := resp.JSONMap()["id"].(string)

	// Create tax IDs for customer 1
	stripePostWithParams(tc, "/v1/customers/"+custID1+"/tax_ids", map[string]string{
		"type": "eu_vat", "value": "DE111111111",
	}).AssertStatus(200)
	stripePostWithParams(tc, "/v1/customers/"+custID1+"/tax_ids", map[string]string{
		"type": "eu_vat", "value": "DE222222222",
	}).AssertStatus(200)

	// Create tax ID for customer 2
	stripePostWithParams(tc, "/v1/customers/"+custID2+"/tax_ids", map[string]string{
		"type": "us_ein", "value": "99-8765432",
	}).AssertStatus(200)

	// List customer 1 tax IDs - should have 2
	resp = stripeGet(tc, "/v1/customers/"+custID1+"/tax_ids")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	data := m["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 tax IDs for customer 1, got %d", len(data))
	}

	// List customer 2 tax IDs - should have 1
	resp = stripeGet(tc, "/v1/customers/"+custID2+"/tax_ids")
	resp.AssertStatus(200)
	m = resp.JSONMap()
	data = m["data"].([]any)
	if len(data) != 1 {
		t.Errorf("expected 1 tax ID for customer 2, got %d", len(data))
	}

	// Top-level list should have all 3
	resp = stripeGet(tc, "/v1/tax_ids")
	resp.AssertStatus(200)
	m = resp.JSONMap()
	data = m["data"].([]any)
	if len(data) != 3 {
		t.Errorf("expected 3 total tax IDs, got %d", len(data))
	}
}

func TestCreateTaxIDRequiresParams(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a customer
	resp := stripePost(tc, "/v1/customers", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Missing type and value
	resp = stripePost(tc, "/v1/customers/"+custID+"/tax_ids", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("parameter_missing")
}

func TestCreateTaxIDCustomerNotFound(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePostWithParams(tc, "/v1/customers/cus_nonexistent/tax_ids", map[string]string{
		"type": "eu_vat", "value": "DE123456789",
	})
	resp.AssertStatus(404)
	resp.AssertBodyContains("resource_missing")
}
