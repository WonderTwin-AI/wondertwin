package seed

import "testing"

func TestWebhookSampleHasTheDocumentedFireableBodiesAndNoOthers(t *testing.T) {
	for _, key := range []string{"ITEM.NEW_ACCOUNTS_AVAILABLE", "ITEM.LOGIN_REPAIRED", "ITEM.PENDING_DISCONNECT",
		"ITEM.USER_PERMISSION_REVOKED", "ITEM.USER_ACCOUNT_REVOKED", "TRANSACTIONS.DEFAULT_UPDATE",
		"TRANSACTIONS.SYNC_UPDATES_AVAILABLE", "TRANSACTIONS.RECURRING_TRANSACTIONS_UPDATE", "AUTH.DEFAULT_UPDATE"} {
		body, ok := WebhookSample(key)
		if !ok || body["webhook_type"] == nil || body["webhook_code"] == nil || key != body["webhook_type"].(string)+"."+body["webhook_code"].(string) {
			t.Errorf("%s: %v, found %v", key, body, ok)
		}
	}
	for _, key := range []string{"ASSETS.ERROR", "ASSETS.PRODUCT_READY", "ITEM.AUTHORIZATION_GRANTED", "IDENTITY.DEFAULT_UPDATE"} {
		if _, ok := WebhookSample(key); ok {
			t.Errorf("%s has no published sample and must not be present", key)
		}
	}
	a, _ := WebhookSample("ITEM.LOGIN_REPAIRED")
	a["item_id"] = "changed"
	if b, _ := WebhookSample("ITEM.LOGIN_REPAIRED"); b["item_id"] == "changed" {
		t.Error("WebhookSample returned a shared map, not a copy")
	}
}
