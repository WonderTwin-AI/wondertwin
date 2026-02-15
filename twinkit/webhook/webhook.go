// Package webhook provides an outbound webhook dispatcher with delivery,
// retry, and pluggable signing for WonderTwin twins.
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

// Signer signs webhook payloads. Each twin implements its own signing scheme.
type Signer interface {
	// Sign returns headers to add to the webhook request for signature verification.
	Sign(payload []byte, secret string) map[string]string
}

// Event represents a webhook event to be dispatched.
type Event struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"data"`
	CreatedAt time.Time      `json:"created_at"`
}

// Delivery records a webhook delivery attempt.
type Delivery struct {
	EventID    string    `json:"event_id"`
	URL        string    `json:"url"`
	StatusCode int       `json:"status_code"`
	Error      string    `json:"error,omitempty"`
	Attempt    int       `json:"attempt"`
	Timestamp  time.Time `json:"timestamp"`
}

// Dispatcher manages outbound webhook delivery.
type Dispatcher struct {
	mu          sync.RWMutex
	url         string
	secret      string
	signer      Signer
	logger      *slog.Logger
	queue       []Event
	deliveries  []Delivery
	maxRetries  int
	retryDelay  time.Duration
	client      *http.Client
	eventPrefix string
	counter     int
	autoDeliver bool
}

// Config configures the webhook dispatcher.
type Config struct {
	URL         string
	Secret      string
	Signer      Signer
	Logger      *slog.Logger
	MaxRetries  int
	RetryDelay  time.Duration
	EventPrefix string // e.g., "evt" for Stripe-style events
	AutoDeliver bool   // automatically deliver events when queued
}

// NewDispatcher creates a new webhook dispatcher.
func NewDispatcher(cfg Config) *Dispatcher {
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = 1 * time.Second
	}
	if cfg.EventPrefix == "" {
		cfg.EventPrefix = "evt"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &Dispatcher{
		url:         cfg.URL,
		secret:      cfg.Secret,
		signer:      cfg.Signer,
		logger:      cfg.Logger,
		queue:       make([]Event, 0),
		deliveries:  make([]Delivery, 0),
		maxRetries:  cfg.MaxRetries,
		retryDelay:  cfg.RetryDelay,
		client:      &http.Client{Timeout: 30 * time.Second},
		eventPrefix: cfg.EventPrefix,
		autoDeliver: cfg.AutoDeliver,
	}
}

// SetURL updates the webhook delivery URL.
func (d *Dispatcher) SetURL(url string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.url = url
}

// SetSecret updates the webhook signing secret.
func (d *Dispatcher) SetSecret(secret string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.secret = secret
}

// Enqueue adds an event to the dispatch queue. If AutoDeliver is true,
// it will be delivered asynchronously.
func (d *Dispatcher) Enqueue(eventType string, payload map[string]any) Event {
	d.mu.Lock()
	d.counter++
	evt := Event{
		ID:        fmt.Sprintf("%s_%06d", d.eventPrefix, d.counter),
		Type:      eventType,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
	d.queue = append(d.queue, evt)
	autoDeliver := d.autoDeliver
	d.mu.Unlock()

	if autoDeliver {
		go d.deliverEvent(evt)
	}

	return evt
}

// Flush delivers all queued events synchronously.
func (d *Dispatcher) Flush() error {
	d.mu.RLock()
	events := make([]Event, len(d.queue))
	copy(events, d.queue)
	d.mu.RUnlock()

	var lastErr error
	for _, evt := range events {
		if err := d.deliverEvent(evt); err != nil {
			lastErr = err
		}
	}

	d.mu.Lock()
	d.queue = d.queue[:0]
	d.mu.Unlock()

	return lastErr
}

// FlushWebhooks implements admin.WebhookFlusher.
func (d *Dispatcher) FlushWebhooks() error {
	return d.Flush()
}

func (d *Dispatcher) deliverEvent(evt Event) error {
	d.mu.RLock()
	url := d.url
	secret := d.secret
	signer := d.signer
	d.mu.RUnlock()

	if url == "" {
		d.logger.Debug("no webhook URL configured, skipping delivery", "event_id", evt.ID)
		return nil
	}

	payload, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= d.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		if signer != nil && secret != "" {
			for k, v := range signer.Sign(payload, secret) {
				req.Header.Set(k, v)
			}
		}

		resp, err := d.client.Do(req)
		delivery := Delivery{
			EventID:   evt.ID,
			URL:       url,
			Attempt:   attempt,
			Timestamp: time.Now(),
		}

		if err != nil {
			delivery.Error = err.Error()
			delivery.StatusCode = 0
			lastErr = err
		} else {
			io.ReadAll(resp.Body)
			resp.Body.Close()
			delivery.StatusCode = resp.StatusCode
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				d.mu.Lock()
				d.deliveries = append(d.deliveries, delivery)
				d.mu.Unlock()
				return nil
			}
			lastErr = fmt.Errorf("webhook delivery failed: status %d", resp.StatusCode)
		}

		d.mu.Lock()
		d.deliveries = append(d.deliveries, delivery)
		d.mu.Unlock()

		if attempt < d.maxRetries {
			time.Sleep(d.retryDelay)
		}
	}

	return lastErr
}

// Deliveries returns all delivery records.
func (d *Dispatcher) Deliveries() []Delivery {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Delivery, len(d.deliveries))
	copy(out, d.deliveries)
	return out
}

// QueuedEvents returns all queued but undelivered events.
func (d *Dispatcher) QueuedEvents() []Event {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Event, len(d.queue))
	copy(out, d.queue)
	return out
}

// AllEvents returns all events (queued + delivered).
func (d *Dispatcher) AllEvents() []Event {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Event, len(d.queue))
	copy(out, d.queue)
	return out
}

// Reset clears all events, deliveries, and the queue.
func (d *Dispatcher) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queue = d.queue[:0]
	d.deliveries = d.deliveries[:0]
	d.counter = 0
}
