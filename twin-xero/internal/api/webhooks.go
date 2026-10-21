package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	xerowh "github.com/wondertwin-ai/wondertwin/twin-xero/internal/webhook"
	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	pkgwh "github.com/wondertwin-ai/wondertwin/twinkit/webhook"
)

// categoryPaths maps an event category to the Accounting path its resourceUrl
// points at. SUBSCRIPTION is the App Store's and is out of the target.
var categoryPaths = map[string]string{
	"CONTACT": "Contacts", "INVOICE": "Invoices", "CREDITNOTE": "CreditNotes",
	"PREPAYMENT": "Prepayments", "OVERPAYMENT": "Overpayments",
}

// eventDate is the zoneless ISO form eventDateUtc takes in Xero's examples.
const eventDate = "2006-01-02T15:04:05.000"

// newDispatcher builds the kit's dispatcher for Xero: the body is Xero's
// envelope, signed as it is sent, and the endpoints are the OK subscriptions.
func newDispatcher(s *store.MemoryStore, target func(id string) bool) *pkgwh.Dispatcher {
	d := pkgwh.NewDispatcher(pkgwh.Config{EventPrefix: "xero", Signer: xerowh.Signer{}, Encode: encodeEvent})
	d.SetEndpointProvider(endpoints{s, target})
	return d
}

func encodeEvent(evt pkgwh.Event) ([]byte, error) {
	ev, _ := evt.Payload["event"].(xerowh.Event)
	seq, _ := evt.Payload["seq"].(int64)
	entropy, _ := evt.Payload["entropy"].(string)
	return xerowh.Payload{Events: []xerowh.Event{ev}, FirstEventSequence: seq, LastEventSequence: seq, Entropy: entropy}.Body(), nil
}

type endpoints struct {
	s      *store.MemoryStore
	target func(id string) bool
}

// ActiveEndpoints lists the subscriptions that passed their intent-to-receive
// check and that the event being delivered is for.
func (e endpoints) ActiveEndpoints() []pkgwh.Endpoint {
	var out []pkgwh.Endpoint
	for _, w := range e.s.Webhooks.List() {
		if w.Status == store.WebhookOK && e.target(w.ID) {
			out = append(out, pkgwh.Endpoint{URL: w.URL, Secret: w.Key, EnabledEvents: w.Categories, Enabled: true})
		}
	}
	return out
}

// Dispatcher is the webhook dispatcher, for the admin flush route.
func (h *Handler) Dispatcher() *pkgwh.Dispatcher { return h.dispatcher }

func (h *Handler) entropy() string { return strings.ToUpper(sim.NewIDGenerator(h.store.Rand).Hex(10)) }

type pendingEvent struct {
	category string
	payload  map[string]any
	subs     map[string]bool // the subscriptions that existed, and wanted the category, when it was emitted
}

// isDeliveryTarget reports whether the event being delivered is for a subscription.
func (h *Handler) isDeliveryTarget(id string) bool {
	subs := h.deliverTo.Load()
	return subs != nil && (*subs)[id]
}

// subscribersOf is the subscriptions an event of the category goes to now. An
// event is for those and no others: a subscription registered after the write
// never hears of it, however long the delivery queue is.
func (h *Handler) subscribersOf(category string) map[string]bool {
	subs := map[string]bool{}
	for _, w := range h.store.Webhooks.List() {
		if w.Status == store.WebhookOK && slices.Contains(w.Categories, category) {
			subs[w.ID] = true
		}
	}
	return subs
}

