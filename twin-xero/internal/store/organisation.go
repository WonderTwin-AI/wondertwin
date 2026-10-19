package store

import "time"

// Organisation is the tenant's company record, as GET /Organisation returns it.
// The dates are written as /Date(ms+0000)/ by the API layer (time-001), which
// shadows these three fields.
type Organisation struct {
	OrganisationID               string         `json:"OrganisationID"`
	APIKey                       string         `json:"APIKey,omitempty"`
	Name                         string         `json:"Name"`
	LegalName                    string         `json:"LegalName"`
	PaysTax                      bool           `json:"PaysTax"`
	Version                      string         `json:"Version"`
	OrganisationType             string         `json:"OrganisationType"`
	BaseCurrency                 string         `json:"BaseCurrency"`
	CountryCode                  string         `json:"CountryCode"`
	IsDemoCompany                bool           `json:"IsDemoCompany"`
	OrganisationStatus           string         `json:"OrganisationStatus"`
	RegistrationNumber           string         `json:"RegistrationNumber,omitempty"`
	EmployerIdentificationNumber string         `json:"EmployerIdentificationNumber,omitempty"`
	TaxNumber                    string         `json:"TaxNumber,omitempty"`
	FinancialYearEndDay          int            `json:"FinancialYearEndDay"`
	FinancialYearEndMonth        int            `json:"FinancialYearEndMonth"`
	SalesTaxBasis                string         `json:"SalesTaxBasis,omitempty"`
	SalesTaxPeriod               string         `json:"SalesTaxPeriod,omitempty"`
	DefaultSalesTax              string         `json:"DefaultSalesTax,omitempty"`
	DefaultPurchasesTax          string         `json:"DefaultPurchasesTax,omitempty"`
	PeriodLockDate               *time.Time     `json:"PeriodLockDate,omitempty"`
	EndOfYearLockDate            *time.Time     `json:"EndOfYearLockDate,omitempty"`
	CreatedDateUTC               time.Time      `json:"CreatedDateUTC"`
	Timezone                     string         `json:"Timezone"`
	OrganisationEntityType       string         `json:"OrganisationEntityType"`
	ShortCode                    string         `json:"ShortCode"`
	Class                        string         `json:"Class"`
	Edition                      string         `json:"Edition"`
	LineOfBusiness               string         `json:"LineOfBusiness,omitempty"`
	Addresses                    []Address      `json:"Addresses"`
	Phones                       []Phone        `json:"Phones"`
	ExternalLinks                []ExternalLink `json:"ExternalLinks"`
	PaymentTerms                 *PaymentTerms  `json:"PaymentTerms,omitempty"`
}

// Address is an organisation address.
type Address struct {
	AddressType  string `json:"AddressType"`
	AddressLine1 string `json:"AddressLine1,omitempty"`
	AddressLine2 string `json:"AddressLine2,omitempty"`
	AddressLine3 string `json:"AddressLine3,omitempty"`
	AddressLine4 string `json:"AddressLine4,omitempty"`
	City         string `json:"City,omitempty"`
	Region       string `json:"Region,omitempty"`
	PostalCode   string `json:"PostalCode,omitempty"`
	Country      string `json:"Country,omitempty"`
	AttentionTo  string `json:"AttentionTo,omitempty"`
}

// Phone is an organisation phone number.
type Phone struct {
	PhoneType        string `json:"PhoneType"`
	PhoneNumber      string `json:"PhoneNumber,omitempty"`
	PhoneAreaCode    string `json:"PhoneAreaCode,omitempty"`
	PhoneCountryCode string `json:"PhoneCountryCode,omitempty"`
}

// ExternalLink is a link on the organisation record.
type ExternalLink struct {
	LinkType string `json:"LinkType"`
	URL      string `json:"Url,omitempty"`
}

// PaymentTerms are the organisation's default bill and sales terms.
type PaymentTerms struct {
	Bills *PaymentTerm `json:"Bills,omitempty"`
	Sales *PaymentTerm `json:"Sales,omitempty"`
}

// PaymentTerm is a day count and what it counts from.
type PaymentTerm struct {
	Day  int    `json:"Day"`
	Type string `json:"Type"`
}

const defaultOrganisationID = "00000001-0000-4000-8000-000000000001"

// seedTenant restores the default tenant's organisation. The edition is GLOBAL
// until the operator picks one (OD-4).
func (s *MemoryStore) seedTenant() {
	now := s.Clock.Now().UTC()
	s.Organisations.Set(DefaultTenantID, Organisation{
		OrganisationID: defaultOrganisationID, Name: DefaultTenantName, LegalName: DefaultTenantName,
		PaysTax: true, Version: "GLOBAL", OrganisationType: "COMPANY", BaseCurrency: "USD", CountryCode: "US",
		IsDemoCompany: true, OrganisationStatus: "ACTIVE", FinancialYearEndDay: 31, FinancialYearEndMonth: 12,
		SalesTaxBasis: "ACCRUAL", SalesTaxPeriod: "QUARTERLY", DefaultSalesTax: "Tax Exclusive", DefaultPurchasesTax: "Tax Exclusive",
		CreatedDateUTC: now, Timezone: "UTC", OrganisationEntityType: "COMPANY", ShortCode: "!demo01", Class: "DEMO", Edition: "BUSINESS",
		Addresses: []Address{{AddressType: "POBOX"}}, Phones: []Phone{{PhoneType: "OFFICE"}}, ExternalLinks: []ExternalLink{},
	})
}
