package api

import (
	"net/http"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// summaryCreditNote is a credit note without its line items, which is what a
// list without page returns (page-002).
func summaryCreditNote(cn store.CreditNote) store.CreditNote {
	cn.LineItems = nil
	return cn
}

// creditNoteGetter reads a credit note's fields for a where expression: the
// scalar fields by their JSON names, the optional dates (null when unset), and
// Contact.ContactID.
func creditNoteGetter(cn store.CreditNote) Getter {
	base := fieldGetter(cn)
	date := map[string]*time.Time{"Date": cn.Date, "DueDate": cn.DueDate, "FullyPaidOnDate": cn.FullyPaidOnDate}
	return func(path string) (any, bool) {
		if t, ok := date[path]; ok {
			if t == nil {
				return nil, true
			}
			return *t, true
		}
		if path == "Contact.ContactID" {
			if cn.Contact == nil {
				return nil, true
			}
			return cn.Contact.ContactID, true
		}
		return base(path)
	}
}

// CreditNotes serves GET /api.xro/2.0/CreditNotes: the tenant's credit notes,
// narrowed by where and paged (page-001 to page-003). Xero's credit note list
// has no ID, number or status filters, only where, order, page, pageSize,
// unitdp and If-Modified-Since; order and If-Modified-Since are accepted and
// ignored (GAP-002), and so is unitdp.
func (h *Handler) CreditNotes(w http.ResponseWriter, r *http.Request) {
	var where Where
	if src := r.URL.Query().Get("where"); src != "" {
		var err error
		if where, err = ParseWhere(src); err != nil {
			writeWhereError(w, err)
			return
		}
	}
	var matched []store.CreditNote
	for _, cn := range h.store.CreditNotes.For(tenantID(r)).List() {
		if where != nil {
			ok, err := where.Match(creditNoteGetter(cn))
			if err != nil {
				writeWhereError(w, err)
				return
			}
			if !ok {
				continue
			}
		}
		matched = append(matched, cn)
	}
	page, pg := paginate(r, matched, func(cn store.CreditNote) (time.Time, string) { return cn.UpdatedDateUTC, cn.CreditNoteID }, summaryCreditNote)
	out := make([]creditNote, len(page))
	for i, cn := range page {
		out[i] = h.wireCreditNote(tenantID(r), cn)
	}
	h.writeListEnvelope(w, http.StatusOK, "CreditNotes", out, pg)
}
