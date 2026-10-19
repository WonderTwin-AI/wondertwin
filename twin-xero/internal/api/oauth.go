package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

const (
	issuer = "https://identity.xero.com"
	// accessTokenLifetime is how long Xero says an access token lives (auth-003).
	accessTokenLifetime = 1800 * time.Second
	// defaultScope is granted to a client-credentials call that names none.
	// Scopes are echoed and never enforced.
	defaultScope = "accounting.transactions accounting.contacts accounting.settings"
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

// Token serves POST /connect/token. Only the client_credentials grant exists
// so far; the code and refresh grants arrive in the next slice.
func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
	client, ok := clientID(r)
	if !ok {
		oauthError(w, "invalid_client")
		return
	}
	if r.PostFormValue("grant_type") != "client_credentials" {
		oauthError(w, "unsupported_grant_type")
		return
	}
	scope := r.PostFormValue("scope")
	if scope == "" {
		scope = defaultScope
	}
	h.issue(w, client, scope)
}

// issue mints an access token, records it, and answers.
func (h *Handler) issue(w http.ResponseWriter, client, scope string) {
	now := h.store.Clock.Now().UTC()
	exp := now.Add(accessTokenLifetime)
	authEvent := h.store.NewID()
	access := signJWT(map[string]any{
		"iss": issuer, "aud": issuer + "/resources", "client_id": client,
		"iat": now.Unix(), "nbf": now.Unix(), "exp": exp.Unix(), "jti": h.store.NewID(),
		"authentication_event_id": authEvent, "scope": strings.Fields(scope),
	})
	h.store.AccessTokens.Set(access, store.AccessToken{ClientID: client, Scope: scope, AuthEventID: authEvent, ExpiresAt: exp})
	twincore.JSON(w, http.StatusOK, map[string]any{
		"access_token": access, "expires_in": int(accessTokenLifetime.Seconds()),
		"token_type": "Bearer", "scope": scope,
	})
}
