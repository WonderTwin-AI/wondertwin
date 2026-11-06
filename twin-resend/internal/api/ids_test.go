package api_test

import (
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

var (
	uuidV7 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

// setupPinned starts an emulator whose clock is pinned, so ids depend only on
// the seed and the clock.
func setupPinned(t *testing.T, at time.Time) (*store.MemoryStore, *testutil.TwinClient) {
	t.Helper()
	twin := twincore.New(&twincore.Config{Name: "twin-resend-test"})
	s := store.New()
	s.Rand = twin.Rand
	s.Clock.Pin(at)
	api.NewHandler(s, twin.Middleware()).Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	return s, testutil.NewTwinClient(t, srv)
}

func sendOne(tc *testutil.TwinClient) string {
	resp := resendPost(tc, "/emails", map[string]any{
		"from": "a@example.com", "to": []string{"delivered@resend.dev"}, "subject": "s", "html": "<p>h</p>",
	})
	resp.AssertStatus(200)
	return resp.JSONMap()["id"].(string)
}

func batchIDs(tc *testutil.TwinClient) []string {
	resp := resendPost(tc, "/emails/batch", []map[string]any{
		{"from": "a@example.com", "to": []string{"delivered@resend.dev"}, "subject": "1", "html": "<p>1</p>"},
		{"from": "a@example.com", "to": []string{"delivered@resend.dev"}, "subject": "2", "html": "<p>2</p>"},
	})
	resp.AssertStatus(200)
	var ids []string
	for _, item := range resp.JSONMap()["data"].([]any) {
		ids = append(ids, item.(map[string]any)["id"].(string))
	}
	return ids
}

func TestIDsAreUUIDs(t *testing.T) {
	at := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	s, tc := setupPinned(t, at)

	email := sendOne(tc)
	if !uuidV7.MatchString(email) {
		t.Fatalf("email id %q is not a UUIDv7", email)
	}
	ms := at.UnixMilli()
	wantPrefix := regexp.MustCompile(`^` + regexp.QuoteMeta(
		formatHex48(ms)[0:8]+"-"+formatHex48(ms)[8:12]) + `-`)
	if !wantPrefix.MatchString(email) {
		t.Errorf("email id %q does not carry the clock's time %d ms", email, ms)
	}
	for _, id := range batchIDs(tc) {
		if !uuidV7.MatchString(id) {
			t.Errorf("batch email id %q is not a UUIDv7", id)
		}
	}

	s.Clock.Pin(at.Add(time.Second))
	if later := sendOne(tc); later <= email {
		t.Errorf("a later email id %q does not sort after %q", later, email)
	}

	for _, path := range []string{"/domains", "/api-keys", "/audiences"} {
		resp := resendPost(tc, path, map[string]any{"name": "smoke.example.com"})
		id, _ := resp.JSONMap()["id"].(string)
		if !uuidV4.MatchString(id) {
			t.Errorf("%s id %q is not a UUIDv4", path, id)
		}
	}
}

func TestIDsAreDeterministic(t *testing.T) {
	at := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	_, a := setupPinned(t, at)
	_, b := setupPinned(t, at)
	if x, y := sendOne(a), sendOne(b); x != y {
		t.Errorf("same seed and clock issued %q and %q", x, y)
	}
	x := resendPost(a, "/domains", map[string]any{"name": "a.example.com"}).JSONMap()["id"]
	y := resendPost(b, "/domains", map[string]any{"name": "a.example.com"}).JSONMap()["id"]
	if x != y {
		t.Errorf("same seed issued domain ids %v and %v", x, y)
	}
}

// formatHex48 renders the low 48 bits of ms as 12 hex digits, the UUIDv7
// timestamp field.
func formatHex48(ms int64) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 12)
	for i := 11; i >= 0; i-- {
		out[i] = digits[ms&0xf]
		ms >>= 4
	}
	return string(out)
}
