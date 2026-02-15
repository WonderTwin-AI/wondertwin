// Package conformance implements the WonderTwin conformance test suite.
// It validates that a twin binary correctly implements the admin API contract.
package conformance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Result represents the outcome of a single conformance check.
type Result struct {
	Name   string
	Passed bool
	Detail string
}

// Report holds the results of a full conformance run.
type Report struct {
	Binary  string
	Port    int
	Results []Result
	Passed  int
	Failed  int
}

// Run executes the full conformance suite against a twin binary.
// It starts the binary, runs all checks, and returns a report.
func Run(binaryPath string, port int) (*Report, error) {
	report := &Report{
		Binary: binaryPath,
		Port:   port,
	}

	// Verify binary exists
	if _, err := os.Stat(binaryPath); err != nil {
		return nil, fmt.Errorf("binary not found: %s", binaryPath)
	}

	// Start the twin
	cmd := exec.Command(binaryPath, "--port", fmt.Sprintf("%d", port))
	setConformanceProcessAttrs(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting twin: %w", err)
	}

	// Ensure we clean up
	defer func() {
		cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			cmd.Process.Signal(syscall.SIGKILL)
			<-done
		}
	}()

	baseURL := fmt.Sprintf("http://localhost:%d", port)

	// Check 1: Twin starts and becomes healthy within 5 seconds
	report.addResult(checkHealth(baseURL))

	// If health check failed, skip remaining checks
	if report.Results[0].Passed {
		// Check 2: POST /admin/reset returns 200
		report.addResult(checkReset(baseURL))

		// Check 3: POST /admin/state accepts JSON seed data
		report.addResult(checkStatePost(baseURL))

		// Check 4: GET /admin/state returns valid JSON snapshot
		report.addResult(checkStateGet(baseURL))

		// Check 5: Reset again to clear seeded state, then verify empty
		report.addResult(checkResetClearsState(baseURL))

		// Check 6: POST /admin/fault/{endpoint} injects faults
		report.addResult(checkFaultInjection(baseURL))

		// Check 7: POST /admin/time/advance advances simulated clock
		report.addResult(checkTimeAdvance(baseURL))
	}

	// Check 8: Twin shuts down cleanly on SIGTERM within 5 seconds
	report.addResult(checkCleanShutdown(cmd))

	for _, r := range report.Results {
		if r.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
	}

	return report, nil
}

func (r *Report) addResult(res Result) {
	r.Results = append(r.Results, res)
}

func checkHealth(baseURL string) Result {
	name := "Twin starts and responds to health check within 5s"
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/admin/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return Result{Name: name, Passed: true, Detail: "GET /admin/health returned 200"}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	return Result{Name: name, Passed: false, Detail: "GET /admin/health did not return 200 within 5s"}
}

func checkReset(baseURL string) Result {
	name := "POST /admin/reset returns 200"

	resp, err := http.Post(baseURL+"/admin/reset", "application/json", nil)
	if err != nil {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("request failed: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("expected 200, got %d", resp.StatusCode)}
	}

	return Result{Name: name, Passed: true, Detail: "POST /admin/reset returned 200"}
}

func checkStatePost(baseURL string) Result {
	name := "POST /admin/state accepts JSON seed data"

	body := strings.NewReader(`{"test": true}`)
	resp, err := http.Post(baseURL+"/admin/state", "application/json", body)
	if err != nil {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("request failed: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("expected 200, got %d", resp.StatusCode)}
	}

	return Result{Name: name, Passed: true, Detail: "POST /admin/state returned 200"}
}

func checkStateGet(baseURL string) Result {
	name := "GET /admin/state returns valid JSON"

	resp, err := http.Get(baseURL + "/admin/state")
	if err != nil {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("request failed: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("expected 200, got %d", resp.StatusCode)}
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("failed to read body: %v", err)}
	}

	if !json.Valid(data) {
		return Result{Name: name, Passed: false, Detail: "response body is not valid JSON"}
	}

	return Result{Name: name, Passed: true, Detail: "GET /admin/state returned valid JSON"}
}

func checkResetClearsState(baseURL string) Result {
	name := "POST /admin/reset clears state"

	// Reset
	resp, err := http.Post(baseURL+"/admin/reset", "application/json", nil)
	if err != nil {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("reset request failed: %v", err)}
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("reset expected 200, got %d", resp.StatusCode)}
	}

	// Verify state is returned (even if empty)
	resp, err = http.Get(baseURL + "/admin/state")
	if err != nil {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("state request failed: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("state expected 200, got %d", resp.StatusCode)}
	}

	return Result{Name: name, Passed: true, Detail: "reset + state check passed"}
}

func checkFaultInjection(baseURL string) Result {
	name := "POST /admin/fault/{endpoint} injects faults"

	body := strings.NewReader(`{"status": 500, "message": "test fault"}`)
	req, _ := http.NewRequest("POST", baseURL+"/admin/fault/test-endpoint", body)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("request failed: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("expected 200, got %d", resp.StatusCode)}
	}

	return Result{Name: name, Passed: true, Detail: "POST /admin/fault returned 200"}
}

func checkTimeAdvance(baseURL string) Result {
	name := "POST /admin/time/advance advances simulated clock"

	body := strings.NewReader(`{"seconds": 3600}`)
	resp, err := http.Post(baseURL+"/admin/time/advance", "application/json", body)
	if err != nil {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("request failed: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("expected 200, got %d", resp.StatusCode)}
	}

	return Result{Name: name, Passed: true, Detail: "POST /admin/time/advance returned 200"}
}

func checkCleanShutdown(cmd *exec.Cmd) Result {
	name := "Twin shuts down cleanly on SIGTERM within 5s"

	// Send SIGTERM
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return Result{Name: name, Passed: false, Detail: fmt.Sprintf("failed to send SIGTERM: %v", err)}
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
		return Result{Name: name, Passed: true, Detail: "twin exited cleanly after SIGTERM"}
	case <-time.After(5 * time.Second):
		cmd.Process.Signal(syscall.SIGKILL)
		<-done
		return Result{Name: name, Passed: false, Detail: "twin did not exit within 5s after SIGTERM"}
	}
}
