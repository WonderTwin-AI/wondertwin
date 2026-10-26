package api

import "net/http"

// bearer rejects a call that carries no bearer token. Under StrictAuth it also
// rejects one that was not issued here or has expired on the simulated clock,
// before anything else about the call, so an invalid token answers the same 401
// whatever else it sent (auth-007). Without StrictAuth any non-empty token
// passes, as it does on xero-local.
func (h *Handler) bearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok, sent := h.authorised(r); !ok {
			h.writeUnauthorized(w, r, sent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
