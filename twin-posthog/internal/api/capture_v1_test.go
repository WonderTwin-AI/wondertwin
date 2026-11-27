package api

import (
	"net/http"
	"testing"
)

const (
	v1UUID = "0190a7c5-aaaa-7bbb-8ccc-0123456789ab"
	v1Time = "2026-11-17T10:00:00Z"
)

// v1Headers returns a complete, valid header set; a test drops or changes
// entries to reach one rejection.
func v1Headers(mut func(http.Header)) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer phc_test")
	h.Set("PostHog-Sdk-Info", "posthog-go/1.44.1")
	h.Set("PostHog-Attempt", "1")
	h.Set("PostHog-Request-Id", v1UUID)
	h.Set("PostHog-Request-Timestamp", v1Time)
	h.Set("Content-Type", "application/json")
	if mut != nil {
		mut(h)
	}
	return h
}

const v1Event = `{"event":"pv","uuid":"` + v1UUID + `","distinct_id":"u1","timestamp":"` + v1Time + `","options":{},"properties":{"a":1}}`

// TestParseCaptureV1 holds the capture v1 request parser against the
// vendor's answers observed on 2026-11-17 (ph-br-001, ph-br-002, ph-ref-013):
// the status, error code and description of every request-level rejection,
// and the record a valid request yields.
func TestParseCaptureV1(t *testing.T) {
	const allMissing = `request is missing required headers: ["PostHog-Sdk-Info", "PostHog-Attempt", "PostHog-Request-Id", "PostHog-Request-Timestamp"]`
	cases := []struct {
		name   string
		header http.Header
		body   string
		status int
		code   string
		desc   string
	}{
		{"no authorization", v1Headers(func(h http.Header) { h.Del("Authorization") }), `{}`, 401, "missing_authorization", "missing Authorization header"},
		{"bearer only", func() http.Header {
			h := http.Header{}
			h.Set("Authorization", "Bearer phc_test")
			return h
		}(), `{}`, 400, "missing_required_headers", allMissing},
		{"one header missing", v1Headers(func(h http.Header) { h.Del("PostHog-Attempt") }), `{}`, 400, "missing_required_headers", `request is missing required headers: ["PostHog-Attempt"]`},
		{"attempt zero", v1Headers(func(h http.Header) { h.Set("PostHog-Attempt", "0") }), `{}`, 400, "invalid_header_value", "invalid header value: PostHog-Attempt must be a positive integer: 0"},
		{"attempt not a number", v1Headers(func(h http.Header) { h.Set("PostHog-Attempt", "abc") }), `{}`, 400, "invalid_header_value", "invalid header value: PostHog-Attempt must be a positive integer: abc"},
		{"request id not a uuid", v1Headers(func(h http.Header) { h.Set("PostHog-Request-Id", "nope") }), `{}`, 400, "invalid_header_value", "invalid header value: PostHog-Request-Id is not a valid UUID: nope"},
		{"timestamp not rfc3339", v1Headers(func(h http.Header) { h.Set("PostHog-Request-Timestamp", "yesterday") }), `{}`, 400, "invalid_header_value", "invalid header value: PostHog-Request-Timestamp is not valid RFC 3339: yesterday"},
		{"content type text/plain", v1Headers(func(h http.Header) { h.Set("Content-Type", "text/plain") }), `{}`, 415, "unsupported_content_type", "unsupported content type: text/plain"},
		{"empty body", v1Headers(nil), ``, 400, "empty_body", "request body is empty"},
		{"created_at missing", v1Headers(nil), `{"batch":[]}`, 400, "request_parsing_error", "Failed to parse request body: created_at is missing."},
		{"empty batch", v1Headers(nil), `{"created_at":"` + v1Time + `","batch":[]}`, 400, "empty_batch", "request batch is empty"},
		{"event key missing rejects the request", v1Headers(nil),
			`{"created_at":"` + v1Time + `","batch":[` + v1Event + `,{"uuid":"` + v1UUID + `","distinct_id":"u1","timestamp":"` + v1Time + `","options":{},"properties":{}}]}`,
			400, "request_parsing_error", "Failed to parse request body: batch[1].event is missing."},
		{"wrong body shape", v1Headers(nil), `[]`, 400, "request_parsing_error", "Failed to parse request body: wrong JSON type or value at line 1, column 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCaptureV1(tc.header, []byte(tc.body))
			if err == nil {
				t.Fatalf("parsed %+v, want %s", got, tc.code)
			}
			if err.Status != tc.status || err.Code != tc.code || err.Description != tc.desc {
				t.Errorf("got %d %s %q\nwant %d %s %q", err.Status, err.Code, err.Description, tc.status, tc.code, tc.desc)
			}
			if b := err.body(); b["error_uri"] != "https://posthog.com/docs/api" {
				t.Errorf("error_uri %q", b["error_uri"])
			}
		})
	}

	t.Run("valid request", func(t *testing.T) {
		body := `{"created_at":"` + v1Time + `","historical_migration":true,"batch":[` + v1Event + `]}`
		got, err := parseCaptureV1(v1Headers(nil), []byte(body))
		if err != nil {
			t.Fatalf("rejected: %v", err)
		}
		if got.Token != "phc_test" || got.Attempt != 1 || got.RequestID != v1UUID || got.SdkInfo != "posthog-go/1.44.1" {
			t.Errorf("envelope = %+v", got)
		}
		if !got.HistoricalMigration || got.CreatedAt != v1Time || len(got.Batch) != 1 {
			t.Errorf("body = %+v", got)
		}
		if e := got.Batch[0]; e.Event != "pv" || e.UUID != v1UUID || e.DistinctID != "u1" || e.Properties["a"] != float64(1) {
			t.Errorf("event = %+v", e)
		}
	})

	t.Run("an all-zeros uuid is accepted (ph-br-003)", func(t *testing.T) {
		zero := "00000000-0000-0000-0000-000000000000"
		body := `{"created_at":"` + v1Time + `","batch":[{"event":"pv","uuid":"` + zero + `","distinct_id":"u1","timestamp":"` + v1Time + `","options":{},"properties":{}}]}`
		got, err := parseCaptureV1(v1Headers(nil), []byte(body))
		if err != nil || got.Batch[0].UUID != zero {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
}
