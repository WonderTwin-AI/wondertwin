package mcp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/wondertwin-ai/wondertwin/internal/config"
	"github.com/wondertwin-ai/wondertwin/internal/platform"
	"github.com/wondertwin-ai/wondertwin/internal/registry"
)

// captureBundleCommitter swaps the package-level bundleCommitter for
// a fake that records calls. Returns a cleanup func — defer t.Cleanup
// in the test.
func captureBundleCommitter(t *testing.T) *bundleCommitRecorder {
	t.Helper()
	rec := &bundleCommitRecorder{}
	prev := bundleCommitter
	bundleCommitter = rec.commit
	t.Cleanup(func() { bundleCommitter = prev })
	return rec
}

type bundleCommitRecorder struct {
	mu       sync.Mutex
	called   int
	twinName string
	bundle   registry.InstallBundle
	err      error
}

func (r *bundleCommitRecorder) commit(twinName string, bundle registry.InstallBundle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.called++
	r.twinName = twinName
	r.bundle = bundle
	return r.err
}

// envelopeResp returns an httptest server that responds to the MCP
// install endpoint with the given envelope.
func envelopeResp(t *testing.T, env platform.InstallEnvelope) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(env)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runInstallTool(t *testing.T, srv *httptest.Server, cfg *config.Config, params map[string]string) ToolResult {
	t.Helper()
	pc := platform.New(srv.URL, "test-key")
	entry := installTool(pc, cfg)
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return entry.Handler(nil, nil, raw)
}

func unmarshalResult(t *testing.T, res ToolResult) map[string]any {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("tool result: empty content")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].Text), &m); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}
	return m
}

func sampleInstallBundle(t *testing.T) (platform.InstallBundle, []byte) {
	t.Helper()
	binaryBytes := []byte("twin binary v1.0.0")
	sum := sha256.Sum256(binaryBytes)
	return platform.InstallBundle{
		BinaryURL:       "https://cdn.example.com/twin-stripe-1.0.0.tar.gz",
		BinarySHA256:    hex.EncodeToString(sum[:]),
		BinaryVersion:   "1.0.0",
		LicenseFileB64:  base64.StdEncoding.EncodeToString([]byte(`{"format":"v2"}`)),
		LicenseIssuedAt: "2026-05-31T12:00:00Z",
		LicenseNotAfter: "2026-06-30T12:00:00Z",
	}, binaryBytes
}

func TestInstallTool_MissingTwinNameErrors(t *testing.T) {
	t.Parallel()
	srv := envelopeResp(t, platform.InstallEnvelope{})
	res := runInstallTool(t, srv, &config.Config{APIKey: "k", OrgID: "org_t"}, map[string]string{})
	got := unmarshalResult(t, res)
	if got["error"] == nil || !strings.Contains(got["error"].(string), "twin_name") {
		t.Errorf("want twin_name error, got %+v", got)
	}
}

func TestInstallTool_NotAuthenticatedReturnsSetupRequired(t *testing.T) {
	t.Parallel()
	srv := envelopeResp(t, platform.InstallEnvelope{})
	res := runInstallTool(t, srv, &config.Config{}, map[string]string{"twin_name": "stripe"})
	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomeSetupRequired) {
		t.Errorf("outcome: want setup_required, got %v", got["outcome"])
	}
	detail, _ := got["outcome_detail"].(map[string]any)
	if detail["setup_url"] == nil {
		t.Errorf("setup_url missing in detail: %+v", detail)
	}
	if detail["reason_code"] != "no_account" {
		t.Errorf("reason_code: want no_account, got %v", detail["reason_code"])
	}
}

func TestInstallTool_InstalledOutcomeCommitsBundle(t *testing.T) {
	// Not Parallel: mutates package-level bundleCommitter
	bundle, _ := sampleInstallBundle(t)
	srv := envelopeResp(t, platform.InstallEnvelope{
		Outcome: platform.OutcomeInstalled,
		TwinID:  "stripe", SchemaVersion: 1,
		ClientFacingMessage: "Installed stripe.",
		OutcomeDetail: map[string]any{
			"subscription_id": "sub_x",
			"twin_version":    "1.0.0",
			"install_bundle": map[string]any{
				"binary_url":        bundle.BinaryURL,
				"binary_sha256":     bundle.BinarySHA256,
				"binary_version":    bundle.BinaryVersion,
				"license_file":      bundle.LicenseFileB64,
				"license_issued_at": bundle.LicenseIssuedAt,
				"license_not_after": bundle.LicenseNotAfter,
			},
		},
	})

	rec := captureBundleCommitter(t)
	res := runInstallTool(t, srv, &config.Config{APIKey: "k", OrgID: "org_t"}, map[string]string{
		"twin_name": "stripe",
	})

	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomeInstalled) {
		t.Errorf("outcome: want installed, got %v", got["outcome"])
	}
	if rec.called != 1 {
		t.Errorf("bundle commit calls: want 1, got %d", rec.called)
	}
	if rec.twinName != "stripe" {
		t.Errorf("commit twin name: want stripe, got %s", rec.twinName)
	}
	if rec.bundle.BinaryVersion != "1.0.0" {
		t.Errorf("commit bundle version: got %s", rec.bundle.BinaryVersion)
	}
}

