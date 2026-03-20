package quirks

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadRules(t *testing.T) {
	e := NewEngine()
	e.LoadRules([]Rule{
		{ID: "Q-001", Enabled: true, Summary: "test quirk"},
		{ID: "Q-002", Enabled: false, Summary: "disabled quirk"},
	})
	if e.RuleCount() != 2 {
		t.Errorf("count = %d, want 2", e.RuleCount())
	}
}

func TestLoadRulesJSON(t *testing.T) {
	e := NewEngine()
	data := `[{"id":"Q-001","enabled":true,"conditions":[],"action":{"type":"reject"},"summary":"test"}]`
	if err := e.LoadRulesJSON([]byte(data)); err != nil {
		t.Fatalf("load: %v", err)
	}
	if e.RuleCount() != 1 {
		t.Errorf("count = %d, want 1", e.RuleCount())
	}
}

func TestEnableDisable(t *testing.T) {
	e := NewEngine()
	e.LoadRules([]Rule{{ID: "Q-001", Enabled: false}})

	if e.IsEnabled("Q-001") {
		t.Error("should start disabled")
	}
	e.EnableQuirk("Q-001")
	if !e.IsEnabled("Q-001") {
		t.Error("should be enabled")
	}
	e.DisableQuirk("Q-001")
	if e.IsEnabled("Q-001") {
		t.Error("should be disabled again")
	}
}

func TestEnableNonexistent(t *testing.T) {
	e := NewEngine()
	if err := e.EnableQuirk("nope"); err == nil {
		t.Error("expected error for nonexistent quirk")
	}
}

func TestListQuirks(t *testing.T) {
	e := NewEngine()
	e.LoadRules([]Rule{
		{ID: "Q-001", Enabled: true, Summary: "first"},
		{ID: "Q-002", Enabled: false, Summary: "second"},
	})
	list := e.ListQuirks()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	if list[0].ID != "Q-001" || list[0].Summary != "first" {
		t.Error("first quirk mismatch")
	}
}

// --- Condition Evaluation ---

func TestCondition_Eq(t *testing.T) {
	ctx := &RequestContext{Method: "POST", Path: "/v1/charges"}
	conds := []Condition{{Field: "request.method", Operator: "eq", Value: "POST"}}
	if !evaluateConditions(conds, ctx, nil) {
		t.Error("should match POST")
	}
	conds[0].Value = "GET"
	if evaluateConditions(conds, ctx, nil) {
		t.Error("should not match GET")
	}
}

func TestCondition_BodyPath(t *testing.T) {
	ctx := &RequestContext{
		Body: map[string]any{
			"payment_method_data": map[string]any{
				"type": "sepa_debit",
			},
		},
	}
	conds := []Condition{
		{Field: "request.body.payment_method_data.type", Operator: "eq", Value: "sepa_debit"},
	}
	if !evaluateConditions(conds, ctx, nil) {
		t.Error("should match nested body field")
	}
}

func TestCondition_NumericGt(t *testing.T) {
	ctx := &RequestContext{Body: map[string]any{"amount": float64(999999)}}
	conds := []Condition{
		{Field: "request.body.amount", Operator: "gt", Value: float64(100000)},
	}
	if !evaluateConditions(conds, ctx, nil) {
		t.Error("999999 > 100000 should match")
	}
}

func TestCondition_Exists(t *testing.T) {
	ctx := &RequestContext{Body: map[string]any{"currency": "usd"}}
	if !evaluateConditions([]Condition{{Field: "request.body.currency", Operator: "exists"}}, ctx, nil) {
		t.Error("currency exists")
	}
	if evaluateConditions([]Condition{{Field: "request.body.missing", Operator: "exists"}}, ctx, nil) {
		t.Error("missing should not exist")
	}
}

func TestCondition_In(t *testing.T) {
	ctx := &RequestContext{Body: map[string]any{"status": "failed"}}
	conds := []Condition{
		{Field: "request.body.status", Operator: "in", Value: []any{"failed", "canceled"}},
	}
	if !evaluateConditions(conds, ctx, nil) {
		t.Error("failed should be in [failed, canceled]")
	}
}

