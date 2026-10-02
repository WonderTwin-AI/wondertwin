package api_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"
)

// received is one request the test receiver got.
type received struct {
	header http.Header
	body   []byte
}

// receiver is a stand-in for the app's Request URL. status answers each
// request in turn; once it runs out, requests get 200. The first request
// waits firstDelay before answering.
type receiver struct {
	srv        *httptest.Server
	mu         sync.Mutex
	got        []received
	status     []int
	firstDelay time.Duration
}

func newReceiver(t *testing.T, status ...int) *receiver {
	t.Helper()
	rc := &receiver{status: status}
	rc.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rc.mu.Lock()
		rc.got = append(rc.got, received{header: r.Header.Clone(), body: body})
		code := http.StatusOK
		if n := len(rc.got); n <= len(rc.status) {
			code = rc.status[n-1]
		}
		first := len(rc.got) == 1
		rc.mu.Unlock()
		if first {
			time.Sleep(rc.firstDelay)
		}
		w.WriteHeader(code)
	}))
	t.Cleanup(rc.srv.Close)
	return rc
}

// wait returns the requests once n have arrived.
func (rc *receiver) wait(t *testing.T, n int) []received {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		rc.mu.Lock()
		if len(rc.got) >= n {
			out := append([]received{}, rc.got...)
			rc.mu.Unlock()
			return out
		}
		rc.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("receiver got %d requests, want %d", len(rc.got), n)
	return nil
}

func configureEvents(t *testing.T, srv *httptest.Server, cfg map[string]any) map[string]any {
	t.Helper()
	body, _ := json.Marshal(cfg)
	status, m := call(t, srv, "POST", "/admin/events/config", jsonType, string(body), false)
	if status != 200 {
		t.Fatalf("configure events: %d %v", status, m)
	}
	return m
}

// envelope decodes a delivered event_callback.
func envelope(t *testing.T, r received) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("delivered body is not JSON: %q", r.body)
	}
	return m
}

// verify checks a delivery's signature the way Slack's SDK verifiers do.
func verify(t *testing.T, r received, secret string) {
	t.Helper()
	ts := r.header.Get("X-Slack-Request-Timestamp")
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || time.Since(time.Unix(n, 0)) > 5*time.Minute {
		t.Fatalf("X-Slack-Request-Timestamp %q is not a recent epoch", ts)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + ts + ":"))
	mac.Write(r.body)
	if want := "v0=" + hex.EncodeToString(mac.Sum(nil)); r.header.Get("X-Slack-Signature") != want {
		t.Fatalf("X-Slack-Signature %q, want %q", r.header.Get("X-Slack-Signature"), want)
	}
}

func TestPostedMessageIsDeliveredAsASignedEventCallback(t *testing.T) {
	srv, tc := setupSlack(t)
	rc := newReceiver(t)
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL, "signing_secret": "s3cret"})
	ch := seedChannel(tc, "events")

	_, posted := call(t, srv, "POST", "/api/chat.postMessage", formType, url.Values{"channel": {ch}, "text": {"hello"}}.Encode(), true)
	mustOK(t, 200, posted)

	got := rc.wait(t, 1)[0]
	verify(t, got, "s3cret")
	if ct := got.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type %q", ct)
	}
	env := envelope(t, got)
	if env["type"] != "event_callback" || env["team_id"] != "T0001" || env["api_app_id"] == "" || env["token"] == "" {
		t.Errorf("envelope: %v", env)
	}
	if id, _ := env["event_id"].(string); len(id) < 3 || id[:2] != "Ev" {
		t.Errorf("event_id %v", env["event_id"])
	}
	if _, ok := env["event_time"].(float64); !ok {
		t.Errorf("event_time %v", env["event_time"])
	}
	if auths, _ := env["authorizations"].([]any); len(auths) != 1 {
		t.Errorf("authorizations %v", env["authorizations"])
	}
	ev := env["event"].(map[string]any)
	if ev["type"] != "message" || ev["channel"] != ch || ev["text"] != "hello" || ev["ts"] != posted["ts"] || ev["event_ts"] != posted["ts"] || ev["channel_type"] != "channel" {
		t.Errorf("event: %v", ev)
	}
}

func TestNoRequestURLSendsNothing(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "quiet")
	_, posted := call(t, srv, "POST", "/api/chat.postMessage", formType, url.Values{"channel": {ch}, "text": {"unsent"}}.Encode(), true)
	mustOK(t, 200, posted)
	_, m := call(t, srv, "GET", "/admin/events/deliveries", "", "", false)
	if d := m["deliveries"].([]any); len(d) != 0 {
		t.Errorf("deliveries with no Request URL: %v", d)
	}

	// The unsent event does not hold up the next one: with Slack's one and
	// five minute retry schedule in force, a message posted once a Request URL
	// is set is delivered at once.
	rc := newReceiver(t)
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL})
	call(t, srv, "POST", "/api/chat.postMessage", formType, url.Values{"channel": {ch}, "text": {"sent"}}.Encode(), true)
	got := rc.wait(t, 1)
	if ev := envelope(t, got[0])["event"].(map[string]any); ev["text"] != "sent" {
		t.Errorf("first delivery after a Request URL is set: %v", ev)
	}
}

