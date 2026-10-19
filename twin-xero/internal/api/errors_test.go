package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func newTestHandler() *Handler {
	return NewHandler(store.New(), twincore.New(&twincore.Config{Name: "twin-xero-test"}).Middleware())
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, rec.Body)
	}
	return m
}

func TestErrorsUnauthorizedIsProblemDetailsWithAChallenge(t *testing.T) {
	cases := []struct {
		name      string
		tokenSent bool
		challenge string
	}{
		{"no token", false, "Bearer"},
		{"bad token", true, `Bearer error="invalid_token"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api.xro/2.0/Invoices", nil)
			req.Header.Set("Accept", "text/xml") // Xero answers JSON regardless
			newTestHandler().writeUnauthorized(rec, req, tc.tokenSent)

			if rec.Code != 401 {
				t.Fatalf("status = %d", rec.Code)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != tc.challenge {
				t.Fatalf("WWW-Authenticate = %q, want %q", got, tc.challenge)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("Content-Type = %q", ct)
			}
			body := decode(t, rec)
			if v, ok := body["Type"]; !ok || v != nil {
				t.Fatalf("Type = %v (present %v), want null", v, ok)
			}
			if body["Title"] != "Unauthorized" || body["Status"] != float64(401) || body["Detail"] != "AuthenticationUnsuccessful" {
				t.Fatalf("body = %v", body)
			}
			if ext, ok := body["Extensions"].(map[string]any); !ok || len(ext) != 0 {
				t.Fatalf("Extensions = %v, want {}", body["Extensions"])
			}
			if body["Instance"] == "" {
				t.Fatal("Instance is empty")
			}
		})
	}
}

func TestErrorsInstanceIsTheCorrelationID(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(twincore.ContextWithCorrelationID(req.Context(), "corr-123"))
	newTestHandler().writeUnauthorized(rec, req, false)

	if got := decode(t, rec)["Instance"]; got != "corr-123" {
		t.Fatalf("Instance = %v, want corr-123", got)
	}
}

func TestErrorsValidationExceptionEchoesTheSubmittedObject(t *testing.T) {
	rec := httptest.NewRecorder()
	writeValidationException(rec,
		validationElement{
			Object:   map[string]any{"Name": "", "EmailAddress": "a@b.c"},
			Messages: []string{"Please enter a contact name"},
		},
		validationElement{Object: nil},
	)

	if rec.Code != 400 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := decode(t, rec)
	if body["ErrorNumber"] != float64(10) || body["Type"] != "ValidationException" ||
		body["Message"] != "A validation exception occurred" {
		t.Fatalf("body = %v", body)
	}
	els := body["Elements"].([]any)
	if len(els) != 2 {
		t.Fatalf("Elements = %v", els)
	}
	first := els[0].(map[string]any)
	if first["EmailAddress"] != "a@b.c" {
		t.Fatalf("element does not echo the submitted object: %v", first)
	}
	msgs := first["ValidationErrors"].([]any)
	if len(msgs) != 1 || msgs[0].(map[string]any)["Message"] != "Please enter a contact name" {
		t.Fatalf("ValidationErrors = %v", msgs)
	}
	if errs, ok := els[1].(map[string]any)["ValidationErrors"].([]any); !ok || len(errs) != 0 {
		t.Fatalf("an element with no messages must carry [], got %v", els[1])
	}
}

var msdateValue = regexp.MustCompile(`^/Date\(-?\d+\+0000\)/$`)

func TestEnvelopeWrapsAPluralArray(t *testing.T) {
	h := newTestHandler()
	rec := httptest.NewRecorder()
	h.writeEnvelope(rec, http.StatusOK, "Invoices", []map[string]string{{"InvoiceID": "abc"}})

	if !strings.HasPrefix(rec.Body.String(), `{"Id":"`) {
		t.Fatalf("Id must lead the envelope: %s", rec.Body)
	}
	body := decode(t, rec)
	if body["Status"] != "OK" || body["ProviderName"] != providerName {
		t.Fatalf("body = %v", body)
	}
	if s, _ := body["DateTimeUTC"].(string); !msdateValue.MatchString(s) {
		t.Fatalf("DateTimeUTC = %v, want /Date(ms+0000)/", body["DateTimeUTC"])
	}
	if inv, ok := body["Invoices"].([]any); !ok || len(inv) != 1 {
		t.Fatalf("Invoices = %v, want a one-element array", body["Invoices"])
	}
}

func TestEnvelopeNilListIsAnEmptyArray(t *testing.T) {
	var none []string
	rec := httptest.NewRecorder()
	newTestHandler().writeEnvelope(rec, http.StatusOK, "Contacts", none)

	if got, ok := decode(t, rec)["Contacts"].([]any); !ok || len(got) != 0 {
		t.Fatalf("Contacts = %v, want []", decode(t, rec)["Contacts"])
	}
}

func TestEnvelopeKeepsTheStatusAndFollowsTheSimulatedClock(t *testing.T) {
	h := newTestHandler()
	h.store.Clock.Advance(24 * time.Hour)
	rec := httptest.NewRecorder()
	h.writeEnvelope(rec, http.StatusAccepted, "Payments", []string{})

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	var at struct {
		DateTimeUTC Msdate `json:"DateTimeUTC"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &at); err != nil {
		t.Fatal(err)
	}
	if d := time.Until(at.DateTimeUTC.Time()); d < 23*time.Hour || d > 25*time.Hour {
		t.Fatalf("DateTimeUTC is %s from now, want about 24h", d)
	}
}
