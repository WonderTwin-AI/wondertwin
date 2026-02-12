package twincore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// JSON helper
// ---------------------------------------------------------------------------

func TestJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, http.StatusOK, map[string]string{"key": "value"})

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}
	if body["key"] != "value" {
		t.Errorf("expected key=value, got %+v", body)
	}
}

func TestJSONNilBody(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, http.StatusNoContent, nil)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body, got %s", rec.Body.String())
	}
}

func TestJSONWithDifferentStatuses(t *testing.T) {
	tests := []struct {
		status int
		body   any
	}{
		{http.StatusCreated, map[string]string{"id": "123"}},
		{http.StatusAccepted, map[string]bool{"ok": true}},
	}

	for _, tt := range tests {
		rec := httptest.NewRecorder()
		JSON(rec, tt.status, tt.body)

		if rec.Code != tt.status {
			t.Errorf("expected %d, got %d", tt.status, rec.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// Error helper
// ---------------------------------------------------------------------------

func TestError(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, http.StatusBadRequest, "something went wrong")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %+v", body)
	}
	if errObj["message"] != "something went wrong" {
		t.Errorf("expected message 'something went wrong', got %v", errObj["message"])
	}
	if errObj["type"] != "Bad Request" {
		t.Errorf("expected type 'Bad Request', got %v", errObj["type"])
	}
	if int(errObj["code"].(float64)) != 400 {
		t.Errorf("expected code 400, got %v", errObj["code"])
	}
}

func TestErrorNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, http.StatusNotFound, "resource not found")

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	errObj := body["error"].(map[string]any)
	if errObj["type"] != "Not Found" {
		t.Errorf("expected type 'Not Found', got %v", errObj["type"])
	}
}

// ---------------------------------------------------------------------------
// StripeError helper
// ---------------------------------------------------------------------------

func TestStripeError(t *testing.T) {
	rec := httptest.NewRecorder()
	StripeError(rec, http.StatusPaymentRequired, "card_error", "card_declined", "Your card was declined.")

	if rec.Code != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %+v", body)
	}
	if errObj["type"] != "card_error" {
		t.Errorf("expected type 'card_error', got %v", errObj["type"])
	}
	if errObj["code"] != "card_declined" {
		t.Errorf("expected code 'card_declined', got %v", errObj["code"])
	}
	if errObj["message"] != "Your card was declined." {
		t.Errorf("expected message, got %v", errObj["message"])
	}
}

// ---------------------------------------------------------------------------
// Twin creation
// ---------------------------------------------------------------------------

func TestNewTwin(t *testing.T) {
	cfg := &Config{
		Port:    9999,
		Name:    "test-twin",
		Verbose: false,
	}
	twin := New(cfg)

	if twin == nil {
		t.Fatal("expected non-nil Twin")
	}
	if twin.Config != cfg {
		t.Error("expected Config to match")
	}
	if twin.Router == nil {
		t.Error("expected non-nil Router")
	}
	if twin.Logger == nil {
		t.Error("expected non-nil Logger")
	}
	if twin.Middleware() == nil {
		t.Error("expected non-nil Middleware")
	}
}

func TestNewTwinVerbose(t *testing.T) {
	cfg := &Config{
		Port:    9998,
		Name:    "verbose-twin",
		Verbose: true,
	}
	twin := New(cfg)

	if twin == nil {
		t.Fatal("expected non-nil Twin")
	}
}

func TestNewTwinWithLatencyAndFailRate(t *testing.T) {
	cfg := &Config{
		Port:     9997,
		Name:     "configured-twin",
		Latency:  100,
		FailRate: 0.5,
	}
	twin := New(cfg)

	if twin == nil {
		t.Fatal("expected non-nil Twin")
	}
}

func TestTwinServeHTTP(t *testing.T) {
	cfg := &Config{Name: "test-twin"}
	twin := New(cfg)

	// Mount a test handler
	twin.Router.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"pong": "true"})
	})

	req := httptest.NewRequest("GET", "/ping", nil)
	rec := httptest.NewRecorder()
	twin.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if body["pong"] != "true" {
		t.Errorf("expected pong=true, got %+v", body)
	}
}

// ---------------------------------------------------------------------------
// statusRecorder
// ---------------------------------------------------------------------------

func TestStatusRecorderDefaultCode(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, statusCode: 200}

	// Without explicit WriteHeader, statusCode should remain 200 (the default).
	if sr.statusCode != 200 {
		t.Errorf("expected default status 200, got %d", sr.statusCode)
	}
}

func TestStatusRecorderExplicitCode(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, statusCode: 200}

	sr.WriteHeader(404)
	if sr.statusCode != 404 {
		t.Errorf("expected 404, got %d", sr.statusCode)
	}
}
