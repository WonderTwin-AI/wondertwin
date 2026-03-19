package api

import (
	"net/http"
	"time"
)

// AgedReceivables computes AR aging from unpaid invoices, grouped by customer.
func (h *Handler) AgedReceivables(w http.ResponseWriter, r *http.Request) {
	now := h.store.Clock.Now()

	type customerAging struct {
		CustomerName string
		Current      float64
		Days1_30     float64
		Days31_60    float64
		Days61_90    float64
		Days91Plus   float64
		Total        float64
	}

	aging := make(map[string]*customerAging)

	for _, inv := range h.store.Invoices.List() {
		if inv.Balance <= 0 {
			continue
		}
		custID := "unknown"
		custName := ""
		if inv.CustomerRef != nil {
			custID = inv.CustomerRef.Value
			custName = inv.CustomerRef.Name
			if custName == "" {
				if c, ok := h.store.Customers.Get(custID); ok {
					custName = c.DisplayName
				}
			}
		}

		ca, ok := aging[custID]
		if !ok {
			ca = &customerAging{CustomerName: custName}
			aging[custID] = ca
		}

		dueDate := inv.DueDate
		if dueDate == "" {
			dueDate = inv.TxnDate
		}
		if dueDate == "" {
			dueDate = inv.MetaData.CreateTime
		}

		days := daysSince(dueDate, now)

		switch {
		case days <= 0:
			ca.Current += inv.Balance
		case days <= 30:
			ca.Days1_30 += inv.Balance
		case days <= 60:
			ca.Days31_60 += inv.Balance
		case days <= 90:
			ca.Days61_90 += inv.Balance
		default:
			ca.Days91Plus += inv.Balance
		}
		ca.Total += inv.Balance
	}

	var rows []map[string]any
	for _, ca := range aging {
		rows = append(rows, map[string]any{
			"CustomerName": ca.CustomerName,
			"Current":      ca.Current,
			"1-30":         ca.Days1_30,
			"31-60":        ca.Days31_60,
			"61-90":        ca.Days61_90,
			"91+":          ca.Days91Plus,
			"Total":        ca.Total,
		})
	}

	qboJSON(w, http.StatusOK, map[string]any{
		"Header": map[string]any{
			"ReportName": "AgedReceivables",
			"Time":       h.store.Now(),
		},
		"Rows": rows,
	})
}

// AgedPayables computes AP aging from unpaid bills, grouped by vendor.
func (h *Handler) AgedPayables(w http.ResponseWriter, r *http.Request) {
	now := h.store.Clock.Now()

	type vendorAging struct {
		VendorName string
		Current    float64
		Days1_30   float64
		Days31_60  float64
		Days61_90  float64
		Days91Plus float64
		Total      float64
	}

	aging := make(map[string]*vendorAging)

	for _, bill := range h.store.Bills.List() {
		if bill.Balance <= 0 {
			continue
		}
		vendID := "unknown"
		vendName := ""
		if bill.VendorRef != nil {
			vendID = bill.VendorRef.Value
			vendName = bill.VendorRef.Name
			if vendName == "" {
				if v, ok := h.store.Vendors.Get(vendID); ok {
					vendName = v.DisplayName
				}
			}
		}

		va, ok := aging[vendID]
		if !ok {
			va = &vendorAging{VendorName: vendName}
			aging[vendID] = va
		}

		dueDate := bill.DueDate
		if dueDate == "" {
			dueDate = bill.TxnDate
		}
		if dueDate == "" {
			dueDate = bill.MetaData.CreateTime
		}

		days := daysSince(dueDate, now)

		switch {
		case days <= 0:
			va.Current += bill.Balance
		case days <= 30:
			va.Days1_30 += bill.Balance
		case days <= 60:
			va.Days31_60 += bill.Balance
		case days <= 90:
			va.Days61_90 += bill.Balance
		default:
			va.Days91Plus += bill.Balance
		}
		va.Total += bill.Balance
	}

	var rows []map[string]any
	for _, va := range aging {
		rows = append(rows, map[string]any{
			"VendorName": va.VendorName,
			"Current":    va.Current,
			"1-30":       va.Days1_30,
			"31-60":      va.Days31_60,
			"61-90":      va.Days61_90,
			"91+":        va.Days91Plus,
			"Total":      va.Total,
		})
	}

	qboJSON(w, http.StatusOK, map[string]any{
		"Header": map[string]any{
			"ReportName": "AgedPayables",
			"Time":       h.store.Now(),
		},
		"Rows": rows,
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

// daysSince parses a date string and returns the number of days between it and now.
func daysSince(dateStr string, now time.Time) int {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05-07:00",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return int(now.Sub(t).Hours() / 24)
		}
	}
	return 0
}
