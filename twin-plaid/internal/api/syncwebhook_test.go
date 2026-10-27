package api_test

import (
	"sync"
	"testing"
)

func refresh(t *testing.T, tcPost func(string, map[string]any) map[string]any, token string) {
	t.Helper()
	tcPost("/transactions/refresh", creds(map[string]any{"access_token": token}))
}

func TestSyncWebhookRefreshSendsDefaultUpdateAndSyncUpdatesAvailableOnlyAfterSyncRan(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	post := func(path string, b map[string]any) map[string]any {
		return tc.Post(path, b).AssertStatus(200).JSONMap()
	}
	url, got := receiver(t)
	token := linkedWithHook(tc, []string{"transactions"}, url)
	create := func(n int) {
		var rows []any
		for i := 0; i < n; i++ {
			rows = append(rows, newTx("row", float64(i+1), nil))
		}
		post("/sandbox/transactions/create", creds(map[string]any{"access_token": token, "transactions": rows}))
	}

	create(2)
	nothing(t, got) // creating alone fires nothing
	refresh(t, post, token)
	first := delivered(t, got)
	if first["webhook_type"] != "TRANSACTIONS" || first["webhook_code"] != "DEFAULT_UPDATE" || first["new_transactions"] != 2.0 {
		t.Fatalf("before any sync, a refresh tells the receiver of the new rows only: %v", first)
	}
	nothing(t, got) // SYNC_UPDATES_AVAILABLE waits until sync has run (quirk DELIVERY-006)

	doSync(tc, token, "", 0)
	create(1)
	refresh(t, post, token)
	codes := map[string]map[string]any{}
	for i := 0; i < 2; i++ {
		b := delivered(t, got) // order is not guaranteed (quirk DELIVERY-001)
		codes[b["webhook_code"].(string)] = b
	}
	sync, def := codes["SYNC_UPDATES_AVAILABLE"], codes["DEFAULT_UPDATE"]
	if sync == nil || def == nil || def["new_transactions"] != 1.0 || sync["initial_update_complete"] != true ||
		sync["historical_update_complete"] != true || sync["environment"] != "sandbox" || sync["item_id"] == nil {
		t.Fatalf("after sync ran, a refresh sends both: %v", codes)
	}
}

func TestSyncWebhookRefreshSaysNothingTwiceOrForTheStartingRows(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	post := func(path string, b map[string]any) map[string]any {
		return tc.Post(path, b).AssertStatus(200).JSONMap()
	}
	url, got := receiver(t)
	token := linkedWithHook(tc, []string{"transactions"}, url)

	refresh(t, post, token)
	nothing(t, got) // the two starting rows are not news

	post("/sandbox/transactions/create", creds(map[string]any{"access_token": token, "transactions": []any{newTx("row", 1, nil)}}))
	refresh(t, post, token)
	delivered(t, got)
	refresh(t, post, token)
	nothing(t, got) // nothing changed since the last report

	quiet := linkedWithHook(tc, []string{"transactions"}, "")
	post("/sandbox/transactions/create", creds(map[string]any{"access_token": quiet, "transactions": []any{newTx("row", 1, nil)}}))
	refresh(t, post, quiet) // no URL: nothing to send to, and no error
	if it, _ := mem.ItemForAccessToken(quiet); it.NotifiedChanges != mem.ChangeLogLen(it.ID) {
		t.Fatalf("a refresh must note what it covered: %d of %d", it.NotifiedChanges, mem.ChangeLogLen(it.ID))
	}
}

func TestSyncWebhookGoesToTheChangedItemsOwnURLSigned(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	post := func(path string, b map[string]any) map[string]any {
		return tc.Post(path, b).AssertStatus(200).JSONMap()
	}
	urlA, gotA := receiver(t)
	urlB, gotB := receiver(t)
	a := linkedWithHook(tc, []string{"transactions"}, urlA)
	linkedWithHook(tc, []string{"transactions"}, urlB)

	post("/sandbox/transactions/create", creds(map[string]any{"access_token": a, "transactions": []any{newTx("row", 1, nil)}}))
	refresh(t, post, a)
	body := delivered(t, gotA) // delivered checks the Plaid-Verification header is there
	if body["webhook_code"] != "DEFAULT_UPDATE" {
		t.Fatalf("body = %v", body)
	}
	if _, wrapped := body["data"]; wrapped {
		t.Fatalf("the body must be flat: %v", body)
	}
	nothing(t, gotB)
}

func TestSyncWebhookConcurrentRefreshesFireDefaultUpdateOnce(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	post := func(path string, b map[string]any) map[string]any {
		return tc.Post(path, b).AssertStatus(200).JSONMap()
	}
	url, got := receiver(t)
	token := linkedWithHook(tc, []string{"transactions"}, url)
	post("/sandbox/transactions/create", creds(map[string]any{"access_token": token, "transactions": []any{newTx("row", 1, nil)}}))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			refresh(t, post, token)
		}()
	}
	wg.Wait()
	if first := delivered(t, got); first["webhook_code"] != "DEFAULT_UPDATE" || first["new_transactions"] != 1.0 {
		t.Fatalf("first delivery = %v, want DEFAULT_UPDATE for 1 row", first)
	}
	nothing(t, got)
}