func TestFailedDeliveryIsRetriedWithRetryHeaders(t *testing.T) {
	srv, tc := setupSlack(t)
	rc := newReceiver(t, 500, 503, 200)
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL, "retry_delays_ms": []int{0, 5, 5}})
	ch := seedChannel(tc, "flaky")
	call(t, srv, "POST", "/api/chat.postMessage", formType, url.Values{"channel": {ch}, "text": {"x"}}.Encode(), true)

	got := rc.wait(t, 3)
	time.Sleep(50 * time.Millisecond)
	if n := len(rc.wait(t, 3)); n != 3 {
		t.Fatalf("attempts after a success: %d, want 3", n)
	}
	if got[0].header.Get("X-Slack-Retry-Num") != "" {
		t.Errorf("the first attempt is not a retry: %v", got[0].header)
	}
	for i, r := range got[1:] {
		if r.header.Get("X-Slack-Retry-Num") != strconv.Itoa(i+1) || r.header.Get("X-Slack-Retry-Reason") != "http_error" {
			t.Errorf("retry %d headers: num %q reason %q", i+1, r.header.Get("X-Slack-Retry-Num"), r.header.Get("X-Slack-Retry-Reason"))
		}
		if string(r.body) != string(got[0].body) {
			t.Errorf("retry %d resent a different body", i+1)
		}
		verify(t, r, "slack-emulator-signing-secret")
	}

	_, m := call(t, srv, "GET", "/admin/events/deliveries", "", "", false)
	d := m["deliveries"].([]any)
	if len(d) != 3 || d[2].(map[string]any)["status_code"] != float64(200) || d[1].(map[string]any)["retry_reason"] != "http_error" {
		t.Errorf("delivery log: %v", d)
	}
}

func TestDeliveryStopsAfterThreeRetries(t *testing.T) {
	srv, tc := setupSlack(t)
	rc := newReceiver(t, 500, 500, 500, 500, 500, 500)
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL, "retry_delays_ms": []int{0, 0, 0}})
	ch := seedChannel(tc, "down")
	call(t, srv, "POST", "/api/chat.postMessage", formType, url.Values{"channel": {ch}, "text": {"x"}}.Encode(), true)
	rc.wait(t, 4)
	time.Sleep(100 * time.Millisecond)
	if n := len(rc.wait(t, 4)); n != 4 {
		t.Errorf("attempts: %d, want the first and three retries", n)
	}
}

func TestSlowReceiverIsRetriedAsATimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the three-second ack window")
	}
	srv, tc := setupSlack(t)
	rc := newReceiver(t)
	rc.firstDelay = 3500 * time.Millisecond
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL, "retry_delays_ms": []int{0}})
	ch := seedChannel(tc, "slow")
	call(t, srv, "POST", "/api/chat.postMessage", formType, url.Values{"channel": {ch}, "text": {"x"}}.Encode(), true)
	got := rc.wait(t, 2)
	if got[1].header.Get("X-Slack-Retry-Reason") != "http_timeout" {
		t.Errorf("retry reason after a slow answer: %q", got[1].header.Get("X-Slack-Retry-Reason"))
	}
}

func TestDeleteAndReactionAreDeliveredAsEvents(t *testing.T) {
	srv, tc := setupSlack(t)
	rc := newReceiver(t)
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL})
	ch := seedChannel(tc, "more")
	_, posted := call(t, srv, "POST", "/api/chat.postMessage", formType, url.Values{"channel": {ch}, "text": {"x"}}.Encode(), true)
	ts := posted["ts"].(string)
	call(t, srv, "POST", "/api/reactions.add", formType, url.Values{"channel": {ch}, "timestamp": {ts}, "name": {"tada"}}.Encode(), true)
	call(t, srv, "POST", "/api/chat.delete", formType, url.Values{"channel": {ch}, "ts": {ts}}.Encode(), true)

	got := rc.wait(t, 3)
	rx := envelope(t, got[1])["event"].(map[string]any)
	item, _ := rx["item"].(map[string]any)
	if rx["type"] != "reaction_added" || rx["reaction"] != "tada" || rx["user"] != "U_BOT" || item["ts"] != ts || item["channel"] != ch {
		t.Errorf("reaction_added: %v", rx)
	}
	del := envelope(t, got[2])["event"].(map[string]any)
	if del["type"] != "message" || del["subtype"] != "message_deleted" || del["deleted_ts"] != ts || del["hidden"] != true {
		t.Errorf("message_deleted: %v", del)
	}
}

func TestEventsConfigEndpoint(t *testing.T) {
	srv, _ := setupSlack(t)
	_, m := call(t, srv, "GET", "/admin/events/config", "", "", false)
	if m["request_url"] != "" || m["signing_secret"] == "" {
		t.Errorf("default config: %v", m)
	}
	delays, _ := m["retry_delays_ms"].([]any)
	if len(delays) != 3 || delays[0] != float64(0) || delays[1] != float64(60000) || delays[2] != float64(300000) {
		t.Errorf("the default retry schedule is Slack's: %v", delays)
	}
	m = configureEvents(t, srv, map[string]any{"request_url": "http://example.invalid/events"})
	if m["request_url"] != "http://example.invalid/events" || m["signing_secret"] == "" {
		t.Errorf("a partial update keeps the other settings: %v", m)
	}
	status, _ := call(t, srv, "POST", "/admin/events/config", jsonType, `{"retry_delays_ms":[-1]}`, false)
	if status != 400 {
		t.Errorf("a negative delay: %d", status)
	}
}
