package api

import (
	"net/http"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// installScopes are the bot scopes an install grants. The scopes an app asks
// for are chosen in the browser step at slack.com/oauth/v2/authorize, which
// is not an API call, so the emulator grants a fixed set.
var installScopes = []string{"chat:write", "channels:read", "users:read"}

// OAuthV2Access handles /api/oauth.v2.access, the last step of an install:
// an app exchanges the authorization code from the redirect for a bot token.
// The emulator serves no authorize page, so any code is accepted, once. The
// client credentials come from HTTP Basic, which the docs prefer, or from
// client_id and client_secret arguments. Token rotation is not served.
func (h *Handler) OAuthV2Access(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Code         string `json:"code"`
		GrantType    string `json:"grant_type"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if id, secret, ok := r.BasicAuth(); ok {
		req.ClientID, req.ClientSecret = id, secret
	}
	switch {
	case req.ClientID == "":
		slackError(w, "invalid_client_id")
		return
	case req.ClientSecret == "":
		slackError(w, "bad_client_secret")
		return
	case req.GrantType == "refresh_token":
		// No token the emulator issues expires, so none has a refresh token.
		slackError(w, "invalid_refresh_token")
		return
	case req.GrantType != "" && req.GrantType != "authorization_code":
		slackError(w, "invalid_grant_type")
		return
	case req.Code == "" || h.codeUsed(req.Code):
		slackError(w, "invalid_code")
		return
	}

	team := h.store.Team.ID
	token := h.newBotToken(team)
	h.store.Tokens.Set(token, store.Token{
		Token:     token,
		Type:      store.TokenBot,
		UserID:    store.DefaultBotUserID,
		BotID:     store.DefaultBotID,
		TeamID:    team,
		Scopes:    installScopes,
		OAuthCode: req.Code,
	})
	slackOK(w, map[string]any{
		"access_token":          token,
		"token_type":            "bot",
		"scope":                 strings.Join(installScopes, ","),
		"bot_user_id":           store.DefaultBotUserID,
		"app_id":                appID,
		"team":                  map[string]any{"name": h.store.Team.Name, "id": team},
		"enterprise":            nil,
		"authed_user":           map[string]any{"id": store.DefaultUserID},
		"is_enterprise_install": false,
	})
}

// codeUsed reports whether an authorization code was already exchanged.
func (h *Handler) codeUsed(code string) bool {
	return len(h.store.Tokens.Filter(func(_ string, t store.Token) bool { return t.OAuthCode == code })) > 0
}

// newBotToken mints a bot token no other token uses. The store's counter can
// lag the tokens a state load brought in, so a taken value is skipped rather
// than overwritten.
func (h *Handler) newBotToken(team string) string {
	for {
		token := "xoxb-" + team + "-" + strings.TrimPrefix(h.store.Tokens.NextID(), "tok_")
		if _, taken := h.store.Tokens.Get(token); !taken {
			return token
		}
	}
}
