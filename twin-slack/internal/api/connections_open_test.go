package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// connectionsOpen calls apps.connections.open the way the SDKs do: a form
// POST, usually with an empty body.
func connectionsOpen(t *testing.T, base string, header http.Header, body string) map[string]any {
	t.Helper()
	req, err := http.NewRequest("POST", base+"/api/apps.connections.open", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header = header
	req.Header.Set("Content-Type", formType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("apps.connections.open answer is not JSON: %v", err)
	}
	return m
}

func TestAppsConnectionsOpen(t *testing.T) {
	appToken := http.Header{"Authorization": {"Bearer xapp-1-A_SIM-1-secret"}}

	t.Run("an app token gets a fresh URL that connects", func(t *testing.T) {
		srv, _ := setupSocketSlack(t)
		first := connectionsOpen(t, srv.URL, appToken.Clone(), "")
		second := connectionsOpen(t, srv.URL, appToken.Clone(), "")
		if first["ok"] != true {
			t.Fatalf("apps.connections.open: %v", first)
		}
		raw, _ := first["url"].(string)
		if raw == second["url"] {
			t.Errorf("two calls gave the same URL %s", raw)
		}
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("url %q: %v", raw, err)
		}
		if want := strings.TrimPrefix(srv.URL, "http://"); u.Scheme != "ws" || u.Host != want || u.Path != "/link/" {
			t.Errorf("url = %s, want ws://%s/link/", raw, want)
		}
		if !strings.HasPrefix(u.RawQuery, "ticket=") || u.Query().Get("app_id") != store.DefaultAppID {
			t.Errorf("url query = %q, want a ticket then app_id=%s", u.RawQuery, store.DefaultAppID)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, _, err := dialSocket(t, ctx, raw)
		if err != nil {
			t.Fatalf("dial %s: %v", raw, err)
		}
		readHello(t, ctx, conn)
	})

	t.Run("the URL is on the host the app called", func(t *testing.T) {
		srv, _ := setupSocketSlack(t)
		header := appToken.Clone()
		header.Set("X-Forwarded-Proto", "https")
		header.Set("X-Forwarded-Host", "slack.localhost.localstack.cloud:4566")
		m := connectionsOpen(t, srv.URL, header, "")
		if raw, _ := m["url"].(string); !strings.HasPrefix(raw, "wss://slack.localhost.localstack.cloud:4566/link/?ticket=") {
			t.Errorf("url = %q, want wss on the forwarded host", raw)
		}
	})

	t.Run("only an app token in the header is accepted", func(t *testing.T) {
		srv, _ := setupSocketSlack(t)
		for name, tc := range map[string]struct {
			header http.Header
			body   string
			want   string
		}{
			"bot token":        {http.Header{"Authorization": {"Bearer xoxb-bot"}}, "", "not_allowed_token_type"},
			"user token":       {http.Header{"Authorization": {"Bearer xoxp-user"}}, "", "not_allowed_token_type"},
			"no token":         {http.Header{}, "", "not_authed"},
			"token in body":    {http.Header{}, "token=xapp-1-A_SIM-1-secret", "not_authed"},
			"malformed header": {http.Header{"Authorization": {"Basic abc"}}, "", "invalid_auth"},
		} {
			m := connectionsOpen(t, srv.URL, tc.header, tc.body)
			if m["ok"] != false || m["error"] != tc.want {
				t.Errorf("%s: got %v, want %s", name, m, tc.want)
			}
		}
	})
}
