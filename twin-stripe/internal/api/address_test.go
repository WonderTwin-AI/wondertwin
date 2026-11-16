package api_test

import (
	"net/url"
	"testing"
)

func form(kv ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Add(kv[i], kv[i+1])
	}
	return v.Encode()
}

func TestCustomerAddressIsStoredAndReturned(t *testing.T) {
	_, tc := setupStripe(t)
	resp := stripePost(tc, "/v1/customers?"+form("name", "CH", "address[country]", "CH", "address[postal_code]", "8001",
		"address[city]", "Zurich", "address[line1]", "Teststrasse 1",
		"shipping[name]", "Dock", "shipping[address][country]", "DE"), nil)
	resp.AssertStatus(200)
	cus := resp.JSONMap()
	want := map[string]any{"city": "Zurich", "country": "CH", "line1": "Teststrasse 1", "line2": nil, "postal_code": "8001", "state": nil}
	addr := cus["address"].(map[string]any)
	for k, v := range want {
		if addr[k] != v {
			t.Errorf("address.%s = %v, want %v", k, addr[k], v)
		}
	}
	ship := cus["shipping"].(map[string]any)
	if ship["name"] != "Dock" || ship["address"].(map[string]any)["country"] != "DE" {
		t.Errorf("shipping = %v", ship)
	}

	got := stripeGet(tc, "/v1/customers/"+cus["id"].(string)).JSONMap()
	if got["address"].(map[string]any)["country"] != "CH" {
		t.Errorf("retrieved address = %v", got["address"])
	}

	// An update merges the parts it sends; an empty address clears it.
	upd := stripePost(tc, "/v1/customers/"+cus["id"].(string)+"?"+form("address[postal_code]", "8002"), nil).JSONMap()
	if a := upd["address"].(map[string]any); a["postal_code"] != "8002" || a["country"] != "CH" {
		t.Errorf("updated address = %v", a)
	}
	cleared := stripePost(tc, "/v1/customers/"+cus["id"].(string)+"?address=", nil).JSONMap()
	if cleared["address"] != nil {
		t.Errorf("address after clearing = %v, want null", cleared["address"])
	}
}

func TestCustomerAddressIsNullWhenNotSent(t *testing.T) {
	_, tc := setupStripe(t)
	cus := stripePost(tc, "/v1/customers?name=none", nil).JSONMap()
	if cus["address"] != nil || cus["shipping"] != nil {
		t.Errorf("address = %v, shipping = %v, want both null", cus["address"], cus["shipping"])
	}
}

func TestPriceTaxBehaviorIsStoredAndReturned(t *testing.T) {
	_, tc := setupStripe(t)
	prod := stripePost(tc, "/v1/products?name=p", nil).JSONMap()["id"].(string)
	price := func(extra ...string) map[string]any {
		resp := stripePost(tc, "/v1/prices?"+form(append([]string{"currency", "usd", "unit_amount", "46800", "product", prod}, extra...)...), nil)
		resp.AssertStatus(200)
		return resp.JSONMap()
	}
	if got := price("tax_behavior", "inclusive")["tax_behavior"]; got != "inclusive" {
		t.Errorf("tax_behavior = %v, want inclusive", got)
	}
	plain := price()
	if plain["tax_behavior"] != "unspecified" {
		t.Errorf("default tax_behavior = %v, want unspecified", plain["tax_behavior"])
	}
	upd := stripePost(tc, "/v1/prices/"+plain["id"].(string)+"?tax_behavior=exclusive", nil).JSONMap()
	if upd["tax_behavior"] != "exclusive" {
		t.Errorf("updated tax_behavior = %v, want exclusive", upd["tax_behavior"])
	}
	if got := stripeGet(tc, "/v1/prices/"+plain["id"].(string)).JSONMap()["tax_behavior"]; got != "exclusive" {
		t.Errorf("retrieved tax_behavior = %v", got)
	}
}

func TestCustomerAddressEmptyPartClearsToNull(t *testing.T) {
	_, tc := setupStripe(t)
	cus := stripePost(tc, "/v1/customers?"+form("address[line1]", "Main 1", "address[line2]", "Apt 2",
		"address[country]", "US", "address[state]", ""), nil).JSONMap()
	if a := cus["address"].(map[string]any); a["state"] != nil || a["line2"] != "Apt 2" {
		t.Errorf("created address = %v, want state null", a)
	}
	upd := stripePost(tc, "/v1/customers/"+cus["id"].(string)+"?"+form("address[line2]", ""), nil).JSONMap()
	a := upd["address"].(map[string]any)
	if a["line2"] != nil || a["line1"] != "Main 1" || a["country"] != "US" {
		t.Errorf("address after address[line2]= = %v, want line2 null and the other parts kept", a)
	}
}

func TestPriceTaxBehaviorIsNeverEmpty(t *testing.T) {
	_, tc := setupStripe(t)
	cs := stripePost(tc, "/v1/checkout/sessions?"+form("mode", "payment", "success_url", "https://example.com/s",
		"line_items[0][price_data][currency]", "usd", "line_items[0][price_data][unit_amount]", "2500",
		"line_items[0][price_data][product_data][name]", "T-shirt", "line_items[0][quantity]", "1",
		"line_items[1][price_data][currency]", "usd", "line_items[1][price_data][unit_amount]", "100",
		"line_items[1][price_data][product_data][name]", "Sticker", "line_items[1][price_data][tax_behavior]", "inclusive",
		"line_items[1][quantity]", "1"), nil)
	cs.AssertStatus(200)
	items := stripeGet(tc, "/v1/checkout/sessions/"+cs.JSONMap()["id"].(string)+"/line_items").JSONMap()["data"].([]any)
	for i, want := range []string{"unspecified", "inclusive"} {
		id := items[i].(map[string]any)["price"].(map[string]any)["id"].(string)
		if got := stripeGet(tc, "/v1/prices/"+id).JSONMap()["tax_behavior"]; got != want {
			t.Errorf("price_data price %d tax_behavior = %v, want %s", i, got, want)
		}
	}

	state := tc.Get("/admin/state").JSONMap()
	prices, _ := state["prices"].(map[string]any)
	if prices == nil {
		prices = map[string]any{}
	}
	prices["price_seeded"] = map[string]any{"id": "price_seeded", "object": "price", "active": true, "currency": "usd",
		"product": "prod_x", "unit_amount": 100, "type": "one_time", "billing_scheme": "per_unit", "created": 1}
	state["prices"] = prices
	tc.Post("/admin/state", state).AssertStatus(200)
	if got := stripeGet(tc, "/v1/prices/price_seeded").JSONMap()["tax_behavior"]; got != "unspecified" {
		t.Errorf("seeded price tax_behavior = %v, want unspecified", got)
	}
}
