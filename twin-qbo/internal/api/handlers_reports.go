package api

import (
	"net/http"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
)

func (h *Handler) TrialBalance(w http.ResponseWriter, r *http.Request) {
	asOf := parseDateParam(r.URL.Query().Get("end_date"))
	tb := h.engine.TrialBalanceReport(asOf)

	var rows []map[string]any
	for _, row := range tb.Rows {
		rows = append(rows, map[string]any{
			"AccountId":   row.AccountID,
			"AccountName": row.AccountName,
			"AccountType": string(row.AccountType),
			"Debit":       float64(row.Debit) / 100,
			"Credit":      float64(row.Credit) / 100,
		})
	}

	qboJSON(w, http.StatusOK, map[string]any{
		"Header": map[string]any{
			"ReportName": "TrialBalance",
		},
		"Rows":    rows,
		"Columns": []string{"AccountName", "Debit", "Credit"},
		"time":    time.Now().Format(time.RFC3339),
	})
}

func (h *Handler) ProfitAndLoss(w http.ResponseWriter, r *http.Request) {
	from := parseDateParam(r.URL.Query().Get("start_date"))
	to := parseDateParam(r.URL.Query().Get("end_date"))
	if from.IsZero() {
		from = time.Now().AddDate(0, -1, 0)
	}
	if to.IsZero() {
		to = time.Now()
	}

	pnl := h.engine.ProfitAndLossReport(from, to)

	qboJSON(w, http.StatusOK, map[string]any{
		"Header": map[string]any{
			"ReportName":  "ProfitAndLoss",
			"StartPeriod": from.Format("2006-01-02"),
			"EndPeriod":   to.Format("2006-01-02"),
		},
		"Revenue":   reportSectionToQBO(pnl.Revenue),
		"Expenses":  reportSectionToQBO(pnl.Expenses),
		"NetIncome": float64(pnl.NetProfit) / 100,
		"time":      time.Now().Format(time.RFC3339),
	})
}

func (h *Handler) BalanceSheet(w http.ResponseWriter, r *http.Request) {
	asOf := parseDateParam(r.URL.Query().Get("end_date"))
	bs := h.engine.BalanceSheetReport(asOf)

	qboJSON(w, http.StatusOK, map[string]any{
		"Header": map[string]any{
			"ReportName": "BalanceSheet",
		},
		"Assets":      reportSectionToQBO(bs.Assets),
		"Liabilities": reportSectionToQBO(bs.Liabilities),
		"Equity":      reportSectionToQBO(bs.Equity),
		"time":        time.Now().Format(time.RFC3339),
	})
}

func parseDateParam(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func reportSectionToQBO(s accounting.ReportSection) map[string]any {
	var rows []map[string]any
	for _, row := range s.Rows {
		rows = append(rows, map[string]any{
			"AccountId":   row.AccountID,
			"AccountName": row.AccountName,
			"Amount":      float64(row.Amount) / 100,
		})
	}
	return map[string]any{
		"Title": s.Title,
		"Rows":  rows,
		"Total": float64(s.Total) / 100,
	}
}
