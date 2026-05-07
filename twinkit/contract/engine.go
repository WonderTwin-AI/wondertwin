package contract

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// Engine is the contract workflow engine. It owns the canonical state
// machine, party model with routing-order semantics, and audit log.
// Storage delegates to a per-twin StateStore; downstream effects compose
// via Hooks.
type Engine struct {
	mu        sync.Mutex
	store     StateStore
	hooks     Hooks
	clock     *state.Clock
	idCounter int
	idPrefix  string
}

// Option configures the Engine.
type Option func(*Engine)

// WithStore sets the per-twin StateStore.
func WithStore(s StateStore) Option {
	return func(e *Engine) { e.store = s }
}

// WithHooks sets the Hooks implementation. Defaults to NoOpHooks.
func WithHooks(h Hooks) Option {
	return func(e *Engine) { e.hooks = h }
}

// WithClock sets the clock used for all engine timestamps. Defaults to
// a fresh state.Clock. Tests should pin the clock for determinism.
func WithClock(c *state.Clock) Option {
	return func(e *Engine) { e.clock = c }
}

// WithIDPrefix sets the prefix used for engine-generated contract IDs.
// Default "ctr". Twins typically override per platform vocabulary
// ("envelope" for DocuSign, "contract" for Measure).
func WithIDPrefix(p string) Option {
	return func(e *Engine) { e.idPrefix = p }
}

