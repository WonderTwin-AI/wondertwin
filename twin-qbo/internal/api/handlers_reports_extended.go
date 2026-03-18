package api

import (
	"math"
	"net/http"
	"time"
)

// AgedReceivables computes AR aging from unpaid invoices.
func (h *Handler) AgedReceivables(w http.ResponseWriter, r *http.Request) {
	now := h.store.Clock.Now()
	var rows []map[string]any
	var total float64

	for _, inv := range h.store.Invoices.List() {
		if inv.Balance <= 0 {
			continue
		}
		dueDate := parseDateParam(inv.DueDate)
		daysOverdue := 0
		if !dueDate.IsZero() {
			daysOverdue = int(math.Max(0, now.Sub(dueDate).Hours()/24))
		}
		bucket := agingBucket(daysOverdue)
		rows = append(rows, map[string]any{
			"InvoiceId":    inv.Id,
			"CustomerRef":  inv.CustomerRef,
			"DueDate":      inv.DueDate,
			"Balance":      inv.Balance,
			"DaysOverdue":  daysOverdue,
			"AgingBucket":  bucket,
		})
		total += inv.Balance
	}

	qboJSON(w, http.StatusOK, map[string]any{
		"Header": map[string]any{"ReportName": "AgedReceivables"},
		"Rows":   rows,
		"Total":  total,
		"time":   time.Now().Format(time.RFC3339),
	})
}

// AgedPayables computes AP aging from unpaid bills.
func (h *Handler) AgedPayables(w http.ResponseWriter, r *http.Request) {
	now := h.store.Clock.Now()
	var rows []map[string]any
	var total float64

	for _, bill := range h.store.Bills.List() {
		if bill.Balance <= 0 {
			continue
		}
		dueDate := parseDateParam(bill.DueDate)
		daysOverdue := 0
		if !dueDate.IsZero() {
			daysOverdue = int(math.Max(0, now.Sub(dueDate).Hours()/24))
		}
		bucket := agingBucket(daysOverdue)
		rows = append(rows, map[string]any{
			"BillId":       bill.Id,
			"VendorRef":    bill.VendorRef,
			"DueDate":      bill.DueDate,
			"Balance":      bill.Balance,
			"DaysOverdue":  daysOverdue,
			"AgingBucket":  bucket,
		})
		total += bill.Balance
	}

	qboJSON(w, http.StatusOK, map[string]any{
		"Header": map[string]any{"ReportName": "AgedPayables"},
		"Rows":   rows,
		"Total":  total,
		"time":   time.Now().Format(time.RFC3339),
	})
}

// CustomerBalance computes outstanding balance per customer from unpaid invoices.
func (h *Handler) CustomerBalance(w http.ResponseWriter, r *http.Request) {
	balances := map[string]float64{}
	names := map[string]string{}

	for _, inv := range h.store.Invoices.List() {
		if inv.Balance <= 0 || inv.CustomerRef == nil {
			continue
		}
		balances[inv.CustomerRef.Value] += inv.Balance
		if inv.CustomerRef.Name != "" {
			names[inv.CustomerRef.Value] = inv.CustomerRef.Name
		}
	}

	var rows []map[string]any
	var total float64
	for id, bal := range balances {
		rows = append(rows, map[string]any{
			"CustomerId":   id,
			"CustomerName": names[id],
			"Balance":      bal,
		})
		total += bal
	}

	qboJSON(w, http.StatusOK, map[string]any{
		"Header": map[string]any{"ReportName": "CustomerBalance"},
		"Rows":   rows,
		"Total":  total,
		"time":   time.Now().Format(time.RFC3339),
	})
}

// VendorBalance computes outstanding balance per vendor from unpaid bills.
func (h *Handler) VendorBalance(w http.ResponseWriter, r *http.Request) {
	balances := map[string]float64{}
	names := map[string]string{}

	for _, bill := range h.store.Bills.List() {
		if bill.Balance <= 0 || bill.VendorRef == nil {
			continue
		}
		balances[bill.VendorRef.Value] += bill.Balance
		if bill.VendorRef.Name != "" {
			names[bill.VendorRef.Value] = bill.VendorRef.Name
		}
	}

	var rows []map[string]any
	var total float64
	for id, bal := range balances {
		rows = append(rows, map[string]any{
			"VendorId":   id,
			"VendorName": names[id],
			"Balance":    bal,
		})
		total += bal
	}

	qboJSON(w, http.StatusOK, map[string]any{
		"Header": map[string]any{"ReportName": "VendorBalance"},
		"Rows":   rows,
		"Total":  total,
		"time":   time.Now().Format(time.RFC3339),
	})
}

// GeneralLedger lists all journal transactions.
func (h *Handler) GeneralLedger(w http.ResponseWriter, r *http.Request) {
	txs := h.engine.Journal().Transactions()
	var rows []map[string]any
	for _, tx := range txs {
		for _, entry := range tx.Entries {
			rows = append(rows, map[string]any{
				"TransactionId": tx.ID,
				"Date":          tx.CreatedAt.Format("2006-01-02"),
				"AccountId":     entry.AccountID,
				"Type":          string(entry.Type),
				"Amount":        float64(entry.Amount) / 100,
				"SourceType":    entry.SourceType,
				"SourceId":      entry.SourceID,
			})
		}
	}

	qboJSON(w, http.StatusOK, map[string]any{
		"Header": map[string]any{"ReportName": "GeneralLedger"},
		"Rows":   rows,
		"time":   time.Now().Format(time.RFC3339),
	})
}

func agingBucket(daysOverdue int) string {
	switch {
	case daysOverdue == 0:
		return "Current"
	case daysOverdue <= 30:
		return "1-30"
	case daysOverdue <= 60:
		return "31-60"
	case daysOverdue <= 90:
		return "61-90"
	default:
		return "91+"
	}
}
