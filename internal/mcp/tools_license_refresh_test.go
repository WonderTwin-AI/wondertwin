package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/wondertwin-ai/wondertwin/internal/config"
	"github.com/wondertwin-ai/wondertwin/internal/platform"
)

// licenseCommitRecorder is the test-mode replacement for the
// commitLicenseToHome production licenseCommitter. Mirrors the
// bundleCommitRecorder pattern from tools_install_test.go.
type licenseCommitRecorder struct {
	mu     sync.Mutex
	called int
	bundle *platform.LicenseBundle
	err    error
}

func (r *licenseCommitRecorder) commit(bundle *platform.LicenseBundle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.called++
	r.bundle = bundle
	return r.err
}

// captureLicenseCommitter swaps the package-level licenseCommitter
// for a recording fake. Returns the recorder + a cleanup func that
// t.Cleanup restores.
func captureLicenseCommitter(t *testing.T) *licenseCommitRecorder {
	t.Helper()
	rec := &licenseCommitRecorder{}
	prev := licenseCommitter
	licenseCommitter = rec.commit
	t.Cleanup(func() { licenseCommitter = prev })
	return rec
}

// licenseEnvelopeResp returns an httptest server that responds at
// the license-refresh endpoint with the given envelope.
func licenseEnvelopeResp(t *testing.T, env platform.InstallEnvelope) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(env)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runRefreshTool(t *testing.T, srv *httptest.Server, cfg *config.Config) ToolResult {
	t.Helper()
	pc := platform.New(srv.URL, "test-key")
	entry := licenseRefreshTool(pc, cfg)
	return entry.Handler(nil, nil, nil)
}

func sampleLicenseBundle(t *testing.T) platform.LicenseBundle {
	t.Helper()
	return platform.LicenseBundle{
		LicenseFileB64:  base64.StdEncoding.EncodeToString([]byte(`{"format":"v2","account_id":"org_t","issued_at":"2026-06-01T12:00:00Z"}`)),
		LicenseIssuedAt: "2026-06-01T12:00:00Z",
		LicenseNotAfter: "2026-07-01T12:00:00Z",
	}
}

func TestLicenseRefreshTool_UnauthenticatedReturnsSetupRequired(t *testing.T) {
	t.Parallel()
	srv := licenseEnvelopeResp(t, platform.InstallEnvelope{})
	res := runRefreshTool(t, srv, &config.Config{})
	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomeSetupRequired) {
		t.Errorf("outcome: want setup_required, got %v", got["outcome"])
	}
	detail, _ := got["outcome_detail"].(map[string]any)
	if detail["reason_code"] != "no_account" {
		t.Errorf("reason_code: want no_account, got %v", detail["reason_code"])
	}
}

func TestLicenseRefreshTool_HappyPathCommitsBundle(t *testing.T) {
	// Not Parallel: mutates package-level licenseCommitter
	bundle := sampleLicenseBundle(t)
	srv := licenseEnvelopeResp(t, platform.InstallEnvelope{
		Outcome:             platform.OutcomeLicenseRefreshed,
		TwinID:              "*",
		ClientFacingMessage: "License refreshed.",
		SchemaVersion:       1,
		OutcomeDetail: map[string]any{
			"license_bundle": map[string]any{
				"license_file":      bundle.LicenseFileB64,
				"license_issued_at": bundle.LicenseIssuedAt,
				"license_not_after": bundle.LicenseNotAfter,
			},
		},
	})
	rec := captureLicenseCommitter(t)
	res := runRefreshTool(t, srv, &config.Config{APIKey: "k", OrgID: "org_t"})

	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomeLicenseRefreshed) {
		t.Errorf("outcome: want license_refreshed, got %v", got["outcome"])
	}
	if rec.called != 1 {
		t.Errorf("commit calls: want 1, got %d", rec.called)
	}
	if rec.bundle == nil || rec.bundle.LicenseIssuedAt != bundle.LicenseIssuedAt {
		t.Errorf("committed bundle IssuedAt: got %v", rec.bundle)
	}
}

