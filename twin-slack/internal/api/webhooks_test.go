package api_test

import (
	"encoding/json"
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

func setupSlackStore(t *testing.T) (*store.MemoryStore, *testutil.TwinClient) {
	t.Helper()
	mem := store.New()
	twin := twincore.New(&twincore.Config{Name: "twin-slack-test"})
	handler := api.NewHandler(mem, twin.Middleware())
	handler.Routes(twin.Router)
	admin.NewHandler(handler.AdminState(), twin.Middleware(), mem.Clock).Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	return mem, testutil.NewTwinClient(t, srv)
}

func TestWebhookStore(t *testing.T) {
	mem, tc := setupSlackStore(t)

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
