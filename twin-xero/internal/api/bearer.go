package api

import (
	"net/http"
	"strings"
)

// bearer rejects a call whose access token was not issued here or has expired
// on the simulated clock. The token is checked before anything else about the
// call, so an invalid one answers the same 401 whatever else it sent (auth-007).
func (h *Handler) bearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, sent := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		tok, ok := h.store.AccessTokens.Get(token)
		if !sent || !ok || !h.store.Clock.Now().Before(tok.ExpiresAt) {
			h.writeUnauthorized(w, r, sent && token != "")
			return
		}
		next.ServeHTTP(w, r)
	})
}
