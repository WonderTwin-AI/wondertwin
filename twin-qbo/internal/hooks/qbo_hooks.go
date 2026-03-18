// Package hooks implements AccountingHooks for the QBO twin.
// QBO hooks are simpler than Xero — no document state machine transitions.
// Webhooks fire on entity create/update/delete/void events.
package hooks

import (
	"context"

	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
)

// QBOHooks implements accounting.AccountingHooks for the QBO twin.
type QBOHooks struct {
	accounting.NoOpAccountingHooks
	Dispatcher *webhook.Dispatcher
}

func (h *QBOHooks) OnPaymentApplied(_ context.Context, pmt *accounting.Payment, _ *accounting.Document) error {
	if h.Dispatcher == nil {
		return nil
	}
	h.Dispatcher.Enqueue("Payment.Create", map[string]any{
		"name":      "Payment",
		"id":        pmt.ID,
		"operation": "Create",
	})
	return nil
}
