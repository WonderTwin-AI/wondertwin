package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// summaryBankTransaction is a bank transaction without its line items, which is
// what a list without page returns. Xero's documented list example carries no
// LineItems and the docs say paging returns them, so this is inference.
func summaryBankTransaction(in store.BankTransaction) store.BankTransaction {
	in.LineItems = nil
	return in
}

// bankTransactionGetter reads a bank transaction's fields for a where
// expression: the scalar fields by their JSON names, Date (null when unset), and
// the two paths Xero optimises besides Type, Status and Date: Contact.ContactID
// and BankAccount.Code.
func bankTransactionGetter(in store.BankTransaction) Getter {
	base := fieldGetter(in)
	return func(path string) (any, bool) {
		switch path {
		case "Date":
			if in.Date == nil {
				return nil, true
			}
			return *in.Date, true
		case "Contact.ContactID":
			if in.Contact == nil {
				return nil, true
			}
			return in.Contact.ContactID, true
		case "BankAccount.Code":
			if in.BankAccount == nil {
				return nil, true
			}
			return in.BankAccount.Code, true
		}
		return base(path)
	}
}

// BankTransactions serves GET /api.xro/2.0/BankTransactions: the tenant's bank
// transactions, narrowed by where and References, the one explicit list filter
// Xero documents here (query-002), then paged with the default order
// UpdatedDateUTC then BankTransactionID (page-001 to page-003). order and
// If-Modified-Since are accepted and ignored (GAP-002), and unitdp cannot change
// a stored amount.
func (h *Handler) BankTransactions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var where Where
	if src := q.Get("where"); src != "" {
		var err error
		if where, err = ParseWhere(src); err != nil {
			writeWhereError(w, err)
			return
		}
	}
	filters := Filters{References: ParseFilters(q).References}
	var matched []store.BankTransaction
	for _, bt := range h.store.BankTransactions.For(tenantID(r)).List() {
		if !filters.Match(FilterFields{Reference: bt.Reference}) {
			continue
		}
		if where != nil {
			ok, err := where.Match(bankTransactionGetter(bt))
			if err != nil {
				writeWhereError(w, err)
				return
			}
			if !ok {
				continue
			}
		}
		matched = append(matched, bt)
	}
	page, pg := paginate(r, matched, func(bt store.BankTransaction) (time.Time, string) { return bt.UpdatedDateUTC, bt.BankTransactionID },
		summaryBankTransaction)
	out := make([]bankTransaction, len(page))
	for i, bt := range page {
		out[i] = h.wireBankTransaction(tenantID(r), bt)
	}
	h.writeListEnvelope(w, http.StatusOK, "BankTransactions", out, pg)
}

// BankTransaction serves GET /api.xro/2.0/BankTransactions/{BankTransactionID}:
// one bank transaction in full, in the plural envelope (crud-003). An unknown ID
// is a 404 whose body is a guess, as on invoices.
func (h *Handler) BankTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "BankTransactionID")
	for _, bt := range h.store.BankTransactions.For(tenantID(r)).List() {
		if strings.EqualFold(bt.BankTransactionID, id) {
			h.writeEnvelope(w, http.StatusOK, "BankTransactions", []bankTransaction{h.wireBankTransaction(tenantID(r), bt)})
			return
		}
	}
	h.writeProblem(w, r, http.StatusNotFound, "Not Found", "The resource you're looking for cannot be found")
}
