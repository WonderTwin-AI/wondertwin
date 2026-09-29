package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twinkit/expand"
)

// expandBuffer captures a handler's response without writing it through, so
// expandMiddleware can rewrite the body before it reaches the client.
type expandBuffer struct {
	header     http.Header
	statusCode int
	body       bytes.Buffer
}

func newExpandBuffer() *expandBuffer {
	return &expandBuffer{header: make(http.Header), statusCode: http.StatusOK}
}

func (b *expandBuffer) Header() http.Header  { return b.header }
func (b *expandBuffer) WriteHeader(code int) { b.statusCode = code }
func (b *expandBuffer) Write(p []byte) (int, error) {
	return b.body.Write(p)
}

// expandMiddleware applies Stripe-style response expansion (the `expand[]`
// request parameter) to JSON responses. It parses expand[] up front — from
// the query string and, for form/JSON POST bodies, from r.Form — and only
// buffers the response when at least one expand path was requested;
// requests without expand[] pass straight through with no extra copying.
//
// This is the mechanical, permissive layer: it resolves any field named in
// expand[] whose value is a known object ID, regardless of whether real
// Stripe's API allows expanding that field on that particular endpoint. See
// expand.Resolve on MemoryStore and the twinkit/expand package doc.
func (h *Handler) expandMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm() // safe to call again later in the handler; see net/http docs on idempotent ParseForm
		paths := r.Form["expand[]"]
		if len(paths) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		buf := newExpandBuffer()
		next.ServeHTTP(buf, r)

		if buf.statusCode < 200 || buf.statusCode >= 300 ||
			!strings.Contains(buf.header.Get("Content-Type"), "application/json") {
			flushBuffer(w, buf)
			return
		}

		var body map[string]any
		if err := json.Unmarshal(buf.body.Bytes(), &body); err != nil {
			// Not a JSON object (e.g. the delete-endpoint {"deleted": true}
			// shape still decodes fine, but a bare array or scalar would
			// not) - nothing to expand, pass the original bytes through.
			flushBuffer(w, buf)
			return
		}

		expand.Apply(body, paths, h.store)
		expandIDArrays(body, paths, h.store)

		encoded, err := json.Marshal(body)
		if err != nil {
			flushBuffer(w, buf)
			return
		}
		encoded = append(encoded, '\n')

		copyHeader(w.Header(), buf.header)
		w.Header().Set("Content-Length", strconv.Itoa(len(encoded)))
		w.WriteHeader(buf.statusCode)
		_, _ = w.Write(encoded)
	})
}

// expandRequested reports whether path was requested in expand[] (either
// array form; see arrayParamsMiddleware).
func expandRequested(r *http.Request, path string) bool {
	_ = r.ParseForm()
	for _, p := range r.Form["expand[]"] {
		if p == path {
			return true
		}
	}
	return false
}

func flushBuffer(w http.ResponseWriter, buf *expandBuffer) {
	copyHeader(w.Header(), buf.header)
	w.WriteHeader(buf.statusCode)
	_, _ = w.Write(buf.body.Bytes())
}

func copyHeader(dst, src http.Header) {
	for k, v := range src {
		dst[k] = v
	}
}

// expandIDArrays expands fields that hold an array of IDs, such as dahlia's
// discounts, which the kit's expander (single ID fields only) leaves alone.
// It handles a top-level field and the "data.<field>" list form.
func expandIDArrays(body map[string]any, paths []string, resolver expand.Resolver) {
	for _, p := range paths {
		if field, ok := strings.CutPrefix(p, "data."); ok && !strings.Contains(field, ".") {
			if data, ok := body["data"].([]any); ok {
				for _, item := range data {
					if m, ok := item.(map[string]any); ok {
						resolveIDArray(m, field, resolver)
					}
				}
			}
			continue
		}
		if !strings.Contains(p, ".") {
			resolveIDArray(body, p, resolver)
		}
	}
}

func resolveIDArray(obj map[string]any, field string, resolver expand.Resolver) {
	arr, ok := obj[field].([]any)
	if !ok {
		return
	}
	for i, v := range arr {
		if id, ok := v.(string); ok {
			if resolved, ok := resolver.Resolve(id); ok {
				arr[i] = resolved
			}
		}
	}
}
