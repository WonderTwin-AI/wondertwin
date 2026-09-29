package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Endpoint is a registered webhook endpoint as delivery needs it.
type Endpoint struct {
	URL           string
	Secret        string
	APIVersion    string // empty: render in the account default
	EnabledEvents []string
}

// EndpointSource supplies the enabled webhook endpoints.
type EndpointSource interface {
	StripeEndpoints() []Endpoint
}

// Config configures a Deliverer.
type Config struct {
	// URL and Secret are the optional --webhook-url fallback, which receives
	// every event in the account default version.
	URL    string
	Secret string
	// Endpoints supplies the endpoints registered through the API.
	Endpoints EndpointSource
	Clock     ClockSource
	Logger    *slog.Logger
	Client    *http.Client
	// MaxAttempts per delivery; zero means 3.
	MaxAttempts int
}

// Deliverer posts Stripe event objects, signed with Stripe-Signature, to every
// matching endpoint.
//
// The body is the event itself ({"id":"evt_...","object":"event",
// "api_version":...,"data":{"object":...},...}), which is what
// stripe.webhooks.constructEvent and its Go and Python equivalents parse.
// Each endpoint receives the event labelled with its own api_version, falling
// back to the account default the event was created with.
type Deliverer struct {
	cfg    Config
	signer *StripeSigner
	logger *slog.Logger
	client *http.Client

	inflight sync.WaitGroup
	mu       sync.Mutex
	sent     []Delivery
}

// Delivery records one delivery attempt.
type Delivery struct {
	EventID    string `json:"event_id"`
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
	Attempt    int    `json:"attempt"`
}

// NewDeliverer returns a Deliverer for cfg.
func NewDeliverer(cfg Config) *Deliverer {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Deliverer{cfg: cfg, signer: NewStripeSigner(cfg.Clock), logger: logger, client: client}
}

// Publish delivers event, a Stripe event object, to every endpoint enabled for
// its type, asynchronously.
func (d *Deliverer) Publish(event map[string]any) {
	if d == nil {
		return
	}
	eventType, _ := event["type"].(string)
	type target struct {
		url, secret string
		body        []byte
	}
	var targets []target
	if d.cfg.Endpoints != nil {
		for _, ep := range d.cfg.Endpoints.StripeEndpoints() {
			if !enabledFor(ep.EnabledEvents, eventType) {
				continue
			}
			body, err := render(event, ep.APIVersion)
			if err != nil {
				d.logger.Error("render webhook event", "error", err)
				continue
			}
			targets = append(targets, target{ep.URL, ep.Secret, body})
		}
	}
	if d.cfg.URL != "" {
		if body, err := render(event, ""); err == nil {
			targets = append(targets, target{d.cfg.URL, d.cfg.Secret, body})
		}
	}
	eventID, _ := event["id"].(string)
	for _, t := range targets {
		d.inflight.Add(1)
		go func() {
			defer d.inflight.Done()
			d.deliver(eventID, t.url, t.secret, t.body)
		}()
	}
}

// FlushWebhooks waits for in-flight deliveries (admin.WebhookFlusher).
func (d *Deliverer) FlushWebhooks() error {
	d.inflight.Wait()
	return nil
}

// Deliveries returns the delivery attempts made so far.
func (d *Deliverer) Deliveries() []Delivery {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Delivery, len(d.sent))
	copy(out, d.sent)
	return out
}

func (d *Deliverer) deliver(eventID, url, secret string, body []byte) {
	for attempt := 1; attempt <= d.cfg.MaxAttempts; attempt++ {
		rec := Delivery{EventID: eventID, URL: url, Attempt: attempt}
		status, err := d.post(url, secret, body)
		rec.StatusCode = status
		if err != nil {
			rec.Error = err.Error()
		}
		d.mu.Lock()
		d.sent = append(d.sent, rec)
		d.mu.Unlock()
		if err == nil && status >= 200 && status < 300 {
			return
		}
		time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
	}
	d.logger.Warn("webhook delivery failed", "event_id", eventID, "url", url)
}

func (d *Deliverer) post(url, secret string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "Stripe/1.0 (+https://stripe.com/docs/webhooks)")
	if secret != "" {
		for k, v := range d.signer.Sign(body, secret) {
			req.Header.Set(k, v)
		}
	}
	//nolint:gosec // G704: the URL is a webhook endpoint the caller registered;
	// delivering to it is the point.
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// render encodes event, relabelled with apiVersion when one is given.
func render(event map[string]any, apiVersion string) ([]byte, error) {
	out := event
	if apiVersion != "" {
		out = make(map[string]any, len(event))
		for k, v := range event {
			out[k] = v
		}
		out["api_version"] = apiVersion
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("marshal event: %w", err)
	}
	return b, nil
}

func enabledFor(enabled []string, eventType string) bool {
	for _, e := range enabled {
		if e == "*" || e == eventType {
			return true
		}
	}
	return false
}
