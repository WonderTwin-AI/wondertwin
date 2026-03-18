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

// Subscription represents a Stripe subscription.
type Subscription struct {
	ID                 string            `json:"id"`
	Object             string            `json:"object"`
	Customer           string            `json:"customer"`
	Status             string            `json:"status"` // trialing, active, past_due, canceled, unpaid, incomplete, incomplete_expired, paused
	CurrentPeriodStart int64             `json:"current_period_start"`
	CurrentPeriodEnd   int64             `json:"current_period_end"`
	CancelAtPeriodEnd  bool              `json:"cancel_at_period_end"`
	CancelAt           int64             `json:"cancel_at,omitempty"`
	CanceledAt         int64             `json:"canceled_at,omitempty"`
	TrialStart         int64             `json:"trial_start,omitempty"`
	TrialEnd           int64             `json:"trial_end,omitempty"`
	Items              *SubscriptionItems `json:"items"`
	LatestInvoice      string            `json:"latest_invoice,omitempty"`
	DefaultPaymentMethod string          `json:"default_payment_method,omitempty"`
	CollectionMethod   string            `json:"collection_method"` // charge_automatically or send_invoice
	Livemode           bool              `json:"livemode"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Created            int64             `json:"created"`
}

// SubscriptionItems wraps the list of subscription items.
type SubscriptionItems struct {
	Object  string             `json:"object"`
	Data    []SubscriptionItem `json:"data"`
	HasMore bool               `json:"has_more"`
	URL     string             `json:"url"`
}

// SubscriptionItem represents one price in a subscription.
type SubscriptionItem struct {
	ID           string            `json:"id"`
	Object       string            `json:"object"`
	Price        Price             `json:"price"`
	Quantity     int64             `json:"quantity"`
	Subscription string            `json:"subscription,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Created      int64             `json:"created"`
}

// Invoice represents a Stripe invoice.
type Invoice struct {
	ID                    string            `json:"id"`
	Object                string            `json:"object"`
	Customer              string            `json:"customer"`
	Subscription          string            `json:"subscription,omitempty"`
	Status                string            `json:"status"` // draft, open, paid, uncollectible, void
	AmountDue             int64             `json:"amount_due"`
	AmountPaid            int64             `json:"amount_paid"`
	AmountRemaining       int64             `json:"amount_remaining"`
	Total                 int64             `json:"total"`
	Subtotal              int64             `json:"subtotal"`
	Currency              string            `json:"currency"`
	CollectionMethod      string            `json:"collection_method"`
	Paid                  bool              `json:"paid"`
	Lines                 *InvoiceLines     `json:"lines,omitempty"`
	DefaultPaymentMethod  string            `json:"default_payment_method,omitempty"`
	PaymentIntent         string            `json:"payment_intent,omitempty"`
	HostedInvoiceURL      string            `json:"hosted_invoice_url,omitempty"`
	Number                string            `json:"number,omitempty"`
	DueDate               int64             `json:"due_date,omitempty"`
	PeriodStart           int64             `json:"period_start,omitempty"`
	PeriodEnd             int64             `json:"period_end,omitempty"`
	Livemode              bool              `json:"livemode"`
	Metadata              map[string]string `json:"metadata,omitempty"`
	Created               int64             `json:"created"`
}

// InvoiceLines wraps invoice line items.
type InvoiceLines struct {
	Object  string        `json:"object"`
	Data    []InvoiceLine `json:"data"`
	HasMore bool          `json:"has_more"`
	URL     string        `json:"url"`
}

// InvoiceLine represents a line item on an invoice.
type InvoiceLine struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	Description string `json:"description,omitempty"`
	Price       *Price `json:"price,omitempty"`
	Quantity    int64  `json:"quantity,omitempty"`
	Period      *Period `json:"period,omitempty"`
	Type        string `json:"type"` // subscription or invoiceitem
}

// Period represents a billing period.
type Period struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// InvoiceItem represents a standalone invoice item.
type InvoiceItem struct {
	ID          string            `json:"id"`
	Object      string            `json:"object"`
	Customer    string            `json:"customer"`
	Invoice     string            `json:"invoice,omitempty"`
	Price       string            `json:"price,omitempty"`
	Amount      int64             `json:"amount"`
	Currency    string            `json:"currency"`
	Description string            `json:"description,omitempty"`
	Quantity    int64             `json:"quantity"`
	UnitAmount  int64             `json:"unit_amount,omitempty"`
	Livemode    bool              `json:"livemode"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Created     int64             `json:"created"`
}

// Coupon represents a Stripe coupon.
type Coupon struct {
	ID               string            `json:"id"`
	Object           string            `json:"object"`
	AmountOff        int64             `json:"amount_off,omitempty"`
	PercentOff       float64           `json:"percent_off,omitempty"`
	Currency         string            `json:"currency,omitempty"`
	Duration         string            `json:"duration"` // forever, once, repeating
	DurationInMonths int               `json:"duration_in_months,omitempty"`
	MaxRedemptions   int               `json:"max_redemptions,omitempty"`
	TimesRedeemed    int               `json:"times_redeemed"`
	Name             string            `json:"name,omitempty"`
	Valid            bool              `json:"valid"`
	Livemode         bool              `json:"livemode"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	Created          int64             `json:"created"`
}

