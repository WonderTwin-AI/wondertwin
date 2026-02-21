package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-loyaltylion/internal/store"
)

type contextKey string

const merchantAPIKeyCtxKey contextKey = "merchant_api_key"

// Handler holds all API handler state.
type Handler struct {
	store *store.MemoryStore
	mw    *twincore.Middleware

	// Rate limit tracking per API key
	rateMu       sync.Mutex
	rateCounters map[string]*rateCounter
}

type rateCounter struct {
	count     int
	windowEnd time.Time
}

// NewHandler creates a new API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	return &Handler{
		store:        s,
		mw:           mw,
		rateCounters: make(map[string]*rateCounter),
	}
}

// Routes mounts the API endpoints.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/v2", func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.rateLimitHeaders)
		r.Use(h.mw.FaultInjection)

		// Customers
		r.Get("/customers", h.ListCustomers)
		r.Post("/customers", h.CreateCustomer)
		r.Get("/customers/{id}", h.GetCustomer)

		// Points (by merchant_id)
		r.Get("/customers/{merchant_id}/points", h.GetPoints)
		r.Post("/customers/{merchant_id}/points", h.AddPoints)
		r.Post("/customers/{merchant_id}/points/remove", h.RemovePoints)

		// Rewards & Redemptions (by merchant_id)
		r.Get("/customers/{merchant_id}/available_rewards", h.ListAvailableRewards)
		r.Post("/customers/{merchant_id}/claimed_rewards", h.ClaimReward)
		r.Post("/customers/{merchant_id}/claimed_rewards/{id}/refund", h.RefundClaimedReward)

		// Activities
		r.Post("/activities", h.RecordActivity)
	})
}

// authMiddleware validates Basic auth and maps API key to merchant.
func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			twincore.Error(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		if !strings.HasPrefix(auth, "Basic ") {
			twincore.Error(w, http.StatusUnauthorized, "invalid authorization format")
			return
		}

		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
		if err != nil {
			twincore.Error(w, http.StatusUnauthorized, "invalid base64 encoding")
			return
		}

		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			twincore.Error(w, http.StatusUnauthorized, "invalid credentials format")
			return
		}

		apiKey := parts[0]
		apiSecret := parts[1]

		merchant, ok := h.store.GetMerchantByAPIKey(apiKey)
		if !ok {
			twincore.Error(w, http.StatusUnauthorized, "invalid API key")
			return
		}

		if merchant.APISecret != apiSecret {
			twincore.Error(w, http.StatusUnauthorized, "invalid API secret")
			return
		}

		ctx := context.WithValue(r.Context(), merchantAPIKeyCtxKey, apiKey)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// getAPIKey extracts the authenticated merchant's API key from context.
func getAPIKey(r *http.Request) string {
	return r.Context().Value(merchantAPIKeyCtxKey).(string)
}

// rateLimitHeaders adds X-RateLimit-* headers to responses.
func (h *Handler) rateLimitHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := getAPIKey(r)
		now := time.Now()

		h.rateMu.Lock()
		rc, ok := h.rateCounters[apiKey]
		if !ok || now.After(rc.windowEnd) {
			rc = &rateCounter{count: 0, windowEnd: now.Add(time.Second)}
			h.rateCounters[apiKey] = rc
		}
		rc.count++
		remaining := 20 - rc.count
		if remaining < 0 {
			remaining = 0
		}
		h.rateMu.Unlock()

		w.Header().Set("X-RateLimit-Limit", "20")
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		next.ServeHTTP(w, r)
	})
}