func TestLicenseRefreshTool_PolicyErrorDoesNotCommit(t *testing.T) {
	// Not Parallel: mutates package-level licenseCommitter
	srv := licenseEnvelopeResp(t, platform.InstallEnvelope{
		Outcome:             platform.OutcomePolicyError,
		TwinID:              "*",
		ClientFacingMessage: "License refresh failed: no_active_entitlements.",
		SchemaVersion:       1,
		OutcomeDetail: map[string]any{
			"error_code":    "no_active_entitlements",
			"error_message": "no active entitlements",
		},
	})
	rec := captureLicenseCommitter(t)
	res := runRefreshTool(t, srv, &config.Config{APIKey: "k", OrgID: "org_t"})

	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomePolicyError) {
		t.Errorf("outcome: want policy_error, got %v", got["outcome"])
	}
	if rec.called != 0 {
		t.Errorf("policy_error should not commit; got %d calls", rec.called)
	}
}

func TestLicenseRefreshTool_CommitFailureReturnsPolicyError(t *testing.T) {
	// Not Parallel: mutates package-level licenseCommitter
	bundle := sampleLicenseBundle(t)
	srv := licenseEnvelopeResp(t, platform.InstallEnvelope{
		Outcome:             platform.OutcomeLicenseRefreshed,
		TwinID:              "*",
		ClientFacingMessage: "License refreshed.",
		SchemaVersion:       1,
		OutcomeDetail: map[string]any{
			"license_bundle": map[string]any{
				"license_file":      bundle.LicenseFileB64,
				"license_issued_at": bundle.LicenseIssuedAt,
				"license_not_after": bundle.LicenseNotAfter,
			},
		},
	})
	rec := captureLicenseCommitter(t)
	rec.err = errors.New("disk full")
	res := runRefreshTool(t, srv, &config.Config{APIKey: "k", OrgID: "org_t"})

	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomePolicyError) {
		t.Errorf("outcome: want policy_error on commit failure, got %v", got["outcome"])
	}
	detail, _ := got["outcome_detail"].(map[string]any)
	if detail["error_code"] != "license_commit_failed" {
		t.Errorf("error_code: want license_commit_failed, got %v", detail["error_code"])
	}
}

func TestLicenseRefreshTool_HTTP5xxBubblesAsError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	pc := platform.New(srv.URL, "test-key")
	entry := licenseRefreshTool(pc, &config.Config{APIKey: "k", OrgID: "org_t"})
	res := entry.Handler(nil, nil, nil)
	got := unmarshalResult(t, res)
	if got["error"] == nil {
		t.Errorf("want error on HTTP 500, got %+v", got)
	} else if !strings.Contains(got["error"].(string), "license-refresh") {
		t.Errorf("error should mention license-refresh, got %v", got["error"])
	}
}

// --- refreshLicenseIfStale tests ---

func TestRefreshLicenseIfStale_NoMetadataIsNoOp(t *testing.T) {
	t.Parallel()
	// Server should never be called.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server should not be called when no metadata")
	}))
	defer srv.Close()
	pc := platform.New(srv.URL, "test-key")
	refreshLicenseIfStale(context.Background(), pc,
		&config.Config{APIKey: "k", OrgID: "org_t"},
		&platform.InstallEnvelope{Outcome: platform.OutcomeInstalled},
		slog.Default())
}

func TestRefreshLicenseIfStale_UnauthenticatedIsNoOp(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server should not be called when unauthenticated")
	}))
	defer srv.Close()
	pc := platform.New(srv.URL, "test-key")
	env := &platform.InstallEnvelope{
		MCPMetadata: &platform.EnvelopeMeta{
			LicenseCurrentIssuedAt: "2099-01-01T00:00:00Z",
		},
	}
	refreshLicenseIfStale(context.Background(), pc, &config.Config{}, env, slog.Default())
}
