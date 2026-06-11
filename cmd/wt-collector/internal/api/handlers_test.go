package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-collector/internal/store"
)

const testIngestKey = "test-ingest-secret"

// testServer creates an httptest.Server backed by an in-memory SQLite database.
func testServer(t *testing.T) *httptest.Server {
	t.Helper()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	events := store.NewEventStore(db)
	handler := NewHandler(events, testIngestKey)

	r := chi.NewRouter()
	handler.Routes(r)

	return httptest.NewServer(r)
}

// postJSON is a helper that POSTs JSON and returns the response.
func postJSON(t *testing.T, url string, body any, headers ...string) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// sampleEvents returns a batch of valid telemetry events for testing.
func sampleEvents() []store.RawEvent {
	return []store.RawEvent{
		{
			EventType:   "api.call",
			Twin:        "stripe",
			TwinVersion: "0.8.0",
			OrgID:       "org_acme",
			Timestamp:   1700000000,
			Payload:     map[string]any{"method": "POST", "path": "/v1/charges"},
		},
		{
			EventType:   "api.call",
			Twin:        "stripe",
			TwinVersion: "0.8.0",
			OrgID:       "org_acme",
			Timestamp:   1700000001,
			Payload:     map[string]any{"method": "GET", "path": "/v1/customers"},
		},
		{
			EventType:   "webhook.sent",
			Twin:        "qbo",
			TwinVersion: "0.3.0",
			OrgID:       "org_bigcorp",
			Timestamp:   1700000002,
			Payload:     map[string]any{"event": "invoice.created"},
		},
	}
}

func TestHealth(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestIngest_Success(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	events := sampleEvents()
	resp := postJSON(t, srv.URL+"/telemetry/v1/ingest", events,
		"Authorization", "Bearer "+testIngestKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	accepted, ok := body["accepted"].(float64)
	if !ok || int(accepted) != len(events) {
		t.Errorf("accepted = %v, want %d", body["accepted"], len(events))
	}
	total, ok := body["total"].(float64)
	if !ok || int(total) != len(events) {
		t.Errorf("total = %v, want %d", body["total"], len(events))
	}
}

func TestIngest_EmptyBatch(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/telemetry/v1/ingest", []store.RawEvent{},
		"Authorization", "Bearer "+testIngestKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestIngest_InvalidJSON(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/telemetry/v1/ingest",
		strings.NewReader("not valid json{{{"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testIngestKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestIngest_AuthRequired(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	// No Authorization header.
	resp := postJSON(t, srv.URL+"/telemetry/v1/ingest", sampleEvents())
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestIngest_WrongKey(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/telemetry/v1/ingest", sampleEvents(),
		"Authorization", "Bearer wrong-key")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// TestIngest_BearerCompareEdgeCases exercises the constant-time bearer
// compare across the three failure shapes that share a single code
// path: same-length-wrong, different-length-wrong, and empty token
// after "Bearer ". A regression to plain != would still pass the
// same-length case via early-return on byte mismatch — the value of
// the table is forcing the compare to be branch-free per case.
func TestIngest_BearerCompareEdgeCases(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	// testIngestKey == "test-ingest-secret" (18 bytes).
	cases := []struct {
		name   string
		bearer string
	}{
		{"wrong-same-length", "wrong-ingest-stuff"},   // also 18 bytes
		{"wrong-shorter", "short"},
		{"wrong-longer", "test-ingest-secret-but-longer"},
		{"empty-after-prefix", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/telemetry/v1/ingest", sampleEvents(),
				"Authorization", "Bearer "+tc.bearer)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s: status = %d, want %d", tc.name, resp.StatusCode, http.StatusUnauthorized)
			}
		})
	}
}

func TestStats(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	// Ingest some events first.
	resp := postJSON(t, srv.URL+"/telemetry/v1/ingest", sampleEvents(),
		"Authorization", "Bearer "+testIngestKey)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ingest status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Now check stats.
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/telemetry/v1/stats", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testIngestKey)

	statsResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer statsResp.Body.Close()

	if statsResp.StatusCode != http.StatusOK {
		t.Fatalf("stats status = %d, want %d", statsResp.StatusCode, http.StatusOK)
	}

	var body map[string]any
	json.NewDecoder(statsResp.Body).Decode(&body)

	totalEvents, ok := body["total_events"].(float64)
	if !ok || int(totalEvents) != 3 {
		t.Errorf("total_events = %v, want 3", body["total_events"])
	}

	byTwin, ok := body["by_twin"].(map[string]any)
	if !ok {
		t.Fatalf("by_twin is not a map: %T", body["by_twin"])
	}

	stripeCount, ok := byTwin["stripe"].(float64)
	if !ok || int(stripeCount) != 2 {
		t.Errorf("by_twin[stripe] = %v, want 2", byTwin["stripe"])
	}

	qboCount, ok := byTwin["qbo"].(float64)
	if !ok || int(qboCount) != 1 {
		t.Errorf("by_twin[qbo] = %v, want 1", byTwin["qbo"])
	}
}
