package webhook

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type staticEndpoints []Endpoint

func (s staticEndpoints) StripeEndpoints() []Endpoint { return s }

func TestDeliverer_PostsSignedStripeEventPerEndpointVersion(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	var sigs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, b)
		sigs = append(sigs, r.Header.Get("Stripe-Signature"))
		mu.Unlock()
	}))
	defer srv.Close()

	d := NewDeliverer(Config{Endpoints: staticEndpoints{
		{URL: srv.URL, Secret: "whsec_a", APIVersion: "2026-07-29.dahlia", EnabledEvents: []string{"customer.created"}},
		{URL: srv.URL, Secret: "whsec_b", EnabledEvents: []string{"invoice.paid"}},
	}})
	d.Publish(map[string]any{
		"id": "evt_1", "object": "event", "type": "customer.created",
		"api_version": "2026-08-26.dahlia",
		"data":        map[string]any{"object": map[string]any{"id": "cus_1", "object": "customer"}},
	})
	_ = d.FlushWebhooks()

	if len(bodies) != 1 {
		t.Fatalf("expected one delivery (only the matching endpoint), got %d", len(bodies))
	}
	var evt map[string]any
	if err := json.Unmarshal(bodies[0], &evt); err != nil {
		t.Fatal(err)
	}
	if evt["object"] != "event" || evt["api_version"] != "2026-07-29.dahlia" {
		t.Fatalf("expected a Stripe event labelled with the endpoint version, got %v", evt)
	}
	if evt["data"].(map[string]any)["object"].(map[string]any)["id"] != "cus_1" {
		t.Fatalf("expected data.object to be the customer, got %v", evt["data"])
	}

	// Verify the signature the way the SDKs do: v1 = HMAC(secret, "t.body").
	parts := strings.Split(sigs[0], ",")
	ts, _ := strconv.ParseInt(strings.TrimPrefix(parts[0], "t="), 10, 64)
	if want := "v1=" + ComputeSignature(ts, bodies[0], "whsec_a"); parts[1] != want {
		t.Fatalf("signature mismatch: got %s want %s", parts[1], want)
	}
}
