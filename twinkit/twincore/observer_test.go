package twincore

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateObserverEndpoint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      ObserverEndpoint
		wantErr bool
	}{
		{"disabled with empty url", ObserverEndpoint{}, false},
		{"disabled with public url is fine", ObserverEndpoint{URL: "http://example.com"}, false},
		{"unix socket", ObserverEndpoint{Enabled: true, URL: "unix:///tmp/x.sock"}, false},
		{"loopback ipv4", ObserverEndpoint{Enabled: true, URL: "http://127.0.0.1:9999"}, false},
		{"loopback ipv4 with path", ObserverEndpoint{Enabled: true, URL: "http://127.0.0.1:9999/observe"}, false},
		{"loopback ipv6", ObserverEndpoint{Enabled: true, URL: "http://[::1]:9999"}, false},
		{"localhost", ObserverEndpoint{Enabled: true, URL: "http://localhost:9999"}, false},
		{"empty url with enabled", ObserverEndpoint{Enabled: true}, true},
		{"public hostname", ObserverEndpoint{Enabled: true, URL: "http://example.com:9999"}, true},
		{"public ipv4", ObserverEndpoint{Enabled: true, URL: "http://10.0.0.1:9999"}, true},
		{"https loopback rejected", ObserverEndpoint{Enabled: true, URL: "https://127.0.0.1:9999"}, true},
		{"tcp scheme rejected", ObserverEndpoint{Enabled: true, URL: "tcp://127.0.0.1:9999"}, true},
		{"unix without path", ObserverEndpoint{Enabled: true, URL: "unix://"}, true},
		{"malformed url", ObserverEndpoint{Enabled: true, URL: "://bad"}, true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateObserverEndpoint(tc.in)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateObserverEndpoint(%+v) = nil, want error", tc.in)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateObserverEndpoint(%+v) = %v, want nil", tc.in, err)
			}
			if tc.wantErr && tc.in.Enabled && tc.in.URL != "" && !strings.Contains(err.Error(), "twincore:") {
				t.Errorf("error should be prefixed with twincore: got %q", err)
			}
		})
	}
}

func TestNewRejectsNonLocalObserverEndpointAtServe(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Name: "twin-test",
		Port: 0,
		ObserverEndpoint: ObserverEndpoint{
			Enabled: true,
			URL:     "http://example.com:9999",
		},
	}
	twin := New(cfg)
	if twin.configErr == nil {
		t.Fatal("New must record a config error for non-local observer URL")
	}
	err := twin.Serve()
	if err == nil {
		t.Fatal("Serve must return the config error")
	}
	if !errors.Is(err, twin.configErr) && err.Error() != twin.configErr.Error() {
		t.Errorf("Serve returned %v, want config error %v", err, twin.configErr)
	}
}

func TestNewAcceptsLoopbackObserverEndpoint(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Name: "twin-test",
		Port: 0,
		ObserverEndpoint: ObserverEndpoint{
			Enabled: true,
			URL:     "http://127.0.0.1:9999/observe",
		},
	}
	twin := New(cfg)
	if twin.configErr != nil {
		t.Fatalf("New rejected loopback URL: %v", twin.configErr)
	}
}

func TestNewLocalObserverSatisfiesObserverInterface(t *testing.T) {
	t.Parallel()
	o := newLocalObserver(ObserverEndpoint{Enabled: true, URL: "http://127.0.0.1:9999"})
	o.Observe(replayEntryForTest()) // must not panic
}
