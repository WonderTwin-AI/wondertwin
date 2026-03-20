package twincore

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimitConfig defines rate limiting behavior.
type RateLimitConfig struct {
	Limit  int           `json:"limit"`  // requests per window
	Window time.Duration `json:"window"` // time window
	Scope  string        `json:"scope"`  // "token", "ip", "global"
}

// RateLimiter tracks request counts per scope key within sliding windows.
type RateLimiter struct {
	mu      sync.Mutex
	config  *RateLimitConfig
	windows map[string]*rateLimitWindow
}

type rateLimitWindow struct {
	count   int
	resetAt time.Time
}

// NewRateLimiter creates a new rate limiter. Pass nil config to disable.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		windows: make(map[string]*rateLimitWindow),
	}
}

// Configure sets or updates the rate limit configuration.
func (rl *RateLimiter) Configure(cfg RateLimitConfig) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.config = &cfg
	rl.windows = make(map[string]*rateLimitWindow) // reset on reconfigure
}

// Disable clears the rate limit configuration.
func (rl *RateLimiter) Disable() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.config = nil
	rl.windows = make(map[string]*rateLimitWindow)
}

// Config returns the current config, or nil if disabled.
func (rl *RateLimiter) Config() *RateLimitConfig {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.config == nil {
		return nil
	}
	cfg := *rl.config
	return &cfg
}

// Reset clears all tracked windows.
func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.windows = make(map[string]*rateLimitWindow)
}

// check tests whether a request is allowed and returns remaining count and reset time.
// Returns (allowed, remaining, resetAt).
func (rl *RateLimiter) check(key string, now time.Time) (bool, int, time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.config == nil {
		return true, 0, time.Time{}
	}

	w, ok := rl.windows[key]
	if !ok || now.After(w.resetAt) {
		// New window.
		w = &rateLimitWindow{
			count:   0,
			resetAt: now.Add(rl.config.Window),
		}
		rl.windows[key] = w
	}

	w.count++
	remaining := rl.config.Limit - w.count
	if remaining < 0 {
		remaining = 0
	}

	return w.count <= rl.config.Limit, remaining, w.resetAt
}

// scopeKey extracts the scope key from a request.
func (rl *RateLimiter) scopeKey(r *http.Request) string {
	if rl.config == nil {
		return "global"
	}
	switch rl.config.Scope {
	case "token":
		auth := r.Header.Get("Authorization")
		if auth != "" {
			return "token:" + auth
		}
		// Fall back to API key headers.
		if key := r.Header.Get("X-Api-Key"); key != "" {
			return "token:" + key
		}
		return "token:anonymous"
	case "ip":
		return "ip:" + r.RemoteAddr
	default:
		return "global"
	}
}

// RateLimit returns chi middleware that enforces rate limits.
func (m *Middleware) RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.RateLimiter == nil || m.RateLimiter.Config() == nil {
			next.ServeHTTP(w, r)
			return
		}

		key := m.RateLimiter.scopeKey(r)
		allowed, remaining, resetAt := m.RateLimiter.check(key, time.Now())

		// Always set rate limit headers.
		cfg := m.RateLimiter.Config()
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(cfg.Limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

		if !allowed {
			retryAfter := int(time.Until(resetAt).Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprintf(w, `{"error":{"message":"rate limit exceeded","type":"rate_limit_error"}}`)
			return
		}

		next.ServeHTTP(w, r)
	})
}
