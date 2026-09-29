package store

import "strings"

// DefaultLogin is the user a well-formed token authenticates as when the
// token has not been registered to a specific user.
const DefaultLogin = "twin-bot"

// Token maps a credential to the principal it authenticates. Seed tokens
// through the "tokens" key of the state snapshot to give a test several
// principals (an author and a reviewer, say).
type Token struct {
	Token string `json:"token"`
	Login string `json:"login"`
	// Kind is "user" or "installation".
	Kind           string `json:"kind"`
	InstallationID int64  `json:"installation_id,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
}

// userTokenPrefixes are the prefixes GitHub documents for tokens that act as
// a user: classic and fine-grained personal access tokens, OAuth app tokens,
// user-to-server tokens and refresh tokens.
var userTokenPrefixes = []string{"ghp_", "github_pat_", "gho_", "ghu_", "ghr_"}

// LooksLikeUserToken reports whether tok has the shape of a GitHub user token.
// GitHub rejects any other string with 401 Bad credentials.
func LooksLikeUserToken(tok string) bool {
	for _, p := range userTokenPrefixes {
		if strings.HasPrefix(tok, p) && len(tok) > len(p) {
			return true
		}
	}
	return false
}

// LooksLikeJWT reports whether tok has the three-segment shape of a JSON web
// token, which is how a GitHub App authenticates as itself.
func LooksLikeJWT(tok string) bool {
	return strings.HasPrefix(tok, "eyJ") && strings.Count(tok, ".") == 2
}

// seedDefaults puts the state every fresh emulator starts with: the default
// user and a GitHub App installed on its account. It runs from New, after
// Reset and after a state load.
func (s *MemoryStore) seedDefaults() {
	if _, ok := s.Users.Get(DefaultLogin); !ok {
		s.Users.Set(DefaultLogin, User{
			ID:    s.NewID(KindUser),
			Login: DefaultLogin,
			Type:  "User",
			Name:  "Twin Bot",
			Email: "bot@wondertwin.dev",
		})
	}
	s.seedApp()
	s.ensureBots()
}
