// Package store provides Xero-shaped entity types and the in-memory store.
package store

import "time"

// Contact represents a Xero contact (customer/supplier).
type Contact struct {
	ContactID     string `json:"ContactID"`
	Name          string `json:"Name"`
	FirstName     string `json:"FirstName,omitempty"`
	LastName      string `json:"LastName,omitempty"`
	EmailAddress  string `json:"EmailAddress,omitempty"`
	ContactStatus string `json:"ContactStatus"` // ACTIVE, ARCHIVED
	IsSupplier    bool   `json:"IsSupplier"`
	IsCustomer    bool   `json:"IsCustomer"`
	Balances       *ContactBalances `json:"Balances,omitempty"`
	UpdatedDateUTC string `json:"UpdatedDateUTC,omitempty"`
}

// ContactBalances holds receivable and payable balances for a contact.
type ContactBalances struct {
	AccountsReceivable ContactBalance `json:"AccountsReceivable,omitempty"`
	AccountsPayable    ContactBalance `json:"AccountsPayable,omitempty"`
}

// ContactBalance holds outstanding and overdue amounts.
type ContactBalance struct {
	Outstanding float64 `json:"Outstanding"`
	Overdue     float64 `json:"Overdue"`
}

// Account represents a Xero chart-of-accounts entry.
type Account struct {
	AccountID   string `json:"AccountID"`
	Code        string `json:"Code"`
	Name        string `json:"Name"`
	Type        string `json:"Type"`   // ASSET, EQUITY, EXPENSE, LIABILITY, REVENUE
	Class       string `json:"Class"`  // e.g., CURRENT, FIXED
	Status      string `json:"Status"` // ACTIVE, ARCHIVED
	Currency    string `json:"CurrencyCode,omitempty"`
	Description string `json:"Description,omitempty"`
	EnablePaymentsToAccount bool `json:"EnablePaymentsToAccount,omitempty"`
}

// Invoice represents a Xero invoice or bill.
type Invoice struct {
	InvoiceID     string        `json:"InvoiceID"`
	Type          string        `json:"Type"`   // ACCREC (invoice) or ACCPAY (bill)
	Status        string        `json:"Status"` // DRAFT, SUBMITTED, AUTHORISED, PAID, VOIDED, DELETED
	Contact       InvoiceContact `json:"Contact"`
	LineItems     []InvoiceLineItem `json:"LineItems"`
	CurrencyCode  string        `json:"CurrencyCode"`
	SubTotal      float64       `json:"SubTotal"`
	Total         float64       `json:"Total"`
	AmountDue     float64       `json:"AmountDue"`
	AmountPaid    float64       `json:"AmountPaid"`
	Date          string        `json:"Date,omitempty"`
	DueDate       string        `json:"DueDate,omitempty"`
	Reference     string        `json:"Reference,omitempty"`
	CurrencyRate  float64       `json:"CurrencyRate,omitempty"`
	UpdatedDateUTC string       `json:"UpdatedDateUTC,omitempty"`
}

// InvoiceContact is the contact reference within an invoice.
type InvoiceContact struct {
	ContactID string `json:"ContactID"`
	Name      string `json:"Name,omitempty"`
}

// InvoiceLineItem is a line item on a Xero invoice.
type InvoiceLineItem struct {
	Description string  `json:"Description"`
	Quantity    float64 `json:"Quantity"`
	UnitAmount  float64 `json:"UnitAmount"`
	LineAmount  float64 `json:"LineAmount"`
	AccountCode string  `json:"AccountCode"`
	ItemCode    string  `json:"ItemCode,omitempty"`
	TaxType     string  `json:"TaxType,omitempty"`
	TaxAmount   float64 `json:"TaxAmount,omitempty"`
}

// Allocation links a credit note to an invoice.
type Allocation struct {
	Invoice InvoiceRef `json:"Invoice"`
	Amount  float64    `json:"Amount"`
	Date    string     `json:"Date,omitempty"`
}

