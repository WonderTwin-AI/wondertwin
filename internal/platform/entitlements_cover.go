package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// MissingTwin mirrors the server-side billing.MissingTwin (which
// itself comes from subscriptions.MissingTwin). Wire shape matches.
// ReasonCode is one of: not_entitled, entitlement_lapsed,
// entitlement_cancelled, entitlement_unknown_status.
type MissingTwin struct {
	TwinName   string `json:"twin_name"`
	ReasonCode string `json:"reason_code"`
}

// EntitlementsCoverResponse is the typed return from
// /v1/accounts/{id}/entitlements/cover. Covered is the list of twin
// names that are entitled (active / trial / cooling_off / pending);
// Missing is the per-twin reason record for the rest.
type EntitlementsCoverResponse struct {
	Covered []string      `json:"covered"`
	Missing []MissingTwin `json:"missing"`
}

// EntitlementsCover queries the server's coverage view for a list of
// twin names against the given account. Backbone of the `wt verify`
// CI gate per adr-ci-twin-verification.
//
// Returns a Go error only for transport-layer failures (network,
// non-200 HTTP, malformed JSON). Empty input returns the empty
// envelope (no server call) — CI tooling with an empty lockfile
// shouldn't pay for a round-trip.
func (c *Client) EntitlementsCover(ctx context.Context, accountID string, twins []string) (*EntitlementsCoverResponse, error) {
	if accountID == "" {
		return nil, fmt.Errorf("entitlements-cover: accountID is required")
	}
	cleaned := dedupeNonEmpty(twins)
	if len(cleaned) == 0 {
		return &EntitlementsCoverResponse{Covered: []string{}, Missing: []MissingTwin{}}, nil
	}

	q := url.Values{}
	q.Set("twins", strings.Join(cleaned, ","))
	endpoint := fmt.Sprintf("%s/v1/accounts/%s/entitlements/cover?%s",
		c.baseURL, url.PathEscape(accountID), q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("entitlements-cover: create request: %w", err)
	}
	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("entitlements-cover: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("entitlements-cover: HTTP %d: %s", resp.StatusCode, body)
	}
	var out EntitlementsCoverResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("entitlements-cover: decode response: %w", err)
	}
	return &out, nil
}

// dedupeNonEmpty drops empty strings and duplicates while preserving
// order. The server is defensive about both, but it's cheap to do
// here and keeps the URL clean.
func dedupeNonEmpty(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
