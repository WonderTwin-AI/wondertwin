package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
)

// idempotencyTTL is how long Resend remembers a key, in store-clock time.
const idempotencyTTL = 24 * time.Hour

// idempotency honours the Idempotency-Key header on the send routes (quirk
// lifecycle-004). A repeat with the same body within 24 hours replays the
// first answer with idempotent-replayed: true and sends nothing; the same key
// with another body answers 409 invalid_idempotent_request; a repeat while
// the first is still running answers 409 concurrent_idempotent_requests; a
// key outside 1 to 256 characters answers 400. Only successful answers are
// kept, so a request that failed validation can be retried with its key.
func (h *Handler) idempotency(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vals, ok := r.Header[http.CanonicalHeaderKey("Idempotency-Key")]
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		key := vals[0]
		if n := utf8.RuneCountInString(key); n < 1 || n > 256 {
			writeError(w, http.StatusBadRequest, "invalid_idempotency_key",
				"Idempotency keys, if present, must have between 1 and 256 characters.")
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", "Request body must be valid JSON.")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		id := r.Method + " " + strings.TrimSuffix(r.URL.Path, "/") + " " + key
		hash := requestHash(body)
		now := h.store.Clock.Now()

		h.idemMu.Lock()
		rec, found := h.store.Idempotency.Get(id)
		if found && !now.Before(rec.CreatedAt.Add(idempotencyTTL)) {
			found = false
		}
		switch {
		case found && rec.InFlight:
			h.idemMu.Unlock()
			writeError(w, http.StatusConflict, "concurrent_idempotent_requests",
				"Same idempotency key used while original request is still in progress.")
			return
		case found && rec.RequestHash != hash:
			h.idemMu.Unlock()
			writeError(w, http.StatusConflict, "invalid_idempotent_request",
				"This idempotency key has been used with this HTTP method and endpoint within the last 24 hours, but the request body was modified and doesn't match the original request.")
			return
		case found:
			h.idemMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(rec.Status)
			_, _ = w.Write(rec.Body)
			return
		}
		h.store.Idempotency.Set(id, store.IdempotencyRecord{RequestHash: hash, CreatedAt: now, InFlight: true})
		h.idemMu.Unlock()

		w.Header().Set("Idempotent-Replayed", "false")
		cw := &capturingWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(cw, r)

		h.idemMu.Lock()
		defer h.idemMu.Unlock()
		if cw.status >= 300 {
			h.store.Idempotency.Delete(id)
			return
		}
		h.store.Idempotency.Set(id, store.IdempotencyRecord{
			RequestHash: hash,
			CreatedAt:   now,
			Status:      cw.status,
			Body:        json.RawMessage(cw.body.Bytes()),
		})
	})
}

// requestHash fingerprints a request body. JSON is compared by value, so
// whitespace and key order do not make two equal bodies differ.
func requestHash(body []byte) string {
	canonical := body
	var v any
	if err := json.Unmarshal(body, &v); err == nil {
		if b, err := json.Marshal(v); err == nil {
			canonical = b
		}
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// capturingWriter passes a response through and keeps a copy of it.
type capturingWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (c *capturingWriter) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *capturingWriter) Write(b []byte) (int, error) {
	c.body.Write(b)
	return c.ResponseWriter.Write(b)
}
