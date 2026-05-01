// Package admin provides the shared /admin/* control plane handlers
// used by all WonderTwin twins for state management, fault injection, and inspection.
package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/replay"
	"github.com/wondertwin-ai/wondertwin/twinkit/state"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
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

// ConfigProvider exposes runtime configuration for reading and updating.
type ConfigProvider interface {
	GetConfig() map[string]any
	UpdateConfig(updates map[string]any) error
}

// RunIDSetter is implemented by twins that can record an active run id.
// When set on a Handler, POST /admin/reset?run_id=<id> forwards the id
// before clearing transient state.
type RunIDSetter interface {
	SetRunID(runID string)
}

// Reseeder is implemented by twins whose deterministic random source
// supports re-seeding. POST /admin/runs/start invokes this when a
// `seed` is supplied in the request body.
type Reseeder interface {
	Reseed(seed int64)
}

// telemetryEmitter is the package-private contract h.tel satisfies;
// it lets admin tests stub the reporter without depending on
// telemetry.Reporter directly. Callers wire the production reporter
// via SetTelemetry which accepts the concrete *telemetry.Reporter.
type telemetryEmitter interface {
	Record(ev telemetry.Event)
}

// RunAttribution is implemented by callers that can supply telemetry
// attribution fields for run lifecycle events. The admin handler is
// pure plumbing — twin metadata flows through this interface so
// admin tests can stub it.
//
// ModeString returns "ci" or "local"; the method is named to avoid
// colliding with Twin.Mode (a cimode.Mode field).
type RunAttribution interface {
	TwinName() string
	TwinVersion() string
	ModeString() string
	Platform() string
	OrgHash() string
	LicenseID() string
	LicenseOK() bool
	LicenseReason() string
}

// QuirkStore manages behavioral quirks that can be toggled at runtime.
type QuirkStore interface {
	ListQuirks() []QuirkStatus
	EnableQuirk(id string) error
	DisableQuirk(id string) error
	IsEnabled(id string) bool
}

// QuirkStatus describes the state of a single quirk.
type QuirkStatus struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Enabled  bool   `json:"enabled"`
	Type     string `json:"type"`
	Severity string `json:"severity"`
}

// Handler provides the shared admin endpoints.
type Handler struct {
	state    StateStore
	flusher  WebhookFlusher
	mw       *twincore.Middleware
	clock    *state.Clock
	config   ConfigProvider
	quirks   QuirkStore
	runIDs   RunIDSetter
	recorder *replay.Recorder
	rand     Reseeder
	tel      telemetryEmitter
	attr     RunAttribution

	runMu        sync.Mutex
	currentRun   string
	startedAt    time.Time
	currentSeed  int64
	currentSeedSrc string
}

