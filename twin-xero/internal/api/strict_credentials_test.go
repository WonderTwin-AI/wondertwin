package api

import (
	"net/url"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// Audit finding F10: under strict_auth the token endpoint checks the caller
// against the clients registered through the admin plane (quirk auth-006).

func TestStrictModeChecksTheClientAndItsSecret(t *testing.T) {
	h := strictWithClient(t)
	cc := url.Values{"grant_type": {"client_credentials"}}
	for name, basic := range map[string]string{"an unknown client": "other:s3cret", "a wrong secret": "app:nope", "an empty secret": "app:"} {
		if rec := postToken(h, cc, basic); rec.Code != 400 || decode(t, rec)["error"] != "invalid_client" {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
	if rec := postToken(h, cc, "app:s3cret"); rec.Code != 200 {
		t.Errorf("the right secret over Basic: %d %s", rec.Code, rec.Body)
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {"app"}, "client_secret": {"s3cret"}}
	if rec := postToken(h, form, ""); rec.Code != 200 {
		t.Errorf("the right secret in the form: %d %s", rec.Code, rec.Body)
	}
}

func TestStrictModeRedeemsAnAuthorizationCodeOnceForItsClientAndRedirect(t *testing.T) {
	h := strictWithClient(t)
	h.store.Clients.Set("rival", store.Client{Secret: "x", RedirectURIs: []string{"https://app.test/cb"}})
	redeem := func(code, redirect, basic string) (int, map[string]any) {
		rec := postToken(h, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect}}, basic)
		return rec.Code, decode(t, rec)
	}

	code := issueCode(t, h, "app", "https://app.test/cb")
	if c, b := redeem(code, "https://elsewhere.test/cb", "app:s3cret"); c != 400 || b["error"] != "invalid_grant" {
		t.Errorf("another redirect URI: %d %v", c, b)
	}
	if c, b := redeem(code, "https://app.test/cb", "rival:x"); c != 400 || b["error"] != "invalid_grant" {
		t.Errorf("another client's code: %d %v", c, b)
	}
	if c, b := redeem("made-up", "https://app.test/cb", "app:s3cret"); c != 400 || b["error"] != "invalid_grant" {
		t.Errorf("a code nobody issued: %d %v", c, b)
	}
	c, b := redeem(code, "https://app.test/cb", "app:s3cret")
	if c != 200 || b["refresh_token"] == nil {
		t.Fatalf("the right code: %d %v", c, b)
	}
	if c, b := redeem(code, "https://app.test/cb", "app:s3cret"); c != 400 || b["error"] != "invalid_grant" {
		t.Errorf("the same code twice: %d %v", c, b)
	}

	refresh := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {b["refresh_token"].(string)}}
	if rec := postToken(h, refresh, "rival:x"); rec.Code != 400 || decode(t, rec)["error"] != "invalid_grant" {
		t.Errorf("another client's refresh token: %d %s", rec.Code, rec.Body)
	}
	if rec := postToken(h, refresh, "app:s3cret"); rec.Code != 200 {
		t.Errorf("its own refresh token: %d %s", rec.Code, rec.Body)
	}
}

func TestByDefaultAnyClientAndAnyCodeStillWork(t *testing.T) {
	h := newTestHandler(t)
	if rec := postToken(h, url.Values{"grant_type": {"client_credentials"}}, "anyone:anything"); rec.Code != 200 {
		t.Errorf("client credentials: %d %s", rec.Code, rec.Body)
	}
	if rec := postToken(h, url.Values{"grant_type": {"authorization_code"}, "code": {"any"}}, "anyone:anything"); rec.Code != 200 {
		t.Errorf("a made-up code: %d %s", rec.Code, rec.Body)
	}
}
