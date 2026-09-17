package expand

import "testing"

type mapResolver map[string]map[string]any

func (m mapResolver) Resolve(id string) (map[string]any, bool) {
	obj, ok := m[id]
	return obj, ok
}

func TestApply_NoPaths(t *testing.T) {
	body := map[string]any{"id": "sub_1", "customer": "cus_1"}
	got := Apply(body, nil, mapResolver{})
	if got["customer"] != "cus_1" {
		t.Errorf("expected customer left untouched, got %v", got["customer"])
	}
}

func TestApply_NilBodyOrResolver(t *testing.T) {
	if got := Apply(nil, []string{"customer"}, mapResolver{}); got != nil {
		t.Errorf("expected nil body to stay nil, got %v", got)
	}
	body := map[string]any{"customer": "cus_1"}
	got := Apply(body, []string{"customer"}, nil)
	if got["customer"] != "cus_1" {
		t.Errorf("expected no-op with nil resolver, got %v", got["customer"])
	}
}

func TestApply_SingleLevel(t *testing.T) {
	resolver := mapResolver{
		"cus_1": {"id": "cus_1", "object": "customer", "email": "a@example.com"},
	}
	body := map[string]any{"id": "sub_1", "object": "subscription", "customer": "cus_1"}

	Apply(body, []string{"customer"}, resolver)

	cust, ok := body["customer"].(map[string]any)
	if !ok {
		t.Fatalf("expected customer to be expanded to an object, got %T: %v", body["customer"], body["customer"])
	}
	if cust["email"] != "a@example.com" {
		t.Errorf("expected expanded customer email, got %v", cust["email"])
	}
}

func TestApply_DeepNested(t *testing.T) {
	resolver := mapResolver{
		"in_1": {"id": "in_1", "object": "invoice", "payment_intent": "pi_1"},
		"pi_1": {"id": "pi_1", "object": "payment_intent", "status": "succeeded"},
	}
	body := map[string]any{"id": "sub_1", "latest_invoice": "in_1"}

	Apply(body, []string{"latest_invoice.payment_intent"}, resolver)

	inv, ok := body["latest_invoice"].(map[string]any)
	if !ok {
		t.Fatalf("expected latest_invoice to be expanded, got %T", body["latest_invoice"])
	}
	pi, ok := inv["payment_intent"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested payment_intent to be expanded, got %T: %v", inv["payment_intent"], inv["payment_intent"])
	}
	if pi["status"] != "succeeded" {
		t.Errorf("expected expanded payment_intent status, got %v", pi["status"])
	}
}

func TestApply_MultiplePathsShareTopLevelField(t *testing.T) {
	resolver := mapResolver{
		"in_1":  {"id": "in_1", "object": "invoice", "payment_intent": "pi_1", "customer": "cus_1"},
		"pi_1":  {"id": "pi_1", "object": "payment_intent"},
		"cus_1": {"id": "cus_1", "object": "customer"},
	}
	body := map[string]any{"latest_invoice": "in_1"}

	Apply(body, []string{"latest_invoice.payment_intent", "latest_invoice.customer"}, resolver)

	inv := body["latest_invoice"].(map[string]any)
	if _, ok := inv["payment_intent"].(map[string]any); !ok {
		t.Errorf("expected payment_intent expanded within latest_invoice, got %v", inv["payment_intent"])
	}
	if _, ok := inv["customer"].(map[string]any); !ok {
		t.Errorf("expected customer expanded within latest_invoice, got %v", inv["customer"])
	}
}

func TestApply_ListDataPrefix(t *testing.T) {
	resolver := mapResolver{
		"cus_1": {"id": "cus_1", "object": "customer", "email": "a@example.com"},
		"cus_2": {"id": "cus_2", "object": "customer", "email": "b@example.com"},
	}
	body := map[string]any{
		"object": "list",
		"data": []any{
			map[string]any{"id": "ch_1", "customer": "cus_1"},
			map[string]any{"id": "ch_2", "customer": "cus_2"},
		},
	}

	Apply(body, []string{"data.customer"}, resolver)

	data := body["data"].([]any)
	first := data[0].(map[string]any)
	second := data[1].(map[string]any)
	if first["customer"].(map[string]any)["email"] != "a@example.com" {
		t.Errorf("expected first item customer expanded, got %v", first["customer"])
	}
	if second["customer"].(map[string]any)["email"] != "b@example.com" {
		t.Errorf("expected second item customer expanded, got %v", second["customer"])
	}
}

func TestApply_UnresolvedIDLeftUnchanged(t *testing.T) {
	body := map[string]any{"customer": "cus_missing"}
	Apply(body, []string{"customer"}, mapResolver{})
	if body["customer"] != "cus_missing" {
		t.Errorf("expected unresolved ID left as string, got %v", body["customer"])
	}
}

func TestApply_MissingOrNonStringFieldIgnored(t *testing.T) {
	resolver := mapResolver{"cus_1": {"id": "cus_1"}}
	body := map[string]any{"amount": 100}
	Apply(body, []string{"customer", "amount"}, resolver)
	if body["amount"] != 100 {
		t.Errorf("expected non-string field left alone, got %v", body["amount"])
	}
	if _, ok := body["customer"]; ok {
		t.Errorf("expected absent field to stay absent, got %v", body["customer"])
	}
}

func TestApply_EmptyStringIDIgnored(t *testing.T) {
	resolver := mapResolver{"": {"id": "should-not-resolve"}}
	body := map[string]any{"customer": ""}
	Apply(body, []string{"customer"}, resolver)
	if body["customer"] != "" {
		t.Errorf("expected empty-string ID left untouched, got %v", body["customer"])
	}
}
