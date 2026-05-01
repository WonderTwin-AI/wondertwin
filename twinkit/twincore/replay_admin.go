package twincore

import (
	"encoding/json"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twinkit/replay"
)

// mountReplayRoutes wires the stopgap /admin/replay endpoints onto the
// twin's router. The Recorder is owned by the Twin so this happens
// automatically — twins don't need to call a setter.
//
// These endpoints are stopgaps that ship with Wave 1A. The full
// run-lifecycle surface (/admin/runs/{start,finish,current}) lands in
// session 2B and will subsume them.
func (t *Twin) mountReplayRoutes() {
	if t.Replay == nil {
		return
	}
	t.Router.Get("/admin/replay", t.handleReplayGet)
	t.Router.Post("/admin/replay/start", t.handleReplayStart)
	t.Router.Post("/admin/replay/finish", t.handleReplayFinish)
}

func (t *Twin) handleReplayGet(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("format") == "manifest" {
		JSON(w, http.StatusOK, t.Replay.CurrentManifest())
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	if err := t.Replay.Export(w); err != nil {
		Error(w, http.StatusInternalServerError, "export failed: "+err.Error())
		return
	}
}

func (t *Twin) handleReplayStart(w http.ResponseWriter, r *http.Request) {
	runID := r.URL.Query().Get("run_id")
	if runID == "" && r.Body != nil {
		var body struct {
			RunID string `json:"run_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		runID = body.RunID
	}
	t.Replay.StartRun(runID)
	if runID != "" {
		t.SetRunID(runID)
	}
	JSON(w, http.StatusOK, map[string]string{"status": "started", "run_id": runID})
}

func (t *Twin) handleReplayFinish(w http.ResponseWriter, _ *http.Request) {
	JSON(w, http.StatusOK, t.Replay.FinishRun())
}

// emptyReplayManifest is exposed for callers that want to assert the
// shape of a no-recorder GET /admin/replay response in tests. The
// Twin always wires a recorder, so this is rarely needed in practice.
func emptyReplayManifest() replay.Manifest {
	return replay.Manifest{SchemaVersion: replay.SchemaVersion}
}
