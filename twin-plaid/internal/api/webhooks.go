package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/seed"
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

// Webhooks is the dispatcher, for tests to inspect deliveries. The admin plane's
// webhook flush is not wired to it: delivery is automatic, so a flush would send
// every event a second time.
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
	it, found := h.store.UpdateItem(it.ID, func(i *store.Item) { i.Webhook = url })
	if !found {
		h.writeAccessTokenError(w, store.ErrItemRemoved)
		return
	}
	if url != "" {
		h.fireWebhook(it, "ITEM", "WEBHOOK_UPDATE_ACKNOWLEDGED", map[string]any{"new_webhook_url": url, "error": nil})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"item": newItemJSON(it)})
}

// fireable maps "TYPE.CODE" to the Item product the webhook needs, for the
// webhooks /sandbox/item/fire_webhook can fire here: those Plaid documents
// with a sample body. The rest of its 12 codes are not supported.
var fireable = map[string]string{
	"ITEM.NEW_ACCOUNTS_AVAILABLE": "", "ITEM.LOGIN_REPAIRED": "", "ITEM.PENDING_DISCONNECT": "",
	"ITEM.USER_PERMISSION_REVOKED": "", "ITEM.USER_ACCOUNT_REVOKED": "",
	"TRANSACTIONS.DEFAULT_UPDATE": "transactions", "TRANSACTIONS.SYNC_UPDATES_AVAILABLE": "transactions",
	"TRANSACTIONS.RECURRING_TRANSACTIONS_UPDATE": "transactions", "AUTH.DEFAULT_UPDATE": "auth",
}

// defaultType is the webhook_type a code fires under when the request names none.
func defaultType(code string) string {
	switch code {
	case "DEFAULT_UPDATE", "SYNC_UPDATES_AVAILABLE", "RECURRING_TRANSACTIONS_UPDATE":
		return "TRANSACTIONS"
	}
	return "ITEM"
}

// sampleBody makes the body to send from Plaid's sample: the Item's id, the
// sandbox environment, no user_id (only user-based Items carry one), and the
// account ids of this Item where the sample names accounts.
func (h *Handler) sampleBody(it store.Item, key string, newTransactions int) map[string]any {
	body, _ := seed.WebhookSample(key)
	delete(body, "user_id")
	body["item_id"], body["environment"] = it.ID, "sandbox"
	var accounts []string
	for _, a := range h.store.AccountsForItem(it.ID) {
		accounts = append(accounts, a.ID)
	}
	switch key {
	case "TRANSACTIONS.DEFAULT_UPDATE":
		body["new_transactions"] = newTransactions
	case "TRANSACTIONS.RECURRING_TRANSACTIONS_UPDATE":
		body["account_ids"] = accounts
	case "ITEM.USER_ACCOUNT_REVOKED":
		if len(accounts) > 0 {
			body["account_id"] = accounts[0]
		}
	case "AUTH.DEFAULT_UPDATE":
		if len(accounts) > 0 {
			body["account_ids_with_updated_auth"] = map[string][]string{accounts[0]: {"account_number"}}
		}
	}
	return body
}

// sandboxItemFireWebhook is POST /sandbox/item/fire_webhook: it fires a test
// webhook at the Item's URL (quirk DELIVERY-004).
func (h *Handler) sandboxItemFireWebhook(w http.ResponseWriter, r *http.Request) {
	it, ok := h.itemFor(w, r)
	if !ok {
		return
	}
	call, _ := CallFrom(r)
	code, ok1 := asString(call.Body["webhook_code"])
	kind, ok2 := asString(call.Body["webhook_type"])
	n, hasN, ok3 := asInt(call.Body["new_transactions"])
	if !ok1 || !ok2 || !ok3 {
		h.invalidBody(w)
		return
	}
	if code == "" {
		h.writeMissing(w, []string{"webhook_code"})
		return
	}
	if kind == "" {
		kind = defaultType(code)
	}
	key := kind + "." + code
	product, supported := fireable[key]
	if !supported || (hasN && (key != "TRANSACTIONS.DEFAULT_UPDATE" || n < 0)) {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "INVALID_FIELD",
			"webhook_code "+code+" with webhook_type "+kind+" is not supported by this emulator")
		return
	}
	if product != "" && !slices.Contains(it.Products, product) {
		h.writeError(w, http.StatusBadRequest, "SANDBOX_ERROR", "SANDBOX_PRODUCT_NOT_ENABLED",
			"The "+product+" product is not enabled on this item")
		return
	}
	if it.Webhook == "" {
		h.writeError(w, http.StatusBadRequest, "SANDBOX_ERROR", "SANDBOX_WEBHOOK_INVALID",
			"Webhook for this item is either not set up, or invalid. Please update the item's webhook and try again.")
		return
	}
	h.dispatcher().Enqueue(it.ID, h.sampleBody(it, key, n))
	h.writeJSON(w, http.StatusOK, map[string]any{"webhook_fired": true})
}
