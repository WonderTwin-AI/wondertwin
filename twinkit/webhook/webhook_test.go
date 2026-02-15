package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Mock signer
// ---------------------------------------------------------------------------

type mockSigner struct{}

func (m *mockSigner) Sign(payload []byte, secret string) map[string]string {
	return map[string]string{
		"X-Signature": "sig_" + secret,
	}
}

// ---------------------------------------------------------------------------
// NewDispatcher
// ---------------------------------------------------------------------------

func TestNewDispatcher(t *testing.T) {
	d := NewDispatcher(Config{})
	if d == nil {
		t.Fatal("expected non-nil dispatcher")
	}
	if d.maxRetries != 3 {
		t.Errorf("expected default maxRetries=3, got %d", d.maxRetries)
	}
	if d.retryDelay != 1*time.Second {
		t.Errorf("expected default retryDelay=1s, got %v", d.retryDelay)
	}
	if d.eventPrefix != "evt" {
		t.Errorf("expected default eventPrefix=evt, got %s", d.eventPrefix)
	}
}

func TestNewDispatcherCustomConfig(t *testing.T) {
	d := NewDispatcher(Config{
		MaxRetries:  5,
		RetryDelay:  2 * time.Second,
		EventPrefix: "whevt",
	})
	if d.maxRetries != 5 {
		t.Errorf("expected maxRetries=5, got %d", d.maxRetries)
	}
	if d.retryDelay != 2*time.Second {
		t.Errorf("expected retryDelay=2s, got %v", d.retryDelay)
	}
	if d.eventPrefix != "whevt" {
		t.Errorf("expected eventPrefix=whevt, got %s", d.eventPrefix)
	}
}

// ---------------------------------------------------------------------------
// Enqueue
// ---------------------------------------------------------------------------

func TestEnqueue(t *testing.T) {
	d := NewDispatcher(Config{})
	evt := d.Enqueue("customer.created", map[string]any{"id": "cus_123"})

	if evt.ID != "evt_000001" {
		t.Errorf("expected evt_000001, got %s", evt.ID)
	}
	if evt.Type != "customer.created" {
		t.Errorf("expected customer.created, got %s", evt.Type)
	}
	if evt.Payload["id"] != "cus_123" {
		t.Errorf("unexpected payload: %+v", evt.Payload)
	}

	queued := d.QueuedEvents()
	if len(queued) != 1 {
		t.Fatalf("expected 1 queued event, got %d", len(queued))
	}
}

func TestEnqueueMultiple(t *testing.T) {
	d := NewDispatcher(Config{})
	d.Enqueue("type.a", nil)
	d.Enqueue("type.b", nil)
	d.Enqueue("type.c", nil)

	queued := d.QueuedEvents()
	if len(queued) != 3 {
		t.Fatalf("expected 3 queued events, got %d", len(queued))
	}
	if queued[0].ID != "evt_000001" || queued[2].ID != "evt_000003" {
		t.Errorf("unexpected IDs: %s, %s", queued[0].ID, queued[2].ID)
	}
}

func TestEnqueueAutoDeliver(t *testing.T) {
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(Config{
		URL:         srv.URL,
		AutoDeliver: true,
		MaxRetries:  1,
	})

	d.Enqueue("test.event", nil)

	// Wait for async delivery
	time.Sleep(200 * time.Millisecond)

	if received.Load() != 1 {
		t.Errorf("expected 1 delivery for auto-deliver, got %d", received.Load())
	}
}

// ---------------------------------------------------------------------------
// Flush – successful delivery
// ---------------------------------------------------------------------------

func TestFlushSuccess(t *testing.T) {
	var receivedEvents []Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var evt Event
		json.NewDecoder(r.Body).Decode(&evt)
		receivedEvents = append(receivedEvents, evt)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(Config{
		URL:        srv.URL,
		MaxRetries: 1,
	})

	d.Enqueue("order.created", map[string]any{"order_id": "ord_1"})
	d.Enqueue("order.paid", map[string]any{"order_id": "ord_1"})

	err := d.Flush()
	if err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	if len(receivedEvents) != 2 {
		t.Fatalf("expected 2 delivered events, got %d", len(receivedEvents))
	}
	if receivedEvents[0].Type != "order.created" {
		t.Errorf("expected order.created, got %s", receivedEvents[0].Type)
	}
	if receivedEvents[1].Type != "order.paid" {
		t.Errorf("expected order.paid, got %s", receivedEvents[1].Type)
	}

	// After flush, queue should be empty.
	if len(d.QueuedEvents()) != 0 {
		t.Errorf("expected empty queue after flush, got %d", len(d.QueuedEvents()))
	}

	// Deliveries should be recorded.
	deliveries := d.Deliveries()
	if len(deliveries) != 2 {
		t.Fatalf("expected 2 deliveries, got %d", len(deliveries))
	}
}

// ---------------------------------------------------------------------------
// Flush – retry on failure
// ---------------------------------------------------------------------------

