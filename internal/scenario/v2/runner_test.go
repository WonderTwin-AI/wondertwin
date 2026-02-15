package v2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/internal/manifest"
)

func TestRunner_SimpleStep(t *testing.T) {
	// Create a test server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
	}))
	defer srv.Close()

	// Parse server port
	parts := strings.Split(srv.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])

	m := &manifest.Manifest{
		Twins: map[string]manifest.Twin{
			"stripe": {Port: port, AdminPort: port},
		},
	}

	runner := NewRunner(m)

	scenario := &Scenario{
		Name: "Simple test",
		Steps: []Step{
			{
				Name: "Health check",
				Request: Request{
					Method: "GET",
					URL:    "http://localhost:{{twins.stripe.port}}/health",
				},
				Assert: &Assert{
					Status: 200,
					Body: map[string]any{
						"$.status": "ok",
					},
				},
			},
		},
	}

	result, err := runner.Run(scenario)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected scenario to pass, step errors:")
		for _, sr := range result.Steps {
			if !sr.Passed {
				t.Errorf("  %s: %s", sr.Name, sr.Error)
			}
		}
	}
}

func TestRunner_VariableCapture(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if callCount == 1 {
			// First call: create customer
			json.NewEncoder(w).Encode(map[string]any{
				"id":     "cus_abc123",
				"email":  "test@example.com",
				"object": "customer",
			})
		} else {
			// Second call: retrieve customer (check URL contains captured ID)
			if !strings.Contains(r.URL.Path, "cus_abc123") {
				t.Errorf("expected URL to contain captured ID, got %s", r.URL.Path)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"id":    "cus_abc123",
				"email": "test@example.com",
			})
		}
	}))
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])

	m := &manifest.Manifest{
		Twins: map[string]manifest.Twin{
			"stripe": {Port: port, AdminPort: port},
		},
	}

	runner := NewRunner(m)

	scenario := &Scenario{
		Name: "Capture test",
		Steps: []Step{
			{
				Name: "Create customer",
				Request: Request{
					Method: "POST",
					URL:    "http://localhost:{{twins.stripe.port}}/v1/customers",
				},
				Capture: map[string]string{
					"customer_id": "$.id",
				},
				Assert: &Assert{
					Status: 200,
					Body: map[string]any{
						"$.object": "customer",
					},
				},
			},
			{
				Name: "Retrieve customer",
				Request: Request{
					Method: "GET",
					URL:    "http://localhost:{{twins.stripe.port}}/v1/customers/{{customer_id}}",
				},
				Assert: &Assert{
					Status: 200,
					Body: map[string]any{
						"$.id":    "cus_abc123",
						"$.email": "test@example.com",
					},
				},
			},
		},
	}

	result, err := runner.Run(scenario)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !result.Passed {
		for _, sr := range result.Steps {
			if !sr.Passed {
				t.Errorf("step %q failed: %s", sr.Name, sr.Error)
			}
		}
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", callCount)
	}
}

func TestRunner_BodyContains(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"message": "hello world"}`))
	}))
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])

	m := &manifest.Manifest{
		Twins: map[string]manifest.Twin{
			"test": {Port: port, AdminPort: port},
		},
	}

	runner := NewRunner(m)

	scenario := &Scenario{
		Name: "Body contains test",
		Steps: []Step{
			{
				Name: "Check body",
				Request: Request{
					Method: "GET",
					URL:    "http://localhost:{{twins.test.port}}/test",
				},
				Assert: &Assert{
					Status:       200,
					BodyContains: "hello",
				},
			},
		},
	}

	result, err := runner.Run(scenario)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected scenario to pass, got: %s", result.Steps[0].Error)
	}
}

func TestRunner_FailedAssertion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal"}`))
	}))
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])

	m := &manifest.Manifest{
		Twins: map[string]manifest.Twin{
			"test": {Port: port, AdminPort: port},
		},
	}

	runner := NewRunner(m)

	scenario := &Scenario{
		Name: "Failure test",
		Steps: []Step{
			{
				Name: "Expect 200",
				Request: Request{
					Method: "GET",
					URL:    "http://localhost:{{twins.test.port}}/test",
				},
				Assert: &Assert{
					Status: 200,
				},
			},
		},
	}

	result, err := runner.Run(scenario)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Passed {
		t.Error("expected scenario to fail")
	}
	if result.Steps[0].Passed {
		t.Error("expected step to fail")
	}
}

func TestRunner_InitialVariables(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])

	m := &manifest.Manifest{
		Twins: map[string]manifest.Twin{
			"test": {Port: port, AdminPort: port},
		},
	}

	runner := NewRunner(m)

	scenario := &Scenario{
		Name: "Variables test",
		Variables: map[string]string{
			"base_path": "/api/v1",
		},
		Steps: []Step{
			{
				Name: "Use variable",
				Request: Request{
					Method: "GET",
					URL:    "http://localhost:{{twins.test.port}}{{base_path}}/health",
				},
				Assert: &Assert{
					Status: 200,
				},
			},
		},
	}

	result, err := runner.Run(scenario)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected scenario to pass, got: %s", result.Steps[0].Error)
	}
}

func TestRunner_RequestWithBody(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "created"})
	}))
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])

	m := &manifest.Manifest{
		Twins: map[string]manifest.Twin{
			"test": {Port: port, AdminPort: port},
		},
	}

	runner := NewRunner(m)

	scenario := &Scenario{
		Name: "Body test",
		Steps: []Step{
			{
				Name: "Create resource",
				Request: Request{
					Method: "POST",
					URL:    "http://localhost:{{twins.test.port}}/create",
					Body: map[string]any{
						"name":  "test",
						"count": float64(5),
					},
				},
				Assert: &Assert{
					Status: 200,
				},
			},
		},
	}

	result, err := runner.Run(scenario)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected scenario to pass, got: %s", result.Steps[0].Error)
	}
	if receivedBody["name"] != "test" {
		t.Errorf("expected body name 'test', got %v", receivedBody["name"])
	}
}
