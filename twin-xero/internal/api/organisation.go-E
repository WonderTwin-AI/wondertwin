package api

import (
	"net/http"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// organisation is an Organisation on the wire: the three date fields are
// /Date(ms+0000)/ strings, which xero-python raises on if they are ISO (time-001).
type organisation struct {
	store.Organisation
	PeriodLockDate    *Msdate `json:"PeriodLockDate,omitempty"`
	EndOfYearLockDate *Msdate `json:"EndOfYearLockDate,omitempty"`
	CreatedDateUTC    Msdate  `json:"CreatedDateUTC"`
}

func msdate(t *time.Time) *Msdate {
	if t == nil {
		return nil
	}
	d := Msdate(*t)
	return &d
}

// Organisation serves GET /api.xro/2.0/Organisation: the tenant's organisation,
// in the plural envelope like every other answer (crud-003).
func (h *Handler) Organisation(w http.ResponseWriter, r *http.Request) {
	out := []organisation{}
	if o, ok := h.store.Organisations.Get(tenantID(r)); ok {
		out = append(out, organisation{o, msdate(o.PeriodLockDate), msdate(o.EndOfYearLockDate), Msdate(o.CreatedDateUTC)})
	}
	h.writeEnvelope(w, http.StatusOK, "Organisations", out)
}
