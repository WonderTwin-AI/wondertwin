package api

import (
	"net/http"
	"strings"
)

// authorised reports whether the call carries a bearer token this emulator
// accepts, and whether it sent one. It is the check bearer makes, so a call to a
// path that does not exist is held to the same rule as one that does.
func (h *Handler) authorised(r *http.Request) (ok, sent bool) {
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	sent = found && token != ""
	if sent && !h.store.StrictAuth.Load() {
		return true, true
	}
	tok, ok := h.store.AccessTokens.Get(token)
	return found && ok && h.store.Clock.Now().Before(tok.ExpiresAt), sent
}

// NotFound answers a path no route serves: another path version, another
// family, or a path nobody registered. Authentication precedes routing, so a
// call without a token Xero would accept answers the 401 first (quirk auth-008).
// With one the answer is a 404 problem-details body, whose words are a guess.
func (h *Handler) NotFound(w http.ResponseWriter, r *http.Request) {
	if ok, sent := h.authorised(r); !ok {
		h.writeUnauthorized(w, r, sent)
		return
	}
	h.writeProblem(w, r, http.StatusNotFound, "Not Found", "The requested resource was not found")
}

// MethodNotAllowed answers a method a served path does not take. It is held to
// the same authentication rule, and the body is a 405 problem-details body.
func (h *Handler) MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	if ok, sent := h.authorised(r); !ok {
		h.writeUnauthorized(w, r, sent)
		return
	}
	h.writeProblem(w, r, http.StatusMethodNotAllowed, "Method Not Allowed", "The requested method is not allowed for this resource")
}
