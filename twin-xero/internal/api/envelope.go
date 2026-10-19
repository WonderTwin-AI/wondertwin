package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

// providerName is the name Xero reports for the connected app on every
// response envelope.
const providerName = "WonderTwin"

// writeEnvelope answers with Xero's plural envelope (quirk crud-003). Writes
// and single reads answer an array under the resource's plural name, beside
// Id, Status, ProviderName and DateTimeUTC, so a read of one invoice is still
// {"Invoices": [..]}. A nil list is written as [], never null.
func (h *Handler) writeEnvelope(w http.ResponseWriter, status int, plural string, items any) {
	body, err := json.Marshal(items)
	if err != nil || bytes.Equal(body, []byte("null")) {
		body = []byte("[]")
	}
	var out bytes.Buffer
	out.WriteString(`{"Id":`)
	out.Write(mustJSON(h.store.NewID()))
	out.WriteString(`,"Status":"OK","ProviderName":`)
	out.Write(mustJSON(providerName))
	out.WriteString(`,"DateTimeUTC":`)
	out.Write(mustJSON(Msdate(h.store.Clock.Now().UTC().Truncate(time.Millisecond))))
	out.WriteByte(',')
	out.Write(mustJSON(plural))
	out.WriteByte(':')
	out.Write(body)
	out.WriteString("}\n")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(out.Bytes())
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
