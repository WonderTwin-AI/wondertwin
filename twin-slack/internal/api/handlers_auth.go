package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

type principalKey struct{}

// principal returns who the call's token speaks for. It is set by
// authMiddleware, so it is present in every handler behind it.
func principal(r *http.Request) store.Token {
	t, _ := r.Context().Value(principalKey{}).(store.Token)
	return t
}

// requestToken finds the credential on a call, in the places Slack reads it: the
// Authorization header, else a token field in a form, multipart or text/plain
// POST body. A token in the query string is not accepted, and neither is one
// inside a JSON body, which must use the header.
func requestToken(r *http.Request) (token string, present bool, malformed bool) {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if tok, ok := strings.CutPrefix(auth, "Bearer "); ok && tok != "" {
			return tok, true, false
		}
		return "", true, true
	}
	if a, ok := r.Context().Value(argsKey{}).(*slackArgs); ok {
		if tok := a.body.Get("token"); tok != "" {
			return tok, true, false
		}
	}
	return "", false, false
}

// authMiddleware identifies the caller. A call with no token is not_authed, a
// token that is not shaped like a Slack token is invalid_auth, and one that
// was revoked is token_revoked.
func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, present, malformed := requestToken(r)
		switch {
		case !present:
			slackError(w, "not_authed")
			return
		case malformed:
			slackError(w, "invalid_auth")
			return
		}
		t, status := h.store.ResolveToken(token)
		switch status {
		case store.TokenMalformed:
			slackError(w, "invalid_auth")
			return
		case store.TokenRevoked:
			slackError(w, "token_revoked")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, t)))
	})
}

// AuthTest handles /api/auth.test. It answers for the calling token: a bot
// token also carries its bot_id, a user token does not.
func (h *Handler) AuthTest(w http.ResponseWriter, r *http.Request) {
	t := principal(r)
	fields := map[string]any{
		"url":     "https://" + h.store.Team.Domain + ".slack.com/",
		"team":    h.store.Team.Name,
		"team_id": t.TeamID,
	}
	if t.UserID != "" {
		fields["user"] = h.store.UserName(t)
		fields["user_id"] = t.UserID
	}
	if t.BotID != "" {
		fields["bot_id"] = t.BotID
	}
	slackOK(w, fields)
}

// AuthRevoke handles /api/auth.revoke. With test set, nothing is revoked and
// revoked is false.
func (h *Handler) AuthRevoke(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Test bool `json:"test"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if req.Test {
		slackOK(w, map[string]any{"revoked": false})
		return
	}
	slackOK(w, map[string]any{"revoked": h.store.RevokeToken(principal(r).Token)})
}
