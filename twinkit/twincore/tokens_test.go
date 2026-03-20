package twincore

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTokenRegistry_OpenMode(t *testing.T) {
	tr := NewTokenRegistry()

	// No tokens registered = open mode, any token accepted.
	cfg := tr.Validate("any-random-token")
	if cfg == nil {
		t.Error("open mode should accept any token")
	}
}

func TestTokenRegistry_RegisteredMode(t *testing.T) {
	tr := NewTokenRegistry()
	tr.Register(TokenConfig{Token: "valid-token", Scopes: []string{"read"}})

	// Valid token.
	cfg := tr.Validate("valid-token")
	if cfg == nil {
		t.Error("registered token should be valid")
	}

	// Invalid token.
	cfg = tr.Validate("wrong-token")
	if cfg != nil {
		t.Error("unregistered token should be rejected")
	}
}

func TestTokenRegistry_Expiry(t *testing.T) {
	tr := NewTokenRegistry()
	tr.Register(TokenConfig{
		Token:     "expiring-token",
		ExpiresAt: time.Now().Add(-time.Hour), // already expired
	})

	cfg := tr.Validate("expiring-token")
	if cfg != nil {
		t.Error("expired token should be rejected")
	}
}

func TestTokenRegistry_Scopes(t *testing.T) {
	tr := NewTokenRegistry()
	tr.Register(TokenConfig{Token: "scoped", Scopes: []string{"read:issues", "write:issues"}})

	if !tr.HasScope("scoped", "read:issues") {
		t.Error("should have read:issues scope")
	}
	if tr.HasScope("scoped", "admin") {
		t.Error("should not have admin scope")
	}
}

func TestTokenRegistry_ScopesOpenWhenEmpty(t *testing.T) {
	tr := NewTokenRegistry()
	tr.Register(TokenConfig{Token: "no-scopes"}) // no scopes = full access

	if !tr.HasScope("no-scopes", "anything") {
		t.Error("no scopes should mean full access")
	}
}

func TestTokenRegistry_Revoke(t *testing.T) {
	tr := NewTokenRegistry()
	tr.Register(TokenConfig{Token: "keep"})
	tr.Register(TokenConfig{Token: "temp"})

	if !tr.Revoke("temp") {
		t.Error("should return true for existing token")
	}
	// "keep" is still registered, so we're not in open mode.
	if tr.Validate("temp") != nil {
		t.Error("revoked token should be invalid")
	}
	if tr.Validate("keep") == nil {
		t.Error("non-revoked token should still be valid")
	}
}

func TestTokenRegistry_RefreshToken(t *testing.T) {
	tr := NewTokenRegistry()
	tr.Register(TokenConfig{
		Token:        "access-1",
		RefreshToken: "refresh-1",
	})

	found := tr.FindByRefreshToken("refresh-1")
	if found == nil {
		t.Error("should find token by refresh token")
	}
	if found.Token != "access-1" {
		t.Errorf("token = %q, want access-1", found.Token)
	}
}

func TestTokenAuth_Middleware(t *testing.T) {
	tr := NewTokenRegistry()
	tr.Register(TokenConfig{Token: "valid"})

	handler := tr.TokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Valid token.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer valid")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("valid token: status = %d, want 200", w.Code)
	}

	// Invalid token.
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid token: status = %d, want 401", w.Code)
	}

	// No auth header.
	req = httptest.NewRequest("GET", "/", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no auth: status = %d, want 401", w.Code)
	}

	// X-Api-Key header.
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Api-Key", "valid")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("X-Api-Key: status = %d, want 200", w.Code)
	}
}
