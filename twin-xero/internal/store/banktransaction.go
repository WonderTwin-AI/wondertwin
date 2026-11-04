package store

import "time"

// BankTransaction is money received into or spent from a bank account, as Xero
// returns it. Its dates are written as /Date(ms+0000)/ by the API layer, which
// shadows them; Contact and BankAccount hold only references here, so a snapshot
// keeps the links. IsReconciled is a stored flag: the app emulator models no bank
// statement lines to reconcile against.
type BankTransaction struct {
	BankTransactionID     string            `json:"BankTransactionID"`
	Type                  string            `json:"Type"`
	Contact               *ContactRef       `json:"Contact,omitempty"`
	LineItems             []LineItem        `json:"LineItems"`
	BankAccount           *AccountRef       `json:"BankAccount,omitempty"`
	IsReconciled          bool              `json:"IsReconciled"`
	Date                  *time.Time        `json:"Date,omitempty"`
	Reference             string            `json:"Reference,omitempty"`
	CurrencyCode          string            `json:"CurrencyCode,omitempty"`
	CurrencyRate          *float64          `json:"CurrencyRate,omitempty"`
	URL                   string            `json:"Url,omitempty"`
	Status                string            `json:"Status"`
	LineAmountTypes       string            `json:"LineAmountTypes"`
	SubTotal              float64           `json:"SubTotal"`
	TotalTax              float64           `json:"TotalTax"`
	Total                 float64           `json:"Total"`
	PrepaymentID          string            `json:"PrepaymentID,omitempty"`
	OverpaymentID         string            `json:"OverpaymentID,omitempty"`
	UpdatedDateUTC        time.Time         `json:"UpdatedDateUTC"`
	HasAttachments        bool              `json:"HasAttachments"`
	StatusAttributeString string            `json:"StatusAttributeString,omitempty"`
	ValidationErrors      []ValidationError `json:"ValidationErrors,omitempty"`
	Warnings              []ValidationError `json:"Warnings,omitempty"`
}

// AccountRef is the account a record points at: by ID or by code on the way in,
// with its name added on the way out.
type AccountRef struct {
	AccountID string `json:"AccountID,omitempty"`
	Code      string `json:"Code,omitempty"`
	Name      string `json:"Name,omitempty"`
}
