package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// summaryManualJournal is a manual journal without its lines, which is what a
// list without page returns, as on bank transactions. Whether a paged list
// carries the lines is not settled (page-004): the release notes say it does and
// the spec's example shows an empty JournalLines; this app emulator includes them.
func summaryManualJournal(in store.ManualJournal) store.ManualJournal {
	in.JournalLines = nil
	return in
}

// manualJournalGetter reads a manual journal's fields for a where expression:
// the scalar fields by their JSON names and Date (null when unset).
func manualJournalGetter(in store.ManualJournal) Getter {
	base := fieldGetter(in)
	return func(path string) (any, bool) {
		if path == "Date" {
			if in.Date == nil {
				return nil, true
			}
			return *in.Date, true
		}
		return base(path)
	}
}

// ManualJournals serves GET /api.xro/2.0/ManualJournals: the tenant's manual
// journals, narrowed by where and paged in Xero's default order. Xero documents
// no explicit list filters here. order and If-Modified-Since are accepted and
// ignored (GAP-002).
func (h *Handler) ManualJournals(w http.ResponseWriter, r *http.Request) {
	var where Where
	if src := r.URL.Query().Get("where"); src != "" {
		var err error
		if where, err = ParseWhere(src); err != nil {
			writeWhereError(w, err)
			return
		}
	}
	var matched []store.ManualJournal
	for _, mj := range h.store.ManualJournals.For(tenantID(r)).List() {
		if where != nil {
			ok, err := where.Match(manualJournalGetter(mj))
			if err != nil {
				writeWhereError(w, err)
				return
			}
			if !ok {
				continue
			}
		}
		matched = append(matched, mj)
	}
	page, pg := paginate(r, matched, func(mj store.ManualJournal) (time.Time, string) { return mj.UpdatedDateUTC, mj.ManualJournalID },
		summaryManualJournal)
	out := make([]manualJournal, len(page))
	for i, mj := range page {
		out[i] = wireManualJournal(mj)
	}
	h.writeListEnvelope(w, http.StatusOK, "ManualJournals", out, pg)
}

// ManualJournal serves GET /api.xro/2.0/ManualJournals/{ManualJournalID}: one
// manual journal in full, in the plural envelope. An unknown ID is a 404 whose
// body is a guess, as on invoices.
func (h *Handler) ManualJournal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ManualJournalID")
	for _, mj := range h.store.ManualJournals.For(tenantID(r)).List() {
		if strings.EqualFold(mj.ManualJournalID, id) {
			h.writeEnvelope(w, http.StatusOK, "ManualJournals", []manualJournal{wireManualJournal(mj)})
			return
		}
	}
	h.writeProblem(w, r, http.StatusNotFound, "Not Found", "The resource you're looking for cannot be found")
}
