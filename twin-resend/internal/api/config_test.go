package api_test

import (
	"testing"
	"time"
)

func TestDomainVerificationModes(t *testing.T) {
	tc := setupResendWithConfig(t)
	for _, outcome := range []string{"pending", "failed", "verified"} {
		tc.DoWithHeaders("PUT", "/admin/config", map[string]any{"domain_verification": outcome}, nil).AssertStatus(200)
		id := resendPost(tc, "/domains", map[string]any{"name": outcome + ".example.com"}).AssertStatus(201).JSONMap()["id"].(string)
		resendPost(tc, "/domains/"+id+"/verify", nil).AssertStatus(200)
		got := resendGet(tc, "/domains/"+id).AssertStatus(200).JSONMap()
		if got["status"] != outcome {
			t.Errorf("domain_verification %s: domain status %v", outcome, got["status"])
		}
		for _, rec := range got["records"].([]any) {
			if rec.(map[string]any)["status"] != outcome {
				t.Errorf("domain_verification %s: record status %v", outcome, rec.(map[string]any)["status"])
			}
		}
	}
	tc.DoWithHeaders("PUT", "/admin/config", map[string]any{"domain_verification": "partially"}, nil).AssertStatus(400)
}

func TestRateLimitOptIn(t *testing.T) {
	tc, st := setupResendWithStore(t)
	st.Clock.Pin(time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC))

	// Off by default: Resend's default headers, never 429.
	for i := 0; i < 15; i++ {
		resp := resendGet(tc, "/domains").AssertStatus(200)
		if resp.Headers.Get("ratelimit-limit") != "10" {
			t.Fatalf("default ratelimit-limit: %q", resp.Headers.Get("ratelimit-limit"))
		}
	}

	tc.DoWithHeaders("PUT", "/admin/config", map[string]any{"rate_limit": 2}, nil).AssertStatus(200)
	first := resendGet(tc, "/domains").AssertStatus(200)
	if first.Headers.Get("ratelimit-policy") != "2;w=1" || first.Headers.Get("ratelimit-remaining") != "1" {
		t.Errorf("first request headers: policy %q remaining %q",
			first.Headers.Get("ratelimit-policy"), first.Headers.Get("ratelimit-remaining"))
	}
	resendGet(tc, "/domains").AssertStatus(200)
	limited := resendGet(tc, "/domains").AssertStatus(429).
		AssertBodyContains(`"name":"rate_limit_exceeded"`).
		AssertBodyContains("Too many requests.")
	if limited.Headers.Get("retry-after") != "1" || limited.Headers.Get("ratelimit-remaining") != "0" {
		t.Errorf("429 headers: retry-after %q remaining %q",
			limited.Headers.Get("retry-after"), limited.Headers.Get("ratelimit-remaining"))
	}

	// The next one-second window of store-clock time admits again.
	st.Clock.Pin(time.Date(2026, 11, 6, 12, 0, 1, 0, time.UTC))
	resendGet(tc, "/domains").AssertStatus(200)

	tc.DoWithHeaders("PUT", "/admin/config", map[string]any{"rate_limit": -1}, nil).AssertStatus(400)
	tc.DoWithHeaders("PUT", "/admin/config", map[string]any{"rate_limit": 0}, nil).AssertStatus(200)
	resendGet(tc, "/domains").AssertStatus(200)
}
