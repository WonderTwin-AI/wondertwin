package twincore

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := NewRateLimiter()
	rl.Configure(RateLimitConfig{Limit: 5, Window: time.Minute, Scope: "global"})

	now := time.Now()
	for i := 0; i < 5; i++ {
		allowed, _, _ := rl.check("global", now)
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_RejectsOverLimit(t *testing.T) {
	rl := NewRateLimiter()
	rl.Configure(RateLimitConfig{Limit: 3, Window: time.Minute, Scope: "global"})

	now := time.Now()
	for i := 0; i < 3; i++ {
		rl.check("global", now)
	}

	allowed, remaining, _ := rl.check("global", now)
	if allowed {
		t.Error("4th request should be rejected")
	}
	if remaining != 0 {
		t.Errorf("remaining = %d, want 0", remaining)
	}
}

func TestRateLimiter_WindowResets(t *testing.T) {
	rl := NewRateLimiter()
	rl.Configure(RateLimitConfig{Limit: 2, Window: time.Second, Scope: "global"})

	now := time.Now()
	rl.check("global", now)
	rl.check("global", now)

	// Over limit.
	allowed, _, _ := rl.check("global", now)
	if allowed {
		t.Error("should be rate limited")
	}

	// After window resets.
	future := now.Add(2 * time.Second)
	allowed, _, _ = rl.check("global", future)
	if !allowed {
		t.Error("should be allowed after window reset")
	}
}

func TestRateLimiter_DisabledPassesAll(t *testing.T) {
	rl := NewRateLimiter() // no config = disabled

	allowed, _, _ := rl.check("anything", time.Now())
	if !allowed {
		t.Error("disabled limiter should allow all")
	}
}

func TestRateLimiter_PerTokenScope(t *testing.T) {
	rl := NewRateLimiter()
	rl.Configure(RateLimitConfig{Limit: 2, Window: time.Minute, Scope: "token"})

	now := time.Now()

	// Token A uses 2 requests.
	rl.check("token:A", now)
	rl.check("token:A", now)

	// Token A is rate limited.
	allowed, _, _ := rl.check("token:A", now)
	if allowed {
		t.Error("token A should be limited")
	}

	// Token B still has quota.
	allowed, _, _ = rl.check("token:B", now)
	if !allowed {
		t.Error("token B should still be allowed")
	}
}

func TestRateLimit_Middleware(t *testing.T) {
	cfg := &Config{Name: "test", Port: 0}
	mw := NewMiddleware(cfg, nil)
	mw.RateLimiter.Configure(RateLimitConfig{Limit: 2, Window: time.Minute, Scope: "global"})

	handler := mw.RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 2 requests succeed.
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want 200", i+1, w.Code)
		}
		if w.Header().Get("X-RateLimit-Limit") != "2" {
			t.Error("missing X-RateLimit-Limit header")
		}
	}

	// 3rd request is rate limited.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("3rd request: status = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After header on 429")
	}
}
