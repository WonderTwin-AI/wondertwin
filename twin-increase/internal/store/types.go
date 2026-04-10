// Package store defines the Increase twin's state types and in-memory store.
package store

// Account represents an Increase bank account.
type Account struct {
	ID              string            `json:"id"`
	Type            string            `json:"type"` // "account"
	Name            string            `json:"name"`
	EntityID        string            `json:"entity_id,omitempty"`
	ProgramID       string            `json:"program_id"`
	Status          string            `json:"status"` // open, closed
	Bank            string            `json:"bank"` // blue_ridge_bank, first_internet_bank, grasshopper_bank
	Currency        string            `json:"currency"`
	CurrentBalance  int64             `json:"current_balance"` // cents
	AvailableBalance int64            `json:"available_balance"` // cents
	RoutingNumber   string            `json:"routing_number,omitempty"` // if ACH enabled
	AccountNumber   string            `json:"account_number,omitempty"` // if ACH enabled
	CreatedAt       string            `json:"created_at"`
	ClosedAt        string            `json:"closed_at,omitempty"`
	IdempotencyKey  string            `json:"idempotency_key,omitempty"`
}

// Account status constants
const (
	AccountStatusOpen   = "open"
	AccountStatusClosed = "closed"
)

// AccountNumber represents a routing/account number pair assigned to an account.
type AccountNumber struct {
	ID              string `json:"id"`
	Type            string `json:"type"` // "account_number"
	AccountID       string `json:"account_id"`
	AccountNumber   string `json:"account_number"`
	RoutingNumber   string `json:"routing_number"`
	Name            string `json:"name"`
	Status          string `json:"status"` // active, disabled, canceled
	CreatedAt       string `json:"created_at"`
	IdempotencyKey  string `json:"idempotency_key,omitempty"`
}

// AccountNumber status constants
const (
	AccountNumberStatusActive   = "active"
	AccountNumberStatusDisabled = "disabled"
	AccountNumberStatusCanceled = "canceled"
)

// ExternalAccount represents a bank account at another institution.
type ExternalAccount struct {
	ID              string `json:"id"`
	Type            string `json:"type"` // "external_account"
	RoutingNumber   string `json:"routing_number"`
	AccountNumber   string `json:"account_number"`
	AccountHolder   string `json:"account_holder_name,omitempty"`
	Description     string `json:"description,omitempty"`
	Status          string `json:"status"` // active, archived
	Funding         string `json:"funding"` // checking, savings
	VerificationStatus string `json:"verification_status,omitempty"` // unverified, pending, verified
	CreatedAt       string `json:"created_at"`
	IdempotencyKey  string `json:"idempotency_key,omitempty"`
}

// ExternalAccount status constants
const (
	ExternalAccountStatusActive   = "active"
	ExternalAccountStatusArchived = "archived"
)

// ACHTransfer represents an outbound ACH transfer.
type ACHTransfer struct {
	ID                    string `json:"id"`
	Type                  string `json:"type"` // "ach_transfer"
	AccountID             string `json:"account_id"`
	ExternalAccountID     string `json:"external_account_id,omitempty"`
	RoutingNumber         string `json:"routing_number,omitempty"`
	AccountNumber         string `json:"account_number,omitempty"`
	Amount                int64  `json:"amount"` // cents
	Currency              string `json:"currency"`
	StatementDescriptor   string `json:"statement_descriptor,omitempty"`
	CompanyDescriptiveDate string `json:"company_descriptive_date,omitempty"`
	CompanyDiscretionaryData string `json:"company_discretionary_data,omitempty"`
	CompanyEntryDescription string `json:"company_entry_description,omitempty"`
	CompanyName           string `json:"company_name,omitempty"`
	StandardEntryClassCode string `json:"standard_entry_class_code,omitempty"` // corporate_credit_or_debit, corporate_trade_exchange, etc.
	Funding               string `json:"funding"` // checking, savings
	Status                string `json:"status"` // pending_approval, pending_submission, submitted, acknowledged, settled, returned, canceled
	Approval              *Approval `json:"approval,omitempty"`
	Submission            *Submission `json:"submission,omitempty"`
	Acknowledgement       *Acknowledgement `json:"acknowledgement,omitempty"`
	Return                *ACHReturn `json:"return,omitempty"`
	TransactionID         string `json:"transaction_id,omitempty"` // when settled
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at,omitempty"`
	IdempotencyKey        string `json:"idempotency_key,omitempty"`
}

