package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func setupSlackStore(t *testing.T) (*store.MemoryStore, *httptest.Server, *testutil.TwinClient) {
	t.Helper()
	mem := store.New()
	twin := twincore.New(&twincore.Config{Name: "twin-slack-test"})
	handler := api.NewHandler(mem, twin.Middleware())
	handler.Routes(twin.Router)
	admin.NewHandler(handler.AdminState(), twin.Middleware(), mem.Clock).Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	return mem, srv, testutil.NewTwinClient(t, srv)
}

func TestWebhookStore(t *testing.T) {
	mem, _, tc := setupSlackStore(t)

	a := mem.MintIncomingWebhook("C001", "A_SIM", "B_BOT")
	b := mem.MintIncomingWebhook("C001", "A_SIM", "B_BOT")
	if a.ID == b.ID || a.Secret == b.Secret || a.Service == b.Service {
		t.Errorf("two webhooks share an identity: %+v %+v", a, b)
	}
	if !regexp.MustCompile(`^B[A-Z0-9]{10}$`).MatchString(a.Service) || !regexp.MustCompile(`^[A-Z0-9]{24}$`).MatchString(a.Secret) {
		t.Errorf("service %q, secret %q", a.Service, a.Secret)
	}
	if a.Team != "T0001" || a.Channel != "C001" || a.AppID != "A_SIM" || a.BotID != "B_BOT" {
		t.Errorf("webhook: %+v", a)
	}

	want := "http://localhost:4100/services/T0001/" + a.Service + "/" + a.Secret
	if got := api.WebhookURL("http://localhost:4100", a); got != want {
		t.Errorf("URL %q, want %q", got, want)
	}

	// State export carries the webhooks, and loading it restores them.
	var state struct {
		IncomingWebhooks map[string]store.IncomingWebhook `json:"incoming_webhooks"`
	}
	raw := tc.Get("/admin/state").AssertStatus(200).Body
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.IncomingWebhooks) != 2 || state.IncomingWebhooks[a.ID].Secret != a.Secret {
		t.Fatalf("state export: %s", raw)
	}
	tc.Post("/admin/reset", nil).AssertStatus(200)
	if n := mem.IncomingWebhooks.Count(); n != 0 {
		t.Errorf("%d webhooks after reset", n)
	}
	tc.Post("/admin/state", json.RawMessage(raw)).AssertStatus(200)
	if got, ok := mem.IncomingWebhooks.Get(a.ID); !ok || got != a {
		t.Errorf("restored webhook: %+v, want %+v", got, a)
	}
	// A restored store does not reissue an ID it holds.
	if c := mem.MintIncomingWebhook("C002", "A_SIM", "B_BOT"); c.ID == a.ID || c.ID == b.ID || !strings.HasPrefix(c.ID, "WH") {
		t.Errorf("minted %q after a restore", c.ID)
	}
}

// post sends body to a webhook URL and returns the status, content type and body.
func post(t *testing.T, url, body string) (int, string, string) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Content-Type"), string(b)
}

func TestWebhookPost(t *testing.T) {
	mem, srv, tc := setupSlackStore(t)
	rc := newReceiver(t)
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL})
	ch := seedChannel(tc, "alerts")
	other := seedChannel(tc, "elsewhere")
	wh := mem.MintIncomingWebhook(ch, store.DefaultAppID, store.DefaultBotID)
	url := api.WebhookURL(srv.URL, wh)

	// The answer is plain text ok, with no JSON envelope.
	status, ctype, body := post(t, url, `{"text":"deploy done","channel":"`+other+`","username":"someone","icon_emoji":":x:","thread_ts":""}`)
	if status != 200 || body != "ok" || !strings.HasPrefix(ctype, "text/plain") {
		t.Fatalf("answer: %d %q %q", status, ctype, body)
	}

	// The message lands in the bound channel, not the one the payload named,
	// and is the webhook app's, with the payload's username and icon ignored.
	_, hist := call(t, srv, "GET", "/api/conversations.history?channel="+ch, "", "", true)
	msgs, _ := hist["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("history of the bound channel: %v", hist)
	}
	m := msgs[0].(map[string]any)
	if m["text"] != "deploy done" || m["subtype"] != "bot_message" || m["bot_id"] != store.DefaultBotID || m["app_id"] != store.DefaultAppID || m["username"] != "bot" || m["icons"] != nil {
		t.Errorf("stored message: %v", m)
	}
	if _, oh := call(t, srv, "GET", "/api/conversations.history?channel="+other, "", "", true); len(oh["messages"].([]any)) != 0 {
		t.Errorf("the payload's channel received the post: %v", oh)
	}

	// Blocks, attachments and thread_ts are kept.
	ts := m["ts"].(string)
	post(t, url, `{"text":"reply","thread_ts":"`+ts+`","blocks":[{"type":"divider"}],"attachments":[{"text":"a"}]}`)
	_, rep := call(t, srv, "GET", "/api/conversations.replies?channel="+ch+"&ts="+ts, "", "", true)
	r := rep["messages"].([]any)[1].(map[string]any)
	if r["thread_ts"] != ts || r["blocks"] == nil || len(r["attachments"].([]any)) != 1 {
		t.Errorf("reply: %v", r)
	}

	// The post is delivered as a message event, and is in the delivery log.
	got := rc.wait(t, 2)
	ev := envelope(t, got[0])["event"].(map[string]any)
	if ev["type"] != "message" || ev["subtype"] != "bot_message" || ev["text"] != "deploy done" || ev["channel"] != ch {
		t.Errorf("event: %v", ev)
	}
	if d := deliveryLog(t, srv, 2); len(d) != 2 {
		t.Errorf("delivery log: %v", d)
	}

	// A URL that names no webhook, and a body that is not JSON, post nothing.
	if s, _, b := post(t, srv.URL+"/services/T0001/"+wh.Service+"/WRONG", `{"text":"x"}`); s != 404 || b != "no_service" {
		t.Errorf("wrong secret: %d %q", s, b)
	}
	if s, _, b := post(t, url, `not json`); s != 400 || b != "invalid_payload" {
		t.Errorf("not JSON: %d %q", s, b)
	}
	// A payload with nothing to post is refused, and so is a post to an archived channel.
	if s, _, b := post(t, url, `{"username":"x"}`); s != 400 || b != "no_text" {
		t.Errorf("no content: %d %q", s, b)
	}
	call(t, srv, "POST", "/api/conversations.archive", formType, "channel="+ch, true)
	if s, _, b := post(t, url, `{"text":"late"}`); s != 410 || b != "channel_is_archived" {
		t.Errorf("archived channel: %d %q", s, b)
	}
	_, rep = call(t, srv, "GET", "/api/conversations.replies?channel="+ch+"&ts="+ts, "", "", true)
	if n := len(rep["messages"].([]any)); n != 2 {
		t.Errorf("%d messages in the thread, want the root and the one reply", n)
	}
}
