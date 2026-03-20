package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	stripewh "github.com/wondertwin-ai/wondertwin/twin-stripe/internal/webhook"
)

// webhookReceiver collects incoming webhook deliveries for assertions.
type webhookReceiver struct {
	mu        sync.Mutex
	deliveries []map[string]any
	headers    []http.Header
}

func (wr *webhookReceiver) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		json.Unmarshal(body, &m)
		wr.mu.Lock()
		wr.deliveries = append(wr.deliveries, m)
		wr.headers = append(wr.headers, r.Header.Clone())
		wr.mu.Unlock()
		w.WriteHeader(200)
	}
}

func (wr *webhookReceiver) count() int {
	wr.mu.Lock()
	defer wr.mu.Unlock()
	return len(wr.deliveries)
}

func (wr *webhookReceiver) get(i int) (map[string]any, http.Header) {
	wr.mu.Lock()
	defer wr.mu.Unlock()
	return wr.deliveries[i], wr.headers[i]
}

// setupStripeWithWebhooks creates a test server with a webhook receiver and auto-delivery enabled.
func setupStripeWithWebhooks(t *testing.T, receiver *webhookReceiver) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-stripe-test"}
	twin := twincore.New(cfg)

	whServer := httptest.NewServer(receiver.handler())
	t.Cleanup(whServer.Close)

	dispatcher := webhook.NewDispatcher(webhook.Config{
		URL:         whServer.URL,
		Secret:      "whsec_test_secret",
		Signer:      stripewh.NewStripeSigner(),
		EventPrefix: "evt",
		AutoDeliver: true,
	})
	dispatcher.SetEndpointProvider(memStore)

	handler := api.NewHandler(memStore, dispatcher, twin.Middleware(), nil, nil, nil)
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)
	return srv, tc
}

func TestWebhookDeliveryToRegisteredEndpoint(t *testing.T) {
	// Create a webhook receiver.
	receiver := &webhookReceiver{}
	whServer := httptest.NewServer(receiver.handler())
	defer whServer.Close()

	// Setup stripe without fallback webhook URL — deliveries only via registered endpoints.
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-stripe-test"}
	twin := twincore.New(cfg)
	dispatcher := webhook.NewDispatcher(webhook.Config{
		Secret:      "whsec_fallback",
		Signer:      stripewh.NewStripeSigner(),
		EventPrefix: "evt",
		AutoDeliver: true,
	})
	dispatcher.SetEndpointProvider(memStore)

	handler := api.NewHandler(memStore, dispatcher, twin.Middleware(), nil, nil, nil)
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	defer srv.Close()
	tc := testutil.NewTwinClient(t, srv)

	// Register a webhook endpoint via the API.
	formBody := "url=" + whServer.URL + "&enabled_events[]=customer.created&enabled_events[]=customer.updated"
	r, _ := http.NewRequest("POST", srv.URL+"/v1/webhook_endpoints", strings.NewReader(formBody))
	r.Header.Set("Authorization", "Bearer sk_test_sim_123")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 creating webhook endpoint, got %d", resp.StatusCode)
	}

	// Create a customer — should trigger delivery.
	stripePost(tc, "/v1/customers?name=WebhookTest", nil).AssertStatus(200)

	// Wait briefly for async delivery.
	time.Sleep(200 * time.Millisecond)

	if receiver.count() == 0 {
		t.Fatal("expected at least 1 webhook delivery")
	}

	// Verify the delivered payload contains the event type.
	found := false
	for i := 0; i < receiver.count(); i++ {
		m, _ := receiver.get(i)
		if m["type"] == "customer.created" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected customer.created event to be delivered")
	}
}

