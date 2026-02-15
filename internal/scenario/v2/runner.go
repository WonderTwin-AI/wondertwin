package v2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/manifest"
)

// maxResponseBody is the maximum response body size the runner will read (10 MB).
const maxResponseBody = 10 * 1024 * 1024

// StepResult records the outcome of a single step.
type StepResult struct {
	Name     string
	Passed   bool
	Duration time.Duration
	Error    string // empty when passed
}

// Result records the outcome of an entire scenario.
type Result struct {
	ScenarioName string
	Passed       bool
	Steps        []StepResult
	Duration     time.Duration
}

// Runner executes v2 JSON scenarios against running twins.
type Runner struct {
	manifest *manifest.Manifest
	http     *http.Client
}

// NewRunner creates a Runner with the given manifest and a default HTTP client.
func NewRunner(m *manifest.Manifest) *Runner {
	return &Runner{
		manifest: m,
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Run executes a single scenario and returns its result.
func (r *Runner) Run(s *Scenario) (*Result, error) {
	start := time.Now()
	result := &Result{
		ScenarioName: s.Name,
		Passed:       true,
	}

	// Scoped variables for this run
	vars := make(map[string]string)
	for k, v := range s.Variables {
		vars[k] = v
	}

	// --- Setup phase ---
	if s.Setup != nil {
		if err := r.runSetup(s.Setup); err != nil {
			return nil, fmt.Errorf("setup failed: %w", err)
		}
	}

	// --- Steps phase ---
	var stopEarly bool
	for _, step := range s.Steps {
		if stopEarly {
			sr := StepResult{Name: step.Name, Error: "skipped: previous step failed"}
			result.Steps = append(result.Steps, sr)
			continue
		}
		sr := r.runStep(&step, vars)
		result.Steps = append(result.Steps, sr)
		if !sr.Passed {
			result.Passed = false
			if len(step.Capture) > 0 {
				stopEarly = true
			}
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

// runSetup executes the reset and seed_files operations.
func (r *Runner) runSetup(setup *Setup) error {
	// Reset twins
	for _, name := range setup.Reset {
		twin, err := r.manifest.Twin(name)
		if err != nil {
			return fmt.Errorf("reset %s: %w", name, err)
		}
		resp, err := r.http.Post(
			fmt.Sprintf("http://localhost:%d/admin/reset", twin.AdminPort),
			"application/json", nil,
		)
		if err != nil {
			return fmt.Errorf("reset %s: %w", name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("reset %s: status %d", name, resp.StatusCode)
		}
	}

	// Seed files
	for name, filePath := range setup.SeedFiles {
		twin, err := r.manifest.Twin(name)
		if err != nil {
			return fmt.Errorf("seed %s: %w", name, err)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("seed %s: reading %s: %w", name, filePath, err)
		}
		resp, err := r.http.Post(
			fmt.Sprintf("http://localhost:%d/admin/state", twin.AdminPort),
			"application/json",
			bytes.NewReader(data),
		)
		if err != nil {
			return fmt.Errorf("seed %s: %w", name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("seed %s: status %d", name, resp.StatusCode)
		}
	}

	return nil
}

// runStep executes a single scenario step and returns its result.
func (r *Runner) runStep(step *Step, vars map[string]string) StepResult {
	start := time.Now()
	sr := StepResult{Name: step.Name}
	defer func() { sr.Duration = time.Since(start) }()

	// Expand templates in the URL
	url, err := ExpandTemplates(step.Request.URL, r.manifest, vars)
	if err != nil {
		sr.Error = fmt.Sprintf("template expansion in url: %v", err)
		return sr
	}

	// Build request body
	var reqBody io.Reader
	if step.Request.Body != nil {
		bodyStr, err := buildBody(step.Request.Body, r.manifest, vars)
		if err != nil {
			sr.Error = fmt.Sprintf("building request body: %v", err)
			return sr
		}
		reqBody = strings.NewReader(bodyStr)
	}

	// Build HTTP request
	req, err := http.NewRequest(step.Request.Method, url, reqBody)
	if err != nil {
		sr.Error = fmt.Sprintf("building request: %v", err)
		return sr
	}

	// Set headers with template expansion
	for k, v := range step.Request.Headers {
		expanded, err := ExpandTemplates(v, r.manifest, vars)
		if err != nil {
			sr.Error = fmt.Sprintf("template expansion in header %q: %v", k, err)
			return sr
		}
		req.Header.Set(k, expanded)
	}

	// Set content-type for body requests if not already set
	if step.Request.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Execute request
	resp, err := r.http.Do(req)
	if err != nil {
		sr.Error = fmt.Sprintf("request failed: %v", err)
		return sr
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		sr.Error = fmt.Sprintf("reading response body: %v", err)
		return sr
	}

	// Capture variables from response
	if len(step.Capture) > 0 {
		for varName, jsonPath := range step.Capture {
			val, err := ExtractJSONPath(respBody, jsonPath)
			if err != nil {
				sr.Error = fmt.Sprintf("capture %q: %v", varName, err)
				return sr
			}
			vars[varName] = fmt.Sprintf("%v", val)
		}
	}

	// Run assertions
	if step.Assert != nil {
		if err := runAssertions(step.Assert, resp, respBody, r.manifest, vars); err != nil {
			sr.Error = err.Error()
			return sr
		}
	}

	sr.Passed = true
	return sr
}

// buildBody converts the request body to a JSON string with template expansion.
func buildBody(body any, m *manifest.Manifest, vars map[string]string) (string, error) {
	// If it's a string, expand templates directly
	if s, ok := body.(string); ok {
		return ExpandTemplates(s, m, vars)
	}

	// Otherwise marshal to JSON then expand templates
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling body: %w", err)
	}

	return ExpandTemplates(string(data), m, vars)
}

// runAssertions evaluates all assertions against the HTTP response.
func runAssertions(assert *Assert, resp *http.Response, body []byte, m *manifest.Manifest, vars map[string]string) error {
	// Assert status
	if assert.Status != 0 && resp.StatusCode != assert.Status {
		bodySnippet := string(body)
		if len(bodySnippet) > 200 {
			bodySnippet = bodySnippet[:200] + "..."
		}
		return fmt.Errorf("expected status %d, got %d; body: %s", assert.Status, resp.StatusCode, bodySnippet)
	}

	// Assert body_contains (with template expansion)
	if assert.BodyContains != "" {
		expanded, err := ExpandTemplates(assert.BodyContains, m, vars)
		if err != nil {
			return fmt.Errorf("template expansion in body_contains: %v", err)
		}
		if !strings.Contains(string(body), expanded) {
			return fmt.Errorf("body does not contain %q", expanded)
		}
	}

	// Assert headers
	for key, expected := range assert.Headers {
		actual := resp.Header.Get(key)
		if actual != expected {
			return fmt.Errorf("header %q: expected %q, got %q", key, expected, actual)
		}
	}

	// Assert body (JSONPath-based)
	if len(assert.Body) > 0 {
		// Expand templates in expected values before comparison
		expandedAssertions := make(map[string]any, len(assert.Body))
		for path, expected := range assert.Body {
			if s, ok := expected.(string); ok {
				expanded, err := ExpandTemplates(s, m, vars)
				if err != nil {
					return fmt.Errorf("template expansion in assertion %q: %v", path, err)
				}
				expandedAssertions[path] = expanded
			} else {
				expandedAssertions[path] = expected
			}
		}
		if err := EvaluateBodyAssertions(body, expandedAssertions); err != nil {
			return err
		}
	}

	return nil
}
