package api

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
