// Package platform provides an HTTP client for the wondertwin-app API.
package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/wondertwin-ai/wondertwin/internal/httpclient"
	"github.com/wondertwin-ai/wondertwin/internal/httpio"
	"net/http"
	"net/url"
	"time"
)

const (
	// DefaultBaseURL is the production wondertwin-app API.
	DefaultBaseURL = "https://api.wondertwin.ai"

	requestTimeout = 10 * time.Second
)

// Client communicates with the wondertwin-app platform API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New creates a platform client. If baseURL is empty, DefaultBaseURL is used.
func New(baseURL, apiKey string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: httpclient.New(httpclient.WithTimeout(requestTimeout)),
	}
}

// ValidateKeyResponse is the result of validating an API key.
type ValidateKeyResponse struct {
	OrgID   string `json:"org_id"`
	OrgSlug string `json:"org_slug"`
	KeyID   string `json:"key_id"`
}

// ValidateKey checks an API key against the platform and returns org context.
func (c *Client) ValidateKey(ctx context.Context, apiKey string) (*ValidateKeyResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/auth/validate", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("validate key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("invalid API key")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes))
		return nil, fmt.Errorf("validate key: HTTP %d: %s", resp.StatusCode, body)
	}

	var result ValidateKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// CatalogEntry matches the TwinCatalogEntry from the catalog API.
type CatalogEntry struct {
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	Description string     `json:"description"`
	Category    string     `json:"category"`
	Tier        string     `json:"tier"`
	SDKTargets  SDKTargets `json:"sdk_targets"`
	Coverage    Coverage   `json:"coverage"`
	Status      string     `json:"status"`
}

// SDKTargets groups primary and additional SDK targets.
type SDKTargets struct {
	Primary    SDKTarget   `json:"primary"`
	Additional []SDKTarget `json:"additional,omitempty"`
}

// SDKTarget identifies a language SDK package.
type SDKTarget struct {
	Package  string `json:"package"`
	Language string `json:"language"`
	Version  string `json:"version,omitempty"`
}

// Coverage describes twin API coverage.
type Coverage struct {
	EstimatedPct   int    `json:"estimated_pct"`
	ResourceCount  int    `json:"resource_count"`
	WebhookSupport bool   `json:"webhook_support"`
	AuthPattern    string `json:"auth_pattern"`
}

// OrgCatalogEntry extends CatalogEntry with org-specific state.
type OrgCatalogEntry struct {
	CatalogEntry
	EntitlementState string `json:"entitlement_state"`
	AvailableAction  string `json:"available_action"`
}

// Category represents a catalog category with twin count.
type Category struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ListTwins fetches the public catalog with optional filters.
func (c *Client) ListTwins(ctx context.Context, category, tier, sdkLanguage, search string) ([]CatalogEntry, error) {
	u, err := url.Parse(c.baseURL + "/v1/catalog/twins")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	q := u.Query()
	if category != "" {
		q.Set("category", category)
	}
	if tier != "" {
		q.Set("tier", tier)
	}
	if sdkLanguage != "" {
		q.Set("sdk_language", sdkLanguage)
	}
	if search != "" {
		q.Set("search", search)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list twins: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes))
		return nil, fmt.Errorf("list twins: HTTP %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Twins []CatalogEntry `json:"twins"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result.Twins, nil
}

// ListOrgCatalog fetches the org-scoped catalog with entitlement enrichment.
func (c *Client) ListOrgCatalog(ctx context.Context, orgID, category, search string) ([]OrgCatalogEntry, error) {
	u, err := url.Parse(fmt.Sprintf("%s/v1/orgs/%s/catalog", c.baseURL, orgID))
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	q := u.Query()
	if category != "" {
		q.Set("category", category)
	}
	if search != "" {
		q.Set("search", search)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list org catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes))
		return nil, fmt.Errorf("list org catalog: HTTP %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Twins []OrgCatalogEntry `json:"twins"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result.Twins, nil
}

// ListCategories fetches the catalog categories.
func (c *Client) ListCategories(ctx context.Context) ([]Category, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/catalog/categories", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes))
		return nil, fmt.Errorf("list categories: HTTP %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Categories []Category `json:"categories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result.Categories, nil
}

// SubscribeResponse is the unified response from the subscribe endpoint.
type SubscribeResponse struct {
	Action      string `json:"action"`
	TwinName    string `json:"twin_name"`
	TrialEndsAt string `json:"trial_ends_at,omitempty"`
	CheckoutURL string `json:"checkout_url,omitempty"`
	SignupURL   string `json:"signup_url,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
	Message     string `json:"message,omitempty"`
}

// Subscribe calls the unified subscribe endpoint.
func (c *Client) Subscribe(ctx context.Context, orgID, twinName string) (*SubscribeResponse, error) {
	body, err := json.Marshal(map[string]string{"twin_name": twinName})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/v1/orgs/%s/subscribe", c.baseURL, orgID), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("subscribe: %w", err)
	}
	defer resp.Body.Close()

	var result SubscribeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes))
		return nil, fmt.Errorf("subscribe: HTTP %d: %s", resp.StatusCode, respBody)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("subscribe: HTTP %d: %s", resp.StatusCode, result.Message)
	}

	return &result, nil
}

// TwinRequest represents a twin request in the system.
type TwinRequest struct {
	ID           string `json:"id"`
	OrgID        string `json:"org_id"`
	ServiceName  string `json:"service_name"`
	ServiceURL   string `json:"service_url,omitempty"`
	RequestType  string `json:"request_type"`
	CategoryHint string `json:"category_hint,omitempty"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// SubmitTwinRequest submits a structured twin request.
func (c *Client) SubmitTwinRequest(ctx context.Context, orgID, serviceName, serviceURL, categoryHint string) (*TwinRequest, error) {
	payload := map[string]string{"service_name": serviceName}
	if serviceURL != "" {
		payload["service_url"] = serviceURL
	}
	if categoryHint != "" {
		payload["category_hint"] = categoryHint
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/v1/orgs/%s/twin-requests", c.baseURL, orgID), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("submit twin request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes))
		return nil, fmt.Errorf("submit twin request: HTTP %d: %s", resp.StatusCode, respBody)
	}

	var result TwinRequest
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// ListTwinRequests returns all twin requests for an org.
func (c *Client) ListTwinRequests(ctx context.Context, orgID string) ([]TwinRequest, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/v1/orgs/%s/twin-requests", c.baseURL, orgID), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list twin requests: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes))
		return nil, fmt.Errorf("list twin requests: HTTP %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Requests []TwinRequest `json:"requests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result.Requests, nil
}

// GetTwinRequest returns a single twin request.
func (c *Client) GetTwinRequest(ctx context.Context, orgID, requestID string) (*TwinRequest, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/v1/orgs/%s/twin-requests/%s", c.baseURL, orgID, requestID), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get twin request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("request not found: %s", requestID)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes))
		return nil, fmt.Errorf("get twin request: HTTP %d: %s", resp.StatusCode, body)
	}

	var result TwinRequest
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}
