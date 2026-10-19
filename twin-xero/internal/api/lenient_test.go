package api

import (
	"net/url"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func TestLenientBearerAcceptsAnyNonEmptyToken(t *testing.T) {
	h := newTestHandler()
	if rec := get(h, "/connections", map[string]string{"Authorization": "Bearer xero-local-test-token"}); rec.Code != 200 {
		t.Fatalf("a made-up token must pass by default, got %d", rec.Code)
	}
	for name, hdr := range map[string]map[string]string{
		"no header":    nil,
		"empty token":  {"Authorization": "Bearer "},
		"not a bearer": {"Authorization": "Basic abc"},
	} {
		if rec := get(h, "/connections", hdr); rec.Code != 401 {
			t.Fatalf("%s: want 401 even when lenient, got %d", name, rec.Code)
		}
	}
	h.store.StrictAuth.Store(true)
	if rec := get(h, "/connections", map[string]string{"Authorization": "Bearer xero-local-test-token"}); rec.Code != 401 {
		t.Fatalf("strict auth must refuse the made-up token, got %d", rec.Code)
	}
}

func TestLenientRefreshTakesAnUnknownTokenAsASignIn(t *testing.T) {
	h := newTestHandler()
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"xero-local-test-refresh"}, "client_id": {"app"}}
	rec := postToken(h, form, "")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := decode(t, rec)
	if body["scope"] != consentScope || claimsOf(t, body["access_token"].(string))["authentication_event_id"] != store.DefaultAuthEventID {
		t.Fatalf("the sign-in must carry the consent scope and the default authentication event: %v", body)
	}

	// Single use still holds for a token the emulator issued, lenient or not.
	next := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {body["refresh_token"].(string)}, "client_id": {"app"}}
	if rec := postToken(h, next, ""); rec.Code != 200 {
		t.Fatalf("redeeming an issued token: %d", rec.Code)
	}
	if rec := postToken(h, next, ""); rec.Code != 400 || decode(t, rec)["error"] != "invalid_grant" {
		t.Fatalf("a redeemed token must be refused even when lenient, got %d %s", rec.Code, rec.Body)
	}

	h.store.StrictAuth.Store(true)
	if rec := postToken(h, form, ""); rec.Code != 400 {
		t.Fatalf("strict auth must refuse an unknown refresh token, got %d", rec.Code)
	}
}

func TestConfigStrictAuthIsRuntimeConfiguration(t *testing.T) {
	s := store.New()
	c := NewConfig(twincore.New(&twincore.Config{Name: "twin-xero-test"}), s)
	if c.GetConfig()["strict_auth"] != false || c.GetConfig()["name"] != "twin-xero-test" {
		t.Fatalf("config = %v", c.GetConfig())
	}
	if err := c.UpdateConfig(map[string]any{"strict_auth": true, "verbose": true}); err != nil || !s.StrictAuth.Load() {
		t.Fatalf("strict_auth with a kit key must apply: %v", err)
	}
	for _, bad := range []map[string]any{{"strict_auth": "yes"}, {"strict_auth": false, "nope": 1}} {
		if err := c.UpdateConfig(bad); err == nil {
			t.Fatalf("%v must be refused", bad)
		}
	}
	if !s.StrictAuth.Load() {
		t.Fatal("a refused update must change nothing")
	}
	s.Reset()
	if !s.StrictAuth.Load() {
		t.Fatal("reset must not touch configuration")
	}
}
