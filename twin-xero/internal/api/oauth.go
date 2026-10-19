package api

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

const (
	issuer = "https://identity.xero.com"
	// accessTokenLifetime is how long Xero says an access token lives (auth-003).
	accessTokenLifetime = 1800 * time.Second
	// defaultScope is granted to a client-credentials call that names none, and
	// consentScope to an authorization code that names none. Scopes are echoed
	// and never enforced.
	defaultScope = "accounting.transactions accounting.contacts accounting.settings"
	consentScope = "openid profile email offline_access " + defaultScope
)

// oauthError answers the identity service's error shape (quirk auth-006).
func oauthError(w http.ResponseWriter, code string) {
	twincore.JSON(w, http.StatusBadRequest, map[string]string{"error": code})
}

// clientID returns the calling app's client id, from HTTP Basic or, as
// xero-python sends it, the form body (quirk auth-003). Any credentials are
// accepted; a call with none, or a malformed Basic header, is invalid_client.
func clientID(r *http.Request) (string, bool) {
	if user, _, ok := r.BasicAuth(); ok && user != "" {
		return user, true
	}
	if r.Header.Get("Authorization") != "" {
		return "", false
	}
	id := r.PostFormValue("client_id")
	return id, id != ""
}

// Token serves POST /connect/token: the authorization_code, refresh_token and
// client_credentials grants. An authorization code is any non-empty value, since
// no consent screen exists to issue one.
func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
	client, ok := clientID(r)
	if !ok {
		oauthError(w, "invalid_client")
		return
	}
	scope, authEvent, user := r.PostFormValue("scope"), h.store.NewID(), true
	switch r.PostFormValue("grant_type") {
	case "authorization_code":
		if r.PostFormValue("code") == "" {
			oauthError(w, "invalid_request")
			return
		}
		if scope == "" {
			scope = consentScope
		}
	case "refresh_token":
		token := r.PostFormValue("refresh_token")
		h.redeem.Lock()
		old, found := h.store.RefreshTokens.Get(token)
		if !found || old.Used {
			h.redeem.Unlock()
			oauthError(w, "invalid_grant")
			return
		}
		old.Used = true
		h.store.RefreshTokens.Set(token, old)
		h.redeem.Unlock()
		client, scope, authEvent = old.ClientID, old.Scope, old.AuthEventID
	case "client_credentials":
		user = false
		if scope == "" {
			scope = defaultScope
		}
	default:
		oauthError(w, "unsupported_grant_type")
		return
	}
	h.issue(w, client, scope, authEvent, user)
}

// issue mints the tokens, records them, and answers. A user grant gets a refresh
// token for offline_access and an id_token for the openid scope; client
// credentials get neither.
func (h *Handler) issue(w http.ResponseWriter, client, scope, authEvent string, user bool) {
	now := h.store.Clock.Now().UTC()
	exp := now.Add(accessTokenLifetime)
	scopes := strings.Fields(scope)
	access := signJWT(map[string]any{
		"iss": issuer, "aud": issuer + "/resources", "client_id": client,
		"iat": now.Unix(), "nbf": now.Unix(), "exp": exp.Unix(), "jti": h.store.NewID(),
		"authentication_event_id": authEvent, "scope": scopes,
	})
	h.store.AccessTokens.Set(access, store.AccessToken{ClientID: client, Scope: scope, AuthEventID: authEvent, ExpiresAt: exp})
	resp := map[string]any{
		"access_token": access, "expires_in": int(accessTokenLifetime.Seconds()),
		"token_type": "Bearer", "scope": scope,
	}
	if user && slices.Contains(scopes, "offline_access") {
		rt := strings.ReplaceAll(h.store.NewID(), "-", "")
		h.store.RefreshTokens.Set(rt, store.RefreshToken{ClientID: client, Scope: scope, AuthEventID: authEvent})
		resp["refresh_token"] = rt
	}
	if user && slices.Contains(scopes, "openid") {
		resp["id_token"] = signJWT(map[string]any{
			"iss": issuer, "aud": client, "sub": authEvent, "sid": authEvent,
			"iat": now.Unix(), "nbf": now.Unix(), "exp": exp.Unix(),
		})
	}
	twincore.JSON(w, http.StatusOK, resp)
}
