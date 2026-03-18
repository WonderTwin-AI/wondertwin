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
		items := FilterItems(h.store.Customers.List(), pq)
		if pq.IsCount {
			qboJSON(w, http.StatusOK, queryResponse("Customer", nil, 0, 0, len(items)))
		} else {
			page := Paginate(items, pq.StartPosition, pq.MaxResults)
			qboJSON(w, http.StatusOK, queryResponse("Customer", page, pq.StartPosition, len(page), len(items)))
		}
	case "vendor":
		items := FilterItems(h.store.Vendors.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Vendor", page, pq.StartPosition, len(page), len(items)))
	case "account":
		items := FilterItems(h.store.Accounts.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Account", page, pq.StartPosition, len(page), len(items)))
	case "item":
		items := FilterItems(h.store.Items.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Item", page, pq.StartPosition, len(page), len(items)))
	case "invoice":
		items := FilterItems(h.store.Invoices.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Invoice", page, pq.StartPosition, len(page), len(items)))
	case "bill":
		items := FilterItems(h.store.Bills.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Bill", page, pq.StartPosition, len(page), len(items)))
	case "payment":
		items := FilterItems(h.store.Payments.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Payment", page, pq.StartPosition, len(page), len(items)))
	case "billpayment":
		items := FilterItems(h.store.BillPayments.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("BillPayment", page, pq.StartPosition, len(page), len(items)))
	case "creditmemo":
		items := FilterItems(h.store.CreditMemos.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("CreditMemo", page, pq.StartPosition, len(page), len(items)))
	case "vendorcredit":
		items := FilterItems(h.store.VendorCredits.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("VendorCredit", page, pq.StartPosition, len(page), len(items)))
	case "salesreceipt":
		items := FilterItems(h.store.SalesReceipts.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("SalesReceipt", page, pq.StartPosition, len(page), len(items)))
	case "deposit":
		items := FilterItems(h.store.Deposits.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Deposit", page, pq.StartPosition, len(page), len(items)))
	case "transfer":
		items := FilterItems(h.store.Transfers.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Transfer", page, pq.StartPosition, len(page), len(items)))
	case "journalentry":
		items := FilterItems(h.store.JournalEntries.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("JournalEntry", page, pq.StartPosition, len(page), len(items)))
	case "estimate":
		items := FilterItems(h.store.Estimates.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Estimate", page, pq.StartPosition, len(page), len(items)))
	case "purchase":
		items := FilterItems(h.store.Purchases.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Purchase", page, pq.StartPosition, len(page), len(items)))
	case "employee":
		items := FilterItems(h.store.Employees.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Employee", page, pq.StartPosition, len(page), len(items)))
	case "class":
		items := FilterItems(h.store.Classes.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Class", page, pq.StartPosition, len(page), len(items)))
	case "department":
		items := FilterItems(h.store.Departments.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Department", page, pq.StartPosition, len(page), len(items)))
	case "term":
		items := FilterItems(h.store.Terms.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("Term", page, pq.StartPosition, len(page), len(items)))
	case "paymentmethod":
		items := FilterItems(h.store.PaymentMethods.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("PaymentMethod", page, pq.StartPosition, len(page), len(items)))
	case "taxcode":
		items := FilterItems(h.store.TaxCodes.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("TaxCode", page, pq.StartPosition, len(page), len(items)))
	case "taxrate":
		items := FilterItems(h.store.TaxRates.List(), pq)
		page := Paginate(items, pq.StartPosition, pq.MaxResults)
		qboJSON(w, http.StatusOK, queryResponse("TaxRate", page, pq.StartPosition, len(page), len(items)))
	default:
		validationFault(w, "500", "Invalid entity", "Entity '"+pq.Entity+"' is not supported.")
	}
}
