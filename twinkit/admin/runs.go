package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/replay"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// startRunRequest is the POST /admin/runs/start body shape.
type startRunRequest struct {
	RunID    string          `json:"run_id"`
	Seed     int64           `json:"seed"`
	Fixtures json.RawMessage `json:"fixtures"`
}

// startRunResponse is the canonical reply.
type startRunResponse struct {
	RunID      string    `json:"run_id"`
	StartedAt  time.Time `json:"started_at"`
	Seed       int64     `json:"seed,omitempty"`
	Seeded     bool      `json:"seeded"`
	SeedSource string    `json:"seed_source"`
}

// finishRunRequest is the POST /admin/runs/finish body shape (all
// fields optional).
type finishRunRequest struct {
	RunID string `json:"run_id"`
}

// currentRunResponse is the GET /admin/runs/current shape.
type currentRunResponse struct {
	RunID      string    `json:"run_id"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	EntryCount int       `json:"entry_count"`
	Seeded     bool      `json:"seeded,omitempty"`
	SeedSource string    `json:"seed_source,omitempty"`
}

func (h *Handler) handleReplayGet(w http.ResponseWriter, r *http.Request) {
	if h.recorder == nil {
		twincore.JSON(w, http.StatusOK, replay.Manifest{SchemaVersion: replay.SchemaVersion})
		return
	}
	if r.URL.Query().Get("format") == "manifest" {
		twincore.JSON(w, http.StatusOK, h.recorder.CurrentManifest())
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	if err := h.recorder.Export(w); err != nil {
		twincore.Error(w, http.StatusInternalServerError, "export failed: "+err.Error())
		return
	}
}

func (h *Handler) handleRunStart(w http.ResponseWriter, r *http.Request) {
	if h.recorder == nil {
		twincore.Error(w, http.StatusNotFound, "replay recorder not configured")
		return
	}
	var req startRunRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	h.runMu.Lock()
	defer h.runMu.Unlock()

	// Auto-finish any active previous run before starting fresh.
	if h.currentRun != "" {
		prevManifest := h.recorder.FinishRun()
		h.emitRunFinish(prevManifest, h.startedAt)
		h.currentRun = ""
	}

	// Reset transient state in the same order /admin/reset uses.
	h.state.Reset()
	h.mw.ReqLog.Clear()
	h.mw.Faults.Reset()
	h.mw.Idempotent.Reset()
	if h.clock != nil {
		h.clock.Reset()
	}

	// Reseed the deterministic RNG when a seed is supplied. Zero is
	// treated as "no seed change" — callers wanting an explicit zero
	// seed should not send the field at all.
	seeded := req.Seed != 0
	if seeded && h.rand != nil {
		h.rand.Reseed(req.Seed)
	}

	// Apply fixtures via StateStore.LoadState. The blob is
	// twin-specific JSON; the admin handler is format-agnostic and
	// just forwards bytes.
	seedSource := "none"
	if len(req.Fixtures) > 0 {
		if err := h.state.LoadState(req.Fixtures); err != nil {
			twincore.Error(w, http.StatusBadRequest, "load fixtures: "+err.Error())
			return
		}
		seedSource = "byo"
	}

	runID := req.RunID
	if runID == "" {
		runID = generateRunID()
	}
	now := time.Now().UTC()

	if h.runIDs != nil {
		h.runIDs.SetRunID(runID)
	}
	h.recorder.StartRun(runID)
	h.currentRun = runID
	h.startedAt = now
	h.currentSeed = req.Seed
	h.currentSeedSrc = seedSource

	h.emitRunStart(runID, seeded, seedSource, now)

	twincore.JSON(w, http.StatusOK, startRunResponse{
		RunID:      runID,
		StartedAt:  now,
		Seed:       req.Seed,
		Seeded:     seeded,
		SeedSource: seedSource,
	})
}

func (h *Handler) handleRunFinish(w http.ResponseWriter, r *http.Request) {
	if h.recorder == nil {
		twincore.Error(w, http.StatusNotFound, "replay recorder not configured")
		return
	}
	var req finishRunRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	h.runMu.Lock()
	defer h.runMu.Unlock()

	if h.currentRun == "" {
		twincore.JSON(w, http.StatusNotFound, map[string]string{"error": "no run active"})
		return
	}
	if req.RunID != "" && req.RunID != h.currentRun {
		twincore.JSON(w, http.StatusConflict, map[string]string{
			"error":   "run_id mismatch",
			"current": h.currentRun,
		})
		return
	}

	startedAt := h.startedAt
	manifest := h.recorder.FinishRun()
	h.emitRunFinish(manifest, startedAt)

	if h.runIDs != nil {
		h.runIDs.SetRunID("")
	}
	h.currentRun = ""
	h.startedAt = time.Time{}
	h.currentSeed = 0
	h.currentSeedSrc = ""

	twincore.JSON(w, http.StatusOK, manifest)
}

func (h *Handler) handleRunCurrent(w http.ResponseWriter, _ *http.Request) {
	h.runMu.Lock()
	defer h.runMu.Unlock()

	resp := currentRunResponse{}
	if h.currentRun == "" {
		twincore.JSON(w, http.StatusOK, resp)
		return
	}
	resp.RunID = h.currentRun
	resp.StartedAt = h.startedAt
	resp.Seeded = h.currentSeed != 0
	resp.SeedSource = h.currentSeedSrc
	if h.recorder != nil {
		resp.EntryCount = h.recorder.CurrentManifest().EntryCount
	}
	twincore.JSON(w, http.StatusOK, resp)
}

// emitRunStart sends a run_start telemetry event when an emitter is
// configured. Best-effort; non-blocking by Reporter contract.
func (h *Handler) emitRunStart(runID string, seeded bool, seedSource string, ts time.Time) {
	if h.tel == nil {
		return
	}
	ev := h.attribute(telemetry.Event{
		Type:       "run_start",
		RunID:      runID,
		Seeded:     seeded,
		SeedSource: seedSource,
		Timestamp:  ts,
	})
	h.tel.Record(ev)
}

// emitRunFinish sends a run_finish telemetry event keyed off the
// supplied manifest.
func (h *Handler) emitRunFinish(manifest replay.Manifest, startedAt time.Time) {
	if h.tel == nil {
		return
	}
	finishedAt := manifest.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	durationMs := 0
	if !startedAt.IsZero() {
		durationMs = int(finishedAt.Sub(startedAt) / time.Millisecond)
	}
	ev := h.attribute(telemetry.Event{
		Type:       "run_finish",
		RunID:      manifest.RunID,
		EntryCount: manifest.EntryCount,
		DurationMs: durationMs,
		Timestamp:  finishedAt,
	})
	h.tel.Record(ev)
}

// attribute fills in the common attribution fields on ev from the
// configured RunAttribution. ev's existing fields take precedence.
func (h *Handler) attribute(ev telemetry.Event) telemetry.Event {
	if h.attr == nil {
		return ev
	}
	if ev.TwinName == "" {
		ev.TwinName = h.attr.TwinName()
	}
	if ev.TwinVer == "" {
		ev.TwinVer = h.attr.TwinVersion()
	}
	if ev.Mode == "" {
		ev.Mode = h.attr.ModeString()
	}
	if ev.Platform == "" {
		ev.Platform = h.attr.Platform()
	}
	if ev.OrgHash == "" {
		ev.OrgHash = h.attr.OrgHash()
	}
	if ev.LicenseID == "" {
		ev.LicenseID = h.attr.LicenseID()
	}
	if !ev.LicenseOK {
		ev.LicenseOK = h.attr.LicenseOK()
	}
	if ev.LicenseRsn == "" {
		ev.LicenseRsn = h.attr.LicenseReason()
	}
	return ev
}

// generateRunID returns a wt-run-<unix-ms>-<6-hex-rand> identifier.
func generateRunID() string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("wt-run-%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b[:]))
}
