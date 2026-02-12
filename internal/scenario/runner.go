package scenario

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/manifest"
)

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

// Runner executes scenarios against running twins.
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

	// --- Setup phase ---
	if err := r.runSetup(&s.Setup); err != nil {
		return nil, fmt.Errorf("setup failed: %w", err)
	}

	// --- Steps phase ---
	for _, step := range s.Steps {
		sr := r.runStep(&step)
		result.Steps = append(result.Steps, sr)
		if !sr.Passed {
			result.Passed = false
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

// runSetup executes the reset and seed operations defined in a scenario's setup block.
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

	// Seed twins
	for name, filePath := range setup.Seed {
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
			strings.NewReader(string(data)),
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
func (r *Runner) runStep(step *Step) StepResult {
	start := time.Now()
	sr := StepResult{Name: step.Name}

	// Expand templates in the URL
	url, err := ExpandTemplates(step.Request.URL, r.manifest)
	if err != nil {
		sr.Error = fmt.Sprintf("template expansion: %v", err)
		sr.Duration = time.Since(start)
		return sr
	}

	// Expand templates in the body
	body := step.Request.Body
	if body != "" {
		body, err = ExpandTemplates(body, r.manifest)
		if err != nil {
			sr.Error = fmt.Sprintf("template expansion in body: %v", err)
			sr.Duration = time.Since(start)
			return sr
		}
	}

	// Build request
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}

	req, err := http.NewRequest(step.Request.Method, url, reqBody)
	if err != nil {
		sr.Error = fmt.Sprintf("building request: %v", err)
		sr.Duration = time.Since(start)
		return sr
	}

	for k, v := range step.Request.Headers {
		req.Header.Set(k, v)
	}

	// Execute request
	resp, err := r.http.Do(req)
	if err != nil {
		sr.Error = fmt.Sprintf("request failed: %v", err)
		sr.Duration = time.Since(start)
		return sr
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		sr.Error = fmt.Sprintf("reading response body: %v", err)
		sr.Duration = time.Since(start)
		return sr
	}

	// Assert status
	if step.Assert.Status != 0 && resp.StatusCode != step.Assert.Status {
		sr.Error = fmt.Sprintf("expected status %d, got %d", step.Assert.Status, resp.StatusCode)
		sr.Duration = time.Since(start)
		return sr
	}

	// Assert body_contains
	if step.Assert.BodyContains != "" {
		if !strings.Contains(string(respBody), step.Assert.BodyContains) {
			sr.Error = fmt.Sprintf("body does not contain %q", step.Assert.BodyContains)
			sr.Duration = time.Since(start)
			return sr
		}
	}

	// Assert body_json (simple top-level key equality)
	if len(step.Assert.BodyJSON) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			sr.Error = fmt.Sprintf("body is not valid JSON: %v", err)
			sr.Duration = time.Since(start)
			return sr
		}
		for key, expected := range step.Assert.BodyJSON {
			actual, ok := parsed[key]
			if !ok {
				sr.Error = fmt.Sprintf("body_json: key %q not found in response", key)
				sr.Duration = time.Since(start)
				return sr
			}
			actualStr := fmt.Sprintf("%v", actual)
			if actualStr != expected {
				sr.Error = fmt.Sprintf("body_json: key %q expected %q, got %q", key, expected, actualStr)
				sr.Duration = time.Since(start)
				return sr
			}
		}
	}

	// All assertions passed
	sr.Passed = true
	sr.Duration = time.Since(start)
	return sr
}
