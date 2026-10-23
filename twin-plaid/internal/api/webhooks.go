package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
)

// itemEndpoints gives the dispatcher one endpoint per Item that has a webhook
// URL. Plaid keeps one URL per Item, not a subscription list, so each endpoint
// accepts only events whose Type is its Item's id (quirk DELIVERY-005).
type itemEndpoints struct{ store *store.MemoryStore }

func (e itemEndpoints) ActiveEndpoints() []webhook.Endpoint {
	var out []webhook.Endpoint
	for _, it := range e.store.Items.List() {
		if it.Webhook != "" {
			out = append(out, webhook.Endpoint{URL: it.Webhook, EnabledEvents: []string{it.ID}, Enabled: true})
		}
	}
	return out
}

// dispatcher is the webhook dispatcher, built on first use. It sends the flat
// body as Plaid does, pretty-printed with 2-space indentation and sorted keys
// so the signed request_body_sha256 covers exactly the bytes on the wire, and
// signs with the ES256 key, which needs no shared secret. Delivery is
// automatic and a receiver must answer 200 within 10 seconds (documented); the
// kit retries on its own schedule.
func (h *Handler) dispatcher() *webhook.Dispatcher {
	h.hooksOnce.Do(func() {
		h.hooks = webhook.NewDispatcher(webhook.Config{
			Signer: h.WebhookSigner(), SignWithoutSecret: true, AutoDeliver: true, EventPrefix: "plaid",
			Timeout: 10 * time.Second, RetryDelay: time.Second,
			Encode: func(e webhook.Event) ([]byte, error) { return json.MarshalIndent(e.Payload, "", "  ") },
		})
		h.hooks.SetEndpointProvider(itemEndpoints{h.store})
	})
	return h.hooks
}

// Webhooks is the dispatcher, for the admin plane and tests to inspect deliveries.
func (h *Handler) Webhooks() *webhook.Dispatcher { return h.dispatcher() }

// fireWebhook queues a webhook for an Item's URL. The body is flat: type, code,
// item_id and environment, plus the code's own fields (quirk DELIVERY-001: no
// ordering guarantee between webhooks).
func (h *Handler) fireWebhook(it store.Item, webhookType, code string, extra map[string]any) {
	body := map[string]any{"webhook_type": webhookType, "webhook_code": code, "item_id": it.ID, "environment": "sandbox"}
	for k, v := range extra {
		body[k] = v
	}
	h.dispatcher().Enqueue(it.ID, body)
}

// itemWebhookUpdate is POST /item/webhook/update: it sets the Item's webhook
// URL, null or empty to remove it, and acknowledges to the new URL.
func (h *Handler) itemWebhookUpdate(w http.ResponseWriter, r *http.Request) {
	it, ok := h.itemFor(w, r)
	if !ok {
		return
	}
	call, _ := CallFrom(r)
	url, isStr := asString(call.Body["webhook"])
	if !isStr {
		h.invalidBody(w)
		return
	}
	it.Webhook = url
	h.store.PutItem(it)
	if url != "" {
		h.fireWebhook(it, "ITEM", "WEBHOOK_UPDATE_ACKNOWLEDGED", map[string]any{"new_webhook_url": url, "error": nil})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"item": newItemJSON(it)})
}