// NewHandler creates a new admin handler.
func NewHandler(state StateStore, mw *twincore.Middleware, clock *state.Clock) *Handler {
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

// SetConfigProvider sets the config provider (optional).
func (h *Handler) SetConfigProvider(cp ConfigProvider) {
	h.config = cp
}

// SetQuirkStore sets the quirk store (optional).
func (h *Handler) SetQuirkStore(qs QuirkStore) {
	h.quirks = qs
}

// SetRunIDSetter wires a RunIDSetter (typically the Twin) so
// POST /admin/reset?run_id=<id> can forward the run identifier.
func (h *Handler) SetRunIDSetter(s RunIDSetter) {
	h.runIDs = s
}

// SetReplayRecorder wires a replay.Recorder so the /admin/replay
// (GET) and /admin/runs/* (POST/GET) endpoints can serve recorded
// traffic and drive the run lifecycle. Optional; routes return 404
// when unset.
func (h *Handler) SetReplayRecorder(r *replay.Recorder) {
	h.recorder = r
}

// SetReseeder wires a deterministic random source (typically Twin.Rand)
// so POST /admin/runs/start can apply the supplied seed.
func (h *Handler) SetReseeder(r Reseeder) {
	h.rand = r
}

// SetTelemetry wires a telemetry reporter so the admin handler can
// record run_start / run_finish events. Optional.
func (h *Handler) SetTelemetry(r *telemetry.Reporter) {
	if r == nil {
		h.tel = nil
		return
	}
	h.tel = r
}

// SetTelemetryEmitter wires a custom telemetry emitter (used by
// tests). Production callers should prefer SetTelemetry which accepts
// the concrete *telemetry.Reporter.
func (h *Handler) SetTelemetryEmitter(e interface{ Record(telemetry.Event) }) {
	h.tel = e
}

// SetRunAttribution wires the source of run-event attribution fields
// (twin name/version/mode/platform/org/license).
func (h *Handler) SetRunAttribution(a RunAttribution) {
	h.attr = a
}

// TwinSubject aggregates the interfaces a Twin satisfies for admin
// wiring. WireFromTwin uses this so each twin's main.go can call a
// single helper instead of invoking the full setter list.
type TwinSubject interface {
	RunIDSetter
	RunAttribution
	Reseeder
	ReplayRecorder() *replay.Recorder
	TelemetryReporter() *telemetry.Reporter
}

// WireFromTwin connects every optional run-lifecycle dependency from
// a Twin-shaped object. Twins call this once after admin.NewHandler
// instead of invoking each setter individually.
func (h *Handler) WireFromTwin(t TwinSubject) {
	h.SetRunIDSetter(t)
	h.SetRunAttribution(t)
	h.SetReseeder(t)
	if rec := t.ReplayRecorder(); rec != nil {
		h.SetReplayRecorder(rec)
	}
	if rep := t.TelemetryReporter(); rep != nil {
		h.SetTelemetry(rep)
	}
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
		r.Get("/config", h.handleGetConfig)
		r.Put("/config", h.handleUpdateConfig)
		r.Get("/quirks", h.handleListQuirks)
		r.Put("/quirks/{quirk_id}", h.handleEnableQuirk)
		r.Delete("/quirks/{quirk_id}", h.handleDisableQuirk)
		r.Get("/replay", h.handleReplayGet)
		r.Post("/runs/start", h.handleRunStart)
		r.Post("/runs/finish", h.handleRunFinish)
		r.Get("/runs/current", h.handleRunCurrent)
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
	resp := map[string]string{"status": "reset"}
	if runID := r.URL.Query().Get("run_id"); runID != "" {
		if h.runIDs != nil {
			h.runIDs.SetRunID(runID)
		}
		resp["run_id"] = runID
	}
	twincore.JSON(w, http.StatusOK, resp)
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
		"status":    "advanced",
		"duration":  d.String(),
		"offset":    h.clock.Offset().String(),
		"simulated": h.clock.Now().Format(time.RFC3339),
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

func (h *Handler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if h.config == nil {
		twincore.Error(w, http.StatusNotFound, "config provider not configured")
		return
	}
	twincore.JSON(w, http.StatusOK, h.config.GetConfig())
}

func (h *Handler) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if h.config == nil {
		twincore.Error(w, http.StatusNotFound, "config provider not configured")
		return
	}

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid config body: "+err.Error())
		return
	}

	if err := h.config.UpdateConfig(updates); err != nil {
		twincore.Error(w, http.StatusBadRequest, "failed to update config: "+err.Error())
		return
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"status": "updated",
		"config": h.config.GetConfig(),
	})
}

func (h *Handler) handleListQuirks(w http.ResponseWriter, r *http.Request) {
	if h.quirks == nil {
		twincore.JSON(w, http.StatusOK, []QuirkStatus{})
		return
	}
	twincore.JSON(w, http.StatusOK, h.quirks.ListQuirks())
}

func (h *Handler) handleEnableQuirk(w http.ResponseWriter, r *http.Request) {
	if h.quirks == nil {
		twincore.Error(w, http.StatusNotFound, "quirk store not configured")
		return
	}
	id := chi.URLParam(r, "quirk_id")
	if err := h.quirks.EnableQuirk(id); err != nil {
		twincore.Error(w, http.StatusNotFound, "quirk not found: "+err.Error())
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]any{"status": "enabled", "quirk_id": id})
}

func (h *Handler) handleDisableQuirk(w http.ResponseWriter, r *http.Request) {
	if h.quirks == nil {
		twincore.Error(w, http.StatusNotFound, "quirk store not configured")
		return
	}
	id := chi.URLParam(r, "quirk_id")
	if err := h.quirks.DisableQuirk(id); err != nil {
		twincore.Error(w, http.StatusNotFound, "quirk not found: "+err.Error())
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]any{"status": "disabled", "quirk_id": id})
}
