package ledger

import "context"

// AccountingHooks defines extension points for platform-specific behavior.
// The engine calls these at behavioral inflection points. Implementations
// must not call back into the engine.
type AccountingHooks interface {
	// OnJournalEntryCreated is called after journal entries are written for a
	// document state transition or payment application.
	OnJournalEntryCreated(ctx context.Context, doc *Document, entries []JournalEntryInfo) error

	// OnDocumentStateTransition is called after a document transitions state.
	OnDocumentStateTransition(ctx context.Context, doc *Document, from, to DocumentStatus) error

	// OnPaymentApplied is called after a payment is applied to a document.
	OnPaymentApplied(ctx context.Context, payment *Payment, doc *Document) error
}

// JournalEntryInfo provides hook consumers with information about the journal
// entries that were created, without exposing the journal internals.
type JournalEntryInfo struct {
	AccountID  string `json:"account_id"`
	Type       string `json:"type"` // "debit" or "credit"
	Amount     int64  `json:"amount"`
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
}

// NoOpAccountingHooks provides default no-op implementations.
// Embed this in the twin's hook implementation to avoid implementing unused hooks.
type NoOpAccountingHooks struct{}

func (NoOpAccountingHooks) OnJournalEntryCreated(_ context.Context, _ *Document, _ []JournalEntryInfo) error {
	return nil
}

func (NoOpAccountingHooks) OnDocumentStateTransition(_ context.Context, _ *Document, _, _ DocumentStatus) error {
	return nil
}

func (NoOpAccountingHooks) OnPaymentApplied(_ context.Context, _ *Payment, _ *Document) error {
	return nil
}
