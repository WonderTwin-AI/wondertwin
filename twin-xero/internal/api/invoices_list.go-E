package api

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

var guidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// summaryInvoice is an invoice without its line items, which is what a list
// without page returns (page-002). Which parts Xero leaves out is a guess.
func summaryInvoice(in store.Invoice) store.Invoice {
	in.LineItems = nil
	return in
}

// invoiceGetter reads an invoice's fields for a where expression: the scalar
// fields by their JSON names, the optional dates (null when unset), and
// Contact.ContactID, the path most invoice filters use.
func invoiceGetter(in store.Invoice) Getter {
	base := fieldGetter(in)
	date := map[string]*time.Time{"Date": in.Date, "DueDate": in.DueDate, "ExpectedPaymentDate": in.ExpectedPaymentDate,
		"PlannedPaymentDate": in.PlannedPaymentDate, "FullyPaidOnDate": in.FullyPaidOnDate}
	return func(path string) (any, bool) {
		if t, ok := date[path]; ok {
			if t == nil {
				return nil, true
			}
			return *t, true
		}
		if path == "Contact.ContactID" {
			if in.Contact == nil {
				return nil, true
			}
			return in.Contact.ContactID, true
		}
		return base(path)
	}
}

// Invoices serves GET /api.xro/2.0/Invoices: the tenant's invoices, narrowed by
// where and the explicit IDs, InvoiceNumbers, ContactIDs and Statuses filters
// (query-002), then paged (page-001 to page-003). order and If-Modified-Since are
// accepted and ignored (GAP-002), and so are the parameters that cannot change
// the answer here: includeArchived (no invoice is archived), createdByMyApp (the
// one app made them all) and unitdp.
func (h *Handler) Invoices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var where Where
	if src := q.Get("where"); src != "" {
		var err error
		if where, err = ParseWhere(src); err != nil {
			writeWhereError(w, err)
			return
		}
	}
	filters, term := ParseFilters(q), strings.ToLower(q.Get("searchTerm"))
	var matched []store.Invoice
	for _, in := range h.store.Invoices.For(tenantID(r)).List() {
		ref := ""
		if in.Contact != nil {
			ref = in.Contact.ContactID
		}
		if !filters.Match(FilterFields{ID: in.InvoiceID, ContactID: ref, InvoiceNumber: in.InvoiceNumber, Status: in.Status}) {
			continue
		}
		if term != "" && !strings.Contains(strings.ToLower(in.InvoiceNumber), term) && !strings.Contains(strings.ToLower(in.Reference), term) {
			continue
		}
		if where != nil {
			ok, err := where.Match(invoiceGetter(in))
			if err != nil {
				writeWhereError(w, err)
				return
			}
			if !ok {
				continue
			}
		}
		matched = append(matched, in)
	}
	page, pg := paginate(r, matched, func(in store.Invoice) (time.Time, string) { return in.UpdatedDateUTC, in.InvoiceID }, summaryInvoice)
	out := make([]invoice, len(page))
	for i, in := range page {
		if strings.EqualFold(q.Get("summaryOnly"), "true") {
			in = summaryInvoice(in)
		}
		out[i] = h.wireInvoice(tenantID(r), in)
	}
	h.writeListEnvelope(w, http.StatusOK, "Invoices", out, pg)
}

// Invoice serves GET /api.xro/2.0/Invoices/{InvoiceID}: one invoice in full, in
// the plural envelope (crud-003). The path also takes an InvoiceNumber, told from
// an ID by its shape (ident-004). An unknown one is a 404; Xero's real body is not
// observed, so it is a problem-details guess.
func (h *Handler) Invoice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "InvoiceID")
	for _, in := range h.store.Invoices.For(tenantID(r)).List() {
		if (guidRE.MatchString(id) && strings.EqualFold(in.InvoiceID, id)) || (!guidRE.MatchString(id) && in.InvoiceNumber == id) {
			h.writeEnvelope(w, http.StatusOK, "Invoices", []invoice{h.wireInvoice(tenantID(r), in)})
			return
		}
	}
	h.writeProblem(w, r, http.StatusNotFound, "Not Found", "The resource you're looking for cannot be found")
}
