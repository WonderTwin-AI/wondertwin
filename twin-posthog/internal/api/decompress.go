package api

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// readCaptureBody reads a PostHog capture body, transparently handling the
// compression and form-wrapping variants emitted by posthog-js and other SDKs.
//
// Supported shapes:
//   - Raw JSON
//   - Form-encoded "data=<value>" (value may itself be base64+gzip)
//   - ?compression=gzip-js or ?compression=gzip — body (or "data" field) is gzip bytes
//   - ?compression=base64 — body (or "data" field) is base64-encoded JSON
//   - Content-Encoding: gzip — body is gzipped
//
// Ingest bodies are read before any project-key check, so both the wire bytes
// and the decompressed output need a ceiling: a few-KB gzip bomb otherwise
// expands to hundreds of MB on an unauthenticated route. PostHog's own capture
// payloads are far below these.
const (
	maxIngestWireBytes    = 5 << 20
	maxIngestDecodedBytes = 20 << 20
)

var errIngestBodyTooLarge = errors.New("request body exceeds the maximum ingest size")

func readCaptureBody(r *http.Request) ([]byte, string, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxIngestWireBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}
	if len(raw) > maxIngestWireBytes {
		return nil, "", errIngestBodyTooLarge
	}

	// formKey carries an api_key posted as a sibling form field of data=, which
	// is how posthog-js sends it. It would otherwise be discarded here and the
	// request would look keyless to the ingest gate.
	var formKey string
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") ||
		(len(raw) >= 5 && bytes.HasPrefix(raw, []byte("data="))) {
		if values, perr := url.ParseQuery(string(raw)); perr == nil {
			formKey = values.Get("api_key")
			if d := values.Get("data"); d != "" {
				raw = []byte(d)
			}
		}
	}

	compression := r.URL.Query().Get("compression")
	if compression == "" && strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		compression = "gzip"
	}

	switch compression {
	case "gzip", "gzip-js":
		// posthog-js sends raw gzip bytes; the form-wrapped path may have
		// surfaced a base64-encoded gzip payload — try that first.
		if decoded, derr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw))); derr == nil && looksLikeGzip(decoded) {
			raw = decoded
		}
		gz, gerr := gzip.NewReader(bytes.NewReader(raw))
		if gerr != nil {
			return nil, "", fmt.Errorf("gzip reader: %w", gerr)
		}
		defer gz.Close()
		out, rerr := io.ReadAll(io.LimitReader(gz, maxIngestDecodedBytes+1))
		if rerr != nil {
			return nil, "", fmt.Errorf("gzip read: %w", rerr)
		}
		if len(out) > maxIngestDecodedBytes {
			return nil, "", errIngestBodyTooLarge
		}
		return out, formKey, nil
	case "base64":
		decoded, derr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if derr != nil {
			return nil, "", fmt.Errorf("base64 decode: %w", derr)
		}
		return decoded, formKey, nil
	default:
		return raw, formKey, nil
	}
}

func looksLikeGzip(b []byte) bool {
	return len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b
}
