package api

import (
	"bytes"
	"net/http"
)

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
		// Check for cached response
		if status, body, ok := h.mw.Idempotent.Check(key); ok {
			w.Header().Set("Content-Type", "application/json")
			// The replayed body is a stored response, so pin the declared type and
			// stop content sniffing from reinterpreting it as markup (G705).
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(status)
			//nolint:gosec // G705: body is a response this twin generated and stored
			// earlier, replayed verbatim under an explicit JSON content type with
			// nosniff set above. It is not caller-controlled markup.
			w.Write(body)
			return
		}
		// Capture response for caching
		rec := &responseRecorder{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(rec, r)
		h.mw.Idempotent.Store(key, rec.statusCode, rec.body.Bytes())
	})
}
