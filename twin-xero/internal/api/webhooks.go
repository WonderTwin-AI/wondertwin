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

// emit reports a change to every subscription that wants its category. The
// event is queued in sequence under one lock, so deliveries keep their order,
// and delivered in the background: a slow receiver never slows the write.
func (h *Handler) emit(r *http.Request, tenant, category, eventType, resourceID string, data map[string]any) {
	ev := xerowh.Event{
		ResourceURL: fmt.Sprintf("%s/api.xro/2.0/%s/%s", Origin(r), categoryPaths[category], resourceID), ResourceID: resourceID,
		EventDateUTC: h.store.Clock.Now().UTC().Format(eventDate), EventType: eventType, EventCategory: category,
		TenantID: tenant, TenantType: "ORGANISATION", Data: data,
	}
	h.deliver.Lock()
	h.dispatcher.Enqueue(category, map[string]any{"event": ev, "seq": h.store.WebhookSeq.Add(1), "entropy": h.entropy()})
	h.deliver.Unlock()
	go func() {
		h.deliver.Lock()
		defer h.deliver.Unlock()
		_ = h.dispatcher.Flush()
	}()
}