// SetupIntent represents a Stripe setup intent for future payments.
type SetupIntent struct {
	ID                 string            `json:"id"`
	Object             string            `json:"object"`
	Customer           string            `json:"customer,omitempty"`
	PaymentMethod      string            `json:"payment_method,omitempty"`
	Status             string            `json:"status"` // requires_payment_method, requires_confirmation, requires_action, processing, succeeded, canceled
	ClientSecret       string            `json:"client_secret"`
	Usage              string            `json:"usage"` // off_session or on_session
	Livemode           bool              `json:"livemode"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Created            int64             `json:"created"`
}

// TaxRate represents a Stripe tax rate.
type TaxRate struct {
	ID           string            `json:"id"`
	Object       string            `json:"object"`
	Active       bool              `json:"active"`
	DisplayName  string            `json:"display_name"`
	Description  string            `json:"description,omitempty"`
	Inclusive    bool              `json:"inclusive"`
	Percentage   float64           `json:"percentage"`
	Country      string            `json:"country,omitempty"`
	State        string            `json:"state,omitempty"`
	Jurisdiction string            `json:"jurisdiction,omitempty"`
	Livemode     bool              `json:"livemode"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Created      int64             `json:"created"`
}

// Dispute represents a Stripe dispute/chargeback.
type Dispute struct {
	ID              string            `json:"id"`
	Object          string            `json:"object"`
	Amount          int64             `json:"amount"`
	Currency        string            `json:"currency"`
	Charge          string            `json:"charge"`
	PaymentIntent   string            `json:"payment_intent,omitempty"`
	Reason          string            `json:"reason"` // fraudulent, duplicate, etc.
	Status          string            `json:"status"` // needs_response, under_review, won, lost
	Livemode        bool              `json:"livemode"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Created         int64             `json:"created"`
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

// CheckoutSession represents a Stripe Checkout Session.
type CheckoutSession struct {
	ID            string            `json:"id"`
	Object        string            `json:"object"`
	Mode          string            `json:"mode"`
	Status        string            `json:"status"`
	URL           string            `json:"url,omitempty"`
	SuccessURL    string            `json:"success_url,omitempty"`
	CancelURL     string            `json:"cancel_url,omitempty"`
	Customer      string            `json:"customer,omitempty"`
	CustomerEmail string            `json:"customer_email,omitempty"`
	PaymentIntent string            `json:"payment_intent,omitempty"`
	Subscription  string            `json:"subscription,omitempty"`
	PaymentStatus string            `json:"payment_status"`
	Currency      string            `json:"currency,omitempty"`
	AmountTotal   int64             `json:"amount_total"`
	ExpiresAt     int64             `json:"expires_at"`
	Livemode      bool              `json:"livemode"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Created       int64             `json:"created"`
}

// PaymentLink represents a Stripe Payment Link.
type PaymentLink struct {
	ID       string            `json:"id"`
	Object   string            `json:"object"`
	Active   bool              `json:"active"`
	URL      string            `json:"url"`
	Currency string            `json:"currency,omitempty"`
	Livemode bool              `json:"livemode"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Created  int64             `json:"created"`
}

// Token represents a Stripe tokenized payment source.
type Token struct {
	ID       string       `json:"id"`
	Object   string       `json:"object"`
	Type     string       `json:"type"`
	Card     *CardDetails `json:"card,omitempty"`
	Used     bool         `json:"used"`
	Livemode bool         `json:"livemode"`
	Created  int64        `json:"created"`
}

// Source represents a Stripe source object.
type Source struct {
	ID       string            `json:"id"`
	Object   string            `json:"object"`
	Type     string            `json:"type"`
	Status   string            `json:"status"`
	Amount   int64             `json:"amount,omitempty"`
	Currency string            `json:"currency,omitempty"`
	Customer string            `json:"customer,omitempty"`
	Flow     string            `json:"flow,omitempty"`
	Livemode bool              `json:"livemode"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Created  int64             `json:"created"`
}

