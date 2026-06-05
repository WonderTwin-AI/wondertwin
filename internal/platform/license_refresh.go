package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/wondertwin-ai/wondertwin/internal/httpio"
	"net/http"
)

// LicenseBundle is the license-only payload returned by the
// /v1/accounts/{id}/mcp/license-refresh endpoint. Distinct from
// InstallBundle (which carries the binary too) because refresh
// leaves the binary alone — only ~/.wondertwin/license.json is
// atomic-replaced per adr-license-delivery-via-mcp §3.
//
// Field names match the server-side billing.LicenseBundle and the
// schema's installBundle subset for runtime code reuse.
type LicenseBundle struct {
	LicenseFileB64  string `json:"license_file"`
	LicenseIssuedAt string `json:"license_issued_at"`
	LicenseNotAfter string `json:"license_not_after"`
}

// LicenseRefreshedDetail is OutcomeDetail when outcome=license_refreshed.
// JSON-decode into this when env.Outcome is OutcomeLicenseRefreshed.
type LicenseRefreshedDetail struct {
	LicenseBundle *LicenseBundle `json:"license_bundle"`
}

// OutcomeLicenseRefreshed is the InstallOutcome value for a
// successful license-only refresh. Adds to the existing enum and is
// forward-compat per adr-mcp-subscribe-outcomes §4 — older clients
// degrade to policy_error and surface client_facing_message verbatim,
// which is safe because "license refreshed" is informational.
const OutcomeLicenseRefreshed InstallOutcome = "license_refreshed"

// LicenseRefresh calls POST /v1/accounts/{accountID}/mcp/license-refresh
// on wondertwin-app. Returns the full envelope so the caller can
// branch on env.Outcome (license_refreshed on success; policy_error
// on no-active-entitlements / issuer failure / etc.).
//
// Returns a Go error only for transport-layer failures. Every
// expected outcome, including policy_error, comes back as a 200
// with a populated envelope.
func (c *Client) LicenseRefresh(ctx context.Context, accountID string) (*InstallEnvelope, error) {
	if accountID == "" {
		return nil, fmt.Errorf("license-refresh: accountID is required")
	}

	// Empty POST body — refresh takes no inputs beyond the
	// path-scoped accountID and the auth context.
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/v1/accounts/%s/mcp/license-refresh", c.baseURL, accountID),
		bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("license-refresh: create request: %w", err)
	}
	httpReq.Header.Set("Authorization", c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("license-refresh: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes))
		return nil, fmt.Errorf("license-refresh: HTTP %d: %s", resp.StatusCode, respBody)
	}

	var env InstallEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("license-refresh: decode response: %w", err)
	}
	return &env, nil
}

// DecodeLicenseRefreshedDetail re-decodes env.OutcomeDetail (the raw
// any from JSON) into a typed LicenseRefreshedDetail. Returns
// (nil, nil) if the outcome isn't license_refreshed so callers can
// branch cleanly.
func DecodeLicenseRefreshedDetail(env *InstallEnvelope) (*LicenseRefreshedDetail, error) {
	if env == nil || env.Outcome != OutcomeLicenseRefreshed {
		return nil, nil
	}
	raw, err := json.Marshal(env.OutcomeDetail)
	if err != nil {
		return nil, fmt.Errorf("decode license-refreshed detail: re-marshal: %w", err)
	}
	var detail LicenseRefreshedDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return nil, fmt.Errorf("decode license-refreshed detail: %w", err)
	}
	return &detail, nil
}
