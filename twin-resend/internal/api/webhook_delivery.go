package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
)

// svixSigner signs a delivery the way Resend's webhooks are signed, through
// Svix: svix-id, svix-timestamp and svix-signature "v1,<base64 HMAC-SHA256>"
// over "{svix-id}.{svix-timestamp}.{body}", keyed by the base64 key after the
// whsec_ prefix (quirk delivery-002).
type svixSigner struct{}

// Sign implements webhook.Signer. The kit's signer sees only the body, so
// svix-id is derived from it: every attempt at one delivery carries the same
// id, as Svix's retries do. The timestamp is wall-clock time, not the store
// clock, because verifiers reject a timestamp more than five minutes off.
func (svixSigner) Sign(payload []byte, secret string) map[string]string {
	sum := sha256.Sum256(payload)
	id := "msg_" + hex.EncodeToString(sum[:])[:27]
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	return map[string]string{
		"svix-id":        id,
		"svix-timestamp": ts,
		"svix-signature": svixSignature(secret, id, ts, payload),
	}
}

// svixSignature is the "v1,<base64>" signature of id.ts.payload under a
// whsec_ secret.
func svixSignature(secret, id, ts string, payload []byte) string {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		key = []byte(secret)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "."))
	mac.Write(payload)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// webhookEndpoints gives the dispatcher every enabled webhook, each with its
// own secret and the events it subscribed to.
type webhookEndpoints struct{ store *store.MemoryStore }

func (e webhookEndpoints) ActiveEndpoints() []webhook.Endpoint {
	var out []webhook.Endpoint
	for _, hook := range e.store.Webhooks.List() {
		if hook.Status == store.WebhookEnabled {
			out = append(out, webhook.Endpoint{URL: hook.Endpoint, Secret: hook.SigningSecret, EnabledEvents: hook.Events, Enabled: true})
		}
	}
	return out
}

// newDispatcher builds the webhook dispatcher. Each event is delivered as it
// is queued, on the kit's retry schedule, to every enabled webhook
// subscribed to its type. The body is {type, created_at, data} with no
// top-level id (quirk delivery-001). Resend's own retry schedule and its
// auto-disable of failing endpoints are not reproduced (divergences.json).
func newDispatcher(s *store.MemoryStore) *webhook.Dispatcher {
	d := webhook.NewDispatcher(webhook.Config{
		Signer:      svixSigner{},
		AutoDeliver: true,
		EventPrefix: "evt",
		Timeout:     15 * time.Second,
		Encode:      func(e webhook.Event) ([]byte, error) { return json.Marshal(e.Payload) },
	})
	d.SetEndpointProvider(webhookEndpoints{s})
	return d
}

// Webhooks is the dispatcher, for tests to inspect deliveries. The admin
// plane's webhook flush is not wired to it: delivery is automatic, so a flush
// would send every event a second time.
func (h *Handler) Webhooks() *webhook.Dispatcher { return h.hooks }

// emit queues one event for delivery, unless no enabled webhook subscribes
// to it.
func (h *Handler) emit(eventType string, data map[string]any) {
	subscribed := false
	for _, ep := range (webhookEndpoints{h.store}).ActiveEndpoints() {
		for _, e := range ep.EnabledEvents {
			subscribed = subscribed || e == eventType
		}
	}
	if !subscribed {
		return
	}
	h.hooks.Enqueue(eventType, map[string]any{
		"type":       eventType,
		"created_at": h.store.Clock.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"data":       data,
	})
}

// emailEvents are the events a send emits at once: email.scheduled for a
// scheduled send; otherwise email.sent and the event its recipient settles
// on (quirks lifecycle-001, lifecycle-002). A complaint follows delivery.
func emailEvents(e store.Email) []string {
	switch e.LastEvent {
	case store.EmailStatusScheduled:
		return []string{"email.scheduled"}
	case store.EmailStatusComplained:
		return []string{"email.sent", "email.delivered", "email.complained"}
	case store.EmailStatusCanceled:
		return nil
	}
	return []string{"email.sent", "email." + e.LastEvent}
}

// emitEmailEvents queues the events emailEvents names for e.
func (h *Handler) emitEmailEvents(e store.Email) {
	for _, t := range emailEvents(e) {
		h.emit(t, emailEventData(e))
	}
}

// emailEventData is the data of an email event, with the fields Resend's
// docs show: email_id, from, to, subject, created_at and tags as an object
// (quirk delivery-001). Deliveries were not recorded; the shape is from the
// docs and the SDK types.
func emailEventData(e store.Email) map[string]any {
	data := map[string]any{
		"email_id":   e.ID,
		"from":       e.From,
		"to":         e.To,
		"subject":    e.Subject,
		"created_at": eventTime(e.CreatedAt),
	}
	if len(e.Tags) > 0 {
		tags := make(map[string]string, len(e.Tags))
		for _, t := range e.Tags {
			tags[t.Name] = t.Value
		}
		data["tags"] = tags
	}
	return data
}

// eventTime turns a stored Postgres timestamp into the form Resend's event
// data uses, 2024-02-22T23:41:11.894719+00:00.
func eventTime(pg string) string {
	t, err := time.Parse(pgTimestamp, pg)
	if err != nil {
		return pg
	}
	return t.UTC().Format("2006-01-02T15:04:05.000000+00:00")
}
