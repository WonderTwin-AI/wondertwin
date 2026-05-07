package contract

import "context"

// Hooks is the per-twin extension surface for the contract engine. The
// engine invokes these at behavioral inflection points; implementations
// must not call back into the engine (would deadlock).
//
// All methods receive a snapshot of the contract at the point of the
// callback. Mutations to the snapshot are not persisted.
//
// Callback partitioning. The engine routes transitions to two distinct
// observer surfaces so consumers don't have to special-case terminal
// states inside a generic transition handler:
//
//   - OnStateTransition fires for every NON-terminal transition
//     (DRAFT→SENT, SENT→PARTIALLY_SIGNED, PARTIALLY_SIGNED→SIGNED, …).
//     This is the place to emit "contract.updated"-style change events.
//
//   - OnExecuted, OnDeclined, OnVoided, OnExpired fire for the four
//     terminal transitions. Each is the canonical site for the
//     side-effects specific to that terminal state (downstream
//     materialisation on EXECUTED, refund/cleanup on VOIDED, etc.).
//
// A single state change invokes EITHER OnStateTransition OR exactly one
// terminal hook — never both.
type Hooks interface {
	// ValidateCreate is called before a contract is persisted. Returning
	// a non-nil error aborts the Create.
	ValidateCreate(ctx context.Context, c *Contract) error

	// ValidateMutation is called before an Update is persisted. The
	// engine has already merged the mutation into the snapshot.
	ValidateMutation(ctx context.Context, c *Contract) error

	// OnStateTransition is called after a contract transitions to a
	// non-terminal state and is persisted. It does NOT fire for
	// transitions into EXECUTED, DECLINED, VOIDED, or EXPIRED — those
	// route to their dedicated terminal hooks (OnExecuted, OnDeclined,
	// OnVoided, OnExpired). Errors halt the transition and surface to
	// the caller.
	OnStateTransition(ctx context.Context, c *Contract, from, to Status) error

	// OnSignatureRecorded is called after a signature is recorded.
	// Errors are non-fatal.
	OnSignatureRecorded(ctx context.Context, c *Contract, party *Party) error

	// OnExecuted is called when a contract transitions SIGNED → EXECUTED.
	// This is the post-execution observer hook — the canonical place to
	// materialise downstream resources (Measure subscription/invoice,
	// CPQ contract activation, etc.).
	//
	// Returning a non-nil error halts the transition: the contract stays
	// at SIGNED and the error is surfaced to the caller of
	// RecordSignature/Execute. The twin is responsible for retry/repair.
	//
	// OnStateTransition does NOT also fire for SIGNED → EXECUTED.
	OnExecuted(ctx context.Context, c *Contract) error

	// OnDeclined is called after a contract transitions to DECLINED and
	// is persisted. Use this hook to emit decline-specific side effects
	// (notifications, audit forwarding) without having to special-case
	// the status inside OnStateTransition. Errors are surfaced to the
	// caller of Decline; the contract has already moved to DECLINED.
	OnDeclined(ctx context.Context, c *Contract, partyID, reason string) error

	// OnVoided is called after a contract transitions to VOIDED and is
	// persisted. Use this hook for cancellation-specific side effects
	// (refunds, downstream tear-down). Errors are surfaced to the caller
	// of Void; the contract has already moved to VOIDED.
	OnVoided(ctx context.Context, c *Contract, reason string) error

	// OnExpired is called after a contract transitions to EXPIRED and is
	// persisted. Errors are surfaced to the caller of Expire; the
	// contract has already moved to EXPIRED.
	OnExpired(ctx context.Context, c *Contract) error
}

// NoOpHooks is a default no-op implementation. Embed this in a twin's
// hook implementation to override only the methods that matter.
type NoOpHooks struct{}

func (NoOpHooks) ValidateCreate(_ context.Context, _ *Contract) error   { return nil }
func (NoOpHooks) ValidateMutation(_ context.Context, _ *Contract) error { return nil }
func (NoOpHooks) OnStateTransition(_ context.Context, _ *Contract, _, _ Status) error {
	return nil
}
func (NoOpHooks) OnSignatureRecorded(_ context.Context, _ *Contract, _ *Party) error {
	return nil
}
func (NoOpHooks) OnExecuted(_ context.Context, _ *Contract) error              { return nil }
func (NoOpHooks) OnDeclined(_ context.Context, _ *Contract, _, _ string) error { return nil }
func (NoOpHooks) OnVoided(_ context.Context, _ *Contract, _ string) error      { return nil }
func (NoOpHooks) OnExpired(_ context.Context, _ *Contract) error               { return nil }
