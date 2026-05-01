package twincore

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twinkit/replay"
)

// ObserverEndpoint configures a local-only forwarder that mirrors
// recorded entries off-process.
//
// The forwarder exists so observers (synthetic-data sidecars, replay
// inspectors) can run in a separate process without giving the twin
// network egress. URL must be one of:
//
//   - unix:///path/to/socket
//   - http://127.0.0.1:<port>[/path]
//   - http://[::1]:<port>[/path]
//   - http://localhost:<port>[/path]
//
// twincore.New rejects any other form. This is the architectural
// commitment in code: traffic captured for observation never leaves
// the host.
type ObserverEndpoint struct {
	URL     string
	Enabled bool
}

// ValidateObserverEndpoint verifies that o is either disabled or
// configured with a loopback URL. It returns nil when the endpoint is
// safe to use and a descriptive error otherwise.
//
// Exported so callers and tests can perform the check without
// constructing a Twin.
func ValidateObserverEndpoint(o ObserverEndpoint) error {
	if !o.Enabled {
		return nil
	}
	if o.URL == "" {
		return fmt.Errorf("twincore: ObserverEndpoint.URL must be set when Enabled is true")
	}
	if err := isLocalURL(o.URL); err != nil {
		return fmt.Errorf("twincore: ObserverEndpoint.URL must be loopback (unix://, http://127.0.0.1, http://[::1], http://localhost); got %q: %w", o.URL, err)
	}
	return nil
}

func isLocalURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "unix":
		// unix:///path/to/socket — Path must be present.
		if u.Path == "" {
			return fmt.Errorf("unix scheme requires a socket path")
		}
		return nil
	case "http":
		host := u.Hostname()
		if host == "" {
			return fmt.Errorf("http scheme requires a host")
		}
		if strings.EqualFold(host, "localhost") {
			return nil
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("host %q is not an IP literal or localhost", host)
		}
		if !ip.IsLoopback() {
			return fmt.Errorf("ip %s is not a loopback address", ip)
		}
		return nil
	default:
		return fmt.Errorf("scheme %q is not permitted (allowed: unix, http loopback)", u.Scheme)
	}
}

// localObserver is the forwarder constructed when ObserverEndpoint is
// enabled with a valid loopback URL. The current implementation is a
// no-op that satisfies replay.Observer; the actual local POST/UDS
// transport is added in a follow-up.
type localObserver struct {
	url string
}

func newLocalObserver(o ObserverEndpoint) *localObserver {
	return &localObserver{url: o.URL}
}

// Observe satisfies replay.Observer. It performs no I/O in the current
// implementation.
func (l *localObserver) Observe(_ replay.Entry) {}
