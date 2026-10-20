package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

type received struct {
	sig  string
	body []byte
}

// receiver is a webhook receiver that records every delivery and answers the
// given statuses in turn, then 200.
func receiver(t *testing.T, statuses ...int) (*httptest.Server, chan received) {
	t.Helper()
	got := make(chan received, 32)
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- received{r.Header.Get("x-xero-signature"), b}
		mu.Lock()
		status := http.StatusOK
		if len(statuses) > 0 {
			status, statuses = statuses[0], statuses[1:]
		}
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func subscribe(h *Handler, id, url, key string, status string, categories ...string) {
	h.store.Webhooks.Set(id, store.Webhook{ID: id, URL: url, Key: key, Categories: categories, Status: status})
}

func next(t *testing.T, ch chan received) received {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(3 * time.Second):
		t.Fatal("no delivery arrived")
		return received{}
	}
}

func none(t *testing.T, ch chan received) {
	t.Helper()
	select {
	case r := <-ch:
		t.Fatalf("unexpected delivery: %s", r.body)
	case <-time.After(300 * time.Millisecond):
	}
}

func emitRequest() *http.Request {
	r := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:4115/api.xro/2.0/Contacts", nil)
	r.Header.Set("X-Forwarded-Host", "xero.localhost.localstack.cloud:4566")
	r.Header.Set("X-Forwarded-Proto", "https")
	return r
}

func TestWebhookEmitDeliversASignedEnvelopeToASubscription(t *testing.T) {
	h := newTestHandler()
	srv, got := receiver(t)
	subscribe(h, "w1", srv.URL, "the-key", store.WebhookOK, "CONTACT")
	h.store.Clock.Advance(48 * time.Hour)
	h.emit(emitRequest(), store.DefaultTenantID, "CONTACT", "CREATE", "c-1", nil)

	d := next(t, got)
	// Recompute the signature here, from the received bytes, as a receiver would.
	mac := hmac.New(sha256.New, []byte("the-key"))
	mac.Write(d.body)
	if want := base64.StdEncoding.EncodeToString(mac.Sum(nil)); d.sig != want {
		t.Fatalf("x-xero-signature = %q, want %q", d.sig, want)
	}
	var p struct {
		Events             []map[string]any
		FirstEventSequence int
		LastEventSequence  int
		Entropy            string
	}
	if err := json.Unmarshal(d.body, &p); err != nil || len(p.Events) != 1 {
		t.Fatalf("body: %v %s", err, d.body)
	}
	e := p.Events[0]
	if e["resourceUrl"] != "https://xero.localhost.localstack.cloud:4566/api.xro/2.0/Contacts/c-1" ||
		e["eventType"] != "CREATE" || e["eventCategory"] != "CONTACT" || e["tenantId"] != store.DefaultTenantID || e["tenantType"] != "ORGANISATION" {
		t.Fatalf("event = %v", e)
	}
	if ok, _ := regexp.MatchString(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}$`, e["eventDateUtc"].(string)); !ok {
		t.Fatalf("eventDateUtc = %v, want zoneless ISO with milliseconds", e["eventDateUtc"])
	}
	if date, _ := time.Parse("2006-01-02T15:04:05.000", e["eventDateUtc"].(string)); time.Since(date) > -47*time.Hour {
		t.Fatalf("eventDateUtc %v must follow the simulated clock (48h ahead)", e["eventDateUtc"])
	}
	if p.FirstEventSequence != 1 || p.LastEventSequence != 1 {
		t.Fatalf("sequences = %d, %d, want 1, 1", p.FirstEventSequence, p.LastEventSequence)
	}
	if ok, _ := regexp.MatchString(`^[0-9A-F]{20}$`, p.Entropy); !ok {
		t.Fatalf("entropy = %q", p.Entropy)
	}
}

func TestWebhookEmitOnlyReachesSubscribedCategoriesAndPassingSubscriptions(t *testing.T) {
	h := newTestHandler()
	invoices, invGot := receiver(t)
	failed, failedGot := receiver(t)
	subscribe(h, "w1", invoices.URL, "k", store.WebhookOK, "INVOICE")
	subscribe(h, "w2", failed.URL, "k", store.WebhookFailed, "INVOICE", "CONTACT")

	h.emit(emitRequest(), store.DefaultTenantID, "CONTACT", "UPDATE", "c-1", nil)
	none(t, invGot)
	h.emit(emitRequest(), store.DefaultTenantID, "INVOICE", "CREATE", "i-1", nil)
	next(t, invGot)
	none(t, failedGot)
}

func TestWebhookEmitKeepsEventsInOrder(t *testing.T) {
	h := newTestHandler()
	srv, got := receiver(t)
	subscribe(h, "w1", srv.URL, "k", store.WebhookOK, "CONTACT")
	for i := 0; i < 6; i++ {
		h.emit(emitRequest(), store.DefaultTenantID, "CONTACT", "UPDATE", "c", nil)
	}
	for want := 1; want <= 6; want++ {
		var p struct{ FirstEventSequence int }
		if err := json.Unmarshal(next(t, got).body, &p); err != nil || p.FirstEventSequence != want {
			t.Fatalf("delivery %d carried sequence %d (%v)", want, p.FirstEventSequence, err)
		}
	}
}

func TestWebhookEmitRetriesAFailedDeliveryThenStops(t *testing.T) {
	h := newTestHandler()
	srv, got := receiver(t, http.StatusInternalServerError)
	subscribe(h, "w1", srv.URL, "k", store.WebhookOK, "CONTACT")
	h.emit(emitRequest(), store.DefaultTenantID, "CONTACT", "CREATE", "c", nil)

	first, second := next(t, got), next(t, got)
	if string(first.body) != string(second.body) || first.sig != second.sig {
		t.Fatal("a retry must resend the same signed body")
	}
	time.Sleep(100 * time.Millisecond)
	if n := len(h.dispatcher.Deliveries()); n != 2 {
		t.Fatalf("the kit retried %d times, want 1 failure and 1 success", n)
	}
}

func TestWebhookEmitStateIsClearedByReset(t *testing.T) {
	h := newTestHandler()
	srv, got := receiver(t)
	subscribe(h, "w1", srv.URL, "k", store.WebhookOK, "CONTACT")
	h.emit(emitRequest(), store.DefaultTenantID, "CONTACT", "CREATE", "c", nil)
	next(t, got)
	time.Sleep(100 * time.Millisecond)

	h.store.Reset()
	if len(h.dispatcher.Deliveries()) != 0 || h.store.WebhookSeq.Load() != 0 || h.store.Webhooks.Count() != 0 {
		t.Fatal("reset must clear the delivery log, the sequence and the subscriptions")
	}
}
