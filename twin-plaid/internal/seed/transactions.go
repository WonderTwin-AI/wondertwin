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

// CustomTransaction is what a client gives /sandbox/transactions/create.
type CustomTransaction struct {
	Amount         float64
	Description    string
	DatePosted     string
	DateTransacted string
	Currency       string
}

// NewCustomTransaction builds the row for a transaction a client created. It
// starts from the added transaction in Plaid's /transactions/sync example so
// every documented field is present with its presence rule, then sets what the
// client gave and clears what Plaid would have to look up: no merchant, no
// counterparties, no location, no category (the category fields are left out). The posted date is the date and the
// transacted date is the authorized date; both date-times are null.
func NewCustomTransaction(itemID, accountID, id string, in CustomTransaction) store.Transaction {
	body := loadTransactionSamples().Template
	for _, k := range []string{"merchant_name", "merchant_entity_id", "merchant_category_code", "logo_url", "website",
		"pending_transaction_id", "running_balance", "datetime", "authorized_datetime", "unofficial_currency_code"} {
		body[k] = nil
	}
	// Optional and not nullable in the model: absent, never null (quirk ENVELOPE-004).
	delete(body, "personal_finance_category")
	delete(body, "personal_finance_category_icon_url")
	body["location"] = map[string]any{"address": nil, "city": nil, "region": nil, "postal_code": nil, "country": nil,
		"lat": nil, "lon": nil, "store_number": nil}
	body["counterparties"] = []any{}
	body["account_id"], body["transaction_id"] = accountID, id
	body["amount"], body["iso_currency_code"] = in.Amount, in.Currency
	body["date"], body["authorized_date"] = in.DatePosted, in.DateTransacted
	body["name"], body["payment_channel"], body["pending"], body["transaction_type"] = in.Description, "other", false, "unresolved"
	return store.Transaction{ID: id, ItemID: itemID, AccountID: accountID, Date: in.DatePosted, Amount: in.Amount, Body: body}
}