// Mandate represents a Stripe mandate for recurring payments.
type Mandate struct {
	ID            string `json:"id"`
	Object        string `json:"object"`
	PaymentMethod string `json:"payment_method"`
	Status        string `json:"status"`
	Type          string `json:"type"`
	Livemode      bool   `json:"livemode"`
	Created       int64  `json:"created"`
}

// ConfirmationToken represents a Stripe confirmation token.
type ConfirmationToken struct {
	ID            string `json:"id"`
	Object        string `json:"object"`
	PaymentIntent string `json:"payment_intent,omitempty"`
	SetupIntent   string `json:"setup_intent,omitempty"`
	PaymentMethod string `json:"payment_method_preview,omitempty"`
	ExpiresAt     int64  `json:"expires_at"`
	Livemode      bool   `json:"livemode"`
	Created       int64  `json:"created"`
}

// CreditNote represents a Stripe credit note.
type CreditNote struct {
	ID       string            `json:"id"`
	Object   string            `json:"object"`
	Invoice  string            `json:"invoice"`
	Customer string            `json:"customer,omitempty"`
	Amount   int64             `json:"amount"`
	Currency string            `json:"currency"`
	Reason   string            `json:"reason,omitempty"`
	Status   string            `json:"status"`
	Type     string            `json:"type"`
	Memo     string            `json:"memo,omitempty"`
	Number   string            `json:"number,omitempty"`
	Livemode bool              `json:"livemode"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Created  int64             `json:"created"`
}

// PromotionCode represents a Stripe promotion code.
type PromotionCode struct {
	ID             string            `json:"id"`
	Object         string            `json:"object"`
	Code           string            `json:"code"`
	Coupon         string            `json:"coupon"`
	Active         bool              `json:"active"`
	MaxRedemptions int               `json:"max_redemptions,omitempty"`
	TimesRedeemed  int               `json:"times_redeemed"`
	ExpiresAt      int64             `json:"expires_at,omitempty"`
	Livemode       bool              `json:"livemode"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Created        int64             `json:"created"`
}

// Quote represents a Stripe quote.
type Quote struct {
	ID             string            `json:"id"`
	Object         string            `json:"object"`
	Customer       string            `json:"customer,omitempty"`
	Status         string            `json:"status"`
	AmountTotal    int64             `json:"amount_total"`
	AmountSubtotal int64             `json:"amount_subtotal"`
	Currency       string            `json:"currency,omitempty"`
	Description    string            `json:"description,omitempty"`
	ExpiresAt      int64             `json:"expires_at"`
	Header         string            `json:"header,omitempty"`
	Footer         string            `json:"footer,omitempty"`
	Livemode       bool              `json:"livemode"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Created        int64             `json:"created"`
}

// BillingPortalSession represents a Stripe billing portal session.
type BillingPortalSession struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Customer  string `json:"customer"`
	URL       string `json:"url"`
	ReturnURL string `json:"return_url,omitempty"`
	Livemode  bool   `json:"livemode"`
	Created   int64  `json:"created"`
}

// Review represents a Stripe Radar review.
type Review struct {
	ID            string `json:"id"`
	Object        string `json:"object"`
	Charge        string `json:"charge,omitempty"`
	PaymentIntent string `json:"payment_intent,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Status        string `json:"status"`
	OpenedReason  string `json:"opened_reason,omitempty"`
	ClosedReason  string `json:"closed_reason,omitempty"`
	Livemode      bool   `json:"livemode"`
	Created       int64  `json:"created"`
}

// TaxID represents a customer tax ID.
type TaxID struct {
	ID           string             `json:"id"`
	Object       string             `json:"object"`
	Customer     string             `json:"customer"`
	Type         string             `json:"type"`
	Value        string             `json:"value"`
	Verification *TaxIDVerification `json:"verification,omitempty"`
	Livemode     bool               `json:"livemode"`
	Created      int64              `json:"created"`
}

// TaxIDVerification represents the verification status of a tax ID.
type TaxIDVerification struct {
	Status          string `json:"status"`
	VerifiedName    string `json:"verified_name,omitempty"`
	VerifiedAddress string `json:"verified_address,omitempty"`
}

