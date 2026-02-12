// Package admin provides the shared /admin/* control plane handlers
// used by all WonderTwin twins for state management, fault injection, and inspection.
package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/pkg/store"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
)

// StateStore is the interface a twin must implement to support admin state management.
type StateStore interface {
	// Snapshot returns the full state as a JSON-serializable value.
	Snapshot() any
	// LoadState replaces the full state from a JSON body.
	LoadState(data []byte) error
	// Reset clears all state and optionally reloads seed data.
	Reset()
}

// WebhookFlusher is optionally implemented by twins that have pending webhooks.
type WebhookFlusher interface {
	FlushWebhooks() error
}

// Handler provides the shared admin endpoints.
type Handler struct {
	state   StateStore
	flusher WebhookFlusher
	mw      *twincore.Middleware
	clock   *store.Clock
}

// NewHandler creates a new admin handler.
func NewHandler(state StateStore, mw *twincore.Middleware, clock *store.Clock) *Handler {
	return &Handler{
		state: state,
		mw:    mw,
		clock: clock,
	}
}

// SetFlusher sets the webhook flusher (optional).
func (h *Handler) SetFlusher(f WebhookFlusher) {
	h.flusher = f
}

// Routes mounts the admin endpoints on the given router.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/admin", func(r chi.Router) {
		r.Post("/reset", h.handleReset)
		r.Get("/state", h.handleGetState)
		r.Post("/state", h.handleLoadState)
		r.Post("/fault/{endpoint}", h.handleInjectFault)
		r.Delete("/fault/{endpoint}", h.handleRemoveFault)
		r.Get("/faults", h.handleListFaults)
		r.Get("/requests", h.handleGetRequests)
		r.Post("/webhooks/flush", h.handleFlushWebhooks)
		r.Post("/time/advance", h.handleTimeAdvance)
		r.Get("/time", h.handleGetTime)
		r.Get("/health", h.handleHealth)
	})
}

func (h *Handler) handleReset(w http.ResponseWriter, r *http.Request) {
	h.state.Reset()
	h.mw.ReqLog.Clear()
	h.mw.Faults.Reset()
	h.mw.Idempotent.Reset()
	if h.clock != nil {
		h.clock.Reset()
	}
	twincore.JSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func (h *Handler) handleGetState(w http.ResponseWriter, r *http.Request) {
	twincore.JSON(w, http.StatusOK, h.state.Snapshot())
}

func (h *Handler) handleLoadState(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		twincore.Error(w, http.StatusBadRequest, "failed to read body: "+err.Error())
		return
	}
	if err := h.state.LoadState(body); err != nil {
		twincore.Error(w, http.StatusBadRequest, "failed to load state: "+err.Error())
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]string{"status": "loaded"})
}

func (h *Handler) handleInjectFault(w http.ResponseWriter, r *http.Request) {
	endpoint := "/" + chi.URLParam(r, "endpoint")

	var fault twincore.FaultConfig
	if err := json.NewDecoder(r.Body).Decode(&fault); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid fault config: "+err.Error())
		return
	}
	h.mw.Faults.Set(endpoint, fault)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"status":   "injected",
		"endpoint": endpoint,
		"fault":    fault,
	})
}

func (h *Handler) handleRemoveFault(w http.ResponseWriter, r *http.Request) {
	endpoint := "/" + chi.URLParam(r, "endpoint")
	if h.mw.Faults.Remove(endpoint) {
		twincore.JSON(w, http.StatusOK, map[string]any{"status": "removed", "endpoint": endpoint})
	} else {
		twincore.Error(w, http.StatusNotFound, "no fault registered for "+endpoint)
	}
}

func (h *Handler) handleListFaults(w http.ResponseWriter, r *http.Request) {
	twincore.JSON(w, http.StatusOK, h.mw.Faults.All())
}

func (h *Handler) handleGetRequests(w http.ResponseWriter, r *http.Request) {
	twincore.JSON(w, http.StatusOK, h.mw.ReqLog.Entries())
}

func (h *Handler) handleFlushWebhooks(w http.ResponseWriter, r *http.Request) {
	if h.flusher == nil {
		twincore.JSON(w, http.StatusOK, map[string]string{"status": "no webhooks configured"})
		return
	}
	if err := h.flusher.FlushWebhooks(); err != nil {
		twincore.Error(w, http.StatusInternalServerError, "flush failed: "+err.Error())
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]string{"status": "flushed"})
}

func (h *Handler) handleTimeAdvance(w http.ResponseWriter, r *http.Request) {
	if h.clock == nil {
		twincore.Error(w, http.StatusBadRequest, "simulated clock not configured")
		return
	}

	var req struct {
		Duration string `json:"duration"` // Go duration string, e.g., "24h", "30m"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	d, err := time.ParseDuration(req.Duration)
	if err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid duration: "+err.Error())
		return
	}

	h.clock.Advance(d)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"status":     "advanced",
		"duration":   d.String(),
		"offset":     h.clock.Offset().String(),
		"simulated":  h.clock.Now().Format(time.RFC3339),
	})
}

func (h *Handler) handleGetTime(w http.ResponseWriter, r *http.Request) {
	if h.clock == nil {
		twincore.JSON(w, http.StatusOK, map[string]any{
			"real": time.Now().Format(time.RFC3339),
		})
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]any{
		"real":      time.Now().Format(time.RFC3339),
		"simulated": h.clock.Now().Format(time.RFC3339),
		"offset":    h.clock.Offset().String(),
	})
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	twincore.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
