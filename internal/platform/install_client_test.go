package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, New(srv.URL, "test-api-key")
}

func TestInstall_HappyPathDecodesInstalledEnvelope(t *testing.T) {
	t.Parallel()
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/org_t/mcp/install" {
			t.Errorf("path: want /v1/accounts/org_t/mcp/install, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "test-api-key" {
			t.Errorf("auth header missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(InstallEnvelope{
			Outcome:             OutcomeInstalled,
			TwinID:              "stripe",
			ClientFacingMessage: "Installed stripe.",
			SchemaVersion:       1,
			OutcomeDetail: map[string]any{
				"subscription_id": "sub_x",
				"twin_version":    "1.0.0",
				"install_bundle": map[string]any{
					"binary_url":        "https://cdn.example.com/stripe-1.0.0.tar.gz",
					"binary_sha256":     "0000000000000000000000000000000000000000000000000000000000000000",
					"binary_version":    "1.0.0",
					"license_file":      "eyJmb28iOiJiYXIifQ==",
					"license_issued_at": "2026-05-31T12:00:00Z",
					"license_not_after": "2026-06-30T12:00:00Z",
				},
			},
		})
	})

	env, err := client.Install(context.Background(), "org_t", InstallRequest{
		TwinID:    "stripe",
		PricingID: "price_commercial_monthly",
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if env.Outcome != OutcomeInstalled {
		t.Errorf("outcome: want installed, got %s", env.Outcome)
	}
	detail, err := DecodeInstalledDetail(env)
	if err != nil {
		t.Fatalf("DecodeInstalledDetail: %v", err)
	}
	if detail == nil || detail.InstallBundle == nil {
		t.Fatalf("detail or bundle nil: %+v", detail)
	}
	if detail.InstallBundle.BinaryURL != "https://cdn.example.com/stripe-1.0.0.tar.gz" {
		t.Errorf("binary_url: got %q", detail.InstallBundle.BinaryURL)
	}
	if detail.InstallBundle.LicenseFileB64 != "eyJmb28iOiJiYXIifQ==" {
		t.Errorf("license_file: got %q", detail.InstallBundle.LicenseFileB64)
	}
}

func TestInstall_QueuedOutcomeDecodesCorrectly(t *testing.T) {
	t.Parallel()
	_, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(InstallEnvelope{
			Outcome:             OutcomeQueued,
			TwinID:              "stripe",
			ClientFacingMessage: "Queued stripe.",
			SchemaVersion:       1,
			OutcomeDetail: map[string]any{
				"queue_id":    "asq_abc",
				"expires_at":  "2026-06-14T12:00:00Z",
				"status_url":  "wt://subscribe/status/asq_abc",
				"reason_code": "over_budget",
			},
		})
	})

	env, err := client.Install(context.Background(), "org_t", InstallRequest{
		TwinID: "stripe", PricingID: "price_commercial_monthly",
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	q, err := DecodeQueuedDetail(env)
	if err != nil {
		t.Fatalf("DecodeQueuedDetail: %v", err)
	}
	if q == nil || q.QueueID != "asq_abc" {
		t.Errorf("queued detail: got %+v", q)
	}
	if q.ReasonCode != "over_budget" {
		t.Errorf("reason_code: want over_budget, got %s", q.ReasonCode)
	}

	// Non-queued envelopes decode to nil through this helper.
	installed := &InstallEnvelope{Outcome: OutcomeInstalled}
	gotQ, err := DecodeQueuedDetail(installed)
	if err != nil || gotQ != nil {
		t.Errorf("DecodeQueuedDetail on installed: want (nil,nil), got (%+v,%v)", gotQ, err)
	}
}

func TestInstall_DeniedOutcomeDecodesCorrectly(t *testing.T) {
	t.Parallel()
	_, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(InstallEnvelope{
			Outcome:             OutcomeDenied,
			TwinID:              "stripe",
			ClientFacingMessage: "Denied stripe.",
			SchemaVersion:       1,
			OutcomeDetail:       map[string]any{"reason_code": "no_policy"},
		})
	})

	env, err := client.Install(context.Background(), "org_t", InstallRequest{
		TwinID: "stripe", PricingID: "price_commercial_monthly",
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	d, err := DecodeDeniedDetail(env)
	if err != nil {
		t.Fatalf("DecodeDeniedDetail: %v", err)
	}
	if d == nil || d.ReasonCode != "no_policy" {
		t.Errorf("denied detail: got %+v", d)
	}
}

func TestInstall_HTTP5xxBubblesAsError(t *testing.T) {
	t.Parallel()
	_, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	})

	_, err := client.Install(context.Background(), "org_t", InstallRequest{
		TwinID: "stripe", PricingID: "price_commercial_monthly",
	})
	if err == nil {
		t.Fatal("want error for HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("want error message includes HTTP 500, got %v", err)
	}
}

func TestInstall_MissingAccountIDIsClientError(t *testing.T) {
	t.Parallel()
	client := New("http://unused", "key")
	_, err := client.Install(context.Background(), "", InstallRequest{
		TwinID: "stripe", PricingID: "price_commercial_monthly",
	})
	if err == nil || !strings.Contains(err.Error(), "accountID") {
		t.Errorf("want client error on empty accountID, got %v", err)
	}
}

func TestInstall_MissingTwinIDIsClientError(t *testing.T) {
	t.Parallel()
	client := New("http://unused", "key")
	_, err := client.Install(context.Background(), "org_t", InstallRequest{
		PricingID: "price_commercial_monthly",
	})
	if err == nil || !strings.Contains(err.Error(), "twin_id") {
		t.Errorf("want client error on empty twin_id, got %v", err)
	}
}

func TestInstall_RequestBodyShape(t *testing.T) {
	t.Parallel()
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		// Snake-case wire shape.
		if got["twin_id"] != "stripe" || got["pricing_id"] != "price_commercial_monthly" {
			t.Errorf("body shape: %+v", got)
		}
		if got["agent_client_id"] != "claude-code" {
			t.Errorf("agent_client_id: got %v", got["agent_client_id"])
		}
		_ = json.NewEncoder(w).Encode(InstallEnvelope{
			Outcome: OutcomeInstalled, TwinID: "stripe", SchemaVersion: 1,
			ClientFacingMessage: "ok",
		})
	})
	_, err := client.Install(context.Background(), "org_t", InstallRequest{
		TwinID: "stripe", PricingID: "price_commercial_monthly",
		AgentClientID: "claude-code",
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
}