// WebhookEndpoint represents a Stripe webhook endpoint configuration.
type WebhookEndpoint struct {
	ID            string            `json:"id"`
	Object        string            `json:"object"`
	URL           string            `json:"url"`
	Status        string            `json:"status"`
	EnabledEvents []string          `json:"enabled_events"`
	Secret        string            `json:"secret,omitempty"`
	APIVersion    string            `json:"api_version,omitempty"`
	Description   string            `json:"description,omitempty"`
	Livemode      bool              `json:"livemode"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Created       int64             `json:"created"`
}

// File represents a Stripe file upload.
type File struct {
	ID       string `json:"id"`
	Object   string `json:"object"`
	Purpose  string `json:"purpose"`
	Filename string `json:"filename,omitempty"`
	Size     int64  `json:"size"`
	Type     string `json:"type,omitempty"`
	URL      string `json:"url,omitempty"`
	Livemode bool   `json:"livemode"`
	Created  int64  `json:"created"`
}

// FileLink represents a link to a Stripe file.
type FileLink struct {
	ID        string            `json:"id"`
	Object    string            `json:"object"`
	File      string            `json:"file"`
	URL       string            `json:"url"`
	Expired   bool              `json:"expired"`
	ExpiresAt int64             `json:"expires_at,omitempty"`
	Livemode  bool              `json:"livemode"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Created   int64             `json:"created"`
}

// ShippingRate represents a Stripe shipping rate.
type ShippingRate struct {
	ID          string            `json:"id"`
	Object      string            `json:"object"`
	Active      bool              `json:"active"`
	DisplayName string            `json:"display_name"`
	FixedAmount *ShippingFixed    `json:"fixed_amount,omitempty"`
	Type        string            `json:"type"`
	Livemode    bool              `json:"livemode"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Created     int64             `json:"created"`
}

// ShippingFixed holds fixed-amount shipping rate details.
type ShippingFixed struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// ApplicationFee represents a Stripe application fee collected from a connected account.
type ApplicationFee struct {
	ID                 string `json:"id"`
	Object             string `json:"object"`
	Amount             int64  `json:"amount"`
	AmountRefunded     int64  `json:"amount_refunded"`
	Currency           string `json:"currency"`
	Charge             string `json:"charge,omitempty"`
	Account            string `json:"account,omitempty"`
	BalanceTransaction string `json:"balance_transaction,omitempty"`
	Refunded           bool   `json:"refunded"`
	Livemode           bool   `json:"livemode"`
	Created            int64  `json:"created"`
}

// ApplicationFeeRefund represents a refund of an application fee.
type ApplicationFeeRefund struct {
	ID       string            `json:"id"`
	Object   string            `json:"object"`
	Amount   int64             `json:"amount"`
	Currency string            `json:"currency"`
	Fee      string            `json:"fee"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Created  int64             `json:"created"`
}

// TransferReversal represents a reversal of a Stripe transfer.
type TransferReversal struct {
	ID                 string            `json:"id"`
	Object             string            `json:"object"`
	Amount             int64             `json:"amount"`
	Currency           string            `json:"currency"`
	Transfer           string            `json:"transfer"`
	BalanceTransaction string            `json:"balance_transaction,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Created            int64             `json:"created"`
}

// AccountLink represents an ephemeral Stripe account link for Connect onboarding.
type AccountLink struct {
	Object    string `json:"object"`
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at"`
	Created   int64  `json:"created"`
}

// Person represents a person associated with a Stripe Connect account.
type Person struct {
	ID           string              `json:"id"`
	Object       string              `json:"object"`
	Account      string              `json:"account"`
	FirstName    string              `json:"first_name,omitempty"`
	LastName     string              `json:"last_name,omitempty"`
	Email        string              `json:"email,omitempty"`
	Phone        string              `json:"phone,omitempty"`
	Relationship *PersonRelationship `json:"relationship,omitempty"`
	Livemode     bool                `json:"livemode"`
	Metadata     map[string]string   `json:"metadata,omitempty"`
	Created      int64               `json:"created"`
}

// PersonRelationship describes a person's relationship to the account.
type PersonRelationship struct {
	Director         bool    `json:"director"`
	Executive        bool    `json:"executive"`
	Owner            bool    `json:"owner"`
	Representative   bool    `json:"representative"`
	Title            string  `json:"title,omitempty"`
	PercentOwnership float64 `json:"percent_ownership,omitempty"`
}

// TopUp represents a Stripe top-up to add funds to the platform balance.
type TopUp struct {
	ID                 string            `json:"id"`
	Object             string            `json:"object"`
	Amount             int64             `json:"amount"`
	Currency           string            `json:"currency"`
	Description        string            `json:"description,omitempty"`
	Status             string            `json:"status"`
	BalanceTransaction string            `json:"balance_transaction,omitempty"`
	Livemode           bool              `json:"livemode"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Created            int64             `json:"created"`
}

// Default timestamps for testing.
func Now() int64 {
	return time.Now().Unix()
}
