// Package store defines the Stripe twin's state types and in-memory store.
package store

import "time"

// Account represents a Stripe Connect account.
type Account struct {
	ID                 string            `json:"id"`
	Object             string            `json:"object"`
	Type               string            `json:"type"`
	BusinessType       string            `json:"business_type,omitempty"`
	Email              string            `json:"email,omitempty"`
	Country            string            `json:"country"`
	DefaultCurrency    string            `json:"default_currency"`
	ChargesEnabled     bool              `json:"charges_enabled"`
	PayoutsEnabled     bool              `json:"payouts_enabled"`
	DetailsSubmitted   bool              `json:"details_submitted"`
	Capabilities       map[string]string `json:"capabilities,omitempty"`
	Requirements       *Requirements     `json:"requirements,omitempty"`
	Individual         map[string]any    `json:"individual,omitempty"`
	Company            map[string]any    `json:"company,omitempty"`
	ExternalAccounts   *ExternalAccounts `json:"external_accounts,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	TOSAcceptance      map[string]any    `json:"tos_acceptance,omitempty"`
	BusinessProfile    map[string]any    `json:"business_profile,omitempty"`
	Settings           map[string]any    `json:"settings,omitempty"`
	Created            int64             `json:"created"`
	Updated            int64             `json:"updated,omitempty"`
}

// Requirements tracks KYB/KYC verification requirements.
type Requirements struct {
	CurrentlyDue  []string `json:"currently_due"`
	EventuallyDue []string `json:"eventually_due"`
	PastDue       []string `json:"past_due"`
	Alternatives  []any    `json:"alternatives,omitempty"`
	DisabledReason string  `json:"disabled_reason,omitempty"`
}

// ExternalAccounts wraps the external accounts list.
type ExternalAccounts struct {
	Object  string            `json:"object"`
	Data    []ExternalAccount `json:"data"`
	HasMore bool              `json:"has_more"`
	URL     string            `json:"url"`
}

// ExternalAccount represents a bank account or card attached to a Connect account.
type ExternalAccount struct {
	ID                string `json:"id"`
	Object            string `json:"object"`
	AccountID         string `json:"account"`
	BankName          string `json:"bank_name,omitempty"`
	Country           string `json:"country"`
	Currency          string `json:"currency"`
	Last4             string `json:"last4"`
	RoutingNumber     string `json:"routing_number,omitempty"`
	Status            string `json:"status"`
	DefaultForCurrency bool  `json:"default_for_currency"`
	Fingerprint       string `json:"fingerprint,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// Transfer represents a Stripe transfer to a connected account.
type Transfer struct {
	ID                 string            `json:"id"`
	Object             string            `json:"object"`
	Amount             int64             `json:"amount"`
	AmountReversed     int64             `json:"amount_reversed"`
	Currency           string            `json:"currency"`
	Description        string            `json:"description,omitempty"`
	Destination        string            `json:"destination"`
	DestinationPayment string            `json:"destination_payment,omitempty"`
	Livemode           bool              `json:"livemode"`
	Reversed           bool              `json:"reversed"`
	SourceTransaction  string            `json:"source_transaction,omitempty"`
	TransferGroup      string            `json:"transfer_group,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Created            int64             `json:"created"`
}

// Balance represents a Stripe account balance.
type Balance struct {
	Object    string          `json:"object"`
	Available []BalanceAmount `json:"available"`
	Pending   []BalanceAmount `json:"pending"`
	Livemode  bool            `json:"livemode"`
}

// BalanceAmount is a currency-specific balance entry.
type BalanceAmount struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// Payout represents a payout from a connected account to a bank.
type Payout struct {
	ID                 string            `json:"id"`
	Object             string            `json:"object"`
	Amount             int64             `json:"amount"`
	Currency           string            `json:"currency"`
	ArrivalDate        int64             `json:"arrival_date"`
	Description        string            `json:"description,omitempty"`
	Destination        string            `json:"destination,omitempty"`
	Method             string            `json:"method"`
	Status             string            `json:"status"`
	Type               string            `json:"type"`
	FailureCode        string            `json:"failure_code,omitempty"`
	FailureMessage     string            `json:"failure_message,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Created            int64             `json:"created"`
}

// BalanceTransaction represents a Stripe balance transaction ledger entry.
type BalanceTransaction struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	Description string `json:"description,omitempty"`
	Net         int64  `json:"net"`
	Fee         int64  `json:"fee"`
	Status      string `json:"status"` // "available" or "pending"
	Type        string `json:"type"`   // "transfer", "payout", "adjustment"
	Source      string `json:"source,omitempty"` // transfer/payout ID
	Created     int64  `json:"created"`
}

