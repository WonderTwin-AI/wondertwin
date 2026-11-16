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
