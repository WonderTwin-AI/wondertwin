package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// Principal kinds.
const (
	principalAnonymous    = "anonymous"
	principalUser         = "user"
	principalInstallation = "installation"
	principalApp          = "app"
)

// principal is who a request authenticates as.
type principal struct {
	Kind           string
	Login          string
	Token          string
	InstallationID int64
}

type principalKey struct{}

func principalFrom(r *http.Request) principal {
	if p, ok := r.Context().Value(principalKey{}).(principal); ok {
		return p
	}
	return principal{Kind: principalAnonymous}
}

// actor returns the login a write is attributed to.
func actor(r *http.Request) string {
	if p := principalFrom(r); p.Login != "" {
		return p.Login
	}
	return store.DefaultLogin
}

// bearerToken extracts the credential from "Bearer <t>" or the legacy
// "token <t>" scheme. ok is false when an Authorization header is present
// but unusable.
func bearerToken(auth string) (string, bool) {
	for _, scheme := range []string{"Bearer ", "bearer ", "token ", "Token "} {
		if strings.HasPrefix(auth, scheme) {
			t := strings.TrimSpace(strings.TrimPrefix(auth, scheme))
			return t, t != ""
		}
	}
	return "", false
}

// resolve maps a credential to a principal. GitHub answers 401 Bad
// credentials for anything it did not issue; the emulator accepts tokens it
// minted or was seeded with, any token in one of GitHub's documented formats,
// and JSON web tokens (which only the GitHub App routes accept).
func (h *Handler) resolve(tok string) (principal, bool) {
	if t, ok := h.store.Tokens.Get(tok); ok {
		if t.ExpiresAt != "" {
			if exp, err := time.Parse(time.RFC3339, t.ExpiresAt); err == nil && !h.store.Clock.Now().Before(exp) {
				return principal{}, false
			}
		}
		kind := principalUser
		if t.Kind == principalInstallation {
			kind = principalInstallation
		}
		return principal{Kind: kind, Login: t.Login, Token: tok, InstallationID: t.InstallationID}, true
	}
	switch {
	case store.LooksLikeJWT(tok):
		return principal{Kind: principalApp, Token: tok}, true
	case store.LooksLikeUserToken(tok):
		return principal{Kind: principalUser, Login: store.DefaultLogin, Token: tok}, true
	}
	return principal{}, false
}

// bearerAuthMiddleware authenticates the request. A request with no
// Authorization header proceeds anonymously when it is a read GitHub serves
// without a token; everything else needs a credential.
func (h *Handler) bearerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		var p principal
		if auth == "" {
			if !anonymousAllowed(r) {
				rateHeaders(w, 60)
				ghError(w, http.StatusUnauthorized, "Requires authentication")
				return
			}
			p = principal{Kind: principalAnonymous}
		} else {
			tok, ok := bearerToken(auth)
			if ok {
				p, ok = h.resolve(tok)
			}
			if !ok {
				rateHeaders(w, 60)
				ghError(w, http.StatusUnauthorized, "Bad credentials")
				return
			}
		}

		if p.Kind == principalAnonymous {
			rateHeaders(w, 60)
			if owner, repo, ok := repoFromPath(r.URL.Path); ok {
				if rp, found := h.store.GetRepo(owner, repo); found && rp.Private {
					ghError(w, http.StatusNotFound, "Not Found")
					return
				}
			}
		} else {
			rateHeaders(w, 5000)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

// anonymousAllowed reports whether GitHub serves r without a token: reads of
// public resources, not the authenticated user's own resources, the App
// routes, or repository administration.
func anonymousAllowed(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	p := r.URL.Path
	for _, prefix := range []string{"/user", "/app", "/installation", "/notifications"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return false
		}
	}
	if p == "/issues" {
		return false
	}
	if _, _, ok := repoFromPath(p); ok {
		rest := strings.SplitN(p, "/", 5)
		if len(rest) == 5 {
			for _, admin := range []string{"hooks", "keys", "collaborators", "actions/secrets"} {
				if rest[4] == admin || strings.HasPrefix(rest[4], admin+"/") {
					return false
				}
			}
		}
	}
	if strings.HasPrefix(p, "/orgs/") && strings.Contains(p, "/actions/secrets") {
		return false
	}
	return true
}

// repoFromPath extracts owner and repo from a /repos/{owner}/{repo} path.
func repoFromPath(p string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimPrefix(p, "/"), "/", 4)
	if len(parts) < 3 || parts[0] != "repos" || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// rateHeaders sets the primary rate-limit headers: 60 an hour for
// unauthenticated callers and 5,000 for a token, as GitHub documents.
func rateHeaders(w http.ResponseWriter, limit int) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(limit-1))
	w.Header().Set("X-RateLimit-Used", "1")
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
	w.Header().Set("X-RateLimit-Resource", "core")
}
