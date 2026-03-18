// Package accounting provides an accounting ledger engine that enforces
// double-entry bookkeeping, document state machines, period locking, and
// financial report generation. Platform-specific behavior is delegated to hooks.
package accounting

import "time"

// AccountType classifies an account for chart-of-accounts purposes.
type AccountType string

const (
	AccountTypeAsset     AccountType = "ASSET"
	AccountTypeEquity    AccountType = "EQUITY"
	AccountTypeExpense   AccountType = "EXPENSE"
	AccountTypeLiability AccountType = "LIABILITY"
	AccountTypeRevenue   AccountType = "REVENUE"
)

// AccountClass is a sub-classification within an AccountType.
type AccountClass string

const (
	ClassCurrentAsset     AccountClass = "CURRENT_ASSET"
	ClassFixedAsset       AccountClass = "FIXED_ASSET"
	ClassCurrentLiability AccountClass = "CURRENT_LIABILITY"
	ClassLongTermLiab     AccountClass = "LONG_TERM_LIABILITY"
	ClassEquity           AccountClass = "EQUITY"
	ClassRevenue          AccountClass = "REVENUE"
	ClassDirectCost       AccountClass = "DIRECT_COST"
	ClassExpense          AccountClass = "EXPENSE"
	ClassOtherIncome      AccountClass = "OTHER_INCOME"
	ClassOtherExpense     AccountClass = "OTHER_EXPENSE"
)

// Account represents a ledger account in the chart of accounts.
type Account struct {
	ID       string       `json:"id"`
	Code     string       `json:"code"`
	Name     string       `json:"name"`
	Type     AccountType  `json:"type"`
	Class    AccountClass `json:"class"`
	Currency string       `json:"currency"`
}

// DocumentType distinguishes invoices from bills and credit notes.
type DocumentType string

const (
	DocTypeInvoice    DocumentType = "INVOICE"
	DocTypeBill       DocumentType = "BILL"
	DocTypeCreditNote DocumentType = "CREDIT_NOTE"
)

// DocumentStatus represents the lifecycle state of a document.
type DocumentStatus string

const (
	StatusDraft      DocumentStatus = "DRAFT"
	StatusSubmitted  DocumentStatus = "SUBMITTED"
	StatusAuthorised DocumentStatus = "AUTHORISED"
	StatusPaid       DocumentStatus = "PAID"
	StatusVoided     DocumentStatus = "VOIDED"
	StatusDeleted    DocumentStatus = "DELETED"
)

// LineItem is a single line on a document.
type LineItem struct {
	Description string `json:"description"`
	AccountID   string `json:"account_id"`
	Quantity    int64  `json:"quantity"`    // scaled (e.g., 100 = 1.00)
	UnitAmount  int64  `json:"unit_amount"` // in minor currency units
	Amount      int64  `json:"amount"`      // quantity * unit_amount / 100
}

// Document represents an invoice, bill, or credit note.
type Document struct {
	ID          string         `json:"id"`
	Type        DocumentType   `json:"type"`
	Status      DocumentStatus `json:"status"`
	ContactID   string         `json:"contact_id"`
	Currency    string         `json:"currency"`
	LineItems   []LineItem     `json:"line_items"`
	SubTotal    int64          `json:"sub_total"`
	Total       int64          `json:"total"`
	AmountDue   int64          `json:"amount_due"`
	AmountPaid  int64          `json:"amount_paid"`
	Date        time.Time      `json:"date"`
	DueDate     time.Time      `json:"due_date,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// Payment represents a payment applied against a document.
type Payment struct {
	ID            string    `json:"id"`
	DocumentID    string    `json:"document_id"`
	DocumentType  DocumentType `json:"document_type"`
	BankAccountID string    `json:"bank_account_id"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	Date          time.Time `json:"date"`
	CreatedAt     time.Time `json:"created_at"`
}

// ManualJournal represents a manually posted journal entry.
type ManualJournal struct {
	ID        string             `json:"id"`
	Narration string             `json:"narration"`
	Lines     []ManualJournalLine `json:"lines"`
	Date      time.Time          `json:"date"`
	Status    string             `json:"status"` // DRAFT or POSTED
	CreatedAt time.Time          `json:"created_at"`
}

// ManualJournalLine is a single line in a manual journal.
type ManualJournalLine struct {
	AccountID   string `json:"account_id"`
	Description string `json:"description"`
	DebitAmount int64  `json:"debit_amount"`
	CreditAmount int64 `json:"credit_amount"`
}

// TrialBalanceRow is one row in a trial balance report.
type TrialBalanceRow struct {
	AccountID   string `json:"account_id"`
	AccountName string `json:"account_name"`
	AccountType AccountType `json:"account_type"`
	Debit       int64  `json:"debit"`
	Credit      int64  `json:"credit"`
}

// TrialBalance is the full trial balance report.
type TrialBalance struct {
	Rows       []TrialBalanceRow `json:"rows"`
	TotalDebit int64             `json:"total_debit"`
	TotalCredit int64            `json:"total_credit"`
	AsOf       time.Time         `json:"as_of"`
}

// ReportRow is a row in a P&L or balance sheet report.
type ReportRow struct {
	AccountID   string      `json:"account_id"`
	AccountName string      `json:"account_name"`
	AccountType AccountType `json:"account_type"`
	Amount      int64       `json:"amount"`
}

// ReportSection groups rows under a heading.
type ReportSection struct {
	Title string      `json:"title"`
	Rows  []ReportRow `json:"rows"`
	Total int64       `json:"total"`
}

// ProfitAndLoss is the P&L report.
type ProfitAndLoss struct {
	Revenue     ReportSection `json:"revenue"`
	Expenses    ReportSection `json:"expenses"`
	NetProfit   int64         `json:"net_profit"`
	FromDate    time.Time     `json:"from_date"`
	ToDate      time.Time     `json:"to_date"`
}

// BalanceSheet is the balance sheet report.
type BalanceSheet struct {
	Assets      ReportSection `json:"assets"`
	Liabilities ReportSection `json:"liabilities"`
	Equity      ReportSection `json:"equity"`
	AsOf        time.Time     `json:"as_of"`
}
