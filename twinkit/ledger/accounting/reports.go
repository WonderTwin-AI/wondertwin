package accounting

import "time"

// TrialBalanceReport computes a trial balance from journal entries as of the given time.
// If asOf is zero, uses all entries.
func (e *Engine) TrialBalanceReport(asOf time.Time) TrialBalance {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if asOf.IsZero() {
		asOf = e.clock.Now()
	}

	var rows []TrialBalanceRow
	var totalDebit, totalCredit int64

	for _, acct := range e.accounts {
		var d, c int64
		if asOf.IsZero() {
			d, c, _ = e.journal.Balance(acct.ID)
		} else {
			d, c, _ = e.journal.BalanceAt(acct.ID, asOf)
		}
		if d == 0 && c == 0 {
			continue
		}
		rows = append(rows, TrialBalanceRow{
			AccountID:   acct.ID,
			AccountName: acct.Name,
			AccountType: acct.Type,
			Debit:       d,
			Credit:      c,
		})
		totalDebit += d
		totalCredit += c
	}

	return TrialBalance{
		Rows:        rows,
		TotalDebit:  totalDebit,
		TotalCredit: totalCredit,
		AsOf:        asOf,
	}
}

// ProfitAndLossReport computes a P&L report for the given date range.
// Revenue accounts contribute to Revenue section, Expense accounts to Expenses.
func (e *Engine) ProfitAndLossReport(from, to time.Time) ProfitAndLoss {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var revRows []ReportRow
	var expRows []ReportRow
	var revTotal, expTotal int64

	for _, acct := range e.accounts {
		entries := e.journal.EntriesBetween(acct.ID, from, to)
		if len(entries) == 0 {
			continue
		}

		var d, c int64
		for _, ent := range entries {
			switch ent.Type {
			case "debit":
				d += ent.Amount
			case "credit":
				c += ent.Amount
			}
		}

		switch acct.Type {
		case AccountTypeRevenue:
			// Revenue is normally a credit balance (credit - debit).
			amount := c - d
			revRows = append(revRows, ReportRow{
				AccountID:   acct.ID,
				AccountName: acct.Name,
				AccountType: acct.Type,
				Amount:      amount,
			})
			revTotal += amount
		case AccountTypeExpense:
			// Expenses are normally a debit balance (debit - credit).
			amount := d - c
			expRows = append(expRows, ReportRow{
				AccountID:   acct.ID,
				AccountName: acct.Name,
				AccountType: acct.Type,
				Amount:      amount,
			})
			expTotal += amount
		}
	}

	return ProfitAndLoss{
		Revenue:   ReportSection{Title: "Revenue", Rows: revRows, Total: revTotal},
		Expenses:  ReportSection{Title: "Expenses", Rows: expRows, Total: expTotal},
		NetProfit: revTotal - expTotal,
		FromDate:  from,
		ToDate:    to,
	}
}

// BalanceSheetReport computes a balance sheet as of the given time.
// Assets, Liabilities, and Equity sections are computed from journal balances.
func (e *Engine) BalanceSheetReport(asOf time.Time) BalanceSheet {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if asOf.IsZero() {
		asOf = e.clock.Now()
	}

	var assetRows, liabRows, eqRows []ReportRow
	var assetTotal, liabTotal, eqTotal int64

	for _, acct := range e.accounts {
		var d, c int64
		d, c, _ = e.journal.BalanceAt(acct.ID, asOf)
		if d == 0 && c == 0 {
			continue
		}

		switch acct.Type {
		case AccountTypeAsset:
			// Assets have debit normal balance.
			amount := d - c
			assetRows = append(assetRows, ReportRow{
				AccountID:   acct.ID,
				AccountName: acct.Name,
				AccountType: acct.Type,
				Amount:      amount,
			})
			assetTotal += amount
		case AccountTypeLiability:
			// Liabilities have credit normal balance.
			amount := c - d
			liabRows = append(liabRows, ReportRow{
				AccountID:   acct.ID,
				AccountName: acct.Name,
				AccountType: acct.Type,
				Amount:      amount,
			})
			liabTotal += amount
		case AccountTypeEquity:
			// Equity has credit normal balance.
			amount := c - d
			eqRows = append(eqRows, ReportRow{
				AccountID:   acct.ID,
				AccountName: acct.Name,
				AccountType: acct.Type,
				Amount:      amount,
			})
			eqTotal += amount
		}
	}

	return BalanceSheet{
		Assets:      ReportSection{Title: "Assets", Rows: assetRows, Total: assetTotal},
		Liabilities: ReportSection{Title: "Liabilities", Rows: liabRows, Total: liabTotal},
		Equity:      ReportSection{Title: "Equity", Rows: eqRows, Total: eqTotal},
		AsOf:        asOf,
	}
}
