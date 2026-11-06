package api_test

import (
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

func idemSend(tc *testutil.TwinClient, path, key string, body any) *testutil.Response {
	return tc.DoWithHeaders("POST", path, body, map[string]string{
		"Authorization":   "Bearer re_sim_test_123",
		"Idempotency-Key": key,
	})
}

func idemEmail(subject string) map[string]any {
	return map[string]any{"from": "a@example.com", "to": "delivered@resend.dev", "subject": subject, "html": "<p>hi</p>"}
}

func emailCount(t *testing.T, tc *testutil.TwinClient) int {
	t.Helper()
	return int(tc.Get("/admin/emails").AssertStatus(200).JSONMap()["total"].(float64))
}

func TestIdempotencyReplay(t *testing.T) {
	_, tc := setupResend(t)

	first := idemSend(tc, "/emails", "k1", idemEmail("hi")).AssertStatus(200)
	again := idemSend(tc, "/emails", "k1", idemEmail("hi")).AssertStatus(200)
	if got := first.Headers.Get("Idempotent-Replayed"); got != "false" {
		t.Errorf("first idempotent-replayed = %q, want false", got)
	}
	if got := again.Headers.Get("Idempotent-Replayed"); got != "true" {
		t.Errorf("replay idempotent-replayed = %q, want true", got)
	}
	if first.JSONMap()["id"] != again.JSONMap()["id"] {
		t.Errorf("replay id %v differs from first %v", again.JSONMap()["id"], first.JSONMap()["id"])
	}
	if n := emailCount(t, tc); n != 1 {
		t.Errorf("stored %d emails, want 1", n)
	}
}

func TestIdempotencyKeyReusedWithOtherBody(t *testing.T) {
	_, tc := setupResend(t)

	idemSend(tc, "/emails", "k1", idemEmail("hi")).AssertStatus(200)
	idemSend(tc, "/emails", "k1", idemEmail("changed")).
		AssertStatus(409).
		AssertBodyContains(`"name":"invalid_idempotent_request"`)
}

func TestIdempotencyKeyLength(t *testing.T) {
	_, tc := setupResend(t)

	for _, key := range []string{"", strings.Repeat("k", 257)} {
		idemSend(tc, "/emails", key, idemEmail("hi")).
			AssertStatus(400).
			AssertBodyContains("must have between 1 and 256 characters")
	}
	idemSend(tc, "/emails", strings.Repeat("k", 256), idemEmail("hi")).AssertStatus(200)
}

func TestIdempotencyExpiresAfter24Hours(t *testing.T) {
	_, tc := setupResend(t)
	adm := testutil.NewAdminClient(tc)

	first := idemSend(tc, "/emails", "k1", idemEmail("hi")).AssertStatus(200)
	adm.AdvanceTime("24h").AssertStatus(200)
	later := idemSend(tc, "/emails", "k1", idemEmail("changed")).AssertStatus(200)
	if later.JSONMap()["id"] == first.JSONMap()["id"] {
		t.Error("expired key replayed the first answer")
	}
}

func TestIdempotencyFailedRequestIsNotKept(t *testing.T) {
	_, tc := setupResend(t)

	idemSend(tc, "/emails", "k1", map[string]any{"to": "delivered@resend.dev"}).AssertStatus(422)
	idemSend(tc, "/emails", "k1", idemEmail("hi")).AssertStatus(200)
}

func TestIdempotencyConcurrentRequest(t *testing.T) {
	_, tc := setupResend(t)
	adm := testutil.NewAdminClient(tc)

	adm.LoadState(map[string]any{
		"emails": map[string]any{},
		"idempotency": map[string]any{
			"POST /emails k1": map[string]any{"request_hash": "x", "created_at": "2099-01-01T00:00:00Z", "in_flight": true},
		},
	}).AssertStatus(200)
	idemSend(tc, "/emails", "k1", idemEmail("hi")).
		AssertStatus(409).
		AssertBodyContains(`"name":"concurrent_idempotent_requests"`)
}

func TestIdempotencyBatchAndScope(t *testing.T) {
	_, tc := setupResend(t)

	batch := []any{idemEmail("a"), idemEmail("b")}
	idemSend(tc, "/emails/batch", "k1", batch).AssertStatus(200)
	again := idemSend(tc, "/emails/batch", "k1", batch).AssertStatus(200)
	if got := again.Headers.Get("Idempotent-Replayed"); got != "true" {
		t.Errorf("batch replay idempotent-replayed = %q, want true", got)
	}
	// The key is scoped to method and endpoint, so /emails can reuse it.
	idemSend(tc, "/emails", "k1", idemEmail("hi")).AssertStatus(200)
	if n := emailCount(t, tc); n != 3 {
		t.Errorf("stored %d emails, want 3", n)
	}
}

func TestIdempotencyResetClearsKeys(t *testing.T) {
	_, tc := setupResend(t)
	adm := testutil.NewAdminClient(tc)

	idemSend(tc, "/emails", "k1", idemEmail("hi")).AssertStatus(200)
	if !strings.Contains(string(adm.GetState().Body), "POST /emails k1") {
		t.Error("state snapshot does not hold the key")
	}
	adm.Reset().AssertStatus(200)
	idemSend(tc, "/emails", "k1", idemEmail("changed")).AssertStatus(200)
}