// ACHTransfer status constants
const (
	ACHTransferStatusPendingApproval    = "pending_approval"
	ACHTransferStatusPendingSubmission  = "pending_submission"
	ACHTransferStatusSubmitted          = "submitted"
	ACHTransferStatusAcknowledged       = "acknowledged"
	ACHTransferStatusSettled            = "settled"
	ACHTransferStatusReturned           = "returned"
	ACHTransferStatusCanceled           = "canceled"
)

// Approval tracks who approved a transfer and when.
type Approval struct {
	ApprovedAt string `json:"approved_at"`
	ApprovedBy string `json:"approved_by,omitempty"`
}

// Submission tracks when a transfer was submitted to the ACH network.
type Submission struct {
	SubmittedAt string `json:"submitted_at"`
}

// Acknowledgement tracks when the ACH network acknowledged receipt.
type Acknowledgement struct {
	AcknowledgedAt string `json:"acknowledged_at"`
}

// ACHReturn tracks a returned ACH transfer.
type ACHReturn struct {
	ReturnReasonCode string `json:"return_reason_code"`
	ReturnedAt       string `json:"returned_at"`
}

// InboundACHTransfer represents an incoming ACH transfer.
type InboundACHTransfer struct {
	ID                  string `json:"id"`
	Type                string `json:"type"` // "inbound_ach_transfer"
	AccountID           string `json:"account_id"`
	AccountNumberID     string `json:"account_number_id"`
	Amount              int64  `json:"amount"` // cents
	Currency            string `json:"currency"`
	OriginatorCompanyName string `json:"originator_company_name,omitempty"`
	OriginatorCompanyDescriptiveDate string `json:"originator_company_descriptive_date,omitempty"`
	OriginatorCompanyDiscretionaryData string `json:"originator_company_discretionary_data,omitempty"`
	OriginatorCompanyEntryDescription string `json:"originator_company_entry_description,omitempty"`
	ReceiverIDNumber    string `json:"receiver_id_number,omitempty"`
	ReceiverName        string `json:"receiver_name,omitempty"`
	TraceNumber         string `json:"trace_number,omitempty"`
	Status              string `json:"status"` // pending, accepted, declined, returned
	TransactionID       string `json:"transaction_id,omitempty"` // when accepted
	DeclinedAt          string `json:"declined_at,omitempty"`
	ReturnedAt          string `json:"returned_at,omitempty"`
	ReturnReasonCode    string `json:"return_reason_code,omitempty"`
	CreatedAt           string `json:"created_at"`
}

// InboundACHTransfer status constants
const (
	InboundACHTransferStatusPending  = "pending"
	InboundACHTransferStatusAccepted = "accepted"
	InboundACHTransferStatusDeclined = "declined"
	InboundACHTransferStatusReturned = "returned"
)

// Transaction represents a settled transaction (immutable).
type Transaction struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // "transaction"
	AccountID   string `json:"account_id"`
	Amount      int64  `json:"amount"` // cents, positive = credit, negative = debit
	Currency    string `json:"currency"`
	Description string `json:"description,omitempty"`
	RouteType   string `json:"route_type"` // ach, wire, check, card, etc.
	RouteID     string `json:"route_id"` // ID of the ACH transfer, wire, etc.
	CreatedAt   string `json:"created_at"`
}

// PendingTransaction represents an in-flight transaction affecting available balance.
type PendingTransaction struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // "pending_transaction"
	AccountID   string `json:"account_id"`
	Amount      int64  `json:"amount"` // cents, positive = credit, negative = debit
	Currency    string `json:"currency"`
	Description string `json:"description,omitempty"`
	RouteType   string `json:"route_type"` // ach, wire, check, card, etc.
	RouteID     string `json:"route_id"` // ID of the ACH transfer, wire, etc.
	Status      string `json:"status"` // pending, complete
	CompletedAt string `json:"completed_at,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// Event represents an Increase event (thin events with just ID + category).
type Event struct {
	ID                  string `json:"id"`
	Type                string `json:"type"` // "event"
	Category            string `json:"category"` // account.created, ach_transfer.submitted, etc.
	AssociatedObjectID  string `json:"associated_object_id"`
	AssociatedObjectType string `json:"associated_object_type"`
	CreatedAt           string `json:"created_at"`
}

// EventSubscription represents a webhook endpoint subscription.
type EventSubscription struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"` // "event_subscription"
	URL             string   `json:"url"`
	Status          string   `json:"status"` // active, disabled
	SelectedEventCategories []string `json:"selected_event_category_ids,omitempty"` // empty = all events
	CreatedAt       string   `json:"created_at"`
	IdempotencyKey  string   `json:"idempotency_key,omitempty"`
}

// EventSubscription status constants
const (
	EventSubscriptionStatusActive   = "active"
	EventSubscriptionStatusDisabled = "disabled"
)
