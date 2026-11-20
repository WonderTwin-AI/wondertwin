package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Capture v1 (POST /i/v1/analytics/events) takes a Bearer token, four
// PostHog-* headers and a body of {created_at, historical_migration?,
// capture_internal?, batch[]}. This file parses and validates a request and
// returns the record or the vendor's error; the route that serves it is a
// later slice. The vendor has no docs page for v1 (ph-br-013, 404 on
// 2026-11-16), so the wire format is the pinned capture service source plus
// the answers observed on 2026-11-17 (ph-br-001, ph-br-002, ph-ref-013).
// Wording not marked observed is INFERENCE from the source.

// captureV1ErrorURI is the error_uri every v1 error carries (observed).
const captureV1ErrorURI = "https://posthog.com/docs/api"

// CaptureEventV1 is one event of a v1 batch.
type CaptureEventV1 struct {
	Event      string
	UUID       string
	DistinctID string
	Timestamp  string
	SessionID  string
	WindowID   string
	Properties map[string]any
	Options    map[string]any
}

// CaptureBatchV1 is a validated v1 request.
type CaptureBatchV1 struct {
	Token               string
	SdkInfo             string
	Attempt             int
	RequestID           string
	RequestTimestamp    string
	CreatedAt           string
	HistoricalMigration bool
	CaptureInternal     bool
	Batch               []CaptureEventV1
}

// captureV1Error is a v1 request-level rejection, written as
// {error, error_description, error_uri}.
type captureV1Error struct {
	Status      int
	Code        string
	Description string
}

func (e *captureV1Error) Error() string { return e.Code + ": " + e.Description }

// body returns the JSON the vendor sends for the error.
func (e *captureV1Error) body() map[string]string {
	return map[string]string{
		"error":             e.Code,
		"error_description": e.Description,
		"error_uri":         captureV1ErrorURI,
	}
}

// requiredV1Headers are the four headers every v1 request carries, in the
// order the vendor lists them when they are missing (observed).
var requiredV1Headers = []string{
	"PostHog-Sdk-Info", "PostHog-Attempt", "PostHog-Request-Id", "PostHog-Request-Timestamp",
}

func v1Err(status int, code, format string, a ...any) *captureV1Error {
	return &captureV1Error{Status: status, Code: code, Description: fmt.Sprintf(format, a...)}
}

func v1HeaderErr(format string, a ...any) *captureV1Error {
	return v1Err(http.StatusBadRequest, "invalid_header_value", "invalid header value: "+format, a...)
}

// isUUID reports whether s has the 8-4-4-4-12 hexadecimal form.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
				return false
			}
		}
	}
	return true
}

