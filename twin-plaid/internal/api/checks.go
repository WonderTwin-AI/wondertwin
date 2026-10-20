package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
)

// supportedVersion is the one Plaid-Version the emulator serves (OD-3).
const supportedVersion = "2020-09-14"

// Call is a request that passed the Plaid checks: its credentials and its
// JSON object body, for the handler to read fields from.
type Call struct {
	ClientID string
	Secret   string
	Body     map[string]any
}

type callKey struct{}

// CallFrom returns the Call the checks stored on the request.
func CallFrom(r *http.Request) (Call, bool) {
	c, ok := r.Context().Value(callKey{}).(Call)
	return c, ok
}

// SetCredentials makes the emulator enforce a client_id and a secret. Each is
// enforced independently: a non-empty value must match, and an empty one
// accepts any value. With both empty, any non-empty pair is accepted.
func (h *Handler) SetCredentials(clientID, secret string) {
	h.clientID, h.secret = clientID, secret
}

// checkRequest runs Plaid's checks in the order Plaid runs them (quirk
// ENVELOPE-008): Content-Type, JSON parse and types, credential presence, key
// validity, then the version. The route check came first, in the router.
func (h *Handler) checkRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
			h.writeError(w, http.StatusUnsupportedMediaType, "INVALID_REQUEST", "INVALID_HEADERS",
				"Content-Type must be one of [application/json]")
			return
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			h.invalidBody(w)
			return
		}
		body := map[string]any{}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&body); err != nil || dec.More() {
			h.invalidBody(w)
			return
		}
		// A wrong type is reported as a parse failure, not as a type error.
		creds := map[string]string{}
		for _, k := range []string{"client_id", "secret"} {
			switch v := body[k].(type) {
			case nil:
			case string:
				creds[k] = v
			default:
				h.invalidBody(w)
				return
			}
		}

		// Credentials come from the body or the PLAID-CLIENT-ID and
		// PLAID-SECRET headers (quirk AUTH-001).
		if creds["client_id"] == "" {
			creds["client_id"] = r.Header.Get("PLAID-CLIENT-ID")
		}
		if creds["secret"] == "" {
			creds["secret"] = r.Header.Get("PLAID-SECRET")
		}
		var missing []string
		for _, k := range []string{"client_id", "secret"} {
			if creds[k] == "" {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			h.writeMissing(w, missing)
			return
		}

		if (h.clientID != "" && creds["client_id"] != h.clientID) || (h.secret != "" && creds["secret"] != h.secret) {
			h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "INVALID_API_KEYS",
				"invalid client_id or secret provided")
			return
		}

		// The version is checked after the keys (quirk VERSIONING-002).
		if v := r.Header.Get("Plaid-Version"); v != "" && v != supportedVersion {
			h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "UNSUPPORTED_API_VERSION",
				"Plaid-Version "+v+" is not supported by this emulator; it serves "+supportedVersion)
			return
		}

		call := Call{ClientID: creds["client_id"], Secret: creds["secret"], Body: body}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), callKey{}, call)))
	})
}

func (h *Handler) invalidBody(w http.ResponseWriter) {
	h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "INVALID_BODY",
		"body could not be parsed as JSON")
}
