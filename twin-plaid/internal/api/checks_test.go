package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// probeServer mounts checkRequest on a stand-in route, since the scaffold has
// no Plaid routes yet. The route echoes the client_id the checks stored.
func probeServer(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.NotFound(h.notFound)
	r.MethodNotAllowed(h.notFound)
	r.With(h.checkRequest).Post("/probe", func(w http.ResponseWriter, r *http.Request) {
		c, _ := CallFrom(r)
		h.writeJSON(w, http.StatusOK, map[string]any{"client_id": c.ClientID, "extra": c.Body["extra"]})
	})
	return r
}

type probe struct {
	method, path, contentType, body string
	headers                         map[string]string
}

func (p probe) do(t *testing.T, srv http.Handler) (int, map[string]any) {
	t.Helper()
	method, path, ct := p.method, p.path, p.contentType
	if method == "" {
		method = http.MethodPost
	}
	if path == "" {
		path = "/probe"
	}
	if ct == "" {
		ct = "application/json"
	}
	req := httptest.NewRequest(method, path, strings.NewReader(p.body))
	req.Header.Set("Content-Type", ct)
	for k, v := range p.headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body is not JSON: %q", rec.Body.String())
	}
	return rec.Code, out
}

func TestChecksRunInPlaidOrder(t *testing.T) {
	srv := probeServer(newTestHandler(1))
	cases := []struct {
		name   string
		p      probe
		status int
		code   string
	}{
		{"unknown route", probe{path: "/does/not/exist", body: "{}"}, 404, "NOT_FOUND"},
		{"wrong method is 404", probe{method: http.MethodGet, body: "{}"}, 404, "NOT_FOUND"},
		{"trailing slash is 404", probe{path: "/probe/", body: "{}"}, 404, "NOT_FOUND"},
		{"content type before parse", probe{contentType: "text/plain", body: "{not json"}, 415, "INVALID_HEADERS"},
		{"parse before presence", probe{body: "{not json"}, 400, "INVALID_BODY"},
		{"wrong type is a parse failure", probe{body: `{"client_id": 1, "secret": 2}`}, 400, "INVALID_BODY"},
		{"empty body is a parse failure", probe{body: ""}, 400, "INVALID_BODY"},
		{"presence before keys", probe{body: `{}`}, 400, "MISSING_FIELDS"},
		{"charset parameter accepted", probe{contentType: "application/json; charset=utf-8", body: `{}`}, 400, "MISSING_FIELDS"},
		{"keys before version", probe{body: `{"client_id":"0","secret":"0"}`,
			headers: map[string]string{"Plaid-Version": "banana"}}, 400, "UNSUPPORTED_API_VERSION"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, out := tc.p.do(t, srv)
			if status != tc.status || out["error_code"] != tc.code {
				t.Fatalf("got %d %v, want %d %s", status, out["error_code"], tc.status, tc.code)
			}
		})
	}
}

func TestCredentialsMissingNamesOnlyTheMissingFields(t *testing.T) {
	srv := probeServer(newTestHandler(1))
	for body, want := range map[string]string{
		`{}`:                               "the following required fields are missing: client_id, secret",
		`{"client_id":"a"}`:                "the following required fields are missing: secret",
		`{"secret":"b","access_token":""}`: "the following required fields are missing: client_id",
	} {
		_, out := probe{body: body}.do(t, srv)
		if out["error_code"] != "MISSING_FIELDS" || out["error_message"] != want {
			t.Fatalf("%s: %v", body, out)
		}
	}
}

func TestCredentialsMayComeFromHeaders(t *testing.T) {
	srv := probeServer(newTestHandler(1))
	status, out := probe{body: `{"extra": "kept"}`, headers: map[string]string{
		"PLAID-CLIENT-ID": "hdr-id", "PLAID-SECRET": "hdr-secret",
	}}.do(t, srv)
	if status != 200 || out["client_id"] != "hdr-id" || out["extra"] != "kept" {
		t.Fatalf("got %d %v", status, out)
	}
}

func TestCredentialsConfiguredPairIsEnforced(t *testing.T) {
	h := newTestHandler(1)
	h.SetCredentials("good-id", "good-secret")
	srv := probeServer(h)

	for _, body := range []string{
		`{"client_id":"good-id","secret":"wrong"}`,
		`{"client_id":"wrong","secret":"good-secret"}`,
	} {
		status, out := probe{body: body}.do(t, srv)
		if status != 400 || out["error_code"] != "INVALID_API_KEYS" || out["error_type"] != "INVALID_INPUT" {
			t.Fatalf("%s: %d %v", body, status, out)
		}
	}
	if status, _ := (probe{body: `{"client_id":"good-id","secret":"good-secret"}`}).do(t, srv); status != 200 {
		t.Fatalf("configured pair refused: %d", status)
	}
}

func TestCredentialsAnyNonEmptyPairByDefault(t *testing.T) {
	srv := probeServer(newTestHandler(1))
	if status, _ := (probe{body: `{"client_id":"x","secret":"y"}`}).do(t, srv); status != 200 {
		t.Fatalf("status = %d", status)
	}
}

func TestVersionAbsentAndSupportedPass(t *testing.T) {
	srv := probeServer(newTestHandler(1))
	for _, v := range []string{"", "2020-09-14"} {
		h := map[string]string{}
		if v != "" {
			h["Plaid-Version"] = v
		}
		if status, _ := (probe{body: `{"client_id":"x","secret":"y"}`, headers: h}).do(t, srv); status != 200 {
			t.Fatalf("Plaid-Version %q: status %d", v, status)
		}
	}
}

func TestVersionOtherValuesAreADeclared400(t *testing.T) {
	srv := probeServer(newTestHandler(1))
	for _, v := range []string{"2019-05-29", "2099-01-01", "banana"} {
		status, out := probe{body: `{"client_id":"x","secret":"y"}`,
			headers: map[string]string{"Plaid-Version": v}}.do(t, srv)
		if status != 400 || out["error_type"] != "INVALID_REQUEST" || out["error_code"] != "UNSUPPORTED_API_VERSION" {
			t.Fatalf("Plaid-Version %s: %d %v", v, status, out)
		}
	}
}
