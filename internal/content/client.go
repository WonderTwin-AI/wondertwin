package content

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client fetches content payloads from the Content API.
type Client struct {
	BaseURL string
	Token   string
	http    *http.Client
}

// NewClient creates a content API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchContent retrieves the content payload for a twin at a specific version.
// It returns both the raw bytes (suitable for caching) and the parsed payload.
func (c *Client) FetchContent(twin, version string) ([]byte, *ContentPayload, error) {
	url := fmt.Sprintf("%s/v1/content/%s@%s", c.BaseURL, twin, version)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("creating content request: %w", err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching content for %s@%s: %w", twin, version, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading content response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, nil, fmt.Errorf("content API returned 401: invalid or missing license key")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil, fmt.Errorf("content not found for %s@%s", twin, version)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("content API returned HTTP %d: %s", resp.StatusCode, body)
	}

	var payload ContentPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, nil, fmt.Errorf("parsing content payload: %w", err)
	}

	return body, &payload, nil
}
