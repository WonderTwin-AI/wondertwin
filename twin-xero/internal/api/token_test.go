package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// postToken calls the token endpoint with a form, and HTTP Basic credentials
// when basic is non-empty.
func postToken(h *Handler, form url.Values, basic string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/connect/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basic != "" {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(basic)))
	}
	rec := httptest.NewRecorder()
	h.Token(rec, req)
	return rec
}

func claimsOf(t *testing.T, jwt string) map[string]any {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("%q is not a compact JWT", jwt)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTokenClientCredentialsIssuesAJWT(t *testing.T) {
	h := newTestHandler(t)
	rec := postToken(h, url.Values{"grant_type": {"client_credentials"}}, "app:secret")

	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("status %d, content type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	body := decode(t, rec)
	if body["token_type"] != "Bearer" || body["expires_in"] != float64(1800) || body["scope"] != defaultScope {
		t.Fatalf("body = %v", body)
	}
	for _, k := range []string{"refresh_token", "id_token"} {
		if _, ok := body[k]; ok {
			t.Fatalf("client credentials must not return %s", k)
		}
	}
	access := body["access_token"].(string)
	c := claimsOf(t, access)
	if c["client_id"] != "app" || c["iss"] != issuer || c["exp"].(float64)-c["iat"].(float64) != 1800 {
		t.Fatalf("claims = %v", c)
	}
	if scopes, _ := c["scope"].([]any); len(scopes) != 3 {
		t.Fatalf("scope claim = %v, want the three default scopes", c["scope"])
	}
	if _, ok := h.store.AccessTokens.Get(access); !ok {
		t.Fatal("the issued token is not recorded")
	}
}

func TestTokenAcceptsCredentialsInTheFormBody(t *testing.T) {
	// xero-python sends client_id and client_secret in the body, not as Basic.
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {"sdk"}, "client_secret": {"s"}, "scope": {"accounting.contacts"}}
	rec := postToken(newTestHandler(t), form, "")

	body := decode(t, rec)
	if rec.Code != 200 || body["scope"] != "accounting.contacts" {
		t.Fatalf("status %d, body %v", rec.Code, body)
	}
	if got := claimsOf(t, body["access_token"].(string))["client_id"]; got != "sdk" {
		t.Fatalf("client_id claim = %v", got)
	}
}

func TestTokenErrorsUseTheIdentityShape(t *testing.T) {
	cases := []struct {
		name  string
		form  url.Values
		basic string
		auth  string
		want  string
	}{
		{"no credentials", url.Values{"grant_type": {"client_credentials"}}, "", "", "invalid_client"},
		{"empty Basic user", url.Values{"grant_type": {"client_credentials"}}, ":secret", "", "invalid_client"},
		{"not a Basic header", url.Values{"grant_type": {"client_credentials"}, "client_id": {"x"}}, "", "Bearer abc", "invalid_client"},
		{"grant not served", url.Values{"grant_type": {"device_code"}}, "a:b", "", "unsupported_grant_type"},
		{"code grant without a code", url.Values{"grant_type": {"authorization_code"}}, "a:b", "", "invalid_request"},
		{"refresh grant with an unknown token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"nope"}}, "a:b", "", "invalid_grant"},
		{"no grant", url.Values{}, "a:b", "", "unsupported_grant_type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t)
			h.store.StrictAuth.Store(true)
			h.store.Clients.Set("a", store.Client{Secret: "b"}) // the cases that name a grant reach its check
			req := httptest.NewRequest(http.MethodPost, "/connect/token", strings.NewReader(tc.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.basic != "" {
				req.SetBasicAuth(strings.SplitN(tc.basic, ":", 2)[0], strings.SplitN(tc.basic, ":", 2)[1])
			}
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rec := httptest.NewRecorder()
			h.Token(rec, req)

			if body := decode(t, rec); rec.Code != 400 || len(body) != 1 || body["error"] != tc.want {
				t.Fatalf("status %d, body %v, want 400 {error: %s}", rec.Code, body, tc.want)
			}
		})
	}
}

func TestTokenExpiryFollowsTheSimulatedClock(t *testing.T) {
	h := newTestHandler(t)
	h.store.Clock.Advance(24 * time.Hour)
	body := decode(t, postToken(h, url.Values{"grant_type": {"client_credentials"}}, "a:b"))

	exp := time.Unix(int64(claimsOf(t, body["access_token"].(string))["exp"].(float64)), 0)
	if d := time.Until(exp); d < 24*time.Hour || d > 25*time.Hour {
		t.Fatalf("exp is %s from now, want about 24h30m", d)
	}
}

func TestJwtIsSignedHS256(t *testing.T) {
	parts := strings.Split(signJWT(map[string]any{"a": 1}), ".")
	hdr, _ := base64.RawURLEncoding.DecodeString(parts[0])
	if !strings.Contains(string(hdr), `"alg":"HS256"`) {
		t.Fatalf("header = %s", hdr)
	}
	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if parts[2] != base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) {
		t.Fatal("signature does not verify")
	}
}

func exchangeCode(t *testing.T, h *Handler, extra url.Values) map[string]any {
	t.Helper()
	form := url.Values{"grant_type": {"authorization_code"}, "code": {"any-code"}}
	for k, v := range extra {
		form[k] = v
	}
	rec := postToken(h, form, "app:secret")
	if rec.Code != 200 {
		t.Fatalf("code exchange status %d: %s", rec.Code, rec.Body)
	}
	return decode(t, rec)
}

func TestTokenCodeExchangeIssuesRefreshAndIDTokens(t *testing.T) {
	h := newTestHandler(t)
	body := exchangeCode(t, h, nil)

	if body["scope"] != consentScope || body["expires_in"] != float64(1800) {
		t.Fatalf("body = %v", body)
	}
	rt, _ := body["refresh_token"].(string)
	if len(rt) != 32 {
		t.Fatalf("refresh_token = %q, want 32 hex characters", rt)
	}
	if _, ok := h.store.RefreshTokens.Get(rt); !ok {
		t.Fatal("the refresh token is not recorded")
	}
	if id := claimsOf(t, body["id_token"].(string)); id["aud"] != "app" || id["iss"] != issuer {
		t.Fatalf("id_token claims = %v", id)
	}
}

func TestTokenScopeDecidesWhichTokensComeBack(t *testing.T) {
	body := exchangeCode(t, newTestHandler(t), url.Values{"scope": {"accounting.contacts"}})

	for _, k := range []string{"refresh_token", "id_token"} {
		if _, ok := body[k]; ok {
			t.Fatalf("without offline_access or openid there must be no %s", k)
		}
	}
}

func TestTokenRefreshRotatesAndKeepsTheAuthenticationEvent(t *testing.T) {
	h := newTestHandler(t)
	first := exchangeCode(t, h, nil)
	old := first["refresh_token"].(string)

	// xero-python sends client credentials in the body, not as Basic.
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {old}, "client_id": {"sdk"}, "client_secret": {"s"}}
	rec := postToken(h, form, "")
	if rec.Code != 200 {
		t.Fatalf("refresh status %d: %s", rec.Code, rec.Body)
	}
	next := decode(t, rec)
	if next["refresh_token"] == old || next["scope"] != first["scope"] {
		t.Fatalf("refresh must rotate the token and keep the scope: %v", next)
	}
	was, now := claimsOf(t, first["access_token"].(string)), claimsOf(t, next["access_token"].(string))
	if was["authentication_event_id"] != now["authentication_event_id"] || now["client_id"] != "app" {
		t.Fatalf("authentication_event_id or client changed across a refresh: %v then %v", was, now)
	}

	replay := postToken(h, form, "")
	if body := decode(t, replay); replay.Code != 400 || body["error"] != "invalid_grant" {
		t.Fatalf("a redeemed refresh token must be refused: %d %v", replay.Code, body)
	}
}

func TestTokenRefreshRedemptionIsSingleUseUnderConcurrency(t *testing.T) {
	h := newTestHandler(t)
	old := exchangeCode(t, h, nil)["refresh_token"].(string)
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {old}, "client_id": {"sdk"}}

	const callers = 32
	var wg sync.WaitGroup
	var mu sync.Mutex
	var ok, refused int
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := postToken(h, form, "")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case rec.Code == 200:
				ok++
			case rec.Code == 400 && strings.Contains(rec.Body.String(), "invalid_grant"):
				refused++
			}
		}()
	}
	wg.Wait()
	if ok != 1 || refused != callers-1 {
		t.Fatalf("got %d successes and %d invalid_grant of %d, want 1 and %d", ok, refused, callers, callers-1)
	}
}
