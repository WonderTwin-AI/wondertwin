package store

import "time"

// Payment is a payment against an invoice. Its dates are written as
// /Date(ms+0000)/ by the API layer, which shadows them; Invoice and Account hold
// only the references here, and the API layer replaces them with the full objects.
type Payment struct {
	PaymentID             string            `json:"PaymentID"`
	Invoice               *PaymentInvoice   `json:"Invoice,omitempty"`
	Account               *PaymentAccount   `json:"Account,omitempty"`
	Date                  time.Time         `json:"Date"`
	CurrencyRate          *float64          `json:"CurrencyRate,omitempty"`
	Amount                float64           `json:"Amount"`
	BankAmount            float64           `json:"BankAmount"`
	Reference             string            `json:"Reference,omitempty"`
	IsReconciled          bool              `json:"IsReconciled"`
	Status                string            `json:"Status"`
	PaymentType           string            `json:"PaymentType"`
	UpdatedDateUTC        time.Time         `json:"UpdatedDateUTC"`
	UpdatedDateUTCString  string            `json:"UpdatedDateUTCString"`
	HasAccount            bool              `json:"HasAccount"`
	HasValidationErrors   bool              `json:"HasValidationErrors"`
	StatusAttributeString string            `json:"StatusAttributeString,omitempty"`
	ValidationErrors      []ValidationError `json:"ValidationErrors,omitempty"`
	Warnings              []ValidationError `json:"Warnings,omitempty"`
	BankAccountNumber     string            `json:"BankAccountNumber,omitempty"`
	Particulars           string            `json:"Particulars,omitempty"`
	Details               string            `json:"Details,omitempty"`
	// Code is the account code, which Xero also accepts beside Account.
	Code string `json:"Code,omitempty"`
}

// PaymentInvoice is the invoice a payment is for: by ID, or by number as Xero also allows.
type PaymentInvoice struct {
	InvoiceID     string `json:"InvoiceID,omitempty"`
	InvoiceNumber string `json:"InvoiceNumber,omitempty"`
}

// PaymentAccount is the account a payment is made from or to: by ID or by code.
type PaymentAccount struct {
	AccountID string `json:"AccountID,omitempty"`
	Code      string `json:"Code,omitempty"`
}