// NewEngine creates a contract engine with the given options.
// A StateStore is required; NewEngine panics if one is not provided.
func NewEngine(opts ...Option) *Engine {
	e := &Engine{
		hooks:    NoOpHooks{},
		idPrefix: "ctr",
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.clock == nil {
		e.clock = state.NewClock()
	}
	if e.store == nil {
		panic("contract.NewEngine: WithStore is required")
	}
	return e
}

// -- ID + audit helpers ---------------------------------------------------

// nextID generates the next contract ID. Caller must hold e.mu.
func (e *Engine) nextID() string {
	e.idCounter++
	return fmt.Sprintf("%s_%06d", e.idPrefix, e.idCounter)
}

// appendAudit appends an audit entry to the contract. Caller must own
// the contract reference exclusively (engine holds e.mu).
func (e *Engine) appendAudit(c *Contract, t AuditEventType, partyID string, from, to Status, msg string, extras map[string]any) {
	seq := uint64(len(c.AuditLog) + 1)
	c.AuditLog = append(c.AuditLog, AuditEntry{
		Sequence:  seq,
		At:        e.clock.Now(),
		EventType: t,
		PartyID:   partyID,
		From:      from,
		To:        to,
		Message:   msg,
		Extras:    extras,
	})
}

// -- Lifecycle operations -------------------------------------------------

// Create drafts a new contract. The input parties are validated
// (routing-order, witness pairing, role rules) and the engine assigns
// an ID + timestamps. Status is DRAFT on success.
func (e *Engine) Create(ctx context.Context, in CreateInput) (*Contract, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := validateParties(in.Parties); err != nil {
		return nil, err
	}

	now := e.clock.Now()
	c := &Contract{
		ID:        in.ID,
		Status:    StatusDraft,
		Parties:   cloneParties(in.Parties),
		Documents: cloneDocuments(in.Documents),
		Metadata:  cloneMetadata(in.Metadata),
		AuditLog:  make([]AuditEntry, 0, 4),
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: in.ExpiresAt,
	}
	if c.ID == "" {
		c.ID = e.nextID()
	}

	e.appendAudit(c, AuditCreated, "", "", StatusDraft, "", nil)

	if err := e.hooks.ValidateCreate(ctx, c); err != nil {
		return nil, fmt.Errorf("validate create: %w", err)
	}
	if err := e.store.Save(c); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	return c, nil
}

// Update applies a Mutation to a DRAFT contract. Returns ErrInvalidStatus
// if the contract is past DRAFT.
func (e *Engine) Update(ctx context.Context, id string, mut Mutation) (*Contract, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	c, err := e.store.Load(id)
	if err != nil {
		return nil, err
	}
	if c.Status != StatusDraft {
		return nil, fmt.Errorf("%w: cannot update a %s contract", ErrInvalidStatus, c.Status)
	}

	if mut.Parties != nil {
		if err := validateParties(*mut.Parties); err != nil {
			return nil, err
		}
		c.Parties = cloneParties(*mut.Parties)
	}
	if mut.Documents != nil {
		c.Documents = cloneDocuments(*mut.Documents)
	}
	if mut.Metadata != nil {
		if c.Metadata == nil {
			c.Metadata = make(map[string]any, len(mut.Metadata))
		}
		for k, v := range mut.Metadata {
			c.Metadata[k] = v
		}
	}
	if mut.ExpiresAt != nil {
		c.ExpiresAt = *mut.ExpiresAt
	}
	c.UpdatedAt = e.clock.Now()
	e.appendAudit(c, AuditUpdated, "", c.Status, c.Status, "", nil)

	if err := e.hooks.ValidateMutation(ctx, c); err != nil {
		return nil, fmt.Errorf("validate mutation: %w", err)
	}
	if err := e.store.Save(c); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	return c, nil
}

// Send transitions DRAFT → SENT. From this point, signatures may be
// recorded against parties in the active routing-order group.
func (e *Engine) Send(ctx context.Context, id string) (*Contract, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	c, err := e.store.Load(id)
	if err != nil {
		return nil, err
	}
	if c.Status != StatusDraft {
		return nil, fmt.Errorf("%w: cannot send a %s contract", ErrInvalidStatus, c.Status)
	}

	now := e.clock.Now()
	c.SentAt = now
	c.UpdatedAt = now
	if err := e.transition(ctx, c, StatusSent, AuditSent, "", ""); err != nil {
		return nil, err
	}
	if err := e.store.Save(c); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	return c, nil
}

// SubmitForApproval is reserved for v0.2.
func (e *Engine) SubmitForApproval(_ context.Context, _ string) (*Contract, error) {
	return nil, ErrApprovalChainNotImplemented
}

// Approve is reserved for v0.2.
func (e *Engine) Approve(_ context.Context, _, _ string, _ any, _ string) (*Contract, error) {
	return nil, ErrApprovalChainNotImplemented
}

// RecordSignature records a signature for one party. Engine enforces:
//   - contract is SENT or PARTIALLY_SIGNED
//   - party exists, is a SIGNER (or paired WITNESS), and is in the
//     active routing-order group
//   - party hasn't already signed
//
// On the last required signature, the contract advances SIGNED → and
// (if Hooks.OnExecuted returns nil) → EXECUTED.
func (e *Engine) RecordSignature(ctx context.Context, id, partyID string, att Attestation) (*Contract, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	c, err := e.store.Load(id)
	if err != nil {
		return nil, err
	}
	switch c.Status {
	case StatusSent, StatusPartiallySigned:
		// allowed
	default:
		return nil, fmt.Errorf("%w: cannot record signature on %s", ErrInvalidStatus, c.Status)
	}

	idx := findPartyIdx(c.Parties, partyID)
	if idx < 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidParty, partyID)
	}
	p := &c.Parties[idx]
	if p.Role != RoleSigner && !(p.Role == RoleWitness && p.PairedWith != nil) {
		return nil, fmt.Errorf("%w: role %s cannot sign", ErrInvalidParty, p.Role)
	}
	if !p.SignedAt.IsZero() {
		return nil, ErrAlreadySigned
	}
	if !partyInActiveSet(c.Parties, p) {
		return nil, ErrPartyNotInActiveSet
	}

	now := e.clock.Now()
	att.PartyID = partyID
	if att.Timestamp.IsZero() {
		att.Timestamp = now
	}
	p.SignedAt = att.Timestamp
	p.Attestation = &att
	c.UpdatedAt = now
	e.appendAudit(c, AuditSignatureRecorded, partyID, c.Status, c.Status, "", nil)

	if err := e.hooks.OnSignatureRecorded(ctx, c, p); err != nil {
		// Non-fatal: recorded but hook failed. Persist the signature.
		_ = e.store.Save(c)
		return nil, fmt.Errorf("hook OnSignatureRecorded: %w", err)
	}

	// State transitions driven by remaining required signatures.
	if !allRequiredSigned(c.Parties) {
		// Move to PARTIALLY_SIGNED on first signature if still SENT.
		if c.Status == StatusSent {
			if err := e.transition(ctx, c, StatusPartiallySigned, AuditPartiallySigned, partyID, ""); err != nil {
				return nil, err
			}
		}
		if err := e.store.Save(c); err != nil {
			return nil, fmt.Errorf("save: %w", err)
		}
		return c, nil
	}

	// All required parties signed → SIGNED, then attempt EXECUTED.
	if err := e.transition(ctx, c, StatusSigned, AuditSigned, partyID, ""); err != nil {
		return nil, err
	}
	if err := e.tryExecute(ctx, c); err != nil {
		// Persist SIGNED state; surface the error.
		if saveErr := e.store.Save(c); saveErr != nil {
			return nil, fmt.Errorf("save after execute failure: %w (original: %v)", saveErr, err)
		}
		return c, err
	}
	if err := e.store.Save(c); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	return c, nil
}

