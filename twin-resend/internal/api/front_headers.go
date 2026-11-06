package api

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // G505: the etag format Resend sends is a SHA-1 digest, not a security check.
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
)

// frontHeaders gives every answer except a send the headers Resend's main
// front sends (quirk envelope-007): JSON with charset=utf-8, its security
// headers and a weak etag over the body. POST /emails and POST /emails/batch
// are served by another front that sends none of these, and admin routes are
// the emulator's own.
func (h *Handler) frontHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSendRequest(r) || isAdminPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		buf := &bufferedWriter{header: w.Header(), status: http.StatusOK}
		next.ServeHTTP(buf, r)

		hdr := w.Header()
		if strings.HasPrefix(hdr.Get("Content-Type"), "application/json") {
			hdr.Set("Content-Type", "application/json; charset=utf-8")
		}
		for k, v := range securityHeaders {
			hdr.Set(k, v)
		}
		if buf.body.Len() > 0 {
			hdr.Set("Etag", weakETag(buf.body.Bytes()))
		}
		w.WriteHeader(buf.status)
		_, _ = w.Write(buf.body.Bytes())
	})
}

// securityHeaders are the values recorded on Resend's non-send answers.
var securityHeaders = map[string]string{
	"Strict-Transport-Security": "max-age=63072000; includeSubDomains",
	"X-Content-Type-Options":    "nosniff",
	"X-Frame-Options":           "DENY",
	"Referrer-Policy":           "strict-origin-when-cross-origin",
	"Permissions-Policy":        "camera=(), microphone=(), geolocation=(), payment=()",
	"Content-Security-Policy":   "default-src 'none'; frame-ancestors 'none'",
}

// weakETag is the etag form Resend sends, W/"{hex length}-{27 characters of
// base64 SHA-1}".
func weakETag(body []byte) string {
	sum := sha1.Sum(body) //nolint:gosec // G401: see the import.
	return `W/"` + strconv.FormatInt(int64(len(body)), 16) + "-" +
		base64.StdEncoding.EncodeToString(sum[:])[:27] + `"`
}

// bufferedWriter holds an answer so its headers can be set from its body.
type bufferedWriter struct {
	header http.Header
	status int
	wrote  bool
	body   bytes.Buffer
}

func (b *bufferedWriter) Header() http.Header { return b.header }

func (b *bufferedWriter) WriteHeader(status int) {
	if !b.wrote {
		b.status, b.wrote = status, true
	}
}

func (b *bufferedWriter) Write(p []byte) (int, error) {
	b.wrote = true
	return b.body.Write(p)
}
