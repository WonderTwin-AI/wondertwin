package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	xerowh "github.com/wondertwin-ai/wondertwin/twin-xero/internal/webhook"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func call(h *Handler, method, path, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	h.Routes(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

// checkingReceiver does what Xero's documentation tells a receiver to do: 2xx
// for a correctly signed payload, 401 for any other. It records what it saw.
type checkingReceiver struct {
	mu      sync.Mutex
	key     string
	seen    []bool // whether each request was correctly signed
	bodies  []string
	cookies int
}

func (c *checkingReceiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	defer c.mu.Unlock()
	ok := r.Header.Get(xerowh.SignatureHeader) == xerowh.Signature(b, c.key)
	c.seen, c.bodies = append(c.seen, ok), append(c.bodies, string(b))
	c.cookies += len(r.Cookies())
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
	}
}

func registered(t *testing.T, rec *httptest.ResponseRecorder) store.Webhook {
	t.Helper()
	var w store.Webhook
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &w) != nil {
		t.Fatalf("register: %d %s", rec.Code, rec.Body)
	}
	return w
}

func TestWebhookRegisterRunsTheIntentToReceiveSeriesAndEnables(t *testing.T) {
	h := newTestHandler(t)
	rcv := &checkingReceiver{key: "the-key"}
	srv := httptest.NewServer(rcv)
	t.Cleanup(srv.Close)

	w := registered(t, call(h, "POST", "/admin/webhooks", `{"url":"`+srv.URL+`","key":"the-key","categories":["CONTACT"]}`))
	if w.Status != store.WebhookOK || w.Key != "the-key" || len(w.Categories) != 1 {
		t.Fatalf("registration = %+v", w)
	}
	// Some payloads correctly signed and some not, each with no events and no cookies.
	if len(rcv.seen) != 4 || !rcv.seen[0] || rcv.seen[1] || !rcv.seen[2] || rcv.seen[3] {
		t.Fatalf("series = %v, want signed, unsigned, signed, unsigned", rcv.seen)
	}
	for _, b := range rcv.bodies {
		if !strings.Contains(b, `"events":[]`) || !strings.Contains(b, `"firstEventSequence":0`) || !strings.Contains(b, `"entropy":"`) {
			t.Fatalf("an intent-to-receive body must carry no events: %s", b)
		}
	}
	if rcv.cookies != 0 {
		t.Fatal("the check must set no cookies")
	}
	var list []store.Webhook
	_ = json.Unmarshal(call(h, "GET", "/admin/webhooks", "").Body.Bytes(), &list)
	if len(list) != 1 || list[0].ID != w.ID {
		t.Fatalf("list = %v", list)
	}
}

func TestWebhookRegisterGeneratesAKeyAndDefaultsToEveryCategory(t *testing.T) {
	h := newTestHandler(t)
	srv := httptest.NewServer(&checkingReceiver{})
	t.Cleanup(srv.Close)
	// The check is skipped so the generated key and the defaults can be read back.
	w := registered(t, call(h, "POST", "/admin/webhooks", `{"url":"`+srv.URL+`","intent_to_receive":false}`))
	if len(w.Key) != 40 || len(w.Categories) != 5 || w.Status != store.WebhookOK {
		t.Fatalf("registration = %+v, want a 40 character key, five categories and no check run", w)
	}
}

func TestWebhookRegisterRefusesBadInput(t *testing.T) {
	h := newTestHandler(t)
	for name, body := range map[string]string{
		"not json":         `{`,
		"no url":           `{}`,
		"not an http url":  `{"url":"ftp://x"}`,
		"unknown category": `{"url":"http://127.0.0.1:1","categories":["SUBSCRIPTION"]}`,
	} {
		if rec := call(h, "POST", "/admin/webhooks", body); rec.Code != 400 {
			t.Fatalf("%s: want 400, got %d", name, rec.Code)
		}
	}
	if h.store.Webhooks.Count() != 0 {
		t.Fatal("a refused registration must store nothing")
	}
}

func TestWebhookIntentFailsAReceiverThatDoesNotCheckSignatures(t *testing.T) {
	h := newTestHandler(t)
	lax := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(lax.Close)
	w := registered(t, call(h, "POST", "/admin/webhooks", `{"url":"`+lax.URL+`","key":"k"}`))
	if w.Status != store.WebhookFailed || !strings.Contains(w.Reason, "incorrectly signed payload answered 200, want 401") {
		t.Fatalf("registration = %+v", w)
	}

	srv, got := receiver(t)
	subscribe(h, "w2", srv.URL, "k", store.WebhookFailed, "CONTACT")
	h.emit(emitRequest(), store.DefaultTenantID, "CONTACT", "CREATE", "c", nil)
	none(t, got)
}

func TestWebhookIntentVerifyReenablesASubscriptionOnceTheReceiverIsFixed(t *testing.T) {
	h := newTestHandler(t)
	rcv := &checkingReceiver{key: "right"}
	srv := httptest.NewServer(rcv)
	t.Cleanup(srv.Close)
	w := registered(t, call(h, "POST", "/admin/webhooks", `{"url":"`+srv.URL+`","key":"wrong"}`))
	if w.Status != store.WebhookFailed || !strings.Contains(w.Reason, "correctly signed payload answered 401, want 2xx") {
		t.Fatalf("registration = %+v", w)
	}
	rcv.mu.Lock()
	rcv.key = "wrong"
	rcv.mu.Unlock()
	var again store.Webhook
	_ = json.Unmarshal(call(h, "POST", "/admin/webhooks/"+w.ID+"/verify", "").Body.Bytes(), &again)
	if again.Status != store.WebhookOK || again.Reason != "" {
		t.Fatalf("verify = %+v", again)
	}
	if call(h, "POST", "/admin/webhooks/nope/verify", "").Code != 404 {
		t.Fatal("verifying an unknown subscription must be 404")
	}
}

func TestWebhookRegisterDeleteAndTheDeliveryLog(t *testing.T) {
	h := newTestHandler(t)
	rcv := &checkingReceiver{key: "k"}
	srv := httptest.NewServer(rcv)
	t.Cleanup(srv.Close)
	w := registered(t, call(h, "POST", "/admin/webhooks", `{"url":"`+srv.URL+`","key":"k"}`))
	h.emit(emitRequest(), store.DefaultTenantID, "INVOICE", "CREATE", "i", nil)
	deadline := time.Now().Add(3 * time.Second)
	for len(h.dispatcher.Deliveries()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	var log []map[string]any
	_ = json.Unmarshal(call(h, "GET", "/admin/webhooks/deliveries", "").Body.Bytes(), &log)
	if len(log) != 1 || log[0]["status_code"] != float64(200) {
		t.Fatalf("delivery log = %v", log)
	}
	if call(h, "DELETE", "/admin/webhooks/"+w.ID, "").Code != 200 || call(h, "DELETE", "/admin/webhooks/"+w.ID, "").Code != 404 {
		t.Fatal("delete must succeed once, then be 404")
	}
}

func TestWebhookRegisterRoutesCoexistWithTheKitsAdminRoutes(t *testing.T) {
	twin := twincore.New(&twincore.Config{Name: "twin-xero-test"})
	s := store.New()
	h := NewHandler(s, twin.Middleware())
	t.Cleanup(h.Close)
	h.Routes(twin.Router)
	admin.NewHandler(s, twin.Middleware(), s.Clock).Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	for _, path := range []string{"/admin/webhooks", "/admin/webhooks/deliveries", "/admin/health", "/admin/state"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("GET %s: %v %v", path, err, resp)
		}
		resp.Body.Close()
	}
}
