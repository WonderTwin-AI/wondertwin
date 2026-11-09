package api_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// TestStateRoundTrip takes /admin/state with one of every resource, resets,
// loads the snapshot back and reads each resource again.
func TestStateRoundTrip(t *testing.T) {
	_, tc := setupResend(t)
	adm := testutil.NewAdminClient(tc)

	emailID := resendPost(tc, "/emails", map[string]any{
		"from": "a@example.com", "to": "delivered@resend.dev", "subject": "s", "html": "<p>h</p>",
		"headers":     map[string]string{"X-Entity-Ref-ID": "123"},
		"attachments": []map[string]any{{"filename": "invoice.pdf", "content": "aGVsbG8="}},
	}).AssertStatus(200).JSONMap()["id"].(string)
	domainID := resendPost(tc, "/domains", map[string]any{"name": "example.com"}).AssertStatus(201).JSONMap()["id"].(string)
	keyID := resendPost(tc, "/api-keys", map[string]any{"name": "ci"}).AssertStatus(201).JSONMap()["id"].(string)
	audienceID := resendPost(tc, "/audiences", map[string]any{"name": "General"}).AssertStatus(201).JSONMap()["id"].(string)
	contactID := resendPost(tc, "/audiences/"+audienceID+"/contacts", map[string]any{"email": "c@example.com"}).
		AssertStatus(201).JSONMap()["id"].(string)
	broadcastID := resendPost(tc, "/broadcasts", map[string]any{
		"audience_id": audienceID, "from": "a@example.com", "subject": "news",
	}).AssertStatus(201).JSONMap()["id"].(string)

	snap := adm.GetState().AssertStatus(200).Body
	if !strings.Contains(string(snap), "X-Entity-Ref-ID") || !strings.Contains(string(snap), "invoice.pdf") {
		t.Errorf("state: want email headers and attachment metadata, got %s", snap)
	}
	if strings.Contains(string(snap), "aGVsbG8=") {
		t.Error("state: attachment content must not be kept")
	}

	adm.Reset().AssertStatus(200)
	resendGet(tc, "/domains/"+domainID).AssertStatus(404)
	adm.LoadState(json.RawMessage(snap)).AssertStatus(200)

	resendGet(tc, "/emails/"+emailID).AssertStatus(200)
	resendGet(tc, "/domains/"+domainID).AssertStatus(200)
	resendGet(tc, "/audiences/"+audienceID).AssertStatus(200)
	resendGet(tc, "/audiences/"+audienceID+"/contacts/"+contactID).AssertStatus(200)
	resendGet(tc, "/broadcasts/"+broadcastID).AssertStatus(200)
	if !strings.Contains(string(resendGet(tc, "/api-keys").Body), keyID) {
		t.Error("api key missing after load")
	}
	if again := adm.GetState().Body; string(again) != string(snap) {
		t.Errorf("state changed across the round trip:\n%s\n%s", snap, again)
	}
}

func TestLoadStateDropsInFlightIdempotency(t *testing.T) {
	_, tc := setupResend(t)
	adm := testutil.NewAdminClient(tc)
	adm.LoadState(map[string]any{
		"emails": map[string]any{},
		"idempotency": map[string]any{
			"POST emails k1": map[string]any{"request_hash": "h", "created_at": "2026-11-06T00:00:00Z", "in_flight": true},
			"POST emails k2": map[string]any{"request_hash": "h", "created_at": "2026-11-06T00:00:00Z", "status": 200, "body": map[string]any{"id": "x"}},
		},
	}).AssertStatus(200)
	state := string(adm.GetState().Body)
	if strings.Contains(state, "k1") || !strings.Contains(state, "k2") {
		t.Errorf("want the in-flight record dropped and the finished one kept, got %s", state)
	}
}

// TestResetReseeds checks /admin/reset restores the seed, so the first ids
// after a reset match the first ids of a fresh start.
func TestResetReseeds(t *testing.T) {
	twin := twincore.New(&twincore.Config{Name: "twin-resend-test"})
	s := store.New()
	s.Seed(42)
	api.NewHandler(s, twin.Middleware()).Routes(twin.Router)
	admin.NewHandler(s, twin.Middleware(), s.Clock).Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)

	create := func(name string) string {
		return resendPost(tc, "/domains", map[string]any{"name": name}).AssertStatus(201).JSONMap()["id"].(string)
	}
	first := create("example.com")
	create("example.org")
	testutil.NewAdminClient(tc).Reset().AssertStatus(200)
	if again := create("example.com"); again != first {
		t.Errorf("first id after reset %s, want %s", again, first)
	}
}
