package api

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Stripe's metadata limits (api/metadata): up to 50 keys, key names up to 40
// characters, values up to 500 characters, and no square brackets in keys.
const (
	maxMetadataKeys     = 50
	maxMetadataKeyLen   = 40
	maxMetadataValueLen = 500
)

// metadataParams reads a request's metadata parameters: each metadata[key]
// with its value (empty when the key is being unset), and whether the request
// sent metadata= to clear every key.
func metadataParams(r *http.Request) (map[string]string, bool) {
	_ = r.ParseForm()
	params := map[string]string{}
	for key, vals := range r.Form {
		name, ok := strings.CutPrefix(key, "metadata[")
		if !ok || !strings.HasSuffix(name, "]") || len(vals) == 0 {
			continue
		}
		name = strings.TrimSuffix(name, "]")
		if name == "" {
			// metadata[] is the bracket form arrayParamsMiddleware adds for
			// metadata[N]; the indexed key is read under its own name.
			continue
		}
		params[name] = vals[0]
	}
	vals, sent := r.Form["metadata"]
	return params, sent && len(vals) > 0 && vals[0] == ""
}

// parseMetadata returns the metadata a create request sets, or nil.
func parseMetadata(r *http.Request) map[string]string {
	params, _ := metadataParams(r)
	meta := map[string]string{}
	for k, v := range params {
		if v != "" {
			meta[k] = v
		}
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

// updateMetadata applies an update request's metadata to current, as Stripe
// does: keys merge, a key sent empty is removed, and metadata= removes them
// all. Keys not named are kept. It writes Stripe's error and reports false
// when the result would hold more keys than Stripe allows.
func updateMetadata(w http.ResponseWriter, r *http.Request, current map[string]string) (map[string]string, bool) {
	params, clear := metadataParams(r)
	merged := map[string]string{}
	if !clear {
		for k, v := range current {
			merged[k] = v
		}
	}
	for k, v := range params {
		if v == "" {
			delete(merged, k)
		} else {
			merged[k] = v
		}
	}
	if len(merged) > maxMetadataKeys {
		writeError(w, http.StatusBadRequest, apiError{
			Type:    "invalid_request_error",
			Param:   "metadata",
			Message: fmt.Sprintf("Invalid metadata: metadata can have up to %d keys, but this request would leave %d keys.", maxMetadataKeys, len(merged)),
		})
		return nil, false
	}
	if len(merged) == 0 {
		return nil, true
	}
	return merged, true
}

// metadataMiddleware refuses a request whose metadata breaks Stripe's limits,
// before any handler acts on it.
func (h *Handler) metadataMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			next.ServeHTTP(w, r)
			return
		}
		params, _ := metadataParams(r)
		if len(params) > maxMetadataKeys {
			writeError(w, http.StatusBadRequest, apiError{
				Type:    "invalid_request_error",
				Param:   "metadata",
				Message: fmt.Sprintf("Invalid metadata: metadata can have up to %d keys, but you passed in %d keys.", maxMetadataKeys, len(params)),
			})
			return
		}
		for k, v := range params {
			param := "metadata[" + k + "]"
			var msg string
			switch {
			case strings.ContainsAny(k, "[]"):
				msg = "Invalid metadata: metadata keys can't contain square brackets ([ and ])."
			case utf8.RuneCountInString(k) > maxMetadataKeyLen:
				msg = fmt.Sprintf("Invalid metadata: metadata keys can have up to %d characters, but you passed in a key with %d characters.", maxMetadataKeyLen, utf8.RuneCountInString(k))
			case utf8.RuneCountInString(v) > maxMetadataValueLen:
				msg = fmt.Sprintf("Invalid metadata: metadata values can have up to %d characters, but you passed in a value with %d characters.", maxMetadataValueLen, utf8.RuneCountInString(v))
			default:
				continue
			}
			writeError(w, http.StatusBadRequest, apiError{Type: "invalid_request_error", Param: param, Message: msg})
			return
		}
		next.ServeHTTP(w, r)
	})
}
