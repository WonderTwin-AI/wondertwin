package twincore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func setupOAuthServer() (*OAuthServer, *TokenRegistry, *http.ServeMux) {
	tokens := NewTokenRegistry()
	oauth := NewOAuthServer(OAuthConfig{
		AuthorizePath: "/oauth/authorize",
		TokenPath:     "/oauth/token",
		ClientID:      "test_client",
		ClientSecret:  "test_secret",
		TokenExpiry:   time.Hour,
	}, tokens)

	mux := http.NewServeMux()
	oauth.Routes(mux)
	return oauth, tokens, mux
}

func TestOAuth_AuthorizeRedirects(t *testing.T) {
	_, _, mux := setupOAuthServer()

	req := httptest.NewRequest("GET", "/oauth/authorize?client_id=test_client&redirect_uri=http://localhost/callback&scope=read+write&state=xyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}

	location := w.Header().Get("Location")
	if !strings.HasPrefix(location, "http://localhost/callback?code=") {
		t.Errorf("location = %q, want redirect with code", location)
	}
	if !strings.Contains(location, "state=xyz") {
		t.Error("state parameter should be preserved")
	}
}

func TestOAuth_AuthorizeRequiresFields(t *testing.T) {
	_, _, mux := setupOAuthServer()

	// Missing client_id.
	req := httptest.NewRequest("GET", "/oauth/authorize?redirect_uri=http://localhost/callback", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing client_id: status = %d, want 400", w.Code)
	}
}

func TestOAuth_TokenExchange(t *testing.T) {
	_, tokens, mux := setupOAuthServer()

	// Step 1: Authorize.
	req := httptest.NewRequest("GET", "/oauth/authorize?client_id=test_client&redirect_uri=http://localhost/callback&scope=read+write", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Extract code from redirect URL.
	location, _ := url.Parse(w.Header().Get("Location"))
	code := location.Query().Get("code")
	if code == "" {
		t.Fatal("no code in redirect")
	}

	// Step 2: Exchange code for token.
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {"test_client"},
		"client_secret": {"test_secret"},
	}
	req = httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("token exchange: status = %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	accessToken, ok := resp["access_token"].(string)
	if !ok || accessToken == "" {
		t.Error("missing access_token")
	}
	if resp["token_type"] != "Bearer" {
		t.Errorf("token_type = %v, want Bearer", resp["token_type"])
	}
	if resp["refresh_token"] == nil {
		t.Error("missing refresh_token")
	}

	// Token should be registered.
	if tokens.Validate(accessToken) == nil {
		t.Error("issued token should be valid in registry")
	}
}

func TestOAuth_CodeSingleUse(t *testing.T) {
	_, _, mux := setupOAuthServer()

	// Authorize.
	req := httptest.NewRequest("GET", "/oauth/authorize?client_id=test_client&redirect_uri=http://localhost/callback", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	location, _ := url.Parse(w.Header().Get("Location"))
	code := location.Query().Get("code")

	// First exchange succeeds.
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {"test_client"}, "client_secret": {"test_secret"}}
	req = httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first exchange: %d", w.Code)
	}

	// Second exchange with same code fails.
	req = httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("reuse code: status = %d, want 400", w.Code)
	}
}

func TestOAuth_RefreshToken(t *testing.T) {
	_, tokens, mux := setupOAuthServer()

	// Get initial tokens via authorize + exchange.
	req := httptest.NewRequest("GET", "/oauth/authorize?client_id=test_client&redirect_uri=http://localhost/callback", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	location, _ := url.Parse(w.Header().Get("Location"))
	code := location.Query().Get("code")

	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {"test_client"}, "client_secret": {"test_secret"}}
	req = httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	refreshToken := resp["refresh_token"].(string)
	oldAccessToken := resp["access_token"].(string)

	// Refresh.
	form = url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}}
	req = httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("refresh: status = %d: %s", w.Code, w.Body.String())
	}

	json.Unmarshal(w.Body.Bytes(), &resp)
	newAccessToken := resp["access_token"].(string)

	// Old token should be revoked.
	if tokens.Validate(oldAccessToken) != nil {
		t.Error("old access token should be revoked after refresh")
	}

	// New token should be valid.
	if tokens.Validate(newAccessToken) == nil {
		t.Error("new access token should be valid")
	}
}

func TestOAuth_InvalidClientSecret(t *testing.T) {
	_, _, mux := setupOAuthServer()

	req := httptest.NewRequest("GET", "/oauth/authorize?client_id=test_client&redirect_uri=http://localhost/callback", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	location, _ := url.Parse(w.Header().Get("Location"))
	code := location.Query().Get("code")

	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {"test_client"}, "client_secret": {"wrong_secret"}}
	req = httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("bad secret: status = %d, want 400", w.Code)
	}
}