// Event represents a Stripe webhook event.
type Event struct {
	ID             string    `json:"id"`
	Object         string    `json:"object"`
	Type           string    `json:"type"`
	Data           EventData `json:"data"`
	APIVersion     string    `json:"api_version"`
	Created        int64     `json:"created"`
	Livemode       bool      `json:"livemode"`
	PendingWebhooks int      `json:"pending_webhooks"`
	Request        *EventReq `json:"request,omitempty"`
}

// EventData wraps the event's object.
type EventData struct {
	Object map[string]any `json:"object"`
}

// EventReq tracks the API request that triggered the event.
type EventReq struct {
	ID             string `json:"id,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

// Customer represents a Stripe customer.
type Customer struct {
	ID             string            `json:"id"`
	Object         string            `json:"object"`
	Name           string            `json:"name,omitempty"`
	Email          string            `json:"email,omitempty"`
	Phone          string            `json:"phone,omitempty"`
	Description    string            `json:"description,omitempty"`
	Currency       string            `json:"currency,omitempty"`
	DefaultSource  string            `json:"default_source,omitempty"`
	InvoicePrefix  string            `json:"invoice_prefix,omitempty"`
	Balance        int64             `json:"balance"`
	Delinquent     bool              `json:"delinquent"`
	Livemode       bool              `json:"livemode"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Created        int64             `json:"created"`
}

// Product represents a Stripe product.
type Product struct {
	ID          string            `json:"id"`
	Object      string            `json:"object"`
	Name        string            `json:"name"`
	Active      bool              `json:"active"`
	Description string            `json:"description,omitempty"`
	Images      []string          `json:"images,omitempty"`
	Livemode    bool              `json:"livemode"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	DefaultPrice string           `json:"default_price,omitempty"`
	Created     int64             `json:"created"`
	Updated     int64             `json:"updated"`
}

// Price represents a Stripe price.
type Price struct {
	ID                string            `json:"id"`
	Object            string            `json:"object"`
	Active            bool              `json:"active"`
	Currency          string            `json:"currency"`
	Product           string            `json:"product"`
	UnitAmount        int64             `json:"unit_amount,omitempty"`
	UnitAmountDecimal string            `json:"unit_amount_decimal,omitempty"`
	Type              string            `json:"type"` // one_time or recurring
	BillingScheme     string            `json:"billing_scheme"` // per_unit or tiered
	Recurring         *PriceRecurring   `json:"recurring,omitempty"`
	Nickname          string            `json:"nickname,omitempty"`
	Livemode          bool              `json:"livemode"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Created           int64             `json:"created"`
}

// PriceRecurring holds recurring price configuration.
type PriceRecurring struct {
	Interval      string `json:"interval"` // day, week, month, year
	IntervalCount int    `json:"interval_count"`
	UsageType     string `json:"usage_type,omitempty"` // licensed or metered
}

