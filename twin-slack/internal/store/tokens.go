package store

import "strings"

// The identities of a well-formed token the emulator has not been told about.
const (
	DefaultBotUserID = "U_BOT"
	DefaultBotID     = "B_BOT"
	DefaultUserID    = "U_USER"
)

// tokenPrefixes maps the documented token prefixes to their type.
var tokenPrefixes = map[string]string{
	"xoxb-": TokenBot,
	"xoxp-": TokenUser,
	"xapp-": TokenApp,
}

// TokenStatus is the outcome of looking a token up.
type TokenStatus int

const (
	// TokenOK means the token can be used.
	TokenOK TokenStatus = iota
	// TokenMalformed means it is neither known nor shaped like a Slack token.
	TokenMalformed
	// TokenRevoked means it was revoked by auth.revoke.
	TokenRevoked
)

// ResolveToken finds who a token speaks for. A token that was seeded or minted
// by the emulator is looked up, and a revoked one is reported as such. Any
// other token that carries a documented prefix and something after it is
// accepted as a default principal of its type, so a test can use any
// well-formed token without seeding it first. Anything else is malformed.
func (s *MemoryStore) ResolveToken(token string) (Token, TokenStatus) {
	if t, ok := s.Tokens.Get(token); ok {
		if t.Revoked {
			return t, TokenRevoked
		}
		if t.TeamID == "" {
			t.TeamID = s.Team.ID
		}
		return t, TokenOK
	}
	for prefix, typ := range tokenPrefixes {
		if strings.HasPrefix(token, prefix) && len(token) > len(prefix) {
			return s.defaultToken(token, typ), TokenOK
		}
	}
	return Token{}, TokenMalformed
}

func (s *MemoryStore) defaultToken(token, typ string) Token {
	t := Token{Token: token, Type: typ, TeamID: s.Team.ID}
	switch typ {
	case TokenBot:
		t.UserID, t.BotID = DefaultBotUserID, DefaultBotID
	case TokenUser:
		t.UserID = DefaultUserID
	}
	return t
}

// RevokeToken marks a token revoked, recording it first when the emulator had
// only been treating it as a default principal. It reports false when the
// token is not one the emulator accepts.
func (s *MemoryStore) RevokeToken(token string) bool {
	t, status := s.ResolveToken(token)
	if status == TokenMalformed {
		return false
	}
	t.Revoked = true
	s.Tokens.Set(token, t)
	return true
}

// UserName returns the display name of a user for auth.test: the stored user's
// name when there is one, otherwise a stand-in for the token's type.
func (s *MemoryStore) UserName(t Token) string {
	if u, ok := s.Users.Get(t.UserID); ok && u.Name != "" {
		return u.Name
	}
	if t.Type == TokenBot {
		return "bot"
	}
	return "user"
}