func TestWebhookFilteringByEnabledEvents(t *testing.T) {
	receiver := &webhookReceiver{}
	whServer := httptest.NewServer(receiver.handler())
	defer whServer.Close()

	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-stripe-test"}
	twin := twincore.New(cfg)
	dispatcher := webhook.NewDispatcher(webhook.Config{
		Signer:      stripewh.NewStripeSigner(),
		EventPrefix: "evt",
		AutoDeliver: true,
	})
	dispatcher.SetEndpointProvider(memStore)

	handler := api.NewHandler(memStore, dispatcher, twin.Middleware(), nil, nil, nil)
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	defer srv.Close()
	tc := testutil.NewTwinClient(t, srv)

	// Register endpoint that only listens for product.created.
	formBody := "url=" + whServer.URL + "&enabled_events[]=product.created"
	r, _ := http.NewRequest("POST", srv.URL+"/v1/webhook_endpoints", strings.NewReader(formBody))
	r.Header.Set("Authorization", "Bearer sk_test_sim_123")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ := http.DefaultClient.Do(r)
	resp.Body.Close()

	// Create a customer (should NOT be delivered — not in enabled_events).
	stripePost(tc, "/v1/customers?name=FilterTest", nil).AssertStatus(200)
	time.Sleep(200 * time.Millisecond)

	custDeliveries := 0
	for i := 0; i < receiver.count(); i++ {
		m, _ := receiver.get(i)
		if m["type"] == "customer.created" {
			custDeliveries++
		}
	}
	if custDeliveries > 0 {
		t.Error("customer.created should NOT be delivered to endpoint that only listens for product.created")
	}

	// Create a product (SHOULD be delivered).
	beforeCount := receiver.count()
	stripePost(tc, "/v1/products?name=FilterProd", nil).AssertStatus(200)
	time.Sleep(200 * time.Millisecond)

	found := false
	for i := beforeCount; i < receiver.count(); i++ {
		m, _ := receiver.get(i)
		if m["type"] == "product.created" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected product.created event to be delivered to matching endpoint")
	}
}

func TestWebhookWildcardEnabledEvents(t *testing.T) {
	receiver := &webhookReceiver{}
	whServer := httptest.NewServer(receiver.handler())
	defer whServer.Close()

	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-stripe-test"}
	twin := twincore.New(cfg)
	dispatcher := webhook.NewDispatcher(webhook.Config{
		Signer:      stripewh.NewStripeSigner(),
		EventPrefix: "evt",
		AutoDeliver: true,
	})
	dispatcher.SetEndpointProvider(memStore)

	handler := api.NewHandler(memStore, dispatcher, twin.Middleware(), nil, nil, nil)
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	defer srv.Close()
	tc := testutil.NewTwinClient(t, srv)

	// Register endpoint with wildcard ["*"].
	formBody := "url=" + whServer.URL + "&enabled_events[]=*"
	r, _ := http.NewRequest("POST", srv.URL+"/v1/webhook_endpoints", strings.NewReader(formBody))
	r.Header.Set("Authorization", "Bearer sk_test_sim_123")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ := http.DefaultClient.Do(r)
	resp.Body.Close()

	// Create a customer — should be delivered (wildcard matches all).
	stripePost(tc, "/v1/customers?name=WildcardTest", nil).AssertStatus(200)
	time.Sleep(200 * time.Millisecond)

	if receiver.count() == 0 {
		t.Fatal("expected webhook delivery with wildcard enabled_events")
	}
}

func TestWebhookStripeSignaturePresent(t *testing.T) {
	receiver := &webhookReceiver{}
	_, tc := setupStripeWithWebhooks(t, receiver)

	stripePost(tc, "/v1/customers?name=SigTest", nil).AssertStatus(200)
	time.Sleep(200 * time.Millisecond)

	if receiver.count() == 0 {
		t.Fatal("expected webhook delivery")
	}

	// Check that the Stripe-Signature header is present.
	_, headers := receiver.get(0)
	sig := headers.Get("Stripe-Signature")
	if sig == "" {
		t.Error("expected Stripe-Signature header on webhook delivery")
	}
	if !strings.Contains(sig, "t=") || !strings.Contains(sig, "v1=") {
		t.Errorf("expected Stripe-Signature to contain t= and v1=, got %s", sig)
	}
}
