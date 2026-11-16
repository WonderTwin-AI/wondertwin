package api

import (
	"net/http"
	"strings"
)

// authorised reports whether the call carries a bearer token this emulator
// accepts, and whether it sent one. It is the check bearer makes, so a call to a
// path that does not exist is held to the same rule as one that does. The
// scheme name is matched case-insensitively, as RFC 7235 section 2.1 requires.
func (h *Handler) authorised(r *http.Request) (ok, sent bool) {
	token, found := bearerToken(r.Header.Get("Authorization"))
	sent = found && token != ""
	if sent && !h.store.StrictAuth.Load() {
		return true, true
	}
	tok, ok := h.store.AccessTokens.Get(token)
	return found && ok && h.store.Clock.Now().Before(tok.ExpiresAt), sent
}

// bearerToken is the credential an Authorization header carries under the
// Bearer scheme, and whether the header names that scheme at all.
func bearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	return strings.TrimLeft(token, " "), true
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
