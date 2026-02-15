// Package testutil provides HTTP twin client, admin client, and assertion
// helpers for testing WonderTwin twins.
package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TwinClient is an HTTP client for interacting with a WonderTwin twin in tests.
type TwinClient struct {
	BaseURL    string
	HTTPClient *http.Client
	t          *testing.T
}

// NewTwinClient creates a client pointed at a test server.
func NewTwinClient(t *testing.T, server *httptest.Server) *TwinClient {
	return &TwinClient{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		t:          t,
	}
}

// NewTwinClientURL creates a client pointed at a specific URL.
func NewTwinClientURL(t *testing.T, baseURL string) *TwinClient {
	return &TwinClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{},
		t:          t,
	}
}

// Response wraps an HTTP response with helper methods.
type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
	t          *testing.T
}

// JSON unmarshals the response body into v.
func (r *Response) JSON(v any) {
	r.t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		r.t.Fatalf("failed to unmarshal response: %v\nbody: %s", err, string(r.Body))
	}
}

// JSONMap returns the response body as a map.
func (r *Response) JSONMap() map[string]any {
	r.t.Helper()
	var m map[string]any
	r.JSON(&m)
	return m
}

// AssertStatus asserts the response has the expected status code.
func (r *Response) AssertStatus(expected int) *Response {
	r.t.Helper()
	if r.StatusCode != expected {
		r.t.Errorf("expected status %d, got %d\nbody: %s", expected, r.StatusCode, string(r.Body))
	}
	return r
}

// AssertBodyContains asserts the response body contains the given substring.
func (r *Response) AssertBodyContains(substr string) *Response {
	r.t.Helper()
	if !strings.Contains(string(r.Body), substr) {
		r.t.Errorf("expected body to contain %q, got: %s", substr, string(r.Body))
	}
	return r
}

// Get performs a GET request.
func (c *TwinClient) Get(path string) *Response {
	c.t.Helper()
	return c.do("GET", path, nil, nil)
}

// Post performs a POST request with a JSON body.
func (c *TwinClient) Post(path string, body any) *Response {
	c.t.Helper()
	return c.do("POST", path, body, nil)
}

// PostForm performs a POST request with form-encoded body.
func (c *TwinClient) PostForm(path string, values map[string]string) *Response {
	c.t.Helper()
	form := make([]string, 0, len(values))
	for k, v := range values {
		form = append(form, fmt.Sprintf("%s=%s", k, v))
	}
	body := strings.Join(form, "&")

	req, err := http.NewRequest("POST", c.BaseURL+path, strings.NewReader(body))
	if err != nil {
		c.t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.doReq(req)
}

// Patch performs a PATCH request with a JSON body.
func (c *TwinClient) Patch(path string, body any) *Response {
	c.t.Helper()
	return c.do("PATCH", path, body, nil)
}

// Delete performs a DELETE request.
func (c *TwinClient) Delete(path string) *Response {
	c.t.Helper()
	return c.do("DELETE", path, nil, nil)
}

// DoWithHeaders performs a request with custom headers.
func (c *TwinClient) DoWithHeaders(method, path string, body any, headers map[string]string) *Response {
	c.t.Helper()
	return c.do(method, path, body, headers)
}

func (c *TwinClient) do(method, path string, body any, headers map[string]string) *Response {
	c.t.Helper()

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("failed to marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		c.t.Fatalf("failed to create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.doReq(req)
}

func (c *TwinClient) doReq(req *http.Request) *Response {
	c.t.Helper()

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		c.t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("failed to read response: %v", err)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    resp.Header,
		t:          c.t,
	}
}

// AdminClient provides convenience methods for the /admin/* control plane.
type AdminClient struct {
	*TwinClient
}

// NewAdminClient creates an admin client from a twin client.
func NewAdminClient(tc *TwinClient) *AdminClient {
	return &AdminClient{tc}
}

// Reset calls POST /admin/reset.
func (ac *AdminClient) Reset() *Response {
	ac.t.Helper()
	return ac.Post("/admin/reset", nil)
}

// GetState calls GET /admin/state.
func (ac *AdminClient) GetState() *Response {
	ac.t.Helper()
	return ac.Get("/admin/state")
}

// LoadState calls POST /admin/state with the given state data.
func (ac *AdminClient) LoadState(state any) *Response {
	ac.t.Helper()
	return ac.Post("/admin/state", state)
}

// InjectFault calls POST /admin/fault/{endpoint}.
func (ac *AdminClient) InjectFault(endpoint string, fault any) *Response {
	ac.t.Helper()
	return ac.Post("/admin/fault/"+strings.TrimPrefix(endpoint, "/"), fault)
}

// RemoveFault calls DELETE /admin/fault/{endpoint}.
func (ac *AdminClient) RemoveFault(endpoint string) *Response {
	ac.t.Helper()
	return ac.Delete("/admin/fault/" + strings.TrimPrefix(endpoint, "/"))
}

// GetRequests calls GET /admin/requests.
func (ac *AdminClient) GetRequests() *Response {
	ac.t.Helper()
	return ac.Get("/admin/requests")
}

// FlushWebhooks calls POST /admin/webhooks/flush.
func (ac *AdminClient) FlushWebhooks() *Response {
	ac.t.Helper()
	return ac.Post("/admin/webhooks/flush", nil)
}

// AdvanceTime calls POST /admin/time/advance.
func (ac *AdminClient) AdvanceTime(duration string) *Response {
	ac.t.Helper()
	return ac.Post("/admin/time/advance", map[string]string{"duration": duration})
}

// Health calls GET /admin/health.
func (ac *AdminClient) Health() *Response {
	ac.t.Helper()
	return ac.Get("/admin/health")
}
