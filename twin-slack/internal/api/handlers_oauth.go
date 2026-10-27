package api

import (
	"net/http"
	"slices"
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
		// IncomingWebhookChannel is an emulator extension, not a Slack argument.
		// Slack's install asks for the incoming-webhook scope and has the user
		// pick a channel in the browser step, which the emulator does not
		// serve; naming a channel here stands in for both.
		IncomingWebhookChannel string `json:"incoming_webhook_channel"`
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

	var webhookChannel store.Channel
	if req.IncomingWebhookChannel != "" {
		ch, ok := h.store.Channels.Get(req.IncomingWebhookChannel)
		if !ok {
			// The error Slack answers for a channel that does not exist at
			// install is not documented (events-018).
			slackError(w, "channel_not_found")
			return
		}
		webhookChannel = ch
	}

	scopes := installScopes
	if webhookChannel.ID != "" {
		scopes = append(slices.Clone(installScopes), "incoming-webhook")
	}
	team := h.store.Team.ID
	token := h.newBotToken(team)
	h.store.Tokens.Set(token, store.Token{
		Token:     token,
		Type:      store.TokenBot,
		UserID:    store.DefaultBotUserID,
		BotID:     store.DefaultBotID,
		TeamID:    team,
		Scopes:    scopes,
		OAuthCode: req.Code,
	})
	resp := map[string]any{
		"access_token":          token,
		"token_type":            "bot",
		"scope":                 strings.Join(scopes, ","),
		"bot_user_id":           store.DefaultBotUserID,
		"app_id":                appID,
		"team":                  map[string]any{"name": h.store.Team.Name, "id": team},
		"enterprise":            nil,
		"authed_user":           map[string]any{"id": store.DefaultUserID},
		"is_enterprise_install": false,
	}
	if webhookChannel.ID != "" {
		resp["incoming_webhook"] = h.installWebhook(r, webhookChannel, token)
	}
	slackOK(w, resp)
}

// installWebhook mints the incoming webhook an install bound to ch, tied to the
// bot token the install issued, and renders
// it as the incoming_webhook object of the OAuth response: the channel's name
// as picked, its ID, the webhook URL, and the app settings page for it. The
// name's # prefix, the configuration_url's form, and whether a current
// oauth.v2.access answer carries the object at the top level are unverified
// (events-018). The configuration_url points at this emulator, which serves no
// page there.
func (h *Handler) installWebhook(r *http.Request, ch store.Channel, token string) map[string]any {
	wh := h.store.MintIncomingWebhook(ch.ID, appID, store.DefaultBotID)
	wh.Token = token
	h.store.IncomingWebhooks.Set(wh.ID, wh)
	base := origin(r)
	return map[string]any{
		"channel":           "#" + ch.Name,
		"channel_id":        ch.ID,
		"configuration_url": base + "/services/" + wh.Service,
		"url":               webhookURL(base, wh),
	}
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
