package api_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

// receiver records the webhook deliveries it is sent.
type receiver struct {
	*httptest.Server
	mu  sync.Mutex
	got []delivered
}

type delivered struct {
	header http.Header
	body   []byte
	event  map[string]any
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	rc := &receiver{}
	rc.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var evt map[string]any
		_ = json.Unmarshal(body, &evt)
		rc.mu.Lock()
		rc.got = append(rc.got, delivered{header: r.Header.Clone(), body: body, event: evt})
		rc.mu.Unlock()
	}))
	t.Cleanup(rc.Close)
	return rc
}

// wait returns the deliveries once there are n, sorted by event type order
// of arrival; it fails the test after five seconds.
func (rc *receiver) wait(t *testing.T, n int) []delivered {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		rc.mu.Lock()
		got := append([]delivered(nil), rc.got...)
		rc.mu.Unlock()
		if len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("got %d deliveries, want %d", len(got), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func types(ds []delivered) []string {
	var out []string
	for _, d := range ds {
		out = append(out, d.event["type"].(string))
	}
	return out
}

// verifySvix checks a delivery's Svix headers the way resend.webhooks.verify
// does, independently of the emulator's signer.
func verifySvix(t *testing.T, d delivered, secret string) {
	t.Helper()
	id, ts, sig := d.header.Get("svix-id"), d.header.Get("svix-timestamp"), d.header.Get("svix-signature")
	key, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "." + string(d.body)))
	if want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil)); sig != want || !strings.HasPrefix(id, "msg_") {
		t.Errorf("svix headers id %q ts %q sig %q, want sig %q", id, ts, sig, want)
	}
}

func sendTo(tc *testutil.TwinClient, to string, extra map[string]any) string {
	body := map[string]any{"from": "onboarding@resend.dev", "to": to, "subject": "hooks", "html": "<p>h</p>"}
	for k, v := range extra {
		body[k] = v
	}
	return resendPost(tc, "/emails", body).AssertStatus(200).JSONMap()["id"].(string)
}

// TestEmailEventsDelivered checks a send delivers email.sent and the
// recipient's outcome, signed, with {type, created_at, data} and no
// top-level id (quirks delivery-001, delivery-002).
func TestEmailEventsDelivered(t *testing.T) {
	_, tc := setupResend(t)
	rc := newReceiver(t)
	secret := createWebhook(t, tc, rc.URL, "email.sent", "email.delivered", "email.bounced")["signing_secret"].(string)

	id := sendTo(tc, "delivered@resend.dev", map[string]any{"tags": []map[string]string{{"name": "category", "value": "welcome"}}})
	got := rc.wait(t, 2)
	if ts := types(got); !(reflect.DeepEqual(ts, []string{"email.sent", "email.delivered"}) || reflect.DeepEqual(ts, []string{"email.delivered", "email.sent"})) {
		t.Errorf("event types %v", ts)
	}
	for _, d := range got {
		verifySvix(t, d, secret)
		if keys := sortedKeys(d.event); !reflect.DeepEqual(keys, []string{"created_at", "data", "type"}) {
			t.Errorf("body keys %v", keys)
		}
		data := d.event["data"].(map[string]any)
		if data["email_id"] != id || data["from"] != "onboarding@resend.dev" || data["subject"] != "hooks" ||
			!reflect.DeepEqual(data["to"], []any{"delivered@resend.dev"}) || !reflect.DeepEqual(data["tags"], map[string]any{"category": "welcome"}) {
			t.Errorf("data %v", data)
		}
	}

	sendTo(tc, "bounced@resend.dev", nil)
	if ts := types(rc.wait(t, 4)[2:]); !reflect.DeepEqual(ts, []string{"email.sent", "email.bounced"}) && !reflect.DeepEqual(ts, []string{"email.bounced", "email.sent"}) {
		t.Errorf("bounced event types %v", ts)
	}
}

// TestScheduledEmailEvents checks a scheduled send delivers email.scheduled,
// then email.sent and its outcome once it is read after its time.
func TestScheduledEmailEvents(t *testing.T) {
	_, tc := setupResend(t)
	rc := newReceiver(t)
	createWebhook(t, tc, rc.URL, "email.scheduled", "email.sent", "email.delivered")

	id := sendTo(tc, "delivered@resend.dev", map[string]any{"scheduled_at": "in 1 min"})
	if ts := types(rc.wait(t, 1)); !reflect.DeepEqual(ts, []string{"email.scheduled"}) {
		t.Errorf("event types %v", ts)
	}
	tc.DoWithHeaders("POST", "/admin/time/advance", map[string]any{"duration": "2m"}, nil).AssertStatus(200)
	resendGet(tc, "/emails/"+id).AssertStatus(200)
	resendGet(tc, "/emails/"+id).AssertStatus(200)
	if got := rc.wait(t, 3); len(got) != 3 {
		t.Errorf("deliveries %v, want 3", types(got))
	}
}

// TestDisabledWebhookSkipped checks a disabled webhook, and one not
// subscribed to an event, receive nothing.
func TestDisabledWebhookSkipped(t *testing.T) {
	_, tc := setupResend(t)
	on, off, other := newReceiver(t), newReceiver(t), newReceiver(t)
	createWebhook(t, tc, on.URL, "email.sent", "email.delivered")
	offID := createWebhook(t, tc, off.URL, "email.sent", "email.delivered")["id"].(string)
	createWebhook(t, tc, other.URL, "email.bounced")
	resendPatch(tc, "/webhooks/"+offID, map[string]any{"status": "disabled"}).AssertStatus(200)

	sendTo(tc, "delivered@resend.dev", nil)
	on.wait(t, 2)
	time.Sleep(50 * time.Millisecond)
	if n := len(off.wait(t, 0)) + len(other.wait(t, 0)); n != 0 {
		t.Errorf("%d deliveries to disabled or unsubscribed webhooks", n)
	}
}
