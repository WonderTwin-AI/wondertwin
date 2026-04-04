package ledger

import (
	"context"

	"github.com/wondertwin-ai/wondertwin/twinkit/domainevents"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// WithTelemetry returns an Option that wraps the engine's hooks to automatically
// emit DomainEvent telemetry when hooks fire. The twin's custom hooks are called
// after telemetry emission. If no hooks are set, telemetry-only hooks are used.
func WithTelemetry(emitter domainevents.Emitter) Option {
	return func(e *Engine) {
		inner := e.hooks
		if inner == nil {
			inner = NoOpAccountingHooks{}
		}
		e.hooks = &telemetryBridge{emitter: emitter, inner: inner}
	}
}

type telemetryBridge struct {
	emitter domainevents.Emitter
	inner   AccountingHooks
}

func (b *telemetryBridge) OnDocumentStateTransition(ctx context.Context, doc *Document, from, to DocumentStatus) error {
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "ledger",
			Hook:       "OnDocumentStateTransition",
			Resource:   docTypeToResource(doc.Type),
			Action:     "transition",
			EntityType: string(doc.Type),
			EntityID:   doc.ID,
			StateFrom:  string(from),
			StateTo:    string(to),
			Context:    DocumentTransitionContext(from, to, doc.Type, len(doc.LineItems)),
		},
	)
	return b.inner.OnDocumentStateTransition(ctx, doc, from, to)
}

func (b *telemetryBridge) OnJournalEntryCreated(ctx context.Context, doc *Document, entries []JournalEntryInfo) error {
	accountSet := make(map[string]struct{})
	hasNonzeroDebit := false
	for _, e := range entries {
		accountSet[e.AccountID] = struct{}{}
		if e.Type == "debit" && e.Amount > 0 {
			hasNonzeroDebit = true
		}
	}

	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "ledger",
			Hook:       "OnJournalEntryCreated",
			Resource:   docTypeToResource(doc.Type),
			Action:     "create",
			EntityType: "journal_entry",
			EntityID:   doc.ID,
			Context:    JournalEntryContext(len(entries), len(accountSet), hasNonzeroDebit),
		},
	)
	return b.inner.OnJournalEntryCreated(ctx, doc, entries)
}

func (b *telemetryBridge) OnPaymentApplied(ctx context.Context, payment *Payment, doc *Document) error {
	fullyPaid := doc.AmountPaid >= doc.Total
	overpayment := doc.AmountPaid > doc.Total

	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "ledger",
			Hook:       "OnPaymentApplied",
			Resource:   "payments",
			Action:     "create",
			EntityType: "payment",
			EntityID:   payment.ID,
			Context:    PaymentAppliedContext(string(doc.Type), fullyPaid, overpayment),
		},
	)
	return b.inner.OnPaymentApplied(ctx, payment, doc)
}

func docTypeToResource(dt DocumentType) string {
	switch dt {
	case DocTypeInvoice:
		return "invoices"
	case DocTypeBill:
		return "bills"
	case DocTypeCreditNote:
		return "credit_notes"
	default:
		return "documents"
	}
}
