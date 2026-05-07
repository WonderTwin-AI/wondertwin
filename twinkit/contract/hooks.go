package contract

import "context"

// Hooks is the per-twin extension surface for the contract engine. The
// engine invokes these at behavioral inflection points; implementations
// must not call back into the engine (would deadlock).
//
// All methods receive a snapshot of the contract at the point of the
// callback. Mutations to the snapshot are not persisted.
type Hooks interface {
	// ValidateCreate is called before a contract is persisted. Returning
	// a non-nil error aborts the Create.
	ValidateCreate(ctx context.Context, c *Contract) error

	// ValidateMutation is called before an Update is persisted. The
	// engine has already merged the mutation into the snapshot.
	ValidateMutation(ctx context.Context, c *Contract) error

	// OnStateTransition is called after a contract transitions state and
	// is persisted. Errors are logged via the bridge but do not roll
	// back the transition.
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
	OnExecuted(ctx context.Context, c *Contract) error
}

// NoOpHooks is a default no-op implementation. Embed this in a twin's
// hook implementation to override only the methods that matter.
type NoOpHooks struct{}

func (NoOpHooks) ValidateCreate(_ context.Context, _ *Contract) error  { return nil }
func (NoOpHooks) ValidateMutation(_ context.Context, _ *Contract) error { return nil }
func (NoOpHooks) OnStateTransition(_ context.Context, _ *Contract, _, _ Status) error {
	return nil
}
func (NoOpHooks) OnSignatureRecorded(_ context.Context, _ *Contract, _ *Party) error {
	return nil
}
func (NoOpHooks) OnExecuted(_ context.Context, _ *Contract) error { return nil }
