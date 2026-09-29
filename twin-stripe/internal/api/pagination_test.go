package api_test

import (
	"net/url"
	"testing"
)

// --- Newest-first list tests ---

func TestList_NewestFirstWithCursors(t *testing.T) {
	_, tc := setupStripe(t)

	var ids []string
	for _, email := range []string{"a@example.com", "b@example.com", "c@example.com"} {
		resp := stripeForm(t, tc, "POST", "/v1/customers", url.Values{"email": {email}}, nil).assertStatus(200)
		ids = append(ids, resp.json()["id"].(string))
	}

	first := stripeForm(t, tc, "GET", "/v1/customers", url.Values{"limit": {"2"}}, nil).assertStatus(200).json()
	data := first["data"].([]any)
	if len(data) != 2 || data[0].(map[string]any)["id"] != ids[2] || data[1].(map[string]any)["id"] != ids[1] {
		t.Fatalf("expected newest first [%s %s], got %v", ids[2], ids[1], data)
	}
	if first["has_more"] != true {
		t.Fatalf("expected has_more=true on the first page")
	}

	next := stripeForm(t, tc, "GET", "/v1/customers", url.Values{"limit": {"2"}, "starting_after": {ids[1]}}, nil).assertStatus(200).json()
	data = next["data"].([]any)
	if len(data) != 1 || data[0].(map[string]any)["id"] != ids[0] || next["has_more"] != false {
		t.Fatalf("expected the oldest customer on the last page, got %v", next)
	}

	prev := stripeForm(t, tc, "GET", "/v1/customers", url.Values{"limit": {"1"}, "ending_before": {ids[0]}}, nil).assertStatus(200).json()
	data = prev["data"].([]any)
	if len(data) != 1 || data[0].(map[string]any)["id"] != ids[1] || prev["has_more"] != true {
		t.Fatalf("expected ending_before to return the next newer customer, got %v", prev)
	}
}

func TestList_FilterAppliesBeforePaging(t *testing.T) {
	_, tc := setupStripe(t)

	for _, email := range []string{"keep@example.com", "other@example.com", "keep@example.com"} {
		stripeForm(t, tc, "POST", "/v1/customers", url.Values{"email": {email}}, nil).assertStatus(200)
	}
	resp := stripeForm(t, tc, "GET", "/v1/customers", url.Values{"email": {"keep@example.com"}, "limit": {"1"}}, nil).assertStatus(200).json()
	data := resp["data"].([]any)
	if len(data) != 1 || data[0].(map[string]any)["email"] != "keep@example.com" || resp["has_more"] != true {
		t.Fatalf("expected one filtered customer with has_more, got %v", resp)
	}
}
