package api

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// rateLimiter is Resend's per-team rate limit, off unless /admin/config sets
// rate_limit (architecture decision 10, quirk throttle-001). Windows are one
// second of store-clock time, so a pinned clock holds a window open until
// the test advances it.
type rateLimiter struct {
	mu      sync.Mutex
	max     int
	start   time.Time
	count   int
	nowFunc func() time.Time
}

func (l *rateLimiter) configure(n int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.max, l.count, l.start = n, 0, time.Time{}
}

func (l *rateLimiter) limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.max
}

// rateLimitMsg is Resend's 429 message, without the docs' support link.
const rateLimitMsg = "Too many requests. Please limit the number of requests per second. Or contact support to increase rate limit."

// admit counts one request, sets Resend's ratelimit headers and, past the
// limit, answers 429 rate_limit_exceeded itself and returns false. With the
// limit off it sends Resend's default headers with fixed values.
func (l *rateLimiter) admit(w http.ResponseWriter) bool {
	l.mu.Lock()
	max := l.max
	if max == 0 {
		l.mu.Unlock()
		setRateLimitHeaders(w, 10, 9, 1)
		return true
	}
	now := l.nowFunc()
	if l.start.IsZero() || !now.Before(l.start.Add(time.Second)) {
		l.start, l.count = now, 0
	}
	l.count++
	count := l.count
	reset := int((l.start.Add(time.Second).Sub(now) + time.Second - 1) / time.Second)
	l.mu.Unlock()
	if reset < 1 {
		reset = 1
	}

	remaining := max - count
	if remaining < 0 {
		remaining = 0
	}
	setRateLimitHeaders(w, max, remaining, reset)
	if count > max {
		w.Header().Set("retry-after", strconv.Itoa(reset))
		writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", rateLimitMsg)
		return false
	}
	return true
}

// setRateLimitHeaders sends Resend's ratelimit headers (quirks throttle-001,
// throttle-003). With the limit off the values are Resend's default, 10
// requests per one-second window, as one request into a fresh window.
func setRateLimitHeaders(w http.ResponseWriter, limit, remaining, reset int) {
	h := w.Header()
	h.Set("ratelimit-limit", strconv.Itoa(limit))
	h.Set("ratelimit-policy", strconv.Itoa(limit)+";w=1")
	h.Set("ratelimit-remaining", strconv.Itoa(remaining))
	h.Set("ratelimit-reset", strconv.Itoa(reset))
}
