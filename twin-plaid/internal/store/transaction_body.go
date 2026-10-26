package store

import (
	"encoding/json"
	"fmt"
	"time"
)

// Opt is an optional field that is never null: present with a value, or left
// out. plaid-python rejects null for such a field, so the zero value (unset) is
// omitted from the JSON, and a JSON null reads back as unset.
type Opt[T any] struct {
	Set bool
	V   T
}

// Some is an Opt holding v.
func Some[T any](v T) Opt[T] { return Opt[T]{Set: true, V: v} }

// IsZero lets `omitzero` leave an unset Opt out.
func (o Opt[T]) IsZero() bool { return !o.Set }

func (o Opt[T]) MarshalJSON() ([]byte, error) { return json.Marshal(o.V) }

func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*o = Opt[T]{}
		return nil
	}
	o.Set = true
	return json.Unmarshal(b, &o.V)
}

// Nullable is an optional field that may be null: left out, null, or a value.
// The zero value (unset) is omitted; Null() and Value() are present.
type Nullable[T any] struct {
	Set bool
	V   *T
}

// Null is a present field holding null.
func Null[T any]() Nullable[T] { return Nullable[T]{Set: true} }

// Value is a present field holding v.
func Value[T any](v T) Nullable[T] { return Nullable[T]{Set: true, V: &v} }

// IsZero lets `omitzero` leave an unset Nullable out.
func (n Nullable[T]) IsZero() bool { return !n.Set }

func (n Nullable[T]) MarshalJSON() ([]byte, error) { return json.Marshal(n.V) }

func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.V = nil
		return nil
	}
	n.V = new(T)
	return json.Unmarshal(b, n.V)
}

// Location is a transaction's location. Every field is required and nullable.
type Location struct {
	Address     *string  `json:"address"`
	City        *string  `json:"city"`
	Region      *string  `json:"region"`
	PostalCode  *string  `json:"postal_code"`
	Country     *string  `json:"country"`
	Lat         *float64 `json:"lat"`
	Lon         *float64 `json:"lon"`
	StoreNumber *string  `json:"store_number"`
}

// PaymentMeta is a transaction's payment metadata. Every field is required and
// nullable.
type PaymentMeta struct {
	ReferenceNumber  *string `json:"reference_number"`
	PpdID            *string `json:"ppd_id"`
	Payee            *string `json:"payee"`
	ByOrderOf        *string `json:"by_order_of"`
	Payer            *string `json:"payer"`
	PaymentMethod    *string `json:"payment_method"`
	PaymentProcessor *string `json:"payment_processor"`
	Reason           *string `json:"reason"`
}

// PersonalFinanceCategory is a transaction's category. Primary and detailed are
// required; confidence_level and version are optional.
type PersonalFinanceCategory struct {
	Primary         string           `json:"primary"`
	Detailed        string           `json:"detailed"`
	ConfidenceLevel Nullable[string] `json:"confidence_level,omitzero"`
	Version         Opt[string]      `json:"version,omitzero"`
}

// Counterparty is one counterparty of a transaction. Name, type, website,
// logo_url and phone_number are required, and the last three are nullable.
type Counterparty struct {
	Name            string           `json:"name"`
	Type            string           `json:"type"`
	Website         *string          `json:"website"`
	LogoURL         *string          `json:"logo_url"`
	PhoneNumber     *string          `json:"phone_number"`
	EntityID        Nullable[string] `json:"entity_id,omitzero"`
	ConfidenceLevel Nullable[string] `json:"confidence_level,omitzero"`
}

// TransactionBody is the Transaction object /transactions/get and
// /transactions/sync return, with the presence rule plaid-python applies to each
// field: a plain value or a pointer without omitzero is always sent (a pointer
// sends null when nil); Opt and Nullable fields are optional and sent only when
// set. The model's business_finance_category and client_customization are not
// carried.
type TransactionBody struct {
	AccountID              string      `json:"account_id"`
	Amount                 float64     `json:"amount"`
	IsoCurrencyCode        *string     `json:"iso_currency_code"`
	UnofficialCurrencyCode *string     `json:"unofficial_currency_code"`
	Date                   string      `json:"date"`
	Location               Location    `json:"location"`
	Name                   string      `json:"name"`
	PaymentMeta            PaymentMeta `json:"payment_meta"`
	Pending                bool        `json:"pending"`
	PendingTransactionID   *string     `json:"pending_transaction_id"`
	AccountOwner           *string     `json:"account_owner"`
	TransactionID          string      `json:"transaction_id"`
	AuthorizedDate         *string     `json:"authorized_date"`
	AuthorizedDatetime     *string     `json:"authorized_datetime"`
	Datetime               *string     `json:"datetime"`
	PaymentChannel         string      `json:"payment_channel"`
	TransactionCode        *string     `json:"transaction_code"`

	Category                       Nullable[[]string]           `json:"category,omitzero"`
	CategoryID                     Nullable[string]             `json:"category_id,omitzero"`
	CheckNumber                    Nullable[string]             `json:"check_number,omitzero"`
	MerchantName                   Nullable[string]             `json:"merchant_name,omitzero"`
	OriginalDescription            Nullable[string]             `json:"original_description,omitzero"`
	TransactionType                Opt[string]                  `json:"transaction_type,omitzero"`
	LogoURL                        Nullable[string]             `json:"logo_url,omitzero"`
	Website                        Nullable[string]             `json:"website,omitzero"`
	PersonalFinanceCategory        Opt[PersonalFinanceCategory] `json:"personal_finance_category,omitzero"`
	PersonalFinanceCategoryIconURL Opt[string]                  `json:"personal_finance_category_icon_url,omitzero"`
	Counterparties                 Opt[[]Counterparty]          `json:"counterparties,omitzero"`
	MerchantEntityID               Nullable[string]             `json:"merchant_entity_id,omitzero"`
	MerchantCategoryCode           Nullable[string]             `json:"merchant_category_code,omitzero"`
	RunningBalance                 Nullable[float64]            `json:"running_balance,omitzero"`
}

var (
	paymentChannels  = map[string]bool{"online": true, "in store": true, "other": true}
	transactionTypes = map[string]bool{"digital": true, "place": true, "special": true, "unresolved": true}
)

// Validate reports the first way a body would break plaid-python: a required
// value that is empty, a date that is not YYYY-MM-DD, or a value outside an enum.
// The typed struct already fixes which fields are sent and which may be null.
func (b TransactionBody) Validate() error {
	for _, f := range []struct{ name, v string }{
		{"account_id", b.AccountID}, {"transaction_id", b.TransactionID}, {"date", b.Date}, {"payment_channel", b.PaymentChannel},
	} {
		if f.v == "" {
			return fmt.Errorf("transaction %q: %s is required", b.TransactionID, f.name)
		}
	}
	if _, err := time.Parse("2006-01-02", b.Date); err != nil {
		return fmt.Errorf("transaction %q: date %q is not YYYY-MM-DD", b.TransactionID, b.Date)
	}
	if !paymentChannels[b.PaymentChannel] {
		return fmt.Errorf("transaction %q: payment_channel %q is not online, in store or other", b.TransactionID, b.PaymentChannel)
	}
	if b.TransactionType.Set && !transactionTypes[b.TransactionType.V] {
		return fmt.Errorf("transaction %q: transaction_type %q is not digital, place, special or unresolved", b.TransactionID, b.TransactionType.V)
	}
	return nil
}
