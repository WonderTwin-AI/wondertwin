package webhook

import "encoding/json"

// Event is one change a delivery reports. It carries a pointer to the resource,
// not the resource: the receiver calls ResourceURL back.
type Event struct {
	ResourceURL   string         `json:"resourceUrl"`
	ResourceID    string         `json:"resourceId"`
	EventDateUTC  string         `json:"eventDateUtc"`
	EventType     string         `json:"eventType"`
	EventCategory string         `json:"eventCategory"`
	TenantID      string         `json:"tenantId"`
	TenantType    string         `json:"tenantType"`
	Data          map[string]any `json:"data,omitempty"`
}

// Payload is the body of one delivery. The field order is the order Xero's
// documentation shows. An intent-to-receive check sends no events.
type Payload struct {
	Events             []Event `json:"events"`
	LastEventSequence  int64   `json:"lastEventSequence"`
	FirstEventSequence int64   `json:"firstEventSequence"`
	Entropy            string  `json:"entropy"`
}

// Body is the exact bytes that are sent and signed. events is never null.
func (p Payload) Body() []byte {
	if p.Events == nil {
		p.Events = []Event{}
	}
	b, _ := json.Marshal(p)
	return b
}
