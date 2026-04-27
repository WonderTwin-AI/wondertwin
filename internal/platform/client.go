// Package platform provides an HTTP client for the wondertwin-app API.
package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
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
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("validate key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("invalid API key")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
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
		body, _ := io.ReadAll(resp.Body)
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
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list org catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
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
		body, _ := io.ReadAll(resp.Body)
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
