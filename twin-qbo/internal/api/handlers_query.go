package api

import (
	"net/http"
	"strings"
)

// Query handles the QBO SQL-like query endpoint.
func (h *Handler) Query(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("query")
	if q == "" {
		validationFault(w, "500", "Query required", "The query parameter is required.")
		return
	}

	pq, err := ParseQuery(q)
	if err != nil {
		validationFault(w, "500", "Invalid query", err.Error())
		return
	}

	entity := strings.ToLower(pq.Entity)

	switch entity {
	case "customer":
		items := h.store.Customers.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		if pq.IsCount {
			qboJSON(w, http.StatusOK, queryResponse("Customer", nil, 0, 0, len(items)))
		} else {
			qboJSON(w, http.StatusOK, queryResponse("Customer", page, pq.StartPosition, len(page), len(items)))
		}
	case "vendor":
		items := h.store.Vendors.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Vendor", page, pq.StartPosition, len(page), len(items)))
	case "account":
		items := h.store.Accounts.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Account", page, pq.StartPosition, len(page), len(items)))
	case "item":
		items := h.store.Items.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Item", page, pq.StartPosition, len(page), len(items)))
	case "invoice":
		items := h.store.Invoices.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Invoice", page, pq.StartPosition, len(page), len(items)))
	case "bill":
		items := h.store.Bills.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Bill", page, pq.StartPosition, len(page), len(items)))
	case "payment":
		items := h.store.Payments.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Payment", page, pq.StartPosition, len(page), len(items)))
	case "billpayment":
		items := h.store.BillPayments.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("BillPayment", page, pq.StartPosition, len(page), len(items)))
	case "creditmemo":
		items := h.store.CreditMemos.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("CreditMemo", page, pq.StartPosition, len(page), len(items)))
	case "vendorcredit":
		items := h.store.VendorCredits.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("VendorCredit", page, pq.StartPosition, len(page), len(items)))
	case "salesreceipt":
		items := h.store.SalesReceipts.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("SalesReceipt", page, pq.StartPosition, len(page), len(items)))
	case "deposit":
		items := h.store.Deposits.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Deposit", page, pq.StartPosition, len(page), len(items)))
	case "transfer":
		items := h.store.Transfers.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Transfer", page, pq.StartPosition, len(page), len(items)))
	case "journalentry":
		items := h.store.JournalEntries.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("JournalEntry", page, pq.StartPosition, len(page), len(items)))
	case "estimate":
		items := h.store.Estimates.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Estimate", page, pq.StartPosition, len(page), len(items)))
	case "purchase":
		items := h.store.Purchases.List()
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Purchase", page, pq.StartPosition, len(page), len(items)))
	default:
		validationFault(w, "500", "Invalid entity", "Entity '"+pq.Entity+"' is not supported.")
	}
}