// PaymentIntent represents a Stripe payment intent.
type PaymentIntent struct {
	ID                    string            `json:"id"`
	Object                string            `json:"object"`
	Amount                int64             `json:"amount"`
	AmountReceived        int64             `json:"amount_received"`
	Currency              string            `json:"currency"`
	Customer              string            `json:"customer,omitempty"`
	Description           string            `json:"description,omitempty"`
	Status                string            `json:"status"` // requires_payment_method, requires_confirmation, requires_action, processing, succeeded, canceled
	PaymentMethod         string            `json:"payment_method,omitempty"`
	CaptureMethod         string            `json:"capture_method"` // automatic or manual
	ConfirmationMethod    string            `json:"confirmation_method"` // automatic or manual
	ClientSecret          string            `json:"client_secret"`
	LatestCharge          string            `json:"latest_charge,omitempty"`
	CanceledAt            int64             `json:"canceled_at,omitempty"`
	CancellationReason    string            `json:"cancellation_reason,omitempty"`
	Livemode              bool              `json:"livemode"`
	Metadata              map[string]string `json:"metadata,omitempty"`
	Created               int64             `json:"created"`
}

// PaymentMethod represents a Stripe payment method.
type PaymentMethod struct {
	ID          string            `json:"id"`
	Object      string            `json:"object"`
	Type        string            `json:"type"` // card, us_bank_account, etc.
	Customer    string            `json:"customer,omitempty"`
	Card        *CardDetails      `json:"card,omitempty"`
	BillingDetails *BillingDetails `json:"billing_details,omitempty"`
	Livemode    bool              `json:"livemode"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Created     int64             `json:"created"`
}

// CardDetails holds card payment method details.
type CardDetails struct {
	Brand    string `json:"brand"`
	Last4    string `json:"last4"`
	ExpMonth int    `json:"exp_month"`
	ExpYear  int    `json:"exp_year"`
	Funding  string `json:"funding"` // credit, debit, prepaid
	Country  string `json:"country,omitempty"`
}

// BillingDetails holds customer billing information.
type BillingDetails struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// Charge represents a Stripe charge.
type Charge struct {
	ID              string            `json:"id"`
	Object          string            `json:"object"`
	Amount          int64             `json:"amount"`
	AmountRefunded  int64             `json:"amount_refunded"`
	Currency        string            `json:"currency"`
	Customer        string            `json:"customer,omitempty"`
	Description     string            `json:"description,omitempty"`
	PaymentIntent   string            `json:"payment_intent,omitempty"`
	PaymentMethod   string            `json:"payment_method,omitempty"`
	Status          string            `json:"status"` // succeeded, pending, failed
	Captured        bool              `json:"captured"`
	Refunded        bool              `json:"refunded"`
	Paid            bool              `json:"paid"`
	Disputed        bool              `json:"disputed"`
	Livemode        bool              `json:"livemode"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Created         int64             `json:"created"`
}

// Refund represents a Stripe refund.
type Refund struct {
	ID            string            `json:"id"`
	Object        string            `json:"object"`
	Amount        int64             `json:"amount"`
	Currency      string            `json:"currency"`
	Charge        string            `json:"charge"`
	PaymentIntent string            `json:"payment_intent,omitempty"`
	Reason        string            `json:"reason,omitempty"` // duplicate, fraudulent, requested_by_customer
	Status        string            `json:"status"` // succeeded, pending, failed, canceled
	Metadata      map[string]string `json:"metadata,omitempty"`
	Created       int64             `json:"created"`
}

// AccountBalance tracks per-account balance (available and pending).
type AccountBalance struct {
	Available map[string]int64 // currency -> amount
	Pending   map[string]int64 // currency -> amount
}

// NewAccountBalance creates a zero balance.
func NewAccountBalance() *AccountBalance {
	return &AccountBalance{
		Available: map[string]int64{"usd": 0},
		Pending:   map[string]int64{"usd": 0},
	}
}

// PayoutStatus constants.
const (
	PayoutStatusPending   = "pending"
	PayoutStatusInTransit = "in_transit"
	PayoutStatusPaid      = "paid"
	PayoutStatusFailed    = "failed"
	PayoutStatusCanceled  = "canceled"
)

// Default timestamps for testing.
func Now() int64 {
	return time.Now().Unix()
}
