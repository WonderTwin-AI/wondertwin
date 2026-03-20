package telemetry

import (
	"bytes"
	"io"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// bodyCapture wraps ResponseWriter to capture the response body.
type bodyCapture struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (bc *bodyCapture) WriteHeader(code int) {
	bc.status = code
	bc.ResponseWriter.WriteHeader(code)
}

func (bc *bodyCapture) Write(b []byte) (int, error) {
	bc.body.Write(b) // capture a copy
	return bc.ResponseWriter.Write(b)
}

// Middleware returns HTTP middleware that emits telemetry events.
// It captures request/response body shapes (not values), timing, and
// propagates the correlation ID from chi's RequestID middleware.
func Middleware(emitter *Emitter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if emitter == nil || !emitter.Enabled() {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			// Extract correlation ID from chi's RequestID middleware.
			correlationID := chimw.GetReqID(r.Context())

			// Capture request body.
			var reqBody []byte
			if r.Body != nil {
				reqBody, _ = io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(reqBody))
			}

			// Capture response.
			capture := &bodyCapture{ResponseWriter: w, status: 200}
			next.ServeHTTP(capture, r)

			durationMS := time.Since(start).Milliseconds()

			emitter.EmitHTTPWithContext(
				correlationID,
				r.Method,
				r.URL.Path,
				capture.status,
				durationMS,
				reqBody,
				capture.body.Bytes(),
			)
		})
	}
}
