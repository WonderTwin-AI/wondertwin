// Package auth provides a client for the wt-auth authorization service.
// Used by the CLI to request JIT tokens before fetching content.
package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/wondertwin-ai/wondertwin/internal/httpio"
	"net/http"
	"time"
)

const defaultAuthURL = "https://auth.wondertwin.ai"

// Client calls the wt-auth service to request and verify JIT tokens.
type Client struct {
	BaseURL string
	http    *http.Client
}

// NewClient creates a new auth client.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultAuthURL
	}
	return &Client{
		BaseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// TokenResponse is the response from POST /v1/tokens.
type TokenResponse struct {
	Token             string `json:"token"`
	TwinName          string `json:"twin_name"`
	OrgID             string `json:"org_id"`
	ExpiresAt         int64  `json:"expires_at"`
	TelemetryRequired bool   `json:"telemetry_required"`
}

// ErrorResponse is an error from the auth service.
type ErrorResponse struct {
	Error string `json:"error"`
}

// RequestToken requests a JIT token for a specific twin.
// Returns the token response on success. Returns an error with the auth
// service's error code on failure (e.g., "invalid_license", "twin_not_licensed",
// "license_revoked", "license_expired").
func (c *Client) RequestToken(licenseKey, twinName string) (*TokenResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"license_key": licenseKey,
		"twin_name":   twinName,
	})

	resp, err := c.http.Post(c.BaseURL+"/v1/tokens", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("auth service unavailable: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes))

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		json.Unmarshal(respBody, &errResp)
		return nil, &AuthError{
			StatusCode: resp.StatusCode,
			Code:       errResp.Error,
		}
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	return &tokenResp, nil
}

// AuthError represents an authorization failure from wt-auth.
type AuthError struct {
	StatusCode int
	Code       string // "invalid_license", "twin_not_licensed", "license_revoked", "license_expired"
}

func (e *AuthError) Error() string {
	switch e.Code {
	case "invalid_license":
		return "invalid license key — run 'wt auth login' to set a valid license"
	case "twin_not_licensed":
		return "this twin is not licensed for your organization"
	case "license_revoked":
		return "your license has been revoked — contact support"
	case "license_expired":
		return "your license has expired — renew at wondertwin.ai"
	default:
		return fmt.Sprintf("authorization failed: %s (HTTP %d)", e.Code, e.StatusCode)
	}
}

// IsFatal returns true if this error should prevent the twin from starting.
// All auth errors are fatal for licensed twins.
func (e *AuthError) IsFatal() bool {
	return true
}
