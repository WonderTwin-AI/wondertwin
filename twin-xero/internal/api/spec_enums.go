package api

import (
	"fmt"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// The enums below are copied from the Xero Accounting OpenAPI spec 19.0.0. A
// value outside them is one xero-python 15.2.0 cannot deserialize: reading the
// record back raises ValueError, so the app emulator refuses it on write.

// enumSet is a set of the values a spec enum allows.
type enumSet map[string]bool

func newEnumSet(values ...string) enumSet {
	s := enumSet{}
	for _, v := range values {
		s[v] = true
	}
	return s
}

// specCurrencyCodes is the CurrencyCode enum.
var specCurrencyCodes = newEnumSet(
	"AED", "AFN", "ALL", "AMD", "ANG", "AOA", "ARS", "AUD", "AWG", "AZN", "BAM", "BBD",
	"BDT", "BGN", "BHD", "BIF", "BMD", "BND", "BOB", "BRL", "BSD", "BTN", "BWP", "BYN",
	"BYR", "BZD", "CAD", "CDF", "CHF", "CLF", "CLP", "CNY", "COP", "CRC", "CUC", "CUP",
	"CVE", "CZK", "DJF", "DKK", "DOP", "DZD", "EEK", "EGP", "ERN", "ETB", "EUR", "FJD",
	"FKP", "GBP", "GEL", "GHS", "GIP", "GMD", "GNF", "GTQ", "GYD", "HKD", "HNL", "HRK",
	"HTG", "HUF", "IDR", "ILS", "INR", "IQD", "IRR", "ISK", "JMD", "JOD", "JPY", "KES",
	"KGS", "KHR", "KMF", "KPW", "KRW", "KWD", "KYD", "KZT", "LAK", "LBP", "LKR", "LRD",
	"LSL", "LTL", "LVL", "LYD", "MAD", "MDL", "MGA", "MKD", "MMK", "MNT", "MOP", "MRO",
	"MRU", "MUR", "MVR", "MWK", "MXN", "MXV", "MYR", "MZN", "NAD", "NGN", "NIO", "NOK",
	"NPR", "NZD", "OMR", "PAB", "PEN", "PGK", "PHP", "PKR", "PLN", "PYG", "QAR", "RON",
	"RSD", "RUB", "RWF", "SAR", "SBD", "SCR", "SDG", "SEK", "SGD", "SHP", "SKK", "SLE",
	"SLL", "SOS", "SRD", "STD", "STN", "SVC", "SYP", "SZL", "THB", "TJS", "TMT", "TND",
	"TOP", "TRY", "TTD", "TWD", "TZS", "UAH", "UGX", "USD", "UYU", "UZS", "VEF", "VES",
	"VND", "VUV", "WST", "XAF", "XCD", "XOF", "XPF", "YER", "ZAR", "ZMW", "ZMK", "ZWD",
)

// currencyCodeMessages is the refusal of a CurrencyCode outside the spec enum,
// or nil when it is in it or not sent. Accounts, Invoices, CreditNotes and
// BankTransactions give the same words, which are a guess.
func currencyCodeMessages(code string) []string {
	if code == "" || specCurrencyCodes[code] {
		return nil
	}
	return []string{fmt.Sprintf("Currency Code '%s' is not valid.", code)}
}

// specBankAccountTypes is the Account BankAccountType enum, which allows "".
var specBankAccountTypes = newEnumSet("BANK", "CREDITCARD", "PAYPAL", "NONE", "")

// specTaxabilities is the LineItem Taxability enum.
var specTaxabilities = newEnumSet("TAXABLE", "NON_TAXABLE", "EXEMPT", "PART_TAXABLE", "NOT_APPLICABLE")

// lineTaxabilityMessage is the refusal for line n (from 1) when its Taxability
// is outside the spec enum, or "" when it is in it or not sent. Every write of
// line items (Invoices, CreditNotes, BankTransactions) passes through
// computeLines, which calls it. The wording is a guess.
func lineTaxabilityMessage(n int, l store.LineItem) string {
	if l.Taxability == "" || specTaxabilities[l.Taxability] {
		return ""
	}
	return fmt.Sprintf("Line %d: Taxability '%s' is not valid.", n, l.Taxability)
}

// specLineAmountTypes is the LineAmountTypes enum.
var specLineAmountTypes = newEnumSet("Exclusive", "Inclusive", "NoTax")

// defaultLineAmountTypes is the LineAmountTypes of an invoice or credit note
// sent without one, or with "": the spec says line amounts are exclusive of tax
// by default. xero-python has no "" value, so an update that sends "" takes the
// default too rather than storing "".
const defaultLineAmountTypes = "Exclusive"

// lineAmountTypesMessage is the refusal of a LineAmountTypes outside the spec
// enum, the same on every resource. The wording is a guess.
const lineAmountTypesMessage = "LineAmountTypes must be Exclusive, Inclusive or NoTax."

// The Contact enums. xero-python checks each of the inline ones only when the
// value is not empty, so "" reads back; PaymentTermType is an enum class, which
// has no "" value.
var (
	specContactStatuses               = newEnumSet("ACTIVE", "ARCHIVED", "GDPRREQUEST")
	specTaxNumberTypes                = newEnumSet("SSN", "EIN", "ITIN", "ATIN")
	specContactDefaultLineAmountTypes = newEnumSet("INCLUSIVE", "EXCLUSIVE", "NONE")
	specAddressTypes                  = newEnumSet("POBOX", "STREET")
	specPhoneTypes                    = newEnumSet("DEFAULT", "DDI", "MOBILE", "FAX", "OFFICE")
	specContactGroupStatuses          = newEnumSet("ACTIVE", "DELETED")
	specBrandingThemeTypes            = newEnumSet("INVOICE")
	specPaymentTermTypes              = newEnumSet("DAYSAFTERBILLDATE", "DAYSAFTERBILLMONTH", "OFCURRENTMONTH", "OFFOLLOWINGMONTH")
)

// contactEnumMessages is the refusal of each value on a contact outside its
// spec enum, so that every write of a contact (PUT and POST /Contacts) keeps
// the contact list readable through xero-python. The wording follows the
// Accounts refusals and is a guess.
func contactEnumMessages(c store.Contact) []string {
	var msgs []string
	check := func(field, v string, allowed enumSet, emptyOK bool) {
		if (v == "" && emptyOK) || allowed[v] {
			return
		}
		msgs = append(msgs, fmt.Sprintf("%s '%s' is not valid.", field, v))
	}
	check("Contact Status", c.ContactStatus, specContactStatuses, true)
	check("Tax Number Type", c.TaxNumberType, specTaxNumberTypes, true)
	check("Sales Default Line Amount Type", c.SalesDefaultLineAmountType, specContactDefaultLineAmountTypes, true)
	check("Purchases Default Line Amount Type", c.PurchasesDefaultLineAmountType, specContactDefaultLineAmountTypes, true)
	check("Default Currency", c.DefaultCurrency, specCurrencyCodes, true)
	for _, a := range c.Addresses {
		check("Address Type", a.AddressType, specAddressTypes, true)
	}
	for _, p := range c.Phones {
		check("Phone Type", p.PhoneType, specPhoneTypes, true)
	}
	for _, g := range c.ContactGroups {
		check("Contact Group Status", g.Status, specContactGroupStatuses, true)
	}
	if c.BrandingTheme != nil {
		check("Branding Theme Type", c.BrandingTheme.Type, specBrandingThemeTypes, true)
	}
	if c.PaymentTerms != nil {
		if b := c.PaymentTerms.Bills; b != nil {
			check("Payment Terms Bills Type", b.Type, specPaymentTermTypes, false)
		}
		if s := c.PaymentTerms.Sales; s != nil {
			check("Payment Terms Sales Type", s.Type, specPaymentTermTypes, false)
		}
	}
	return msgs
}
