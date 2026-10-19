package api

import (
	"net/url"
	"strings"
)

// Filters are the explicit comma-separated list filters Xero offers beside
// where (quirk query-002), the fast path for Contacts and Invoices. An empty
// list means the filter was not sent. The parameter names are matched exactly.
type Filters struct {
	IDs, ContactIDs, InvoiceNumbers, Statuses, References []string
}

// FilterFields are the values of a record that the explicit filters test. A
// handler fills in the ones its resource has and sends only the filters it
// supports.
type FilterFields struct {
	ID, ContactID, InvoiceNumber, Status, Reference string
}

// ParseFilters reads the explicit filters from a query. Items are trimmed and
// empty items dropped.
func ParseFilters(q url.Values) Filters {
	return Filters{
		IDs: splitList(q.Get("IDs")), ContactIDs: splitList(q.Get("ContactIDs")),
		InvoiceNumbers: splitList(q.Get("InvoiceNumbers")), Statuses: splitList(q.Get("Statuses")),
		References: splitList(q.Get("References")),
	}
}

func splitList(raw string) []string {
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Match reports whether a record passes every filter that was sent: it must be
// in each sent list, and a list matches any of its items. IDs and ContactIDs are
// GUIDs and match without regard to case; the rest match exactly.
func (f Filters) Match(r FilterFields) bool {
	return in(f.IDs, r.ID, true) && in(f.ContactIDs, r.ContactID, true) &&
		in(f.InvoiceNumbers, r.InvoiceNumber, false) && in(f.Statuses, r.Status, false) &&
		in(f.References, r.Reference, false)
}

func in(list []string, v string, fold bool) bool {
	if len(list) == 0 {
		return true
	}
	for _, item := range list {
		if item == v || fold && strings.EqualFold(item, v) {
			return true
		}
	}
	return false
}
