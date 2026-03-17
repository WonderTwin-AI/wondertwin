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
	UpdatedDateUTC string `json:"UpdatedDateUTC,omitempty"`
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