func TestInstallTool_AlreadyInstalledRefreshAlsoCommits(t *testing.T) {
	// Not Parallel: mutates package-level bundleCommitter
	bundle, _ := sampleInstallBundle(t)
	srv := envelopeResp(t, platform.InstallEnvelope{
		Outcome: platform.OutcomeAlreadyInstalledRefresh,
		TwinID:  "stripe", SchemaVersion: 1,
		ClientFacingMessage: "stripe was already installed; refreshed the local license.",
		OutcomeDetail: map[string]any{
			"subscription_id": "sub_existing",
			"twin_version":    "1.0.0",
			"install_bundle": map[string]any{
				"binary_url":        bundle.BinaryURL,
				"binary_sha256":     bundle.BinarySHA256,
				"binary_version":    bundle.BinaryVersion,
				"license_file":      bundle.LicenseFileB64,
				"license_issued_at": bundle.LicenseIssuedAt,
				"license_not_after": bundle.LicenseNotAfter,
			},
		},
	})
	rec := captureBundleCommitter(t)
	_ = runInstallTool(t, srv, &config.Config{APIKey: "k", OrgID: "org_t"}, map[string]string{
		"twin_name": "stripe",
	})
	if rec.called != 1 {
		t.Errorf("want bundle commit on already_installed_refreshed, got %d calls", rec.called)
	}
}

func TestInstallTool_QueuedOutcomeDoesNotCommitBundle(t *testing.T) {
	// Not Parallel: mutates package-level bundleCommitter
	srv := envelopeResp(t, platform.InstallEnvelope{
		Outcome: platform.OutcomeQueued,
		TwinID:  "stripe", SchemaVersion: 1,
		ClientFacingMessage: "Queued stripe.",
		OutcomeDetail: map[string]any{
			"queue_id":    "asq_abc",
			"expires_at":  "2026-06-14T12:00:00Z",
			"status_url":  "wt://subscribe/status/asq_abc",
			"reason_code": "over_budget",
		},
	})
	rec := captureBundleCommitter(t)
	res := runInstallTool(t, srv, &config.Config{APIKey: "k", OrgID: "org_t"}, map[string]string{
		"twin_name": "stripe",
	})
	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomeQueued) {
		t.Errorf("outcome: want queued, got %v", got["outcome"])
	}
	if rec.called != 0 {
		t.Errorf("queued must not commit bundle, got %d calls", rec.called)
	}
}

func TestInstallTool_DeniedOutcomePassesThrough(t *testing.T) {
	// Not Parallel: mutates package-level bundleCommitter
	srv := envelopeResp(t, platform.InstallEnvelope{
		Outcome: platform.OutcomeDenied,
		TwinID:  "stripe", SchemaVersion: 1,
		ClientFacingMessage: "Denied stripe.",
		OutcomeDetail:       map[string]any{"reason_code": "no_policy"},
	})
	rec := captureBundleCommitter(t)
	res := runInstallTool(t, srv, &config.Config{APIKey: "k", OrgID: "org_t"}, map[string]string{
		"twin_name": "stripe",
	})
	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomeDenied) {
		t.Errorf("outcome: want denied, got %v", got["outcome"])
	}
	if rec.called != 0 {
		t.Errorf("denied must not commit bundle, got %d calls", rec.called)
	}
}

func TestInstallTool_BundleCommitFailureReturnsPolicyError(t *testing.T) {
	// Not Parallel: mutates package-level bundleCommitter
	bundle, _ := sampleInstallBundle(t)
	srv := envelopeResp(t, platform.InstallEnvelope{
		Outcome: platform.OutcomeInstalled,
		TwinID:  "stripe", SchemaVersion: 1,
		ClientFacingMessage: "Installed stripe.",
		OutcomeDetail: map[string]any{
			"subscription_id": "sub_x",
			"twin_version":    "1.0.0",
			"install_bundle": map[string]any{
				"binary_url":        bundle.BinaryURL,
				"binary_sha256":     bundle.BinarySHA256,
				"binary_version":    bundle.BinaryVersion,
				"license_file":      bundle.LicenseFileB64,
				"license_issued_at": bundle.LicenseIssuedAt,
				"license_not_after": bundle.LicenseNotAfter,
			},
		},
	})
	rec := captureBundleCommitter(t)
	rec.err = errFakeCommit
	res := runInstallTool(t, srv, &config.Config{APIKey: "k", OrgID: "org_t"}, map[string]string{
		"twin_name": "stripe",
	})
	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomePolicyError) {
		t.Errorf("outcome: want policy_error on commit failure, got %v (full: %+v)", got["outcome"], got)
	}
	detail, _ := got["outcome_detail"].(map[string]any)
	if detail["error_code"] != "bundle_commit_failed" {
		t.Errorf("error_code: want bundle_commit_failed, got %v", detail["error_code"])
	}
}

func TestInstallTool_DefaultsPricingID(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req platform.InstallRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.PricingID != "price_commercial_monthly" {
			t.Errorf("pricing_id default: want price_commercial_monthly, got %s", req.PricingID)
		}
		_ = json.NewEncoder(w).Encode(platform.InstallEnvelope{
			Outcome: platform.OutcomeDenied, TwinID: "stripe", SchemaVersion: 1,
			ClientFacingMessage: "x", OutcomeDetail: map[string]any{"reason_code": "no_policy"},
		})
	}))
	defer srv.Close()
	_ = runInstallTool(t, srv, &config.Config{APIKey: "k", OrgID: "org_t"}, map[string]string{
		"twin_name": "stripe",
	})
}

// errFakeCommit is a sentinel test error for the bundle commit
// failure path.
var errFakeCommit = newErr("simulated commit failure")

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }
func newErr(m string) error      { return &fakeErr{msg: m} }