func TestCondition_Matches(t *testing.T) {
	ctx := &RequestContext{Path: "/v1/customers/cus_abc123"}
	conds := []Condition{
		{Field: "request.path", Operator: "matches", Value: "/v1/customers/cus_.*"},
	}
	if !evaluateConditions(conds, ctx, nil) {
		t.Error("should match regex")
	}
}

func TestCondition_AND(t *testing.T) {
	ctx := &RequestContext{
		Method: "POST",
		Path:   "/v1/payment_intents",
		Body:   map[string]any{"amount": float64(500)},
	}
	conds := []Condition{
		{Field: "request.method", Operator: "eq", Value: "POST"},
		{Field: "request.path", Operator: "eq", Value: "/v1/payment_intents"},
		{Field: "request.body.amount", Operator: "gt", Value: float64(100)},
	}
	if !evaluateConditions(conds, ctx, nil) {
		t.Error("all three conditions should match")
	}
	// Fail one condition.
	conds[2].Value = float64(1000)
	if evaluateConditions(conds, ctx, nil) {
		t.Error("500 is not > 1000")
	}
}

// --- Action Application ---

func TestModification_Set(t *testing.T) {
	body := map[string]any{"amount": 500}
	result := ApplyRequestModifications(body, []Modification{
		{Op: "set", Path: "currency", Value: "eur"},
	})
	if result["currency"] != "eur" {
		t.Errorf("currency = %v, want eur", result["currency"])
	}
}

func TestModification_Remove(t *testing.T) {
	body := map[string]any{"amount": 500, "currency": "usd"}
	result := ApplyRequestModifications(body, []Modification{
		{Op: "remove", Path: "currency"},
	})
	if _, ok := result["currency"]; ok {
		t.Error("currency should be removed")
	}
	if result["amount"] != 500 {
		t.Error("amount should be preserved")
	}
}

func TestModification_Default(t *testing.T) {
	body := map[string]any{"amount": 500}
	result := ApplyRequestModifications(body, []Modification{
		{Op: "default", Path: "currency", Value: "usd"},
	})
	if result["currency"] != "usd" {
		t.Errorf("currency = %v, want usd (default)", result["currency"])
	}

	// Default should not overwrite existing value.
	body["currency"] = "eur"
	result = ApplyRequestModifications(body, []Modification{
		{Op: "default", Path: "currency", Value: "usd"},
	})
	if result["currency"] != "eur" {
		t.Errorf("currency = %v, want eur (existing should be preserved)", result["currency"])
	}
}

func TestModification_NestedPath(t *testing.T) {
	body := map[string]any{
		"metadata": map[string]any{"key": "value"},
	}
	result := ApplyRequestModifications(body, []Modification{
		{Op: "set", Path: "metadata.internal_flag", Value: true},
	})
	meta := result["metadata"].(map[string]any)
	if meta["internal_flag"] != true {
		t.Error("nested set should work")
	}
	if meta["key"] != "value" {
		t.Error("existing nested key should be preserved")
	}
}

func TestModification_StripPrefix(t *testing.T) {
	body := map[string]any{"amount": 500, "currency": "usd"}
	result := ApplyRequestModifications(body, []Modification{
		{Op: "remove", Path: "request.body.currency"},
	})
	if _, ok := result["currency"]; ok {
		t.Error("should strip request.body. prefix and remove currency")
	}
}

// --- EvaluatePre/Post ---

func TestEvaluatePre_Reject(t *testing.T) {
	e := NewEngine()
	e.LoadRules([]Rule{{
		ID:      "Q-001",
		Enabled: true,
		Conditions: []Condition{
			{Field: "request.body.amount", Operator: "gt", Value: float64(999999)},
		},
		Action: Action{
			Type:       "reject",
			StatusCode: 400,
			Body:       map[string]any{"error": "amount_too_large"},
		},
	}})

	ctx := &RequestContext{Body: map[string]any{"amount": float64(1500000)}}
	actions := e.EvaluatePre(ctx)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Type != "reject" {
		t.Errorf("type = %q, want reject", actions[0].Type)
	}
	if actions[0].StatusCode != 400 {
		t.Errorf("status = %d, want 400", actions[0].StatusCode)
	}
}