// Execute is normally fired automatically by RecordSignature when all
// required parties have signed. Exposed for manual recovery from a
// SIGNED-but-not-EXECUTED state (e.g., OnExecuted previously errored).
func (e *Engine) Execute(ctx context.Context, id string) (*Contract, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	c, err := e.store.Load(id)
	if err != nil {
		return nil, err
	}
	if c.Status != StatusSigned {
		return nil, fmt.Errorf("%w: cannot execute a %s contract", ErrInvalidStatus, c.Status)
	}
	if err := e.tryExecute(ctx, c); err != nil {
		return nil, err
	}
	if err := e.store.Save(c); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	return c, nil
}

// Decline records a party's refusal and transitions the contract to
// DECLINED. The party must be a SIGNER (or paired WITNESS).
func (e *Engine) Decline(ctx context.Context, id, partyID, reason string) (*Contract, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	c, err := e.store.Load(id)
	if err != nil {
		return nil, err
	}
	switch c.Status {
	case StatusSent, StatusPartiallySigned:
	default:
		return nil, fmt.Errorf("%w: cannot decline a %s contract", ErrInvalidStatus, c.Status)
	}

	idx := findPartyIdx(c.Parties, partyID)
	if idx < 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidParty, partyID)
	}
	p := c.Parties[idx]
	if p.Role != RoleSigner && !(p.Role == RoleWitness && p.PairedWith != nil) {
		return nil, fmt.Errorf("%w: role %s cannot decline", ErrInvalidParty, p.Role)
	}

	c.UpdatedAt = e.clock.Now()
	if err := e.transition(ctx, c, StatusDeclined, AuditDeclined, partyID, reason); err != nil {
		return nil, err
	}
	if err := e.store.Save(c); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	return c, nil
}

// Void cancels a non-terminal contract.
func (e *Engine) Void(ctx context.Context, id, reason string) (*Contract, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	c, err := e.store.Load(id)
	if err != nil {
		return nil, err
	}
	if isTerminal(c.Status) {
		return nil, fmt.Errorf("%w: cannot void a %s contract", ErrInvalidStatus, c.Status)
	}

	now := e.clock.Now()
	c.VoidedAt = now
	c.UpdatedAt = now
	if err := e.transition(ctx, c, StatusVoided, AuditVoided, "", reason); err != nil {
		return nil, err
	}
	if err := e.store.Save(c); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	return c, nil
}

// Expire transitions a non-terminal contract to EXPIRED. Typically
// invoked by a scheduler watching Contract.ExpiresAt; the engine does
// not own that timer.
func (e *Engine) Expire(ctx context.Context, id string) (*Contract, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	c, err := e.store.Load(id)
	if err != nil {
		return nil, err
	}
	if isTerminal(c.Status) {
		return nil, fmt.Errorf("%w: cannot expire a %s contract", ErrInvalidStatus, c.Status)
	}
	c.UpdatedAt = e.clock.Now()
	if err := e.transition(ctx, c, StatusExpired, AuditExpired, "", ""); err != nil {
		return nil, err
	}
	if err := e.store.Save(c); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	return c, nil
}

// Get returns a snapshot of the contract by ID.
func (e *Engine) Get(_ context.Context, id string) (*Contract, error) {
	return e.store.Load(id)
}

// List returns all contracts in insertion order.
func (e *Engine) List(_ context.Context) ([]*Contract, error) {
	return e.store.List()
}

// -- internal -------------------------------------------------------------

// transition records a state change and invokes OnStateTransition. The
// caller updates timestamps before this is called. Audit entry is
// emitted; hook errors are non-fatal but surfaced.
func (e *Engine) transition(ctx context.Context, c *Contract, to Status, evt AuditEventType, partyID, msg string) error {
	from := c.Status
	c.Status = to
	e.appendAudit(c, evt, partyID, from, to, msg, nil)
	if err := e.hooks.OnStateTransition(ctx, c, from, to); err != nil {
		return fmt.Errorf("hook OnStateTransition %s→%s: %w", from, to, err)
	}
	return nil
}

// tryExecute attempts to advance a SIGNED contract to EXECUTED via
// Hooks.OnExecuted. If the hook errors, contract stays at SIGNED and
// the error bubbles up.
func (e *Engine) tryExecute(ctx context.Context, c *Contract) error {
	if c.Status != StatusSigned {
		return fmt.Errorf("%w: tryExecute from %s", ErrInvalidStatus, c.Status)
	}
	if err := e.hooks.OnExecuted(ctx, c); err != nil {
		return fmt.Errorf("hook OnExecuted: %w", err)
	}
	now := e.clock.Now()
	c.ExecutedAt = now
	c.UpdatedAt = now
	return e.transition(ctx, c, StatusExecuted, AuditExecuted, "", "")
}

func isTerminal(s Status) bool {
	switch s {
	case StatusExecuted, StatusDeclined, StatusVoided, StatusExpired:
		return true
	}
	return false
}

// Now exposes the engine's clock for twins that want to share the same
// time source for their non-engine timestamps.
func (e *Engine) Now() time.Time { return e.clock.Now() }
