package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// summaryContact is a contact without the heavy parts, which is what a list
// without page returns (page-002). Which parts Xero leaves out is a guess.
func summaryContact(c store.Contact) store.Contact {
	c.ContactPersons, c.ContactGroups, c.Attachments, c.Balances, c.PaymentTerms = nil, nil, nil, nil, nil
	c.SalesTrackingCategories, c.PurchasesTrackingCategories, c.BatchPayments, c.BrandingTheme = nil, nil, nil, nil
	return c
}

// writeWhereError answers 400 for an expression that cannot be read or applied.
// Xero's real body is not observed; this is a ValidationException-shaped guess.
func writeWhereError(w http.ResponseWriter, err error) {
	twincore.JSON(w, http.StatusBadRequest, map[string]any{
		"ErrorNumber": 10, "Type": "ValidationException", "Message": err.Error(), "Elements": []any{},
	})
}

// Contacts serves GET /api.xro/2.0/Contacts: the tenant's contacts, narrowed by
// where, IDs, searchTerm and includeArchived, then paged (page-001 to page-003).
func (h *Handler) Contacts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var where Where
	if src := q.Get("where"); src != "" {
		var err error
		if where, err = ParseWhere(src); err != nil {
			writeWhereError(w, err)
			return
		}
	}
	filters, term, archived := ParseFilters(q), strings.ToLower(q.Get("searchTerm")), strings.EqualFold(q.Get("includeArchived"), "true")
	var matched []store.Contact
	for _, c := range h.store.Contacts.For(tenantID(r)).List() {
		if (c.ContactStatus == "ARCHIVED" && !archived) || !filters.Match(FilterFields{ID: c.ContactID}) || !searches(c, term) {
			continue
		}
		if where != nil {
			ok, err := where.Match(fieldGetter(c))
			if err != nil {
				writeWhereError(w, err)
				return
			}
			if !ok {
				continue
			}
		}
		matched = append(matched, c)
	}
	page, pg := paginate(r, matched, func(c store.Contact) (time.Time, string) { return c.UpdatedDateUTC, c.ContactID }, summaryContact)
	out := make([]contact, len(page))
	for i, c := range page {
		if strings.EqualFold(q.Get("summaryOnly"), "true") {
			c = summaryContact(c)
		}
		out[i] = wireContact(c)
	}
	h.writeListEnvelope(w, http.StatusOK, "Contacts", out, pg)
}

// searches reports whether a lower-cased search term appears in a contact's
// Name, FirstName, LastName, ContactNumber or EmailAddress. No term matches all.
func searches(c store.Contact, term string) bool {
	if term == "" {
		return true
	}
	for _, f := range []string{c.Name, c.FirstName, c.LastName, c.ContactNumber, c.EmailAddress} {
		if strings.Contains(strings.ToLower(f), term) {
			return true
		}
	}
	return false
}
