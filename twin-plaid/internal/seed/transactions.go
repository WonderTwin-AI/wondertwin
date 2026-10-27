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
	GetRows  []store.TransactionBody `json:"get_rows"`
	Template store.TransactionBody   `json:"template"`
}

// loadTransactionSamples decodes the recorded rows into the typed body and
// refuses a row that would break the SDK, so a bad recording fails at start.
func loadTransactionSamples() transactionSamples {
	var f transactionSamples
	if err := json.Unmarshal(transactionsJSON, &f); err != nil {
		panic("seed: transactions.json: " + err.Error())
	}
	for _, b := range append([]store.TransactionBody{f.Template}, f.GetRows...) {
		if err := b.Validate(); err != nil {
			panic("seed: transactions.json: " + err.Error())
		}
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
		if d, err := time.Parse("2006-01-02", r.Date); err == nil && d.After(newest) {
			newest = d
		}
	}
	days := int(now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -1).Sub(newest).Hours() / 24)

	out := make([]store.Transaction, 0, len(rows))
	for _, body := range rows {
		body.AccountID, body.TransactionID = accountID, newID()
		body.Date = shift(body.Date, "2006-01-02", days)
		body.AuthorizedDate = shiftPtr(body.AuthorizedDate, "2006-01-02", days)
		body.AuthorizedDatetime = shiftPtr(body.AuthorizedDatetime, time.RFC3339, days)
		body.Datetime = shiftPtr(body.Datetime, time.RFC3339, days)
		out = append(out, store.Transaction{ID: body.TransactionID, ItemID: itemID, AccountID: accountID,
			Date: body.Date, Amount: body.Amount, Pending: body.Pending, Body: body})
	}
	return out
}

// shift moves a date or date-time by whole days; a value that does not parse is
// left as it is.
func shift(v, layout string, days int) string {
	t, err := time.Parse(layout, v)
	if err != nil {
		return v
	}
	return t.AddDate(0, 0, days).Format(layout)
}

// shiftPtr is shift for a nullable field, leaving null as null.
func shiftPtr(v *string, layout string, days int) *string {
	if v == nil {
		return nil
	}
	out := shift(*v, layout, days)
	return &out
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
	body.MerchantName, body.MerchantEntityID, body.MerchantCategoryCode = store.Null[string](), store.Null[string](), store.Null[string]()
	body.LogoURL, body.Website, body.RunningBalance = store.Null[string](), store.Null[string](), store.Null[float64]()
	body.PendingTransactionID, body.Datetime, body.AuthorizedDatetime, body.UnofficialCurrencyCode = nil, nil, nil, nil
	// Optional and not nullable in the model: unset, so absent and never null (quirk ENVELOPE-004).
	body.PersonalFinanceCategory, body.PersonalFinanceCategoryIconURL = store.Opt[store.PersonalFinanceCategory]{}, store.Opt[string]{}
	body.Location = store.Location{}
	body.Counterparties = store.Some([]store.Counterparty{})
	body.AccountID, body.TransactionID = accountID, id
	body.Amount, body.IsoCurrencyCode = in.Amount, &in.Currency
	body.Date, body.AuthorizedDate = in.DatePosted, &in.DateTransacted
	body.Name, body.PaymentChannel, body.Pending, body.TransactionType = in.Description, "other", false, store.Some("unresolved")
	return store.Transaction{ID: id, ItemID: itemID, AccountID: accountID, Date: in.DatePosted, Amount: in.Amount, Body: body}
}
