package api_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type received struct {
	body    string
	headers http.Header
}

// receiver is a webhook URL that reports every request it gets.
func receiver(t *testing.T) (url string, got chan received) {
	t.Helper()
	got = make(chan received, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- received{string(b), r.Header.Clone()}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, got
}

func next(t *testing.T, got chan received) received {
	t.Helper()
	select {
	case r := <-got:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("no webhook arrived")
		return received{}
	}
}

func nothing(t *testing.T, got chan received) {
	t.Helper()
	select {
	case r := <-got:
		t.Fatalf("an unexpected webhook arrived: %s", r.body)
	case <-time.After(400 * time.Millisecond):
	}
}

func TestItemWebhookUpdateAcknowledgesToTheNewURLWithASignedFlatBody(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	url, got := receiver(t)

	res := tc.Post("/item/webhook/update", creds(map[string]any{"access_token": token, "webhook": url})).AssertStatus(200).JSONMap()
	if res["item"].(map[string]any)["webhook"] != url || res["request_id"] == nil {
		t.Fatalf("response = %v", res)
	}
	r := next(t, got)

	var body map[string]any
	if err := json.Unmarshal([]byte(r.body), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body["webhook_type"] != "ITEM" || body["webhook_code"] != "WEBHOOK_UPDATE_ACKNOWLEDGED" || body["new_webhook_url"] != url ||
		body["environment"] != "sandbox" || body["item_id"] == nil {
		t.Fatalf("body = %v", body)
	}
	if v, ok := body["error"]; !ok || v != nil {
		t.Fatalf("error must be present as null, as in Plaid's sample: %v", body)
	}
	for _, k := range []string{"data", "type", "id", "created_at"} {
		if _, wrapped := body[k]; wrapped {
			t.Fatalf("the body is wrapped in the kit's envelope (%s): %s", k, r.body)
		}
	}
	if !strings.Contains(r.body, "\n  \"webhook_code\"") {
		t.Fatalf("body is not indented with 2 spaces:\n%s", r.body)
	}
	if got := r.headers.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}

	parts := strings.Split(r.headers.Get("Plaid-Verification"), ".")
	if len(parts) != 3 {
		t.Fatalf("Plaid-Verification = %q", r.headers.Get("Plaid-Verification"))
	}
	raw, _ := b64decode(parts[1])
	var claims map[string]any
	_ = json.Unmarshal(raw, &claims)
	sum := sha256.Sum256([]byte(r.body))
	if claims["request_body_sha256"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("request_body_sha256 does not match the body that was sent: %v", claims)
	}
}

func TestItemWebhookUpdateRoutesPerItemAndClearsOnNull(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	tokenA, tokenB := linkedItem(tc), linkedItem(tc)
	urlA, gotA := receiver(t)
	urlB, gotB := receiver(t)
	tc.Post("/item/webhook/update", creds(map[string]any{"access_token": tokenB, "webhook": urlB})).AssertStatus(200)
	next(t, gotB)

	tc.Post("/item/webhook/update", creds(map[string]any{"access_token": tokenA, "webhook": urlA})).AssertStatus(200)
	next(t, gotA)
	nothing(t, gotB)

	cleared := tc.Post("/item/webhook/update", creds(map[string]any{"access_token": tokenA})).AssertStatus(200).JSONMap()
	if v, ok := cleared["item"].(map[string]any)["webhook"]; !ok || v != nil {
		t.Fatalf("webhook must be present as null once removed: %v", cleared)
	}
	nothing(t, gotA)
	got := tc.Post("/item/get", creds(map[string]any{"access_token": tokenA})).JSONMap()["item"].(map[string]any)
	if got["webhook"] != nil {
		t.Fatalf("item/get still shows a webhook: %v", got["webhook"])
	}
}

func TestItemWebhookUpdateChecksItsToken(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	for body, code := range map[string]string{"missing": "MISSING_FIELDS", "unknown": "INVALID_ACCESS_TOKEN", "wrong type": "INVALID_BODY"} {
		b := map[string]any{}
		switch body {
		case "unknown":
			b["access_token"] = "access-sandbox-nope"
		case "wrong type":
			b["access_token"] = linkedItem(tc)
			b["webhook"] = 5
		}
		if m := tc.Post("/item/webhook/update", creds(b)).AssertStatus(400).JSONMap(); m["error_code"] != code {
			t.Fatalf("%s: %v", body, m)
		}
	}
}

var base64RawURL = base64.RawURLEncoding

func b64decode(s string) ([]byte, error) { return base64RawURL.DecodeString(s) }
