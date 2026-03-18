package api_test

import (
	"testing"
)

func TestGetApplicationFeeNotFound(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripeGet(tc, "/v1/application_fees/fee_nonexistent")
	resp.AssertStatus(404)
	resp.AssertBodyContains("resource_missing")
}

func TestListApplicationFees(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripeGet(tc, "/v1/application_fees")
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["object"] != "list" {
		t.Errorf("expected object=list, got %v", m["object"])
	}
	data, ok := m["data"].([]any)
	if !ok {
		t.Fatal("expected data array")
	}
	if len(data) != 0 {
		t.Errorf("expected 0 application fees, got %d", len(data))
	}
}

func TestCreateTransferReversal(t *testing.T) {
	_, tc := setupStripe(t)

	// Create destination account
	resp := stripePost(tc, "/v1/accounts", nil)
	resp.AssertStatus(200)
	acctID := resp.JSONMap()["id"].(string)

	// Fund the platform balance so the transfer can debit it
	tc.Post("/admin/accounts/platform/fund", map[string]any{
		"amount":   100000,
		"currency": "usd",
	})

	// Create a transfer
	resp = stripePostWithParams(tc, "/v1/transfers", map[string]string{
		"amount":      "50000",
		"currency":    "usd",
		"destination": acctID,
	})
	resp.AssertStatus(200)
	transferID := resp.JSONMap()["id"].(string)

	// Create a reversal
	resp = stripePostWithParams(tc, "/v1/transfers/"+transferID+"/reversals", map[string]string{
		"amount": "20000",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["object"] != "transfer_reversal" {
		t.Errorf("expected object=transfer_reversal, got %v", m["object"])
	}
	if m["transfer"] != transferID {
		t.Errorf("expected transfer=%s, got %v", transferID, m["transfer"])
	}
	if int64(m["amount"].(float64)) != 20000 {
		t.Errorf("expected amount=20000, got %v", m["amount"])
	}

	// Get the reversal
	reversalID := m["id"].(string)
	resp = stripeGet(tc, "/v1/transfers/"+transferID+"/reversals/"+reversalID)
	resp.AssertStatus(200)
	if resp.JSONMap()["id"] != reversalID {
		t.Errorf("expected id=%s, got %v", reversalID, resp.JSONMap()["id"])
	}

	// List reversals
	resp = stripeGet(tc, "/v1/transfers/"+transferID+"/reversals")
	resp.AssertStatus(200)
	data := resp.JSONMap()["data"].([]any)
	if len(data) != 1 {
		t.Errorf("expected 1 reversal, got %d", len(data))
	}

	// Verify parent transfer was updated
	resp = stripeGet(tc, "/v1/transfers/"+transferID)
	resp.AssertStatus(200)
	tr := resp.JSONMap()
	if int64(tr["amount_reversed"].(float64)) != 20000 {
		t.Errorf("expected amount_reversed=20000, got %v", tr["amount_reversed"])
	}
}

func TestCreateAccountLink(t *testing.T) {
	_, tc := setupStripe(t)

	// Create an account
	resp := stripePost(tc, "/v1/accounts", nil)
	resp.AssertStatus(200)
	acctID := resp.JSONMap()["id"].(string)

	// Create account link
	resp = stripePostWithParams(tc, "/v1/account_links", map[string]string{
		"account": acctID,
		"type":    "account_onboarding",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["object"] != "account_link" {
		t.Errorf("expected object=account_link, got %v", m["object"])
	}
	url, ok := m["url"].(string)
	if !ok || url == "" {
		t.Error("expected url to be set")
	}
	if m["expires_at"] == nil || m["expires_at"].(float64) == 0 {
		t.Error("expected expires_at to be set")
	}
}

func TestCreateAccountLinkMissingParams(t *testing.T) {
	_, tc := setupStripe(t)

	// Missing account
	resp := stripePostWithParams(tc, "/v1/account_links", map[string]string{
		"type": "account_onboarding",
	})
	resp.AssertStatus(400)
	resp.AssertBodyContains("account")

	// Missing type
	resp = stripePostWithParams(tc, "/v1/account_links", map[string]string{
		"account": "acct_123",
	})
	resp.AssertStatus(400)
	resp.AssertBodyContains("type")
}

func TestPersonCRUD(t *testing.T) {
	_, tc := setupStripe(t)

	// Create account
	resp := stripePost(tc, "/v1/accounts", nil)
	resp.AssertStatus(200)
	acctID := resp.JSONMap()["id"].(string)

	// Create person
	resp = stripePostWithParams(tc, "/v1/accounts/"+acctID+"/persons", map[string]string{
		"first_name": "Jane",
		"last_name":  "Doe",
		"email":      "jane@example.com",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	personID := m["id"].(string)
	if m["object"] != "person" {
		t.Errorf("expected object=person, got %v", m["object"])
	}
	if m["first_name"] != "Jane" {
		t.Errorf("expected first_name=Jane, got %v", m["first_name"])
	}
	if m["last_name"] != "Doe" {
		t.Errorf("expected last_name=Doe, got %v", m["last_name"])
	}
	if m["account"] != acctID {
		t.Errorf("expected account=%s, got %v", acctID, m["account"])
	}

	// Get person
	resp = stripeGet(tc, "/v1/accounts/"+acctID+"/persons/"+personID)
	resp.AssertStatus(200)
	if resp.JSONMap()["id"] != personID {
		t.Errorf("expected id=%s, got %v", personID, resp.JSONMap()["id"])
	}

	// Update person
	resp = stripePostWithParams(tc, "/v1/accounts/"+acctID+"/persons/"+personID, map[string]string{
		"first_name": "Janet",
	})
	resp.AssertStatus(200)
	if resp.JSONMap()["first_name"] != "Janet" {
		t.Errorf("expected first_name=Janet, got %v", resp.JSONMap()["first_name"])
	}

	// List persons
	resp = stripeGet(tc, "/v1/accounts/"+acctID+"/persons")
	resp.AssertStatus(200)
	data := resp.JSONMap()["data"].([]any)
	if len(data) != 1 {
		t.Errorf("expected 1 person, got %d", len(data))
	}

	// Delete person
	resp = tc.DoWithHeaders("DELETE", "/v1/accounts/"+acctID+"/persons/"+personID, nil, map[string]string{
		"Authorization": "Bearer sk_test_sim_123",
	})
	resp.AssertStatus(200)
	dm := resp.JSONMap()
	if dm["deleted"] != true {
		t.Error("expected deleted=true")
	}

	// Verify gone
	resp = stripeGet(tc, "/v1/accounts/"+acctID+"/persons/"+personID)
	resp.AssertStatus(404)
}

func TestCreateAndGetTopUp(t *testing.T) {
	_, tc := setupStripe(t)

	// Create top-up
	resp := stripePostWithParams(tc, "/v1/topups", map[string]string{
		"amount":   "100000",
		"currency": "usd",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["object"] != "topup" {
		t.Errorf("expected object=topup, got %v", m["object"])
	}
	if int64(m["amount"].(float64)) != 100000 {
		t.Errorf("expected amount=100000, got %v", m["amount"])
	}
	if m["currency"] != "usd" {
		t.Errorf("expected currency=usd, got %v", m["currency"])
	}
	if m["status"] != "succeeded" {
		t.Errorf("expected status=succeeded, got %v", m["status"])
	}
	if m["balance_transaction"] == nil || m["balance_transaction"] == "" {
		t.Error("expected balance_transaction to be set")
	}

	// Get top-up
	topupID := m["id"].(string)
	resp = stripeGet(tc, "/v1/topups/"+topupID)
	resp.AssertStatus(200)
	if resp.JSONMap()["id"] != topupID {
		t.Errorf("expected id=%s, got %v", topupID, resp.JSONMap()["id"])
	}

	// List top-ups
	resp = stripeGet(tc, "/v1/topups")
	resp.AssertStatus(200)
	data := resp.JSONMap()["data"].([]any)
	if len(data) != 1 {
		t.Errorf("expected 1 topup, got %d", len(data))
	}
}

func TestTopUpMissingParams(t *testing.T) {
	_, tc := setupStripe(t)

	// Missing amount
	resp := stripePostWithParams(tc, "/v1/topups", map[string]string{
		"currency": "usd",
	})
	resp.AssertStatus(400)
	resp.AssertBodyContains("amount")

	// Missing currency
	resp = stripePostWithParams(tc, "/v1/topups", map[string]string{
		"amount": "100000",
	})
	resp.AssertStatus(400)
	resp.AssertBodyContains("currency")
}

func TestTransferReversalNotFound(t *testing.T) {
	_, tc := setupStripe(t)

	// Non-existent transfer
	resp := stripePost(tc, "/v1/transfers/tr_nonexistent/reversals", nil)
	resp.AssertStatus(404)
	resp.AssertBodyContains("resource_missing")
}

func TestPersonNotFound(t *testing.T) {
	_, tc := setupStripe(t)

	// Non-existent account
	resp := stripeGet(tc, "/v1/accounts/acct_nonexistent/persons")
	resp.AssertStatus(404)
	resp.AssertBodyContains("resource_missing")
}
