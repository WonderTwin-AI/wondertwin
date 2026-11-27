package api

import (
	"encoding/json"
	"testing"
)

// The fill turns a null the handler wrote into [] where Stripe's schema
// declares a non-nullable array, and a list's null data into [], while a
// nullable property keeps its null.
func TestEmptyLists_FillReplacesHandlerNulls(t *testing.T) {
	var body map[string]any
	raw := `{"object":"invoice","id":"in_1","discounts":null,"customer_email":null,
		"lines":{"object":"list","data":null,"has_more":false,"url":"/v1/invoices/in_1/lines"}}`
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatal(err)
	}
	fillResponse(body)

	if d, ok := body["discounts"].([]any); !ok || len(d) != 0 {
		t.Errorf("discounts = %#v, want []", body["discounts"])
	}
	if v, ok := body["customer_email"]; !ok || v != nil {
		t.Errorf("customer_email = %#v, want null (nullable)", v)
	}
	lines := body["lines"].(map[string]any)
	if d, ok := lines["data"].([]any); !ok || len(d) != 0 {
		t.Errorf("lines.data = %#v, want []", lines["data"])
	}

	var list map[string]any
	if err := json.Unmarshal([]byte(`{"object":"list","data":null,"has_more":false,"url":"/v1/customers"}`), &list); err != nil {
		t.Fatal(err)
	}
	fillResponse(list)
	if d, ok := list["data"].([]any); !ok || len(d) != 0 {
		t.Errorf("top-level list data = %#v, want []", list["data"])
	}
}
