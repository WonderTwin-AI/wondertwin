package api_test

import (
	"net/url"
	"testing"
)

// --- Checkout line item tests ---

func TestCheckout_InlinePriceDataAndLineItems(t *testing.T) {
	_, tc := setupStripe(t)
	form := url.Values{
		"mode":                                   {"payment"},
		"success_url":                            {"https://example.com/success"},
		"line_items[0][price_data][currency]":    {"usd"},
		"line_items[0][price_data][unit_amount]": {"2500"},
		"line_items[0][price_data][product_data][name]": {"T-shirt"},
		"line_items[0][quantity]":                       {"2"},
	}
	cs := stripeForm(t, tc, "POST", "/v1/checkout/sessions", form, nil).assertStatus(200).json()
	if cs["amount_total"] != float64(5000) || cs["status"] != "open" {
		t.Fatalf("expected an open session totalling 5000, got %v", cs)
	}
	if _, present := cs["line_items"]; present {
		t.Fatalf("line_items is includable and must be absent unless expanded")
	}
	id := cs["id"].(string)

	got := stripeForm(t, tc, "GET", "/v1/checkout/sessions/"+id, url.Values{"expand[0]": {"line_items"}}, nil).assertStatus(200).json()
	items, _ := got["line_items"].(map[string]any)
	data, _ := items["data"].([]any)
	if items["object"] != "list" || len(data) != 1 || data[0].(map[string]any)["quantity"] != float64(2) {
		t.Fatalf("expected an expanded line item list, got %v", got["line_items"])
	}

	list := stripeForm(t, tc, "GET", "/v1/checkout/sessions/"+id+"/line_items", nil, nil).assertStatus(200).json()
	item := list["data"].([]any)[0].(map[string]any)
	if item["object"] != "item" || item["amount_total"] != float64(5000) || item["description"] != "T-shirt" || list["has_more"] != false {
		t.Fatalf("unexpected line item list: %v", list)
	}
}
