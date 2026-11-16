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
