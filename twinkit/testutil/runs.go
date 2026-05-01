package testutil

import (
	"encoding/json"
	"testing"
	"time"
)

// RunOption configures a run started via WithRun.
type RunOption func(*runOptions)

type runOptions struct {
	runID    string
	seed     int64
	fixtures json.RawMessage
}

// WithSeed sets the deterministic seed applied at run start. Zero is
// the package default; pass a non-zero value to make the run
// reproducible.
func WithSeed(seed int64) RunOption {
	return func(o *runOptions) { o.seed = seed }
}

// WithFixtures provides an arbitrary JSON snapshot blob that will be
// loaded via the twin's StateStore.LoadState after reset.
func WithFixtures(snapshot []byte) RunOption {
	return func(o *runOptions) { o.fixtures = json.RawMessage(snapshot) }
}

// WithRunID sets an explicit run id. When unset the admin handler
// generates one.
func WithRunID(id string) RunOption {
	return func(o *runOptions) { o.runID = id }
}

// StartRunResponse mirrors the JSON returned by POST /admin/runs/start.
type StartRunResponse struct {
	RunID      string    `json:"run_id"`
	StartedAt  time.Time `json:"started_at"`
	Seed       int64     `json:"seed,omitempty"`
	Seeded     bool      `json:"seeded"`
	SeedSource string    `json:"seed_source"`
}

// CurrentRunResponse mirrors the JSON returned by GET /admin/runs/current.
type CurrentRunResponse struct {
	RunID      string    `json:"run_id"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	EntryCount int       `json:"entry_count"`
	Seeded     bool      `json:"seeded,omitempty"`
	SeedSource string    `json:"seed_source,omitempty"`
}

// FinishRunResponse mirrors the replay.Manifest emitted by
// POST /admin/runs/finish. The fields here cover the subset tests
// care about; additional fields decode silently.
type FinishRunResponse struct {
	RunID      string    `json:"run_id,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	EntryCount int       `json:"entry_count,omitempty"`
}

// StartRun calls POST /admin/runs/start with the supplied options and
// decodes the response.
func (ac *AdminClient) StartRun(opts ...RunOption) StartRunResponse {
	ac.t.Helper()
	var o runOptions
	for _, opt := range opts {
		opt(&o)
	}
	body := struct {
		RunID    string          `json:"run_id,omitempty"`
		Seed     int64           `json:"seed,omitempty"`
		Fixtures json.RawMessage `json:"fixtures,omitempty"`
	}{
		RunID:    o.runID,
		Seed:     o.seed,
		Fixtures: o.fixtures,
	}
	resp := ac.Post("/admin/runs/start", body).AssertStatus(200)
	var out StartRunResponse
	resp.JSON(&out)
	return out
}

// FinishRun calls POST /admin/runs/finish, optionally asserting that
// the active run matches runID. Pass the empty string to finish
// whichever run is current without the matching check.
func (ac *AdminClient) FinishRun(runID string) FinishRunResponse {
	ac.t.Helper()
	body := struct {
		RunID string `json:"run_id,omitempty"`
	}{RunID: runID}
	resp := ac.Post("/admin/runs/finish", body).AssertStatus(200)
	var out FinishRunResponse
	resp.JSON(&out)
	return out
}

// CurrentRun calls GET /admin/runs/current.
func (ac *AdminClient) CurrentRun() CurrentRunResponse {
	ac.t.Helper()
	resp := ac.Get("/admin/runs/current").AssertStatus(200)
	var out CurrentRunResponse
	resp.JSON(&out)
	return out
}

// WithRun starts a run on ac, registers a t.Cleanup that finishes it
// (best-effort; cleanup is silent if the run was already finished or
// the server is gone), and returns the run id.
func WithRun(t *testing.T, ac *AdminClient, opts ...RunOption) string {
	t.Helper()
	r := ac.StartRun(opts...)
	t.Cleanup(func() {
		defer func() { _ = recover() }()
		body := struct {
			RunID string `json:"run_id,omitempty"`
		}{RunID: r.RunID}
		_ = ac.Post("/admin/runs/finish", body) // best-effort; ignore status
	})
	return r.RunID
}
