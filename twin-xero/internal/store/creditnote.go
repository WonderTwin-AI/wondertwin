package store

import "time"

// CreditNote is a credit against a contact, as Xero returns it. Its dates are
// written as /Date(ms+0000)/ by the API layer, which shadows them; Contact holds
// only the reference here, so a snapshot keeps the link.
type CreditNote struct {
	Type                  string            `json:"Type"`
	Contact               *ContactRef       `json:"Contact,omitempty"`
	Date                  *time.Time        `json:"Date,omitempty"`
	DueDate               *time.Time        `json:"DueDate,omitempty"`
	Status                string            `json:"Status"`
	LineAmountTypes       string            `json:"LineAmountTypes"`
	LineItems             []LineItem        `json:"LineItems"`
	SubTotal              float64           `json:"SubTotal"`
	TotalTax              float64           `json:"TotalTax"`
	Total                 float64           `json:"Total"`
	CISDeduction          *float64          `json:"CISDeduction,omitempty"`
	CISRate               *float64          `json:"CISRate,omitempty"`
	UpdatedDateUTC        time.Time         `json:"UpdatedDateUTC"`
	UpdatedDateUTCString  string            `json:"UpdatedDateUTCString"`
	CurrencyCode          string            `json:"CurrencyCode,omitempty"`
	FullyPaidOnDate       *time.Time        `json:"FullyPaidOnDate,omitempty"`
	CreditNoteID          string            `json:"CreditNoteID"`
	CreditNoteNumber      string            `json:"CreditNoteNumber,omitempty"`
	Reference             string            `json:"Reference,omitempty"`
	SentToContact         bool              `json:"SentToContact"`
	CurrencyRate          *float64          `json:"CurrencyRate,omitempty"`
	RemainingCredit       float64           `json:"RemainingCredit"`
	Allocations           []Allocation      `json:"Allocations"`
	AppliedAmount         float64           `json:"AppliedAmount"`
	Payments              []PaymentSummary  `json:"Payments"`
	BrandingThemeID       string            `json:"BrandingThemeID,omitempty"`
	StatusAttributeString string            `json:"StatusAttributeString,omitempty"`
	HasAttachments        bool              `json:"HasAttachments"`
	HasErrors             bool              `json:"HasErrors"`
	ValidationErrors      []ValidationError `json:"ValidationErrors,omitempty"`
	Warnings              []ValidationError `json:"Warnings,omitempty"`
	InvoiceAddresses      []InvoiceAddress  `json:"InvoiceAddresses,omitempty"`
}

// Allocation applies part of a credit note to an invoice. Its date is written as
// /Date(ms+0000)/ by the API layer, which shadows it.
type Allocation struct {
	AllocationID          string            `json:"AllocationID"`
	Invoice               *InvoiceRef       `json:"Invoice,omitempty"`
	Amount                float64           `json:"Amount"`
	Date                  *time.Time        `json:"Date,omitempty"`
	IsDeleted             bool              `json:"IsDeleted"`
	StatusAttributeString string            `json:"StatusAttributeString,omitempty"`
	ValidationErrors      []ValidationError `json:"ValidationErrors,omitempty"`
}

// InvoiceRef is the part of an invoice an allocation stores: which one.
type InvoiceRef struct {
	InvoiceID     string `json:"InvoiceID"`
	InvoiceNumber string `json:"InvoiceNumber,omitempty"`
}
