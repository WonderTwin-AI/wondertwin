package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLicenseRefresh_HappyPathDecodesEnvelope(t *testing.T) {
	t.Parallel()
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/org_t/mcp/license-refresh" {
			t.Errorf("path: got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "test-api-key" {
			t.Errorf("auth header missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(InstallEnvelope{
			Outcome:             OutcomeLicenseRefreshed,
			TwinID:              "*",
			ClientFacingMessage: "License refreshed (3 entitlements).",
			SchemaVersion:       1,
			OutcomeDetail: map[string]any{
				"license_bundle": map[string]any{
					"license_file":      "eyJmb28iOiJiYXIifQ==",
					"license_issued_at": "2026-06-01T12:00:00Z",
					"license_not_after": "2026-07-01T12:00:00Z",
				},
			},
			MCPMetadata: &EnvelopeMeta{
				LicenseCurrentIssuedAt: "2026-06-01T12:00:00Z",
			},
		})
	})

	env, err := client.LicenseRefresh(context.Background(), "org_t")
	if err != nil {
		t.Fatalf("LicenseRefresh: %v", err)
	}
	if env.Outcome != OutcomeLicenseRefreshed {
		t.Errorf("outcome: want license_refreshed, got %s", env.Outcome)
	}
	detail, err := DecodeLicenseRefreshedDetail(env)
	if err != nil {
		t.Fatalf("DecodeLicenseRefreshedDetail: %v", err)
	}
	if detail == nil || detail.LicenseBundle == nil {
		t.Fatalf("detail/bundle nil: %+v", detail)
	}
	if detail.LicenseBundle.LicenseFileB64 != "eyJmb28iOiJiYXIifQ==" {
		t.Errorf("license_file: got %q", detail.LicenseBundle.LicenseFileB64)
	}
	if detail.LicenseBundle.LicenseIssuedAt != "2026-06-01T12:00:00Z" {
		t.Errorf("license_issued_at: got %q", detail.LicenseBundle.LicenseIssuedAt)
	}
	if env.MCPMetadata == nil || env.MCPMetadata.LicenseCurrentIssuedAt != "2026-06-01T12:00:00Z" {
		t.Errorf("metadata IssuedAt not propagated: %+v", env.MCPMetadata)
	}
}

func TestLicenseRefresh_PolicyErrorOutcomeReturnsEnvelopeNotGoError(t *testing.T) {
	t.Parallel()
	_, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(InstallEnvelope{
			Outcome:             OutcomePolicyError,
			TwinID:              "*",
			ClientFacingMessage: "License refresh failed: no_active_entitlements.",
			SchemaVersion:       1,
			OutcomeDetail: map[string]any{
				"error_code":    "no_active_entitlements",
				"error_message": "no active entitlements to issue a license for",
			},
		})
	})

	env, err := client.LicenseRefresh(context.Background(), "org_t")
	if err != nil {
		t.Fatalf("policy_error should come back as envelope, not Go error; got %v", err)
	}
	if env.Outcome != OutcomePolicyError {
		t.Errorf("outcome: want policy_error, got %s", env.Outcome)
	}
}

func TestLicenseRefresh_HTTP5xxBubblesAsError(t *testing.T) {
	t.Parallel()
	_, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	_, err := client.LicenseRefresh(context.Background(), "org_t")
	if err == nil {
		t.Fatal("want error on 500")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("want HTTP 500 in error, got %v", err)
	}
}

func TestLicenseRefresh_MissingAccountIDIsClientError(t *testing.T) {
	t.Parallel()
	client := New("http://unused", "key")
	_, err := client.LicenseRefresh(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "accountID") {
		t.Errorf("want accountID-required error, got %v", err)
	}
}

func TestDecodeLicenseRefreshedDetail_NonMatchingOutcomeReturnsNil(t *testing.T) {
	t.Parallel()
	env := &InstallEnvelope{Outcome: OutcomeInstalled}
	d, err := DecodeLicenseRefreshedDetail(env)
	if err != nil || d != nil {
		t.Errorf("non-matching outcome: want (nil, nil), got (%v, %v)", d, err)
	}

	d, err = DecodeLicenseRefreshedDetail(nil)
	if err != nil || d != nil {
		t.Errorf("nil envelope: want (nil, nil), got (%v, %v)", d, err)
	}
}

func TestLicenseRefresh_PostsEmptyJSONBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		if string(body) != "{}" {
			t.Errorf("body: want '{}', got %q", string(body))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type: got %s", r.Header.Get("Content-Type"))
		}
		_ = json.NewEncoder(w).Encode(InstallEnvelope{
			Outcome: OutcomeLicenseRefreshed, TwinID: "*", SchemaVersion: 1,
			ClientFacingMessage: "ok",
		})
	}))
	defer srv.Close()
	client := New(srv.URL, "key")
	_, err := client.LicenseRefresh(context.Background(), "org_t")
	if err != nil {
		t.Fatalf("LicenseRefresh: %v", err)
	}
}
