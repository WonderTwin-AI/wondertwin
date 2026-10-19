package api

import (
	"net/http"
	"strings"
)

// Origin is the scheme and host the client called, so a URL handed back to it
// reaches this emulator however it is fronted. Behind the LocalStack gateway
// the emulator sees Host 127.0.0.1:{port}; the gateway names the host the
// client used in X-Forwarded-Host and X-Forwarded-Proto.
func Origin(r *http.Request) string {
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
