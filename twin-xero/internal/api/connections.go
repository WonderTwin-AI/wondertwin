package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// connection is a Connection as it goes on the wire: dates in the identity
// API's zoneless ISO form.
type connection struct {
	ID             string  `json:"id"`
	AuthEventID    string  `json:"authEventId"`
	TenantID       string  `json:"tenantId"`
	TenantType     string  `json:"tenantType"`
	TenantName     string  `json:"tenantName"`
	CreatedDateUtc ISOTime `json:"createdDateUtc"`
	UpdatedDateUtc ISOTime `json:"updatedDateUtc"`
}

// Connections serves GET /connections and GET /Connections: the tenants the
// app is connected to, as a bare array. xero-python calls the capitalised
// path, xero-node and the docs the lowercase one (ident-001).
func (h *Handler) Connections(w http.ResponseWriter, r *http.Request) {
	out := []connection{}
	for _, c := range h.store.Connections.List() {
		out = append(out, wire(c))
	}
	twincore.JSON(w, http.StatusOK, out)
}

func wire(c store.Connection) connection {
	return connection{c.ID, c.AuthEventID, c.TenantID, c.TenantType, c.TenantName,
		ISOTime(c.CreatedDateUtc), ISOTime(c.UpdatedDateUtc)}
}
