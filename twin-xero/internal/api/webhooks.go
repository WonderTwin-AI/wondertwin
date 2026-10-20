package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	xerowh "github.com/wondertwin-ai/wondertwin/twin-xero/internal/webhook"
	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
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
func newDispatcher(s *store.MemoryStore) *pkgwh.Dispatcher {
	d := pkgwh.NewDispatcher(pkgwh.Config{EventPrefix: "xero", Signer: xerowh.Signer{}, Encode: encodeEvent})
	d.SetEndpointProvider(endpoints{s})
	return d
}

func encodeEvent(evt pkgwh.Event) ([]byte, error) {
	ev, _ := evt.Payload["event"].(xerowh.Event)
	seq, _ := evt.Payload["seq"].(int64)
	entropy, _ := evt.Payload["entropy"].(string)
	return xerowh.Payload{Events: []xerowh.Event{ev}, FirstEventSequence: seq, LastEventSequence: seq, Entropy: entropy}.Body(), nil
}

type endpoints struct{ s *store.MemoryStore }

// ActiveEndpoints lists the subscriptions that passed their intent-to-receive check.
func (e endpoints) ActiveEndpoints() []pkgwh.Endpoint {
	var out []pkgwh.Endpoint
	for _, w := range e.s.Webhooks.List() {
		if w.Status == store.WebhookOK {
			out = append(out, pkgwh.Endpoint{URL: w.URL, Secret: w.Key, EnabledEvents: w.Categories, Enabled: true})
		}
	}
	return out
}

func (h *Handler) entropy() string { return strings.ToUpper(sim.NewIDGenerator(h.store.Rand).Hex(10)) }

type pendingEvent struct {
	category string
	payload  map[string]any
}

// emit reports a change to every subscription that wants its category. It only
// appends to a FIFO under a short lock; one worker delivers the events in that
// order, so a slow receiver delays later deliveries but never a write.
func (h *Handler) emit(r *http.Request, tenant, category, eventType, resourceID string, data map[string]any) {
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
	h.pending = append(h.pending, pendingEvent{category, map[string]any{"event": ev, "seq": h.store.WebhookSeq.Add(1), "entropy": h.entropy()}})
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
