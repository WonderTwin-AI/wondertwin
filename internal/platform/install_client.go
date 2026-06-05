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

// Install calls POST /v1/accounts/{accountID}/mcp/install on
// wondertwin-app. Returns the full InstallEnvelope (always, for
// expected outcomes) — the caller branches on env.Outcome to decide
// what to do next (install bundle, surface error to user, queue
// status polling, etc.).
//
// Returns a Go error only for transport-layer failures (network,
// malformed response, non-200 HTTP) — every expected outcome,
// including denied / queued / policy_error, comes back as a 200 with
// a populated envelope per the server contract documented in
// wondertwin-app/internal/billing/install_handler.go.
//
// OutcomeDetail is left as the raw decoded value (map[string]any or
// nil); callers re-decode into the appropriate per-outcome shape
// (InstalledDetail, QueuedDetail, etc.) once they've branched on
// Outcome.
func (c *Client) Install(ctx context.Context, accountID string, req InstallRequest) (*InstallEnvelope, error) {
	if accountID == "" {
		return nil, fmt.Errorf("install: accountID is required")
	}
	if req.TwinID == "" || req.PricingID == "" {
		return nil, fmt.Errorf("install: twin_id and pricing_id are required")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("install: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/v1/accounts/%s/mcp/install", c.baseURL, accountID),
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("install: create request: %w", err)
	}
	httpReq.Header.Set("Authorization", c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("install: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes))
		return nil, fmt.Errorf("install: HTTP %d: %s", resp.StatusCode, respBody)
	}

	var env InstallEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("install: decode response: %w", err)
	}
	return &env, nil
}

// DecodeInstalledDetail re-decodes env.OutcomeDetail (the raw any
// from JSON) into a typed InstalledDetail. Returns nil if the outcome
// isn't one that carries install bundles.
//
// Provided as a helper so callers don't have to re-marshal/unmarshal
// every time they branch on outcome.
func DecodeInstalledDetail(env *InstallEnvelope) (*InstalledDetail, error) {
	if env == nil {
		return nil, fmt.Errorf("decode installed detail: nil envelope")
	}
	switch env.Outcome {
	case OutcomeInstalled, OutcomeAlreadyInstalledRefresh, OutcomeTrialStartedInstalled:
		// proceed
	default:
		return nil, nil
	}
	raw, err := json.Marshal(env.OutcomeDetail)
	if err != nil {
		return nil, fmt.Errorf("decode installed detail: re-marshal: %w", err)
	}
	var detail InstalledDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return nil, fmt.Errorf("decode installed detail: %w", err)
	}
	return &detail, nil
}

// DecodeQueuedDetail re-decodes env.OutcomeDetail into a typed
// QueuedDetail when the outcome is queued. Same pattern as
// DecodeInstalledDetail.
func DecodeQueuedDetail(env *InstallEnvelope) (*QueuedDetail, error) {
	if env == nil || env.Outcome != OutcomeQueued {
		return nil, nil
	}
	raw, err := json.Marshal(env.OutcomeDetail)
	if err != nil {
		return nil, fmt.Errorf("decode queued detail: re-marshal: %w", err)
	}
	var detail QueuedDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return nil, fmt.Errorf("decode queued detail: %w", err)
	}
	return &detail, nil
}

// DecodeDeniedDetail re-decodes when outcome is denied.
func DecodeDeniedDetail(env *InstallEnvelope) (*DeniedDetail, error) {
	if env == nil || env.Outcome != OutcomeDenied {
		return nil, nil
	}
	raw, err := json.Marshal(env.OutcomeDetail)
	if err != nil {
		return nil, fmt.Errorf("decode denied detail: re-marshal: %w", err)
	}
	var detail DeniedDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return nil, fmt.Errorf("decode denied detail: %w", err)
	}
	return &detail, nil
}
