package api

import (
	"net/http"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/ledger"
)

func (h *Handler) TrialBalance(w http.ResponseWriter, r *http.Request) {
	asOf := parseQueryDate(r, "date")
	tb := h.engine.TrialBalanceReport(asOf)

	var rows []map[string]any
	for _, row := range tb.Rows {
		rows = append(rows, map[string]any{
			"AccountID": row.AccountID,
			"Account":   row.AccountName,
			"Debit":     float64(row.Debit) / 100,
			"Credit":    float64(row.Credit) / 100,
		})
	}

	xeroJSON(w, http.StatusOK, map[string]any{
		"Reports": []map[string]any{{
			"ReportID":    "TrialBalance",
			"ReportName":  "Trial Balance",
			"ReportDate":  tb.AsOf.Format("2006-01-02"),
			"Rows":        rows,
			"TotalDebit":  float64(tb.TotalDebit) / 100,
			"TotalCredit": float64(tb.TotalCredit) / 100,
		}},
	})
}

func (h *Handler) ProfitAndLoss(w http.ResponseWriter, r *http.Request) {
	from := parseQueryDate(r, "fromDate")
	to := parseQueryDate(r, "toDate")
	if from.IsZero() {
		from = time.Now().AddDate(0, -1, 0)
	}
	if to.IsZero() {
		to = time.Now()
	}

	pnl := h.engine.ProfitAndLossReport(from, to)

	xeroJSON(w, http.StatusOK, map[string]any{
		"Reports": []map[string]any{{
			"ReportID":   "ProfitAndLoss",
			"ReportName": "Profit and Loss",
			"Revenue":    reportSectionToXero(pnl.Revenue),
			"Expenses":   reportSectionToXero(pnl.Expenses),
			"NetProfit":  float64(pnl.NetProfit) / 100,
		}},
	})
}

func (h *Handler) BalanceSheet(w http.ResponseWriter, r *http.Request) {
	asOf := parseQueryDate(r, "date")
	bs := h.engine.BalanceSheetReport(asOf)

	xeroJSON(w, http.StatusOK, map[string]any{
		"Reports": []map[string]any{{
			"ReportID":    "BalanceSheet",
			"ReportName":  "Balance Sheet",
			"Assets":      reportSectionToXero(bs.Assets),
			"Liabilities": reportSectionToXero(bs.Liabilities),
			"Equity":      reportSectionToXero(bs.Equity),
		}},
	})
}

func parseQueryDate(r *http.Request, param string) time.Time {
	s := r.URL.Query().Get(param)
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func reportSectionToXero(s ledger.ReportSection) map[string]any {
	var rows []map[string]any
	for _, row := range s.Rows {
		rows = append(rows, map[string]any{
			"AccountID": row.AccountID,
			"Account":   row.AccountName,
			"Amount":    float64(row.Amount) / 100,
		})
	}
	return map[string]any{
		"Title": s.Title,
		"Rows":  rows,
		"Total": float64(s.Total) / 100,
	}
}