func TestEvaluatePre_DisabledRuleSkipped(t *testing.T) {
	e := NewEngine()
	e.LoadRules([]Rule{{
		ID:      "Q-001",
		Enabled: false,
		Conditions: []Condition{
			{Field: "request.method", Operator: "eq", Value: "POST"},
		},
		Action: Action{Type: "reject", StatusCode: 400},
	}})

	ctx := &RequestContext{Method: "POST"}
	actions := e.EvaluatePre(ctx)
	if len(actions) != 0 {
		t.Error("disabled rule should not fire")
	}
}

func TestEvaluatePost_ModifyResponse(t *testing.T) {
	e := NewEngine()
	e.LoadRules([]Rule{{
		ID:      "Q-002",
		Enabled: true,
		Conditions: []Condition{
			{Field: "request.path", Operator: "eq", Value: "/v1/charges"},
		},
		Action: Action{
			Type: "modify_response",
			Modifications: []Modification{
				{Op: "set", Path: "response.body.livemode", Value: false},
			},
		},
	}})

	respCtx := &ResponseContext{
		Request: &RequestContext{Path: "/v1/charges"},
		Body:    map[string]any{"id": "ch_123"},
	}
	actions := e.EvaluatePost(respCtx)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}

	ApplyResponseModifications(respCtx.Body, actions[0].Modifications)
	if respCtx.Body["livemode"] != false {
		t.Error("livemode should be set to false")
	}
}

func TestFiredCount(t *testing.T) {
	e := NewEngine()
	e.LoadRules([]Rule{{
		ID:         "Q-001",
		Enabled:    true,
		Conditions: []Condition{{Field: "request.method", Operator: "eq", Value: "GET"}},
		Action:     Action{Type: "delay"},
	}})

	ctx := &RequestContext{Method: "GET"}
	e.EvaluatePre(ctx)
	e.EvaluatePre(ctx)
	e.EvaluatePre(ctx)

	list := e.ListQuirks()
	if list[0].Fired != 3 {
		t.Errorf("fired = %d, want 3", list[0].Fired)
	}
}

// --- Middleware Integration ---

func TestMiddleware_Reject(t *testing.T) {
	e := NewEngine()
	e.LoadRules([]Rule{{
		ID:      "Q-001",
		Enabled: true,
		Conditions: []Condition{
			{Field: "request.path", Operator: "eq", Value: "/v1/payment_intents"},
			{Field: "request.body.amount", Operator: "gt", Value: float64(999999)},
		},
		Action: Action{
			Type:       "reject",
			StatusCode: 400,
			Body:       map[string]any{"error": map[string]any{"code": "amount_too_large"}},
		},
	}})

	handler := Middleware(e)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"pi_123"}`))
	}))

	body := `{"amount":1500000,"currency":"usd"}`
	req := httptest.NewRequest("POST", "/v1/payment_intents", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]any)
	if errObj["code"] != "amount_too_large" {
		t.Errorf("error code = %v", errObj["code"])
	}
}

func TestMiddleware_ModifyRequest(t *testing.T) {
	e := NewEngine()
	e.LoadRules([]Rule{{
		ID:      "Q-002",
		Enabled: true,
		Conditions: []Condition{
			{Field: "request.body.payment_method_data.type", Operator: "eq", Value: "sepa_debit"},
		},
		Action: Action{
			Type: "modify_request",
			Modifications: []Modification{
				{Op: "remove", Path: "request.body.currency"},
			},
		},
	}})

	var receivedBody map[string]any
	handler := Middleware(e)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(200)
	}))

	body := `{"amount":500,"currency":"eur","payment_method_data":{"type":"sepa_debit"}}`
	req := httptest.NewRequest("POST", "/v1/payment_intents", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	if _, ok := receivedBody["currency"]; ok {
		t.Error("currency should have been removed by quirk")
	}
	if receivedBody["amount"] != float64(500) {
		t.Error("amount should be preserved")
	}
}

func TestMiddleware_NoRules(t *testing.T) {
	e := NewEngine()
	handler := Middleware(e)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/anything", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestMiddleware_NilEngine(t *testing.T) {
	handler := Middleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
