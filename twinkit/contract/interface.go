// Package contract provides a contract-and-signature workflow engine for
// twins that simulate platforms with multi-party agreement lifecycles —
// e-signature (DocuSign-shape), CLM (Ironclad-shape), CPQ contracts, and
// billing-platform contracts (Measure-shape).
//
// The engine owns the canonical state machine, party model with routing-
// order semantics, witness counter-signatory pairing, and an append-only
// audit log per contract. Document storage, webhook emission, and any
// downstream materialisation (e.g. billing twins creating a subscription
// when a contract executes) are delegated to the consuming twin via the
// StateStore and Hooks interfaces.
//
// This is the v0.1 surface. The pre-signature approval chain
// (PENDING_APPROVAL state, ApprovalStep, SubmitForApproval, Approve) is
// reserved as design but returns ErrApprovalChainNotImplemented from the
// engine until v0.2.
package contract

import "time"

// Status is the canonical lifecycle state of a contract. Twins map
// platform-specific vocabularies (DocuSign envelope status, Measure
// contract status, etc.) onto this set via their EventMapper.
type Status string

const (
	StatusDraft           Status = "DRAFT"
	StatusPendingApproval Status = "PENDING_APPROVAL" // reserved for v0.2
	StatusSent            Status = "SENT"
	StatusPartiallySigned Status = "PARTIALLY_SIGNED"
	StatusSigned          Status = "SIGNED"   // all required parties signed; pre-execution
	StatusExecuted        Status = "EXECUTED" // terminal success
	StatusDeclined        Status = "DECLINED"
	StatusVoided          Status = "VOIDED"
	StatusExpired         Status = "EXPIRED"
)

// RoleType is the canonical party role.
type RoleType string

const (
	RoleSigner   RoleType = "SIGNER"
	RoleApprover RoleType = "APPROVER" // observed in v0.1; ignored for state transitions until v0.2
	RoleCC       RoleType = "CC"
	RoleWitness  RoleType = "WITNESS"
	RoleAgent    RoleType = "AGENT"
	RoleEditor   RoleType = "EDITOR"
)

// DocumentRef is an opaque reference to a document attached to a
// contract. The engine never reads or writes document bytes — twins wire
// storage themselves via twinkit/workspace.
type DocumentRef struct {
	ID       string         `json:"id"`
	Name     string         `json:"name,omitempty"`
	Hash     string         `json:"hash,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Party is one signer/approver/observer attached to a contract.
type Party struct {
	ID           string         `json:"id"`
	Role         RoleType       `json:"role"`
	DisplayName  string         `json:"display_name,omitempty"`
	Contact      string         `json:"contact,omitempty"` // email or platform-specific
	RoutingOrder int            `json:"routing_order"`     // 0 = first; equal values = parallel
	Required     bool           `json:"required"`
	PairedWith   *string        `json:"paired_with,omitempty"` // WITNESS only: paired SIGNER's ID
	SignedAt     time.Time      `json:"signed_at,omitempty"`
	Attestation  *Attestation   `json:"attestation,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// Attestation captures the audit-relevant context of a single signature.
// Engines record what's supplied; PKI cert generation is out of scope.
type Attestation struct {
	PartyID    string         `json:"party_id"`
	Timestamp  time.Time      `json:"timestamp"`
	AuthMethod string         `json:"auth_method,omitempty"` // e.g., "email_link", "sso", "in_person"
	Source     string         `json:"source,omitempty"`      // IP, geo, or platform-specific
	Hash       string         `json:"hash,omitempty"`        // optional document hash at signing time
	Extras     map[string]any `json:"extras,omitempty"`      // PKI cert serial, BankID nonce, etc.
}

// AuditEventType enumerates the canonical audit-log entry kinds. The
// engine emits these on every state-changing operation; twins translate
// to platform-specific webhook event names via their EventMapper.
type AuditEventType string

const (
	AuditCreated         AuditEventType = "CONTRACT_CREATED"
	AuditUpdated         AuditEventType = "CONTRACT_UPDATED"
	AuditSent            AuditEventType = "CONTRACT_SENT"
	AuditSignatureRecorded AuditEventType = "SIGNATURE_RECORDED"
	AuditPartiallySigned AuditEventType = "CONTRACT_PARTIALLY_SIGNED"
	AuditSigned          AuditEventType = "CONTRACT_SIGNED"
	AuditExecuted        AuditEventType = "CONTRACT_EXECUTED"
	AuditDeclined        AuditEventType = "CONTRACT_DECLINED"
	AuditVoided          AuditEventType = "CONTRACT_VOIDED"
	AuditExpired         AuditEventType = "CONTRACT_EXPIRED"
)

// AuditEntry is one append-only entry in a contract's audit log.
// Sequence is monotonic per contract starting at 1.
type AuditEntry struct {
	Sequence  uint64         `json:"sequence"`
	At        time.Time      `json:"at"`
	EventType AuditEventType `json:"event_type"`
	PartyID   string         `json:"party_id,omitempty"`
	From      Status         `json:"from,omitempty"`
	To        Status         `json:"to,omitempty"`
	Message   string         `json:"message,omitempty"`
	Extras    map[string]any `json:"extras,omitempty"`
}

// Contract is the canonical contract record.
type Contract struct {
	ID         string         `json:"id"`
	Status     Status         `json:"status"`
	Parties    []Party        `json:"parties"` // in routing order
	Documents  []DocumentRef  `json:"documents,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	AuditLog   []AuditEntry   `json:"audit_log"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	SentAt     time.Time      `json:"sent_at,omitempty"`
	ExecutedAt time.Time      `json:"executed_at,omitempty"`
	VoidedAt   time.Time      `json:"voided_at,omitempty"`
	ExpiresAt  time.Time      `json:"expires_at,omitempty"`
}

// CreateInput is the input to Engine.Create.
type CreateInput struct {
	ID        string         // optional; engine generates if empty
	Parties   []Party        // engine validates routing-order + witness pairing
	Documents []DocumentRef
	Metadata  map[string]any
	ExpiresAt time.Time      // optional
}

// Mutation is a partial update to a draft contract via Engine.Update.
// Nil fields are left unchanged.
type Mutation struct {
	Parties   *[]Party
	Documents *[]DocumentRef
	Metadata  map[string]any // merged shallowly
	ExpiresAt *time.Time
}

// StateStore persists contracts. Implementations are typically the
// twin's in-memory store. The engine holds a reference, never inspects
// internals beyond these methods.
type StateStore interface {
	// Save persists a contract. Implementations must store a snapshot
	// (deep copy) so subsequent engine mutations don't leak.
	Save(c *Contract) error

	// Load returns a snapshot of the contract by ID.
	// Returns ErrContractNotFound if absent.
	Load(id string) (*Contract, error)

	// List returns all contracts, in insertion order.
	List() ([]*Contract, error)
}
