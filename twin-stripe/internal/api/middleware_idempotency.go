package api

import (
	"bytes"
	"net/http"
	"sync"
)

// idempotencyFingerprints remembers, per Idempotency-Key, the request the
// cached response answered, so a reuse with other parameters can be refused.
type idempotencyFingerprints struct {
	mu   sync.Mutex
	keys map[string]string
}

func (f *idempotencyFingerprints) get(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.keys[key]
	return v, ok
}

func (f *idempotencyFingerprints) set(key, fp string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.keys == nil {
		f.keys = map[string]string{}
	}
	f.keys[key] = fp
}

// requestFingerprint identifies a request by endpoint and parameters. Form
// values encode in sorted key order, so parameter order does not matter.
func requestFingerprint(r *http.Request) string {
	_ = r.ParseForm()
	return r.Method + " " + r.URL.Path + "?" + r.Form.Encode()
}

// responseRecorder captures response status and body for idempotency caching.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// idempotencyMiddleware caches POST responses by Idempotency-Key header.
func (h *Handler) idempotencyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}
		fp := requestFingerprint(r)
		// Check for cached response
		if status, body, ok := h.mw.Idempotent.Check(key); ok {
			if prev, seen := h.idempotency.get(key); seen && prev != fp {
				writeError(w, http.StatusBadRequest, apiError{
					Type: "idempotency_error",
					Message: "Keys for idempotent requests can only be used with the same parameters they were first used with. " +
						"Try using a key other than '" + key + "' if you meant to execute a different request.",
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(status)
			//nolint:gosec // G705: body is a response this twin generated and stored
			// earlier, replayed verbatim under an explicit JSON content type. It is
			// not caller-controlled markup. No hardening header is added here: real
			// Stripe does not send one, and this twin is held to header parity.
			w.Write(body)
			return
		}
		// Capture response for caching
		rec := &responseRecorder{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(rec, r)
		h.mw.Idempotent.Store(key, rec.statusCode, rec.body.Bytes())
		h.idempotency.set(key, fp)
	})
}
