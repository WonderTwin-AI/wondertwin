package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// account is an Account on the wire, with UpdatedDateUTC as /Date(ms+0000)/.
type account struct {
	store.Account
	UpdatedDateUTC Msdate `json:"UpdatedDateUTC"`
}

// Accounts serves GET /api.xro/2.0/Accounts: the tenant's chart, in order.
func (h *Handler) Accounts(w http.ResponseWriter, r *http.Request) {
	out := []account{}
	for _, a := range h.store.Accounts.For(tenantID(r)).List() {
		out = append(out, account{a, Msdate(a.UpdatedDateUTC)})
	}
	h.writeEnvelope(w, http.StatusOK, "Accounts", out)
}
