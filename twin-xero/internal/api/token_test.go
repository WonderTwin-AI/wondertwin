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
	"testing"
	"time"
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
	h := newTestHandler()
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
	rec := postToken(newTestHandler(), form, "")

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
		{"code grant not yet served", url.Values{"grant_type": {"authorization_code"}}, "a:b", "", "unsupported_grant_type"},
		{"no grant", url.Values{}, "a:b", "", "unsupported_grant_type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler()
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
	h := newTestHandler()
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
