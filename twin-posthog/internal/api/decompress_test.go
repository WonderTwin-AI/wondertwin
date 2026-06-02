package api_test

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
)

func gzipBytes(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(b); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func postRaw(t *testing.T, base, path, contentType string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func readJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("decode response %q: %v", body, err)
	}
	return m
}

// TestCaptureGzipJS verifies posthog-js's ?compression=gzip-js path: raw gzip
// bytes in the body, JSON inside.
func TestCaptureGzipJS(t *testing.T) {
	srv, tc := setupPostHog(t)

	payload, _ := json.Marshal(map[string]any{
		"api_key":     "phc_test_key",
		"event":       "gzip_event",
		"distinct_id": "user_gz",
	})

	resp := postRaw(t, srv.URL, "/capture?compression=gzip-js", "text/plain", gzipBytes(t, payload))
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	m := readJSON(t, resp)
	if m["status"] != float64(1) {
		t.Errorf("expected status=1, got %v", m["status"])
	}

	r := tc.Get("/admin/events?event=gzip_event")
	r.AssertStatus(200)
	events := r.JSONMap()["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

// TestBatchGzipJS exercises the /batch endpoint with gzip-js compression.
func TestBatchGzipJS(t *testing.T) {
	srv, tc := setupPostHog(t)

	payload, _ := json.Marshal(map[string]any{
		"api_key": "phc_test_key",
		"batch": []map[string]any{
			{"event": "gz_batch_a", "distinct_id": "u1"},
			{"event": "gz_batch_b", "distinct_id": "u2"},
		},
	})

	resp := postRaw(t, srv.URL, "/batch?compression=gzip-js", "text/plain", gzipBytes(t, payload))
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	r := tc.Get("/admin/events?event=gz_batch_a")
	r.AssertStatus(200)
	if len(r.JSONMap()["events"].([]any)) != 1 {
		t.Errorf("missing gz_batch_a event")
	}
}

// TestCaptureContentEncodingGzip verifies the Content-Encoding: gzip fallback
// path (no ?compression query param).
func TestCaptureContentEncodingGzip(t *testing.T) {
	srv, _ := setupPostHog(t)

	payload, _ := json.Marshal(map[string]any{
		"api_key":     "phc_test_key",
		"event":       "ce_gzip",
		"distinct_id": "u",
	})

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/capture", bytes.NewReader(gzipBytes(t, payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// TestCaptureBase64 verifies the ?compression=base64 path.
func TestCaptureBase64(t *testing.T) {
	srv, _ := setupPostHog(t)

	payload, _ := json.Marshal(map[string]any{
		"api_key":     "phc_test_key",
		"event":       "b64_event",
		"distinct_id": "u",
	})
	encoded := base64.StdEncoding.EncodeToString(payload)

	resp := postRaw(t, srv.URL, "/capture?compression=base64", "text/plain", []byte(encoded))
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// TestCaptureFormDataWrapper verifies the legacy "data=<payload>" form-encoded
// shape that posthog-js falls back to for the /e endpoint.
func TestCaptureFormDataWrapper(t *testing.T) {
	srv, _ := setupPostHog(t)

	payload, _ := json.Marshal(map[string]any{
		"api_key":     "phc_test_key",
		"event":       "form_event",
		"distinct_id": "u",
	})
	form := "data=" + url.QueryEscape(string(payload))

	resp := postRaw(t, srv.URL, "/e", "application/x-www-form-urlencoded", []byte(form))
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// TestCaptureFormDataGzipBase64 covers the data=<base64-of-gzip-bytes> shape.
func TestCaptureFormDataGzipBase64(t *testing.T) {
	srv, _ := setupPostHog(t)

	payload, _ := json.Marshal(map[string]any{
		"api_key":     "phc_test_key",
		"event":       "form_gz_event",
		"distinct_id": "u",
	})
	b64 := base64.StdEncoding.EncodeToString(gzipBytes(t, payload))
	form := "data=" + url.QueryEscape(b64)

	resp := postRaw(t, srv.URL, "/e?compression=gzip-js", "application/x-www-form-urlencoded", []byte(form))
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
