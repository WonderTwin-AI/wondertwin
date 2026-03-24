package telemetry

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTemplatePath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/v1/customers/cus_abc123", "/v1/customers/{id}"},
		{"/v1/customers/cus_abc123/payment_methods/pm_xyz789", "/v1/customers/{id}/payment_methods/{id}"},
		{"/v1/payment_intents", "/v1/payment_intents"},
		{"/api.xro/2.0/Invoices", "/api.xro/2.0/Invoices"},
		{"/v1/customers/12345", "/v1/customers/{id}"},
		{"/health", "/health"},
	}
	for _, tt := range tests {
		got := TemplatePath(tt.in)
		if got != tt.want {
			t.Errorf("TemplatePath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestExtractBodyShape(t *testing.T) {
	body := []byte(`{"amount": 500, "currency": "usd", "customer": "cus_abc", "metadata": {"key": "val"}, "items": [1,2]}`)
	shape := ExtractBodyShape(body)
	if shape == nil {
		t.Fatal("expected non-nil shape")
	}
	if shape["amount"] != "number" {
		t.Errorf("amount = %v, want number", shape["amount"])
	}
	if shape["currency"] != "string" {
		t.Errorf("currency = %v, want string", shape["currency"])
	}

	// metadata should now be a nested shape, not just "object"
	meta, ok := shape["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata should be nested shape map, got %T", shape["metadata"])
	}
	if meta["_type"] != "object" {
		t.Errorf("metadata._type = %v, want object", meta["_type"])
	}
	nested, ok := meta["_shape"].(map[string]any)
	if !ok {
		t.Fatalf("metadata._shape should be map, got %T", meta["_shape"])
	}
	if nested["key"] != "string" {
		t.Errorf("metadata._shape.key = %v, want string", nested["key"])
	}

	// items should be a nested array shape
	items, ok := shape["items"].(map[string]any)
	if !ok {
		t.Fatalf("items should be nested shape map, got %T", shape["items"])
	}
	if items["_type"] != "array" {
		t.Errorf("items._type = %v, want array", items["_type"])
	}
	if items["_element"] != "number" {
		t.Errorf("items._element = %v, want number", items["_element"])
	}
}

func TestExtractBodyShapeWithStats(t *testing.T) {
	body := []byte(`{
		"amount": 500,
		"metadata": {"key": "val", "nested": {"deep": 1}},
		"items": [{"id": "x", "qty": 2}]
	}`)
	stats := ExtractBodyShapeWithStats(body)
	if stats.Shape == nil {
		t.Fatal("expected non-nil shape")
	}
	if stats.Depth < 3 {
		t.Errorf("depth = %d, want >= 3", stats.Depth)
	}
	if stats.FieldCount < 5 {
		t.Errorf("field_count = %d, want >= 5", stats.FieldCount)
	}
}

func TestExtractBodyShape_DepthLimit(t *testing.T) {
	// 5 levels deep — should collapse at maxShapeDepth (4)
	body := []byte(`{"a": {"b": {"c": {"d": {"e": "too deep"}}}}}`)
	shape := ExtractBodyShape(body)
	// Navigate to depth 3 — d should be collapsed to "object"
	a := shape["a"].(map[string]any)["_shape"].(map[string]any)
	b := a["b"].(map[string]any)["_shape"].(map[string]any)
	c := b["c"].(map[string]any)["_shape"].(map[string]any)
	if c["d"] != "object" {
		t.Errorf("at depth 4, d should be collapsed to 'object', got %v (type %T)", c["d"], c["d"])
	}
}

func TestExtractBodyShape_Empty(t *testing.T) {
	if ExtractBodyShape(nil) != nil {
		t.Error("nil body should return nil")
	}
	if ExtractBodyShape([]byte{}) != nil {
		t.Error("empty body should return nil")
	}
	if ExtractBodyShape([]byte("not json")) != nil {
		t.Error("invalid JSON should return nil")
	}
}

func TestEmitter_Disabled(t *testing.T) {
	e := NewEmitter(Config{Enabled: false})
	e.EmitHTTP("GET", "/test", 200, 5, nil, nil)
	if e.BatchLen() != 0 {
		t.Error("disabled emitter should not collect events")
	}
}

func TestEmitter_CollectsEvents(t *testing.T) {
	e := NewEmitter(Config{
		Enabled:      true,
		CollectorURL: "http://localhost:9999",
		OrgID:        "testorg",
		TwinName:     "stripe",
		TwinVersion:  "0.3.2",
	})
	e.EmitHTTP("POST", "/v1/charges", 200, 10, nil, nil)
	e.EmitDomain("ledger", "OnJournalCreated", "charge", "", "succeeded")
	if e.BatchLen() != 2 {
		t.Errorf("batch = %d, want 2", e.BatchLen())
	}
}

func TestEmitter_SetsEnvelope(t *testing.T) {
	e := NewEmitter(Config{
		Enabled:      true,
		CollectorURL: "http://localhost:9999",
		OrgID:        "myorg",
		TwinName:     "xero",
		TwinVersion:  "1.0.0",
	})
	e.EmitDomain("workspace", "OnEntityCreated", "invoice", "", "DRAFT")

	e.mu.Lock()
	ev := e.batch[0]
	e.mu.Unlock()

	if ev.Twin != "xero" {
		t.Errorf("twin = %q, want xero", ev.Twin)
	}
	if ev.OrgID != "myorg" {
		t.Errorf("org = %q, want myorg", ev.OrgID)
	}
	if ev.EventType != EventDomainEvent {
		t.Errorf("type = %q, want domain_event", ev.EventType)
	}
	if ev.InstanceID == "" {
		t.Error("event should have an instance ID")
	}
	if ev.Seq != 1 {
		t.Errorf("seq = %d, want 1", ev.Seq)
	}
}

func TestEmitter_SequenceIncrement(t *testing.T) {
	e := NewEmitter(Config{
		Enabled:      true,
		CollectorURL: "http://localhost:9999",
		TwinName:     "test",
		TwinVersion:  "0.1.0",
	})
	e.EmitHTTP("GET", "/a", 200, 1, nil, nil)
	e.EmitHTTP("GET", "/b", 200, 1, nil, nil)
	e.EmitHTTP("GET", "/c", 200, 1, nil, nil)

	e.mu.Lock()
	defer e.mu.Unlock()

	for i, ev := range e.batch {
		want := int64(i + 1)
		if ev.Seq != want {
			t.Errorf("event %d: seq = %d, want %d", i, ev.Seq, want)
		}
		if ev.InstanceID == "" {
			t.Errorf("event %d: missing instance ID", i)
		}
	}
}

func TestEmitter_InstanceIDConsistent(t *testing.T) {
	e := NewEmitter(Config{
		Enabled:      true,
		CollectorURL: "http://localhost:9999",
		TwinName:     "test",
		TwinVersion:  "0.1.0",
	})
	id := e.InstanceID()
	if id == "" {
		t.Fatal("instance ID should not be empty")
	}
	e.EmitHTTP("GET", "/a", 200, 1, nil, nil)
	e.EmitHTTP("GET", "/b", 200, 1, nil, nil)

	e.mu.Lock()
	defer e.mu.Unlock()

	for i, ev := range e.batch {
		if ev.InstanceID != id {
			t.Errorf("event %d: instance ID = %q, want %q", i, ev.InstanceID, id)
		}
	}
}

func TestEmitter_FlushesToCollector(t *testing.T) {
	var received []byte
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer collector.Close()

	e := NewEmitter(Config{
		Enabled:       true,
		CollectorURL:  collector.URL,
		OrgID:         "testorg",
		TwinName:      "stripe",
		TwinVersion:   "0.1.0",
		FlushInterval: 50 * time.Millisecond,
		BatchSize:     100,
	})
	e.Start()

	e.EmitHTTP("GET", "/v1/customers/cus_123", 200, 5, nil, nil)

	time.Sleep(150 * time.Millisecond)
	e.Stop()

	if len(received) == 0 {
		t.Fatal("collector should have received events")
	}

	var events []Event
	json.Unmarshal(received, &events)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != EventHTTPObservation {
		t.Errorf("type = %q", events[0].EventType)
	}
}

func TestEmitter_BatchSizeTrigger(t *testing.T) {
	flushCount := 0
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flushCount++
		w.WriteHeader(200)
	}))
	defer collector.Close()

	e := NewEmitter(Config{
		Enabled:       true,
		CollectorURL:  collector.URL,
		OrgID:         "testorg",
		TwinName:      "stripe",
		TwinVersion:   "0.1.0",
		FlushInterval: time.Hour, // won't trigger on timer
		BatchSize:     3,
	})
	// Don't start the flush loop — we test batch-size trigger only.

	for i := 0; i < 3; i++ {
		e.EmitHTTP("GET", "/test", 200, 1, nil, nil)
	}

	time.Sleep(100 * time.Millisecond) // let the goroutine flush
	if flushCount == 0 {
		t.Error("batch size should have triggered a flush")
	}
}

func TestMiddleware_EmitsObservation(t *testing.T) {
	e := NewEmitter(Config{
		Enabled:      true,
		CollectorURL: "http://localhost:9999",
		OrgID:        "testorg",
		TwinName:     "stripe",
		TwinVersion:  "0.1.0",
	})

	handler := Middleware(e)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		w.Write([]byte(`{"id":"pi_123","status":"created"}`))
	}))

	req := httptest.NewRequest("POST", "/v1/payment_intents",
		bytes.NewReader([]byte(`{"amount":500,"currency":"usd"}`)))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if e.BatchLen() != 1 {
		t.Fatalf("expected 1 event, got %d", e.BatchLen())
	}
}

func TestEmitter_Enable(t *testing.T) {
	e := NewEmitter(Config{Enabled: false, TwinName: "test", TwinVersion: "0.1.0"})
	if e.Enabled() {
		t.Error("should start disabled")
	}
	e.Enable("http://localhost:9999", "key", "org")
	if !e.Enabled() {
		t.Error("should be enabled after Enable()")
	}
	e.Stop()
}
