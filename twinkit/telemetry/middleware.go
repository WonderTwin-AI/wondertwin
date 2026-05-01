package telemetry

import (
	"net/http"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/cimode"
)

// LicenseStatusFunc returns the current license status. Middleware
// invokes it on every request so emitted events reflect the current
// state — important for long-running twins where the license can be
// installed mid-process via `wt license install`.
type LicenseStatusFunc func() (cimode.Status, *cimode.License)

// Middleware returns an http middleware that records an endpoint_hit
// event per request. Internal admin paths (/admin/*) and the legacy
// healthz endpoint are excluded.
//
// Path templating (collapsing IDs in URLs into :id placeholders) is a
// known follow-up — the current implementation emits the raw request
// path. Group counts on the ingestion side will be cardinal-heavy
// until templating lands.
func (r *Reporter) Middleware(twinName, twinVersion, mode, platform string, status LicenseStatusFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if r == nil || r.ch == nil {
				next.ServeHTTP(w, req)
				return
			}
			if skipPath(req.URL.Path) {
				next.ServeHTTP(w, req)
				return
			}

			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, req)

			ev := Event{
				Type:       "endpoint_hit",
				TwinName:   twinName,
				TwinVer:    twinVersion,
				Mode:       mode,
				Platform:   platform,
				Endpoint:   req.Method + " " + req.URL.Path,
				StatusCode: rec.status,
				DurationMs: int(time.Since(start) / time.Millisecond),
				OrgHash:    r.cfg.OrgHash,
				Timestamp:  start.UTC(),
			}
			if status != nil {
				st, lic := status()
				ev.LicenseOK = st.Valid
				ev.LicenseRsn = st.Reason
				if st.Valid && lic != nil {
					ev.LicenseID = lic.Key
				}
			}
			r.Record(ev)
		})
	}
}

func skipPath(path string) bool {
	return strings.HasPrefix(path, "/admin/") || path == "/admin" || path == "/healthz" || path == "/health"
}

// statusRecorder is a minimal response-writer wrapper used to capture
// the final status code without hooking into the rest of twincore's
// middleware machinery. Kept package-local because telemetry's needs
// are simpler than twincore's chain.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.wroteHeader = true
	}
	return s.ResponseWriter.Write(b)
}
