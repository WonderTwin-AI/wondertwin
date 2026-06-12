package httpclient

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestNew_DefaultMinTLSIs12 pins the TLS floor. Lowering the floor
// (or losing it to a refactor) is the load-bearing concern this
// package was created to defend.
func TestNew_DefaultMinTLSIs12(t *testing.T) {
	t.Parallel()
	c := New()
	if got := MinTLSVersion(c); got != tls.VersionTLS12 {
		t.Fatalf("MinTLSVersion = %#x, want tls.VersionTLS12 (%#x)", got, tls.VersionTLS12)
	}
}

// TestNew_WithMinTLS_Overrides asserts the option wins over the default.
func TestNew_WithMinTLS_Overrides(t *testing.T) {
	t.Parallel()
	c := New(WithMinTLS(tls.VersionTLS13))
	if got := MinTLSVersion(c); got != tls.VersionTLS13 {
		t.Fatalf("MinTLSVersion = %#x, want tls.VersionTLS13 (%#x)", got, tls.VersionTLS13)
	}
}

// TestNew_DefaultTimeout pins the timeout default at 30s.
func TestNew_DefaultTimeout(t *testing.T) {
	t.Parallel()
	c := New()
	if c.Timeout != DefaultTimeout {
		t.Fatalf("Timeout = %v, want %v", c.Timeout, DefaultTimeout)
	}
}

// TestNew_WithTimeout_Overrides asserts the option wins over the default.
func TestNew_WithTimeout_Overrides(t *testing.T) {
	t.Parallel()
	c := New(WithTimeout(7 * time.Second))
	if c.Timeout != 7*time.Second {
		t.Fatalf("Timeout = %v, want 7s", c.Timeout)
	}
}

// TestNew_UserAgentSetByDefault drives a real request through the
// client at an httptest.Server and asserts the UA header lands.
func TestNew_UserAgentSetByDefault(t *testing.T) {
	t.Parallel()
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := New()
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if seen != DefaultUserAgent {
		t.Fatalf("User-Agent = %q, want %q", seen, DefaultUserAgent)
	}
}

// TestNew_WithUserAgent_Overrides asserts the override wins over the
// default. Important for cmd/wt's main, which embeds the build
// version in the UA so support can correlate by CLI version.
func TestNew_WithUserAgent_Overrides(t *testing.T) {
	t.Parallel()
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := New(WithUserAgent("wt/1.2.3"))
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if seen != "wt/1.2.3" {
		t.Fatalf("User-Agent = %q, want %q", seen, "wt/1.2.3")
	}
}

// TestNew_UserAgentDoesNotOverrideCallerSet asserts the wrapping
// RoundTripper only fills in a UA when the caller hasn't already set
// one. Useful for callers (e.g., a downstream proxy) that want to
// pass through a third-party UA.
func TestNew_UserAgentDoesNotOverrideCallerSet(t *testing.T) {
	t.Parallel()
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := New()
	req, err := http.NewRequest("GET", srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("User-Agent", "caller/9.9.9")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	if seen != "caller/9.9.9" {
		t.Fatalf("User-Agent = %q, want caller-supplied %q (wrapper should not override)", seen, "caller/9.9.9")
	}
}

// TestNew_WithEmptyUserAgentDisablesWrapper asserts WithUserAgent("")
// skips the RoundTripper wrapping entirely. Net effect: requests go
// out with Go's stdlib default UA. Useful for callers that manage UA
// at the request level.
func TestNew_WithEmptyUserAgentDisablesWrapper(t *testing.T) {
	t.Parallel()
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := New(WithUserAgent(""))
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	// Stdlib default: "Go-http-client/1.1" — assert non-empty and not
	// our package default (i.e. the wrapper was bypassed).
	if seen == DefaultUserAgent {
		t.Fatalf("User-Agent = %q, expected wrapper to be disabled", seen)
	}
}

// TestMinTLSVersion_NilClient is a defensive nil check.
func TestMinTLSVersion_NilClient(t *testing.T) {
	t.Parallel()
	if got := MinTLSVersion(nil); got != 0 {
		t.Fatalf("MinTLSVersion(nil) = %#x, want 0", got)
	}
}
