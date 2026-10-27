package api

import "testing"

func TestAdminResetClearsQueuedWebhooksAndTheDeliveryLog(t *testing.T) {
	h := newTestHandler(1)
	h.Webhooks().Enqueue("item-1", map[string]any{"webhook_type": "ITEM", "webhook_code": "LOGIN_REPAIRED"})
	if len(h.Webhooks().QueuedEvents()) == 0 {
		t.Fatal("the event was not queued")
	}

	h.store.Reset()

	if n := len(h.Webhooks().QueuedEvents()); n != 0 {
		t.Fatalf("%d events survived the reset", n)
	}
	if n := len(h.Webhooks().Deliveries()); n != 0 {
		t.Fatalf("%d deliveries survived the reset", n)
	}
}
