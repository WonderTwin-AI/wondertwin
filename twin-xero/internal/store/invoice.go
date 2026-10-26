package store

import "time"

// Invoice is an invoice or bill. Its dates are written as /Date(ms+0000)/ by the
// API layer, which shadows them, and Contact is replaced there by the full
// contact; here it holds only the reference, so a snapshot keeps the link.
type Invoice struct {
	Type                  string               `json:"Type"`
	Contact               *ContactRef          `json:"Contact,omitempty"`
	LineItems             []LineItem           `json:"LineItems"`
	Date                  *time.Time           `json:"Date,omitempty"`
	DueDate               *time.Time           `json:"DueDate,omitempty"`
	LineAmountTypes       string               `json:"LineAmountTypes"`
	InvoiceNumber         string               `json:"InvoiceNumber,omitempty"`
	Reference             string               `json:"Reference,omitempty"`
	BrandingThemeID       string               `json:"BrandingThemeID,omitempty"`
	URL                   string               `json:"Url,omitempty"`
	CurrencyCode          string               `json:"CurrencyCode,omitempty"`
	CurrencyRate          *float64             `json:"CurrencyRate,omitempty"`
	Status                string               `json:"Status"`
	SentToContact         bool                 `json:"SentToContact"`
	ExpectedPaymentDate   *time.Time           `json:"ExpectedPaymentDate,omitempty"`
	PlannedPaymentDate    *time.Time           `json:"PlannedPaymentDate,omitempty"`
	CISDeduction          *float64             `json:"CISDeduction,omitempty"`
	CISRate               *float64             `json:"CISRate,omitempty"`
	SubTotal              float64              `json:"SubTotal"`
	TotalTax              float64              `json:"TotalTax"`
	Total                 float64              `json:"Total"`
	TotalDiscount         float64              `json:"TotalDiscount"`
	RoundingAmount        *float64             `json:"RoundingAmount,omitempty"`
	EnteredTotal          *float64             `json:"EnteredTotal,omitempty"`
	InvoiceID             string               `json:"InvoiceID"`
	RepeatingInvoiceID    string               `json:"RepeatingInvoiceID,omitempty"`
	HasAttachments        bool                 `json:"HasAttachments"`
	IsDiscounted          bool                 `json:"IsDiscounted"`
	Payments              []PaymentSummary     `json:"Payments"`
	Prepayments           []PrepaymentSummary  `json:"Prepayments"`
	Overpayments          []OverpaymentSummary `json:"Overpayments"`
	AmountDue             float64              `json:"AmountDue"`
	AmountPaid            float64              `json:"AmountPaid"`
	FullyPaidOnDate       *time.Time           `json:"FullyPaidOnDate,omitempty"`
	AmountCredited        float64              `json:"AmountCredited"`
	UpdatedDateUTC        time.Time            `json:"UpdatedDateUTC"`
	UpdatedDateUTCString  string               `json:"UpdatedDateUTCString"`
	CreditNotes           []CreditNoteSummary  `json:"CreditNotes"`
	Attachments           []Attachment         `json:"Attachments"`
	HasErrors             bool                 `json:"HasErrors"`
	StatusAttributeString string               `json:"StatusAttributeString,omitempty"`
	ValidationErrors      []ValidationError    `json:"ValidationErrors,omitempty"`
	Warnings              []ValidationError    `json:"Warnings,omitempty"`
	InvoiceAddresses      []InvoiceAddress     `json:"InvoiceAddresses,omitempty"`
}

// ContactRef is the part of a contact an invoice stores: which one.
type ContactRef struct {
	ContactID string `json:"ContactID"`
}

// InvoiceAddress is an address on an invoice.
type InvoiceAddress struct {
	InvoiceAddressType string `json:"InvoiceAddressType,omitempty"`
	AddressLine1       string `json:"AddressLine1,omitempty"`
	AddressLine2       string `json:"AddressLine2,omitempty"`
	AddressLine3       string `json:"AddressLine3,omitempty"`
	AddressLine4       string `json:"AddressLine4,omitempty"`
	City               string `json:"City,omitempty"`
	Region             string `json:"Region,omitempty"`
	PostalCode         string `json:"PostalCode,omitempty"`
	Country            string `json:"Country,omitempty"`
}
