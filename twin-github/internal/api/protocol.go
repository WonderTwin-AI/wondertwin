package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// APIVersion is the only REST API calendar version this community app
// emulator serves. GitHub serves 2022-11-28 as well and defaults to it; here a
// request with no X-GitHub-Api-Version header is served as APIVersion, and any
// other value is refused the way GitHub refuses an unsupported version.
const APIVersion = "2026-03-10"

const docsURL = "https://docs.github.com/rest"

// versionMiddleware applies GitHub's calendar-version negotiation. GitHub
// ignores the header on unauthenticated requests; this emulator applies it to
// every request so that a client's version handling is exercised whether or
// not it sends a token.
func versionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("X-GitHub-Api-Version"); v != "" && v != APIVersion {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": "Bad Request",
				"errors": fmt.Sprintf("The version you specified in the \"X-GitHub-API-Version\" request header, %q, "+
					"is not a supported version. The following versions are currently supported: %q (most recent).", v, APIVersion),
				"documentation_url": docsURL,
				"status":            "400",
			})
			return
		}
		w.Header().Set("X-GitHub-Api-Version-Selected", APIVersion)
		next.ServeHTTP(w, r)
	})
}

// commonHeaders sets the headers GitHub sends on every REST response.
func commonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-GitHub-Media-Type", "github.v3; format=json")
		next.ServeHTTP(w, r)
	})
}

// origin returns the scheme and host the client used to reach the emulator,
// so that URLs in responses (and in Link headers) point back at the emulator
// rather than at api.github.com. Behind the LocalStack gateway the forwarded
// headers carry the public host.
func origin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = strings.TrimSpace(strings.Split(p, ",")[0])
	}
	host := r.Host
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		host = strings.TrimSpace(strings.Split(fh, ",")[0])
	}
	return scheme + "://" + host
}

// ghJSON writes a successful JSON response with GitHub-standard headers.
func ghJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-RateLimit-Limit", "5000")
	w.Header().Set("X-RateLimit-Remaining", "4999")
	w.Header().Set("X-RateLimit-Used", "1")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ghError writes a GitHub-style error response. GitHub's current error bodies
// carry the status code as a string next to message and documentation_url.
func ghError(w http.ResponseWriter, status int, message string) {
	ghErrorDoc(w, status, message, docsURL)
}

// ghErrorDoc is ghError with an operation-specific documentation URL.
func ghErrorDoc(w http.ResponseWriter, status int, message, doc string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message":           message,
		"documentation_url": doc,
		"status":            strconv.Itoa(status),
	})
}

// ghValidationError writes a 422 validation error.
func ghValidationError(w http.ResponseWriter, resource, field, code string) {
	ghValidationErrors(w, "Validation Failed", map[string]any{"resource": resource, "field": field, "code": code})
}

// ghValidationErrors writes a 422 with GitHub's validation-error shape.
func ghValidationErrors(w http.ResponseWriter, message string, errs ...map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	body := map[string]any{
		"message":           message,
		"documentation_url": docsURL,
		"status":            "422",
	}
	if len(errs) > 0 {
		body["errors"] = errs
	}
	_ = json.NewEncoder(w).Encode(body)
}

// notFound is GitHub's response for a route it does not serve, including a
// known path with a method it does not accept.
func notFound(w http.ResponseWriter, _ *http.Request) {
	ghError(w, http.StatusNotFound, "Not Found")
}
