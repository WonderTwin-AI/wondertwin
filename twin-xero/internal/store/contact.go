package store

import (
	"encoding/json"
	"time"
)

// Contact is a customer or supplier, as Xero returns it. UpdatedDateUTC is
// written as /Date(ms+0000)/ by the API layer, which shadows it. A list that
// Xero always returns is written as [] by the API layer, never null.
type Contact struct {
	ContactID                      string               `json:"ContactID"`
	MergedToContactID              string               `json:"MergedToContactID,omitempty"`
	ContactNumber                  string               `json:"ContactNumber,omitempty"`
	AccountNumber                  string               `json:"AccountNumber,omitempty"`
	ContactStatus                  string               `json:"ContactStatus"`
	Name                           string               `json:"Name"`
	FirstName                      string               `json:"FirstName,omitempty"`
	LastName                       string               `json:"LastName,omitempty"`
	CompanyNumber                  string               `json:"CompanyNumber,omitempty"`
	EmailAddress                   string               `json:"EmailAddress,omitempty"`
	ContactPersons                 []ContactPerson      `json:"ContactPersons"`
	BankAccountDetails             string               `json:"BankAccountDetails,omitempty"`
	TaxNumber                      string               `json:"TaxNumber,omitempty"`
	TaxNumberType                  string               `json:"TaxNumberType,omitempty"`
	AccountsReceivableTaxType      string               `json:"AccountsReceivableTaxType,omitempty"`
	AccountsPayableTaxType         string               `json:"AccountsPayableTaxType,omitempty"`
	Addresses                      []Address            `json:"Addresses"`
	Phones                         []Phone              `json:"Phones"`
	IsSupplier                     bool                 `json:"IsSupplier"`
	IsCustomer                     bool                 `json:"IsCustomer"`
	SalesDefaultLineAmountType     string               `json:"SalesDefaultLineAmountType,omitempty"`
	PurchasesDefaultLineAmountType string               `json:"PurchasesDefaultLineAmountType,omitempty"`
	DefaultCurrency                string               `json:"DefaultCurrency,omitempty"`
	XeroNetworkKey                 string               `json:"XeroNetworkKey,omitempty"`
	SalesDefaultAccountCode        string               `json:"SalesDefaultAccountCode,omitempty"`
	PurchasesDefaultAccountCode    string               `json:"PurchasesDefaultAccountCode,omitempty"`
	SalesTrackingCategories        []TrackingCategoryOf `json:"SalesTrackingCategories"`
	PurchasesTrackingCategories    []TrackingCategoryOf `json:"PurchasesTrackingCategories"`
	TrackingCategoryName           string               `json:"TrackingCategoryName,omitempty"`
	TrackingCategoryOption         string               `json:"TrackingCategoryOption,omitempty"`
	PaymentTerms                   *PaymentTerms        `json:"PaymentTerms,omitempty"`
	UpdatedDateUTC                 time.Time            `json:"UpdatedDateUTC"`
	ContactGroups                  []ContactGroup       `json:"ContactGroups"`
	Website                        string               `json:"Website,omitempty"`
	BrandingTheme                  *BrandingTheme       `json:"BrandingTheme,omitempty"`
	BatchPayments                  *BatchPaymentDetails `json:"BatchPayments,omitempty"`
	Discount                       *float64             `json:"Discount,omitempty"`
	Balances                       *Balances            `json:"Balances,omitempty"`
	Attachments                    []Attachment         `json:"Attachments,omitempty"`
	HasAttachments                 bool                 `json:"HasAttachments"`
	ValidationErrors               []ValidationError    `json:"ValidationErrors,omitempty"`
	HasValidationErrors            bool                 `json:"HasValidationErrors"`
	StatusAttributeString          string               `json:"StatusAttributeString,omitempty"`
}

// ContactPerson is a person on a contact.
type ContactPerson struct {
	FirstName       string `json:"FirstName,omitempty"`
	LastName        string `json:"LastName,omitempty"`
	EmailAddress    string `json:"EmailAddress,omitempty"`
	IncludeInEmails bool   `json:"IncludeInEmails"`
}

// TrackingCategoryOf is a default tracking category and option on a contact
// (the SDK calls it SalesTrackingCategory).
type TrackingCategoryOf struct {
	TrackingCategoryName string `json:"TrackingCategoryName,omitempty"`
	TrackingOptionName   string `json:"TrackingOptionName,omitempty"`
}

// ContactGroup is a group a contact belongs to.
type ContactGroup struct {
	ContactGroupID string `json:"ContactGroupID,omitempty"`
	Name           string `json:"Name,omitempty"`
	Status         string `json:"Status,omitempty"`
}

// BrandingTheme is the invoice theme a contact uses. CreatedDateUTC is kept as
// sent, since nothing here sets it.
type BrandingTheme struct {
	BrandingThemeID string          `json:"BrandingThemeID,omitempty"`
	Name            string          `json:"Name,omitempty"`
	LogoURL         string          `json:"LogoUrl,omitempty"`
	Type            string          `json:"Type,omitempty"`
	SortOrder       int             `json:"SortOrder"`
	CreatedDateUTC  json.RawMessage `json:"CreatedDateUTC,omitempty"`
}

// BatchPaymentDetails are a contact's bank details for batch payments.
type BatchPaymentDetails struct {
	BankAccountNumber string `json:"BankAccountNumber,omitempty"`
	BankAccountName   string `json:"BankAccountName,omitempty"`
	Details           string `json:"Details,omitempty"`
	Code              string `json:"Code,omitempty"`
	Reference         string `json:"Reference,omitempty"`
}

// Balances are what a contact owes and is owed.
type Balances struct {
	AccountsReceivable *Balance `json:"AccountsReceivable,omitempty"`
	AccountsPayable    *Balance `json:"AccountsPayable,omitempty"`
}

// Balance is an outstanding and an overdue amount.
type Balance struct {
	Outstanding float64 `json:"Outstanding"`
	Overdue     float64 `json:"Overdue"`
}

// Attachment is a file attached to a record.
type Attachment struct {
	AttachmentID  string `json:"AttachmentID,omitempty"`
	FileName      string `json:"FileName,omitempty"`
	URL           string `json:"Url,omitempty"`
	MimeType      string `json:"MimeType,omitempty"`
	ContentLength int    `json:"ContentLength"`
	IncludeOnline bool   `json:"IncludeOnline"`
}

// ValidationError is one reason Xero refused a record.
type ValidationError struct {
	Message string `json:"Message"`
}
