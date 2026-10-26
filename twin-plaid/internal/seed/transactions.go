package seed

import (
	_ "embed"
	"encoding/json"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

//go:embed transactions.json
var transactionsJSON []byte

type transactionSamples struct {
	GetRows  []map[string]any `json:"get_rows"`
	Template map[string]any   `json:"template"`
}

func loadTransactionSamples() transactionSamples {
	var f transactionSamples
	if err := json.Unmarshal(transactionsJSON, &f); err != nil {
		panic("seed: transactions.json: " + err.Error())
	}
	return f
}

// DefaultTransactions are the rows a new Item starts with, on its checking
// account: the two transactions in Plaid's /transactions/get example, verbatim
// but for a new id, the account, and dates moved so the newest is the day before
// now. Without the move a query for the last 30 days would find nothing, as the
// examples are dated 2023. This is provisional: Plaid's Sandbox gives a long
// generated history that is not published.
func DefaultTransactions(itemID, accountID string, newID func() string, now time.Time) []store.Transaction {
	rows := loadTransactionSamples().GetRows
	newest := time.Time{}
	for _, r := range rows {
		if d, err := time.Parse("2006-01-02", r["date"].(string)); err == nil && d.After(newest) {
			newest = d
		}
	}
	days := int(now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -1).Sub(newest).Hours() / 24)

	out := make([]store.Transaction, 0, len(rows))
	for _, r := range rows {
		body := r
		body["account_id"], body["transaction_id"] = accountID, newID()
		for _, k := range []string{"date", "authorized_date"} {
			body[k] = shift(body[k], "2006-01-02", days)
		}
		for _, k := range []string{"datetime", "authorized_datetime"} {
			body[k] = shift(body[k], time.RFC3339, days)
		}
		out = append(out, store.Transaction{ID: body["transaction_id"].(string), ItemID: itemID, AccountID: accountID,
			Date: body["date"].(string), Amount: body["amount"].(float64), Pending: body["pending"].(bool), Body: body})
	}
	return out
}

// shift moves a date or date-time field by whole days, leaving null as null.
func shift(v any, layout string, days int) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	t, err := time.Parse(layout, s)
	if err != nil {
		return v
	}
	return t.AddDate(0, 0, days).Format(layout)
}