// parseCaptureV1 validates a v1 request in the order the capture service
// does: Authorization, the four required headers, their values, the content
// type, then the body. body is the decoded payload (the route handles
// Content-Encoding). It returns the record, or the vendor's error.
func parseCaptureV1(h http.Header, body []byte) (*CaptureBatchV1, *captureV1Error) {
	auth := h.Get("Authorization")
	if auth == "" {
		return nil, v1Err(http.StatusUnauthorized, "missing_authorization", "missing Authorization header")
	}

	var missing []string
	for _, name := range requiredV1Headers {
		if h.Get(name) == "" {
			missing = append(missing, strconv.Quote(name))
		}
	}
	if len(missing) > 0 {
		return nil, v1Err(http.StatusBadRequest, "missing_required_headers",
			"request is missing required headers: [%s]", strings.Join(missing, ", "))
	}

	attemptRaw := h.Get("PostHog-Attempt")
	attempt, convErr := strconv.Atoi(attemptRaw)
	if convErr != nil || attempt < 1 {
		return nil, v1HeaderErr("PostHog-Attempt must be a positive integer: %s", attemptRaw)
	}
	if id := h.Get("PostHog-Request-Id"); !isUUID(id) {
		return nil, v1HeaderErr("PostHog-Request-Id is not a valid UUID: %s", id)
	}
	if ts := h.Get("PostHog-Request-Timestamp"); !validRFC3339(ts) {
		return nil, v1HeaderErr("PostHog-Request-Timestamp is not valid RFC 3339: %s", ts)
	}

	ct := h.Get("Content-Type")
	if mt, _, err := mime.ParseMediaType(ct); err != nil || mt != "application/json" {
		return nil, v1Err(http.StatusUnsupportedMediaType, "unsupported_content_type",
			"unsupported content type: %s", ct)
	}

	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if !strings.HasPrefix(auth, "Bearer ") || token == "" {
		// INFERENCE: a present but non-Bearer Authorization passed the header
		// checks above on the vendor (observed); how the token step rejects it
		// was not observed.
		return nil, v1Err(http.StatusUnauthorized, "invalid_api_token", "invalid API token")
	}

	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, v1Err(http.StatusBadRequest, "empty_body", "request body is empty")
	}

	var raw struct {
		CreatedAt           *string           `json:"created_at"`
		HistoricalMigration bool              `json:"historical_migration"`
		CaptureInternal     bool              `json:"capture_internal"`
		Batch               *[]rawCaptureV1El `json:"batch"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, v1ParseErr(body, err)
	}
	if raw.CreatedAt == nil {
		return nil, v1Err(http.StatusBadRequest, "request_parsing_error",
			"Failed to parse request body: created_at is missing.")
	}
	if raw.Batch == nil {
		return nil, v1Err(http.StatusBadRequest, "request_parsing_error",
			"Failed to parse request body: batch is missing.")
	}
	if len(*raw.Batch) == 0 {
		return nil, v1Err(http.StatusBadRequest, "empty_batch", "request batch is empty")
	}

	out := &CaptureBatchV1{
		Token:               token,
		SdkInfo:             h.Get("PostHog-Sdk-Info"),
		Attempt:             attempt,
		RequestID:           h.Get("PostHog-Request-Id"),
		RequestTimestamp:    h.Get("PostHog-Request-Timestamp"),
		CreatedAt:           *raw.CreatedAt,
		HistoricalMigration: raw.HistoricalMigration,
		CaptureInternal:     raw.CaptureInternal,
	}
	for i, el := range *raw.Batch {
		ev, field := el.event()
		if field != "" {
			// One bad event rejects the whole request, though the service
			// README describes a per-event drop (ph-br-001, observed for
			// event; the other fields are INFERENCE).
			return nil, v1Err(http.StatusBadRequest, "request_parsing_error",
				"Failed to parse request body: batch[%d].%s is missing.", i, field)
		}
		out.Batch = append(out.Batch, ev)
	}
	return out, nil
}

// rawCaptureV1El holds an event's fields as pointers so an absent key is
// told from an empty value.
type rawCaptureV1El struct {
	Event      *string        `json:"event"`
	UUID       *string        `json:"uuid"`
	DistinctID *string        `json:"distinct_id"`
	Timestamp  *string        `json:"timestamp"`
	SessionID  string         `json:"session_id"`
	WindowID   string         `json:"window_id"`
	Properties map[string]any `json:"properties"`
	Options    map[string]any `json:"options"`
}

// event returns the event, or the name of the first required key it lacks.
func (r rawCaptureV1El) event() (CaptureEventV1, string) {
	switch {
	case r.Event == nil:
		return CaptureEventV1{}, "event"
	case r.UUID == nil:
		return CaptureEventV1{}, "uuid"
	case r.DistinctID == nil:
		return CaptureEventV1{}, "distinct_id"
	case r.Timestamp == nil:
		return CaptureEventV1{}, "timestamp"
	case r.Properties == nil:
		return CaptureEventV1{}, "properties"
	case r.Options == nil:
		return CaptureEventV1{}, "options"
	}
	return CaptureEventV1{
		Event: *r.Event, UUID: *r.UUID, DistinctID: *r.DistinctID, Timestamp: *r.Timestamp,
		SessionID: r.SessionID, WindowID: r.WindowID, Properties: r.Properties, Options: r.Options,
	}, ""
}

// v1ParseErr maps a JSON decode failure to the vendor's request_parsing_error,
// naming the line and column ('wrong JSON type or value at line 1, column N',
// observed for a wrong body shape). Go reports a byte offset, so the column
// is the offset's position on its line; the vendor's own column differs.
func v1ParseErr(body []byte, err error) *captureV1Error {
	var offset int64
	var syn *json.SyntaxError
	var typ *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syn):
		offset = syn.Offset
	case errors.As(err, &typ):
		offset = typ.Offset
	}
	line, col := 1, 0
	for i := 0; i < int(offset) && i < len(body); i++ {
		if body[i] == '\n' {
			line++
			col = 0
			continue
		}
		col++
	}
	return v1Err(http.StatusBadRequest, "request_parsing_error",
		"Failed to parse request body: wrong JSON type or value at line %d, column %d", line, col)
}

func validRFC3339(s string) bool {
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}