// emit reports a change to every subscription that wants its category. It only
// appends to a FIFO under a short lock; one worker delivers the events in that
// order, so a slow receiver delays later deliveries but never a write.
func (h *Handler) emit(r *http.Request, tenant, category, eventType, resourceID string, data map[string]any) {
	subs := h.subscribersOf(category)
	if len(subs) == 0 {
		return
	}
	ev := xerowh.Event{
		ResourceURL: fmt.Sprintf("%s/api.xro/2.0/%s/%s", Origin(r), categoryPaths[category], resourceID), ResourceID: resourceID,
		EventDateUTC: h.store.Clock.Now().UTC().Format(eventDate), EventType: eventType, EventCategory: category,
		TenantID: tenant, TenantType: "ORGANISATION", Data: data,
	}
	h.pend.Lock()
	defer h.pend.Unlock()
	if h.closed {
		return
	}
	h.pending = append(h.pending, pendingEvent{category, map[string]any{"event": ev, "seq": h.store.WebhookSeq.Add(1), "entropy": h.entropy()}, subs})
	if !h.started {
		h.started = true
		h.worker.Add(1)
		go h.deliverLoop()
	}
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

// deliverLoop hands the dispatcher one event at a time, so its queue never
// holds more than the event being delivered and none is dropped by Flush.
func (h *Handler) deliverLoop() {
	defer h.worker.Done()
	for {
		h.pend.Lock()
		if len(h.pending) == 0 {
			h.pend.Unlock()
			select {
			case <-h.wake:
				continue
			case <-h.stop:
				return
			}
		}
		evt, gen := h.pending[0], h.gen
		h.pending = h.pending[1:]
		h.pend.Unlock()

		h.deliverTo.Store(&evt.subs)
		h.dispatcher.Enqueue(evt.category, evt.payload)
		_ = h.dispatcher.Flush()

		h.pend.Lock()
		if h.gen != gen {
			h.dispatcher.Reset()
		}
		h.pend.Unlock()
		select {
		case <-h.stop:
			return
		default:
		}
	}
}

// resetWebhooks drops the events not yet delivered and the dispatcher's state.
func (h *Handler) resetWebhooks() {
	h.pend.Lock()
	h.pending, h.gen = nil, h.gen+1
	h.pend.Unlock()
	h.dispatcher.Reset()
}

// Close stops the delivery worker once the delivery in flight, if any, ends.
func (h *Handler) Close() {
	h.pend.Lock()
	if !h.closed {
		h.closed = true
		close(h.stop)
	}
	h.pend.Unlock()
	h.worker.Wait()
}

// intentToReceive is Xero's check of a new subscription: payloads with no
// events, some signed with the key and some not. The receiver must answer 2xx
// to the first and 401 to the second, within 5 seconds and with no cookies.
func (h *Handler) intentToReceive(target, key string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	for i, signedRight := range []bool{true, false, true, false} {
		body := xerowh.Payload{Entropy: h.entropy()}.Body()
		signKey := key
		if !signedRight {
			signKey += "-wrong"
		}
		req, _ := http.NewRequest(http.MethodPost, target, strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(xerowh.SignatureHeader, xerowh.Signature(body, signKey))
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("check %d: %v", i+1, err)
		}
		resp.Body.Close()
		if signedRight && (resp.StatusCode < 200 || resp.StatusCode > 299) {
			return fmt.Errorf("check %d: a correctly signed payload answered %d, want 2xx", i+1, resp.StatusCode)
		}
		if !signedRight && resp.StatusCode != http.StatusUnauthorized {
			return fmt.Errorf("check %d: an incorrectly signed payload answered %d, want 401", i+1, resp.StatusCode)
		}
	}
	return nil
}

// verify runs the check and records its outcome on the subscription.
func (h *Handler) verify(w store.Webhook) store.Webhook {
	w.Status, w.Reason = store.WebhookOK, ""
	if err := h.intentToReceive(w.URL, w.Key); err != nil {
		w.Status, w.Reason = store.WebhookFailed, err.Error()
	}
	// The check can take seconds. A subscription deleted meanwhile stays deleted.
	if _, ok := h.store.Webhooks.Get(w.ID); ok {
		h.store.Webhooks.Set(w.ID, w)
	}
	return w
}

// RegisterWebhook serves POST /admin/webhooks, which stands in for Xero's
// developer portal. The key is generated when none is given, categories default
// to all, and intent_to_receive (default true) runs the check before the
// subscription may deliver.
func (h *Handler) RegisterWebhook(rw http.ResponseWriter, r *http.Request) {
	var in struct {
		URL, Key        string
		Categories      []string
		IntentToReceive *bool `json:"intent_to_receive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		twincore.Error(rw, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if u, err := url.Parse(in.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		twincore.Error(rw, http.StatusBadRequest, "url must be an http or https URL")
		return
	}
	if len(in.Categories) == 0 {
		in.Categories = []string{"CONTACT", "INVOICE", "CREDITNOTE", "PREPAYMENT", "OVERPAYMENT"}
	}
	for _, c := range in.Categories {
		if _, ok := categoryPaths[c]; !ok {
			twincore.Error(rw, http.StatusBadRequest, "unknown category "+c)
			return
		}
	}
	if in.Key == "" {
		in.Key = h.entropy() + h.entropy()
	}
	wh := store.Webhook{ID: h.store.NewID(), URL: in.URL, Key: in.Key, Categories: in.Categories, Status: store.WebhookOK, CreatedDateUTC: h.store.Clock.Now().UTC()}
	if in.IntentToReceive == nil || *in.IntentToReceive {
		// Stored disabled until the check passes, so it cannot deliver meanwhile.
		wh.Status, wh.Reason = store.WebhookFailed, "intent-to-receive check in progress"
		h.store.Webhooks.Set(wh.ID, wh)
		wh = h.verify(wh)
	} else {
		h.store.Webhooks.Set(wh.ID, wh)
	}
	twincore.JSON(rw, http.StatusCreated, wh)
}

// ListWebhooks serves GET /admin/webhooks.
func (h *Handler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	out := h.store.Webhooks.List()
	if out == nil {
		out = []store.Webhook{}
	}
	twincore.JSON(w, http.StatusOK, out)
}

// DeleteWebhook serves DELETE /admin/webhooks/{id}.
func (h *Handler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.store.Webhooks.Delete(chi.URLParam(r, "id")) {
		twincore.Error(w, http.StatusNotFound, "no such webhook")
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// VerifyWebhook serves POST /admin/webhooks/{id}/verify: the check run again, as
// when Xero re-enables a subscription.
func (h *Handler) VerifyWebhook(w http.ResponseWriter, r *http.Request) {
	wh, ok := h.store.Webhooks.Get(chi.URLParam(r, "id"))
	if !ok {
		twincore.Error(w, http.StatusNotFound, "no such webhook")
		return
	}
	twincore.JSON(w, http.StatusOK, h.verify(wh))
}

// WebhookDeliveries serves GET /admin/webhooks/deliveries: every attempt made.
func (h *Handler) WebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	twincore.JSON(w, http.StatusOK, h.dispatcher.Deliveries())
}
