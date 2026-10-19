package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
)

func newTestHandler(seed int64) *Handler {
	s := store.New()
	s.Rand = sim.NewRand(seed)
	return NewHandler(s, nil)
}

var hex15 = regexp.MustCompile(`^[0-9a-f]{15}$`)

func TestEnvelopePrettyPrintsSortedWithRequestID(t *testing.T) {
	h := newTestHandler(1)
	rec := httptest.NewRecorder()
	h.writeJSON(rec, http.StatusOK, map[string]any{"zeta": 1, "alpha": []int{1}})

	body := rec.Body.String()
	if strings.HasSuffix(body, "\n") {
		t.Fatalf("body has a trailing newline: %q", body)
	}
	if strings.Index(body, `"alpha"`) > strings.Index(body, `"request_id"`) ||
		strings.Index(body, `"request_id"`) > strings.Index(body, `"zeta"`) {
		t.Fatalf("keys not alphabetical:\n%s", body)
	}
	if !strings.Contains(body, "\n  \"alpha\": [\n    1\n  ]") {
		t.Fatalf("not 2-space indented:\n%s", body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !hex15.MatchString(out["request_id"].(string)) {
		t.Fatalf("request_id = %v, want 15 lowercase hex characters", out["request_id"])
	}
}

type presence struct {
	Optional *string `json:"optional,omitempty"`
	Nullable *string `json:"nullable"`
	Name     string  `json:"name"`
}

func TestEnvelopeKeepsPresenceRules(t *testing.T) {
	h := newTestHandler(1)
	rec := httptest.NewRecorder()
	h.writeJSON(rec, http.StatusOK, presence{Name: "x"})

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["optional"]; ok {
		t.Fatalf("optional non-nullable field was written: %v", out)
	}
	if v, ok := out["nullable"]; !ok || v != nil {
		t.Fatalf("required nullable field must be present as null: %v", out)
	}
}

func TestEnvelopeKeepsLargeNumbers(t *testing.T) {
	h := newTestHandler(1)
	rec := httptest.NewRecorder()
	h.writeJSON(rec, http.StatusOK, map[string]any{"amount": 12.50, "big": int64(9007199254740993)})

	body := rec.Body.String()
	if !strings.Contains(body, `"big": 9007199254740993`) || !strings.Contains(body, `"amount": 12.5`) {
		t.Fatalf("numbers changed:\n%s", body)
	}
}

func TestEnvelopeRequestIDIsSeeded(t *testing.T) {
	id := func(seed int64) string {
		rec := httptest.NewRecorder()
		newTestHandler(seed).writeJSON(rec, http.StatusOK, map[string]any{})
		var out map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out["request_id"]
	}
	if id(7) != id(7) {
		t.Fatal("same seed gave different request ids")
	}
	if id(7) == id(8) {
		t.Fatal("different seeds gave the same request id")
	}
}

func TestErrorsAreFlatWithSevenKeys(t *testing.T) {
	h := newTestHandler(1)
	rec := httptest.NewRecorder()
	h.writeError(rec, http.StatusBadRequest, "INVALID_INPUT", "INVALID_API_KEYS", "invalid client_id or secret provided")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	want := []string{"display_message", "documentation_url", "error_code", "error_message",
		"error_type", "request_id", "suggested_action"}
	if len(out) != len(want) {
		t.Fatalf("keys = %v, want exactly %v", out, want)
	}
	for _, k := range want {
		if _, ok := out[k]; !ok {
			t.Fatalf("missing key %s in %v", k, out)
		}
	}
	if out["display_message"] != nil || out["suggested_action"] != nil {
		t.Fatalf("display_message and suggested_action must be null: %v", out)
	}
	if out["documentation_url"] != "https://plaid.com/docs/errors/invalid-input/#invalid_api_keys" {
		t.Fatalf("documentation_url = %v", out["documentation_url"])
	}
}

func TestErrorsNotFoundIsNeverA405(t *testing.T) {
	h := newTestHandler(1)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/accounts/get"},
		{http.MethodPost, "/does/not/exist"},
		{http.MethodPost, "/accounts/get/"},
	} {
		rec := httptest.NewRecorder()
		h.notFound(rec, httptest.NewRequest(tc.method, tc.path, nil))
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != http.StatusNotFound || out["error_code"] != "NOT_FOUND" ||
			out["error_message"] != "invalid route: "+tc.path {
			t.Fatalf("%s %s: %d %v", tc.method, tc.path, rec.Code, out)
		}
	}
}
