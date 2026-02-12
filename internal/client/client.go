// Package client provides an HTTP client for twin admin API endpoints.
package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// AdminClient talks to twin /admin/* endpoints.
type AdminClient struct {
	http *http.Client
}

// New creates an AdminClient with a 5-second timeout.
func New() *AdminClient {
	return &AdminClient{
		http: &http.Client{Timeout: 5 * time.Second},
	}
}

// Health checks GET /admin/health. Returns (ok, response body or error message).
func (c *AdminClient) Health(adminPort int) (bool, string) {
	resp, err := c.http.Get(fmt.Sprintf("http://localhost:%d/admin/health", adminPort))
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		return true, strings.TrimSpace(string(body))
	}
	return false, fmt.Sprintf("status %d: %s", resp.StatusCode, body)
}

// Reset calls POST /admin/reset on a twin.
func (c *AdminClient) Reset(adminPort int) (string, error) {
	resp, err := c.http.Post(
		fmt.Sprintf("http://localhost:%d/admin/reset", adminPort),
		"application/json", nil,
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("reset returned status %d: %s", resp.StatusCode, body)
	}
	return strings.TrimSpace(string(body)), nil
}

// Seed POSTs the contents of a JSON file to POST /admin/state on a twin.
func (c *AdminClient) Seed(adminPort int, filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading seed file: %w", err)
	}

	resp, err := c.http.Post(
		fmt.Sprintf("http://localhost:%d/admin/state", adminPort),
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("seed failed (status %d): %s", resp.StatusCode, body)
	}
	return strings.TrimSpace(string(body)), nil
}