// InvoiceRef is a minimal reference to an invoice.
type InvoiceRef struct {
	InvoiceID string `json:"InvoiceID"`
}

// CreditNote represents a Xero credit note.
type CreditNote struct {
	CreditNoteID  string            `json:"CreditNoteID"`
	Type          string            `json:"Type"` // ACCRECCREDIT or ACCPAYCREDIT
	Status        string            `json:"Status"`
	Contact       InvoiceContact    `json:"Contact"`
	LineItems     []InvoiceLineItem `json:"LineItems"`
	CurrencyCode  string            `json:"CurrencyCode"`
	SubTotal      float64           `json:"SubTotal"`
	Total         float64           `json:"Total"`
	RemainingCredit float64         `json:"RemainingCredit"`
	Allocations   []Allocation      `json:"Allocations,omitempty"`
	Date          string            `json:"Date,omitempty"`
	Reference     string            `json:"Reference,omitempty"`
	UpdatedDateUTC string           `json:"UpdatedDateUTC,omitempty"`
}

// Payment represents a Xero payment.
type Payment struct {
	PaymentID   string         `json:"PaymentID"`
	Invoice     *PaymentRef    `json:"Invoice,omitempty"`
	CreditNote  *PaymentRef    `json:"CreditNote,omitempty"`
	Account     PaymentAccount `json:"Account"`
	Amount      float64        `json:"Amount"`
	CurrencyRate float64       `json:"CurrencyRate,omitempty"`
	Date        string         `json:"Date,omitempty"`
	Reference   string         `json:"Reference,omitempty"`
	Status      string         `json:"Status"` // AUTHORISED, DELETED
	PaymentType string         `json:"PaymentType,omitempty"`
	UpdatedDateUTC string      `json:"UpdatedDateUTC,omitempty"`
}

// PaymentRef references an invoice or credit note in a payment.
type PaymentRef struct {
	InvoiceID    string `json:"InvoiceID,omitempty"`
	CreditNoteID string `json:"CreditNoteID,omitempty"`
}

// PaymentAccount references a bank account for a payment.
type PaymentAccount struct {
	AccountID string `json:"AccountID"`
	Code      string `json:"Code,omitempty"`
}

// BankTransaction represents a Xero bank transaction.
type BankTransaction struct {
	BankTransactionID string            `json:"BankTransactionID"`
	Type              string            `json:"Type"` // RECEIVE, SPEND
	Contact           InvoiceContact    `json:"Contact"`
	LineItems         []InvoiceLineItem `json:"LineItems"`
	BankAccount       PaymentAccount    `json:"BankAccount"`
	CurrencyCode      string            `json:"CurrencyCode"`
	SubTotal          float64           `json:"SubTotal"`
	Total             float64           `json:"Total"`
	Status            string            `json:"Status"` // AUTHORISED, DELETED
	Date              string            `json:"Date,omitempty"`
	Reference         string            `json:"Reference,omitempty"`
	UpdatedDateUTC    string            `json:"UpdatedDateUTC,omitempty"`
}

// ManualJournal represents a Xero manual journal.
type ManualJournal struct {
	ManualJournalID string              `json:"ManualJournalID"`
	Narration       string              `json:"Narration"`
	JournalLines    []ManualJournalLine `json:"JournalLines"`
	Date            string              `json:"Date,omitempty"`
	Status          string              `json:"Status"` // DRAFT, POSTED, DELETED, VOIDED
	UpdatedDateUTC  string              `json:"UpdatedDateUTC,omitempty"`
}

// ManualJournalLine is a line in a manual journal.
type ManualJournalLine struct {
	AccountCode string  `json:"AccountCode"`
	Description string  `json:"Description,omitempty"`
	LineAmount  float64 `json:"LineAmount"` // positive=debit, negative=credit in Xero
}

