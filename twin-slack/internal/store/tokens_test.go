package store_test

import (
	"encoding/json"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

func TestTokensSurviveASnapshotRoundTrip(t *testing.T) {
	s := store.New()
	s.Tokens.Set("custom", store.Token{Token: "custom", Type: store.TokenUser, UserID: "U1"})
	s.RevokeToken("xoxb-gone")

	data, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	restored := store.New()
	if err := restored.LoadState(data); err != nil {
		t.Fatal(err)
	}

	if tok, status := restored.ResolveToken("custom"); status != store.TokenOK || tok.UserID != "U1" {
		t.Errorf("a seeded token should survive a snapshot: %v %v", tok, status)
	}
	if _, status := restored.ResolveToken("xoxb-gone"); status != store.TokenRevoked {
		t.Errorf("a revoked token should stay revoked after a snapshot, got status %v", status)
	}
}

func TestResetForgetsTokens(t *testing.T) {
	s := store.New()
	s.Tokens.Set("custom", store.Token{Token: "custom", Type: store.TokenUser})
	s.RevokeToken("xoxb-gone")
	s.Reset()

	if _, status := s.ResolveToken("custom"); status != store.TokenMalformed {
		t.Errorf("a seeded token should be gone after a reset, got status %v", status)
	}
	if _, status := s.ResolveToken("xoxb-gone"); status != store.TokenOK {
		t.Errorf("a revoked token should be usable again after a reset, got status %v", status)
	}
}

func TestResolveTokenDefaults(t *testing.T) {
	s := store.New()
	for _, c := range []struct {
		token  string
		status store.TokenStatus
		user   string
		bot    string
	}{
		{"xoxb-a", store.TokenOK, store.DefaultBotUserID, store.DefaultBotID},
		{"xoxp-a", store.TokenOK, store.DefaultUserID, ""},
		{"xapp-1-a", store.TokenOK, "", ""},
		{"xoxb-", store.TokenMalformed, "", ""},
		{"nonsense", store.TokenMalformed, "", ""},
		{"", store.TokenMalformed, "", ""},
	} {
		tok, status := s.ResolveToken(c.token)
		if status != c.status || (status == store.TokenOK && (tok.UserID != c.user || tok.BotID != c.bot)) {
			t.Errorf("ResolveToken(%q) = %+v, %v", c.token, tok, status)
		}
	}
}

func TestRevokeRefusesAMalformedToken(t *testing.T) {
	s := store.New()
	if s.RevokeToken("nonsense") {
		t.Error("a token the emulator does not accept cannot be revoked")
	}
	if s.Tokens.Count() != 0 {
		t.Error("a refused revoke must not record anything")
	}
}