func TestFlushRetryOnFailure(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := attempts.Add(1)
		if count <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(Config{
		URL:        srv.URL,
		MaxRetries: 3,
		RetryDelay: 1 * time.Millisecond, // fast retries for testing
	})

	d.Enqueue("test.retry", nil)

	err := d.Flush()
	if err != nil {
		t.Fatalf("Flush should succeed after retries, got: %v", err)
	}

	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestFlushAllRetriesFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := NewDispatcher(Config{
		URL:        srv.URL,
		MaxRetries: 2,
		RetryDelay: 1 * time.Millisecond,
	})

	d.Enqueue("test.fail", nil)

	err := d.Flush()
	if err == nil {
		t.Fatal("expected error when all retries fail")
	}

	deliveries := d.Deliveries()
	if len(deliveries) != 2 {
		t.Fatalf("expected 2 delivery records (one per attempt), got %d", len(deliveries))
	}
}

// ---------------------------------------------------------------------------
// Flush – no URL configured
// ---------------------------------------------------------------------------

func TestFlushNoURL(t *testing.T) {
	d := NewDispatcher(Config{})
	d.Enqueue("test.event", nil)

	err := d.Flush()
	if err != nil {
		t.Fatalf("expected no error when URL is empty, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Flush with signer
// ---------------------------------------------------------------------------

func TestFlushWithSigner(t *testing.T) {
	var signatureHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signatureHeader = r.Header.Get("X-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(Config{
		URL:        srv.URL,
		Secret:     "whsec_test",
		Signer:     &mockSigner{},
		MaxRetries: 1,
	})

	d.Enqueue("test.signed", nil)
	err := d.Flush()
	if err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	if signatureHeader != "sig_whsec_test" {
		t.Errorf("expected signature header sig_whsec_test, got %s", signatureHeader)
	}
}

// ---------------------------------------------------------------------------
// FlushWebhooks (alias)
// ---------------------------------------------------------------------------

func TestFlushWebhooks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(Config{URL: srv.URL, MaxRetries: 1})
	d.Enqueue("test", nil)

	err := d.FlushWebhooks()
	if err != nil {
		t.Fatalf("FlushWebhooks error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SetURL / SetSecret
// ---------------------------------------------------------------------------

func TestSetURL(t *testing.T) {
	d := NewDispatcher(Config{})
	d.SetURL("http://example.com/webhook")

	// Enqueue and flush to verify URL is used.
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d.SetURL(srv.URL)
	d.Enqueue("test", nil)
	d.Flush()

	if !called {
		t.Error("expected webhook to be delivered to updated URL")
	}
}

func TestSetSecret(t *testing.T) {
	d := NewDispatcher(Config{})
	d.SetSecret("new_secret")
	// Just verify it doesn't panic; secret is used internally.
}

// ---------------------------------------------------------------------------
// Deliveries
// ---------------------------------------------------------------------------

func TestDeliveriesReturnsCopy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(Config{URL: srv.URL, MaxRetries: 1})
	d.Enqueue("test", nil)
	d.Flush()

	deliveries := d.Deliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}

	deliveries[0].EventID = "mutated"
	fresh := d.Deliveries()
	if fresh[0].EventID == "mutated" {
		t.Error("Deliveries did not return a copy; mutation leaked")
	}
}

// ---------------------------------------------------------------------------
// AllEvents / QueuedEvents
// ---------------------------------------------------------------------------

func TestAllEvents(t *testing.T) {
	d := NewDispatcher(Config{})
	d.Enqueue("a", nil)
	d.Enqueue("b", nil)

	all := d.AllEvents()
	if len(all) != 2 {
		t.Errorf("expected 2, got %d", len(all))
	}
}

func TestQueuedEventsReturnsCopy(t *testing.T) {
	d := NewDispatcher(Config{})
	d.Enqueue("a", nil)

	q := d.QueuedEvents()
	q[0].Type = "mutated"

	fresh := d.QueuedEvents()
	if fresh[0].Type == "mutated" {
		t.Error("QueuedEvents did not return a copy; mutation leaked")
	}
}

// ---------------------------------------------------------------------------
// Reset
// ---------------------------------------------------------------------------

func TestReset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(Config{URL: srv.URL, MaxRetries: 1})
	d.Enqueue("a", nil)
	d.Flush()
	d.Enqueue("b", nil)

	d.Reset()

	if len(d.QueuedEvents()) != 0 {
		t.Errorf("expected 0 queued events after reset, got %d", len(d.QueuedEvents()))
	}
	if len(d.Deliveries()) != 0 {
		t.Errorf("expected 0 deliveries after reset, got %d", len(d.Deliveries()))
	}

	// Counter should reset, so next event starts at 1.
	evt := d.Enqueue("c", nil)
	if evt.ID != "evt_000001" {
		t.Errorf("expected evt_000001 after reset, got %s", evt.ID)
	}
}
