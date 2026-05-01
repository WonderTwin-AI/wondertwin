package scrub

import "testing"

func TestDefaultHeaderScrubbing(t *testing.T) {
	t.Parallel()
	s := Default()

	cases := []struct {
		name, value string
		want        string
	}{
		{"Authorization", "Bearer abc", "REDACTED"},
		{"authorization", "Bearer abc", "REDACTED"},
		{"X-Api-Key", "sk_live_xxx", "REDACTED"},
		{"Stripe-Account", "acct_123", "REDACTED"},
		{"Set-Cookie", "sid=abc; Path=/", "REDACTED"},
		{"Cookie", "sid=abc", "REDACTED"},
		{"Content-Type", "application/json", "application/json"},
		{"X-Trace-Id", "trace-1", "trace-1"},
	}

	for _, tc := range cases {
		got := s.ScrubHeader(tc.name, tc.value)
		if got != tc.want {
			t.Errorf("ScrubHeader(%q, %q) = %q, want %q", tc.name, tc.value, got, tc.want)
		}
	}
}

func TestScrubberSetAddHeaderOrder(t *testing.T) {
	t.Parallel()
	s := NewSet()
	s.AddHeader(func(_, v string) string { return v + "-1" })
	s.AddHeader(func(_, v string) string { return v + "-2" })

	got := s.ScrubHeader("X-Test", "raw")
	if got != "raw-1-2" {
		t.Errorf("scrubbers ran out of order: got %q", got)
	}
}

func TestScrubberSetDropsHeaderOnEmpty(t *testing.T) {
	t.Parallel()
	s := NewSet()
	s.AddHeader(func(_, _ string) string { return "" })
	s.AddHeader(func(_, v string) string {
		t.Fatalf("second scrubber must not run after drop")
		return v
	})
	if got := s.ScrubHeader("X-Test", "v"); got != "" {
		t.Errorf("expected empty value, got %q", got)
	}
}

func TestScrubberSetBodyChaining(t *testing.T) {
	t.Parallel()
	s := NewSet()
	s.AddBody(func(_ string, b []byte) []byte { return append(b, '!') })
	s.AddBody(func(_ string, b []byte) []byte { return append(b, '?') })

	got := string(s.ScrubBody("text/plain", []byte("hi")))
	if got != "hi!?" {
		t.Errorf("ScrubBody chaining = %q, want %q", got, "hi!?")
	}
}

func TestScrubberSetIgnoresNil(t *testing.T) {
	t.Parallel()
	s := NewSet()
	s.AddHeader(nil)
	s.AddBody(nil)
	if got := s.ScrubHeader("X-Test", "v"); got != "v" {
		t.Errorf("ScrubHeader passthrough failed: %q", got)
	}
	if got := string(s.ScrubBody("", []byte("v"))); got != "v" {
		t.Errorf("ScrubBody passthrough failed: %q", got)
	}
}