// Item represents a Xero inventory item.
type Item struct {
	ItemID      string  `json:"ItemID"`
	Code        string  `json:"Code"`
	Name        string  `json:"Name"`
	Description string  `json:"Description,omitempty"`
	PurchaseDescription string `json:"PurchaseDescription,omitempty"`
	PurchaseDetails ItemDetails `json:"PurchaseDetails,omitempty"`
	SalesDetails    ItemDetails `json:"SalesDetails,omitempty"`
	UpdatedDateUTC  string      `json:"UpdatedDateUTC,omitempty"`
}

// ItemDetails holds the purchase or sale details for an item.
type ItemDetails struct {
	UnitPrice   float64 `json:"UnitPrice,omitempty"`
	AccountCode string  `json:"AccountCode,omitempty"`
}

// TaxRate represents a Xero tax rate.
type TaxRate struct {
	Name                  string  `json:"Name"`
	TaxType               string  `json:"TaxType"`
	EffectiveRate         float64 `json:"EffectiveRate"`
	Status                string  `json:"Status"` // ACTIVE, DELETED
	CanApplyToAssets      bool    `json:"CanApplyToAssets"`
	CanApplyToEquity      bool    `json:"CanApplyToEquity"`
	CanApplyToExpenses    bool    `json:"CanApplyToExpenses"`
	CanApplyToLiabilities bool    `json:"CanApplyToLiabilities"`
	CanApplyToRevenue     bool    `json:"CanApplyToRevenue"`
}

// TrackingCategory represents a Xero tracking category.
type TrackingCategory struct {
	TrackingCategoryID string           `json:"TrackingCategoryID"`
	Name               string           `json:"Name"`
	Status             string           `json:"Status"` // ACTIVE, ARCHIVED, DELETED
	Options            []TrackingOption `json:"Options,omitempty"`
}

// TrackingOption is an option within a tracking category.
type TrackingOption struct {
	TrackingOptionID string `json:"TrackingOptionID"`
	Name             string `json:"Name"`
	Status           string `json:"Status"` // ACTIVE, ARCHIVED
}

// Prepayment represents a Xero prepayment.
type Prepayment struct {
	PrepaymentID    string            `json:"PrepaymentID"`
	Type            string            `json:"Type"` // RECEIVE-PREPAYMENT, SPEND-PREPAYMENT
	Contact         InvoiceContact    `json:"Contact"`
	LineItems       []InvoiceLineItem `json:"LineItems"`
	BankAccount     PaymentAccount    `json:"BankAccount"`
	CurrencyCode    string            `json:"CurrencyCode"`
	SubTotal        float64           `json:"SubTotal"`
	Total           float64           `json:"Total"`
	RemainingCredit float64           `json:"RemainingCredit"`
	Allocations     []Allocation      `json:"Allocations,omitempty"`
	Status          string            `json:"Status"` // AUTHORISED, PAID, VOIDED
	Date            string            `json:"Date,omitempty"`
	UpdatedDateUTC  string            `json:"UpdatedDateUTC,omitempty"`
}

// Overpayment represents a Xero overpayment.
type Overpayment struct {
	OverpaymentID   string            `json:"OverpaymentID"`
	Type            string            `json:"Type"` // RECEIVE-OVERPAYMENT, SPEND-OVERPAYMENT
	Contact         InvoiceContact    `json:"Contact"`
	LineItems       []InvoiceLineItem `json:"LineItems"`
	BankAccount     PaymentAccount    `json:"BankAccount"`
	CurrencyCode    string            `json:"CurrencyCode"`
	SubTotal        float64           `json:"SubTotal"`
	Total           float64           `json:"Total"`
	RemainingCredit float64           `json:"RemainingCredit"`
	Allocations     []Allocation      `json:"Allocations,omitempty"`
	Status          string            `json:"Status"` // AUTHORISED, PAID, VOIDED
	Date            string            `json:"Date,omitempty"`
	UpdatedDateUTC  string            `json:"UpdatedDateUTC,omitempty"`
}

// XeroDate formats a time.Time as a Xero date string.
func XeroDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02T15:04:05")
}

// XeroDateNow returns the current time formatted as a Xero date.
func XeroDateNow() string {
	return XeroDate(time.Now())
}
