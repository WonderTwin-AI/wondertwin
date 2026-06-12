// Package httpclient is the wondertwin CLI's HTTP client factory.
//
// Every outbound HTTP call from cmd/wt, internal/registry,
// internal/platform, internal/content, internal/mcp, etc., goes
// through New. The reason this package exists rather than continuing
// to write `&http.Client{Timeout: ...}` inline at each call site:
//
//   - TLS floor. Every client returned by New enforces
//     tls.VersionTLS12 as the minimum negotiated version, defending
//     against a deploy that lands on an old Go stdlib default or a
//     middlebox that downgrades.
//   - Sane transport. The default transport is a clone of
//     http.DefaultTransport, so dialer / proxy / keep-alive defaults
//     remain identical to a stdlib client — not the zero-value
//     http.Transport{} that fresh `&http.Client{}` constructions get.
//   - User-Agent. A wrapping RoundTripper sets a default User-Agent on
//     every request that doesn't already declare one, so wondertwin
//     traffic is identifiable in vendor logs.
//
// Construction is option-driven; the zero-arg form is the right
// default for most callers (30s timeout, TLS 1.2 floor, default UA).
// Callers that need a different timeout — registry installer's
// 5-minute downloads, MCP tool calls' 5-second floors — pass
// WithTimeout.
//
// To keep this discipline, the audit-script (when added) will reject
// `&http.Client{` outside _test.go files. New call sites should route
// through this package.
package httpclient

import (
	"crypto/tls"
	"net/http"
	"time"
)

const (
	// DefaultTimeout matches the stdlib examples and the prior most-
	// common timeout (`time.Second * 30`) across the wondertwin call
	// sites. Callers with hard-real-time budgets (MCP at 5s, registry
	// installer at 5m) override with WithTimeout.
	DefaultTimeout = 30 * time.Second

	// DefaultUserAgent identifies wondertwin CLI traffic to vendor
	// dashboards. Concrete tooling (cmd/wt) overrides this with its
	// embedded build version via WithUserAgent so support can
	// correlate by CLI version.
	DefaultUserAgent = "wondertwin-cli"
)

// Option tunes a client built by New.
type Option func(*config)

type config struct {
	timeout   time.Duration
	minTLS    uint16
	userAgent string
}

// WithTimeout overrides DefaultTimeout. Pass 0 to disable the timeout
// entirely (matches *http.Client.Timeout zero-value semantics) —
// callers that disable the timeout must enforce one via context, or
// the request can block forever.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithMinTLS overrides the TLS floor. The default is tls.VersionTLS12;
// production callers should not lower this. Tests may set
// tls.VersionTLS13 to assert preference, or lower to exercise a
// downgrade path against a controlled fixture.
func WithMinTLS(v uint16) Option {
	return func(c *config) { c.minTLS = v }
}

// WithUserAgent overrides DefaultUserAgent. cmd/wt's main package
// typically passes "wt/" + Version so the UA carries the build
// version. Empty string disables the UA-rewriting RoundTripper
// entirely — useful for callers that already manage UA at the
// request level.
func WithUserAgent(s string) Option {
	return func(c *config) { c.userAgent = s }
}

// New returns an *http.Client with the TLS floor, default timeout,
// and a wrapping RoundTripper that sets a default User-Agent on every
// outbound request. The returned client is safe for concurrent use.
//
// Reuse the same client across calls — every New() pays the
// http.DefaultTransport.Clone() cost. Callers that need many one-shot
// clients should hoist a package-level *http.Client built from New.
func New(opts ...Option) *http.Client {
	cfg := config{
		timeout:   DefaultTimeout,
		minTLS:    tls.VersionTLS12,
		userAgent: DefaultUserAgent,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	// Clone DefaultTransport so the dialer, proxy, keep-alive, and
	// idle-connection settings stay aligned with stdlib defaults.
	// Constructing a fresh http.Transport{} would reset those to
	// zero-value defaults — slower in steady state.
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.TLSClientConfig = &tls.Config{MinVersion: cfg.minTLS}

	var rt http.RoundTripper = base
	if cfg.userAgent != "" {
		rt = uaTransport{base: rt, ua: cfg.userAgent}
	}
	return &http.Client{
		Timeout:   cfg.timeout,
		Transport: rt,
	}
}

// MinTLSVersion returns the TLS floor configured on c, or 0 if c was
// not constructed by this package (or if any wrapping transport hides
// the underlying *http.Transport). Used by package tests and by
// observability code that wants to log the negotiated floor.
func MinTLSVersion(c *http.Client) uint16 {
	if c == nil {
		return 0
	}
	t := underlyingTransport(c.Transport)
	if t == nil || t.TLSClientConfig == nil {
		return 0
	}
	return t.TLSClientConfig.MinVersion
}

// underlyingTransport walks past the package's UA wrapper to the
// underlying *http.Transport, if any. Returns nil if the chain leads
// somewhere else (caller passed a custom RoundTripper).
func underlyingTransport(rt http.RoundTripper) *http.Transport {
	for {
		switch v := rt.(type) {
		case *http.Transport:
			return v
		case uaTransport:
			rt = v.base
		default:
			return nil
		}
	}
}

// uaTransport sets a default User-Agent header on every outbound
// request that doesn't already declare one. Wraps base; concurrent-
// safe because http.Request.Clone is.
type uaTransport struct {
	base http.RoundTripper
	ua   string
}

func (t uaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.ua)
	}
	return t.base.RoundTrip(req)
}
