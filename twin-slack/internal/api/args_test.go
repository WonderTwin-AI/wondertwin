package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// call sends one raw request and decodes the JSON answer. Slack answers API
// errors with HTTP 200 and ok:false, so the status is checked by the caller.
func call(t *testing.T, srv *httptest.Server, method, path, contentType, body string, authed bool) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if authed {
		req.Header.Set("Authorization", "Bearer xoxb-test-token")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s %s: answer is not JSON: %q", method, path, raw)
	}
	return resp.StatusCode, out
}

const (
	formType = "application/x-www-form-urlencoded"
	jsonType = "application/json"
)

func mustOK(t *testing.T, status int, m map[string]any) {
	t.Helper()
	if status != 200 || m["ok"] != true {
		t.Fatalf("want ok:true, got status %d body %v", status, m)
	}
}

func wantError(t *testing.T, status int, m map[string]any, code string) {
	t.Helper()
	if status != 200 || m["ok"] != false || m["error"] != code {
		t.Fatalf("want ok:false error:%s with HTTP 200, got status %d body %v", code, status, m)
	}
}

func historyTexts(t *testing.T, srv *httptest.Server, channel string) []string {
	t.Helper()
	status, m := call(t, srv, "GET", "/api/conversations.history?channel="+channel, "", "", true)
	mustOK(t, status, m)
	var texts []string
	for _, v := range m["messages"].([]any) {
		texts = append(texts, v.(map[string]any)["text"].(string))
	}
	return texts
}

func TestArgsGetWithQueryString(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")
	slackPost(tc, "/api/chat.postMessage", map[string]any{"channel": ch, "text": "one"}).AssertStatus(200)
	slackPost(tc, "/api/chat.postMessage", map[string]any{"channel": ch, "text": "two"}).AssertStatus(200)

	status, m := call(t, srv, "GET", "/api/conversations.history?channel="+ch+"&limit=1", "", "", true)
	mustOK(t, status, m)
	if got := len(m["messages"].([]any)); got != 1 {
		t.Fatalf("limit=1 from the query string should return 1 message, got %d", got)
	}
}

func TestArgsFormBodyCoercesTypesAndStructuredFields(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")

	form := url.Values{
		"channel": {ch},
		"text":    {"from a form"},
		"blocks":  {`[{"type":"section","text":{"type":"mrkdwn","text":"hi"}}]`},
	}
	status, m := call(t, srv, "POST", "/api/chat.postMessage", formType, form.Encode(), true)
	mustOK(t, status, m)

	status, h := call(t, srv, "POST", "/api/conversations.history", formType,
		url.Values{"channel": {ch}, "limit": {"5"}}.Encode(), true)
	mustOK(t, status, h)
	msg := h["messages"].([]any)[0].(map[string]any)
	blocks, ok := msg["blocks"].([]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("a JSON string in the blocks form field should be stored as a block list, got %#v", msg["blocks"])
	}
}

func TestArgsQueryAndFormBodyMerge(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")

	status, m := call(t, srv, "POST", "/api/chat.postMessage?channel="+ch, formType,
		url.Values{"text": {"split across query and form"}}.Encode(), true)
	mustOK(t, status, m)
	if got := historyTexts(t, srv, ch); len(got) != 1 || got[0] != "split across query and form" {
		t.Fatalf("history = %v", got)
	}
}

func TestArgsBodyValueWinsOverQuery(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")

	status, m := call(t, srv, "POST", "/api/chat.postMessage?text=from-query", formType,
		url.Values{"channel": {ch}, "text": {"from-body"}}.Encode(), true)
	mustOK(t, status, m)
	if got := historyTexts(t, srv, ch); len(got) != 1 || got[0] != "from-body" {
		t.Fatalf("history = %v", got)
	}
}

func TestArgsContentTypeParameters(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")
	body := url.Values{"channel": {ch}, "text": {"with a charset"}}.Encode()

	for _, ct := range []string{formType + "; charset=utf-8", formType + "; charset=ISO-8859-1", "text/plain"} {
		status, m := call(t, srv, "POST", "/api/chat.postMessage", ct, body, true)
		mustOK(t, status, m)
	}
	if got := len(historyTexts(t, srv, ch)); got != 3 {
		t.Fatalf("expected 3 messages, got %d", got)
	}
}

func TestArgsMultipartForm(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("channel", ch)
	_ = mw.WriteField("text", "from multipart")
	file, _ := mw.CreateFormFile("file", "ignored.txt")
	_, _ = file.Write([]byte("bytes that are not an argument"))
	_ = mw.Close()

	status, m := call(t, srv, "POST", "/api/chat.postMessage", mw.FormDataContentType(), buf.String(), true)
	mustOK(t, status, m)
	if got := historyTexts(t, srv, ch); len(got) != 1 || got[0] != "from multipart" {
		t.Fatalf("history = %v", got)
	}
}

func TestArgsJSONBody(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")

	// An explicit null means "use the default", per Slack's documentation.
	body := `{"channel":"` + ch + `","text":"json","thread_ts":null}`
	status, m := call(t, srv, "POST", "/api/chat.postMessage", jsonType+"; charset=utf-8", body, true)
	mustOK(t, status, m)

	// The query string is not used for arguments when the body is JSON.
	status, m = call(t, srv, "POST", "/api/chat.postMessage?channel=C-wrong", jsonType, body, true)
	mustOK(t, status, m)
}

func TestArgsBodyThatIsNotInterpretedWithoutItsContentType(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")

	// JSON text sent as a form body is not read as JSON.
	status, m := call(t, srv, "POST", "/api/chat.postMessage", formType, `{"channel":"`+ch+`","text":"x"}`, true)
	wantError(t, status, m, "channel_not_found")
}

func TestArgsEmptyBodyIsNoArguments(t *testing.T) {
	srv, _ := setupSlack(t)
	for _, ct := range []string{"", formType, jsonType} {
		status, m := call(t, srv, "POST", "/api/conversations.list", ct, "", true)
		mustOK(t, status, m)
	}
}

func TestArgsErrors(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")

	cases := []struct {
		name        string
		path        string
		contentType string
		body        string
		want        string
	}{
		{"unparseable JSON", "/api/chat.postMessage", jsonType, `{"channel":`, "invalid_json"},
		{"JSON array", "/api/chat.postMessage", jsonType, `["a"]`, "json_not_object"},
		{"JSON scalar", "/api/chat.postMessage", jsonType, `"a"`, "json_not_object"},
		{"body without a content type", "/api/chat.postMessage", "", "channel=" + ch + "&text=x", "missing_post_type"},
		{"content type outside the valid four", "/api/chat.postMessage", "application/xml", "<a/>", "invalid_post_type"},
		{"unparseable content type", "/api/chat.postMessage", "not a type", "a=b", "invalid_post_type"},
		{"charset outside utf-8 and iso-8859-1", "/api/chat.postMessage", formType + "; charset=utf-16", "channel=" + ch, "invalid_charset"},
		{"malformed form data", "/api/chat.postMessage", formType, "channel=%zz", "invalid_form_data"},
		{"multipart without a boundary", "/api/chat.postMessage", "multipart/form-data", "x", "invalid_form_data"},
		{"non-boolean argument", "/api/auth.revoke", formType, "test=maybe", "invalid_arguments"},
		{"array where a string is expected", "/api/chat.postMessage", jsonType, `{"channel":["` + ch + `"],"text":"x"}`, "invalid_array_arg"},
		{"object where a string is expected", "/api/chat.postMessage", jsonType, `{"channel":{"a":1},"text":"x"}`, "invalid_arguments"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, m := call(t, srv, "POST", c.path, c.contentType, c.body, true)
			wantError(t, status, m, c.want)
		})
	}
}

func TestArgsErrorsAreReportedAfterAuthentication(t *testing.T) {
	srv, _ := setupSlack(t)
	status, m := call(t, srv, "POST", "/api/chat.postMessage", jsonType, `{"channel":`, false)
	wantError(t, status, m, "not_authed")
}

func TestArgsErrorsAreReportedForMethodsThatReadNoArguments(t *testing.T) {
	srv, _ := setupSlack(t)
	status, m := call(t, srv, "POST", "/api/auth.test", jsonType, `{"a":`, true)
	wantError(t, status, m, "invalid_json")
}

func TestEveryMethodAnswersGetAndPost(t *testing.T) {
	twin := twincore.New(&twincore.Config{Name: "twin-slack-test"})
	api.NewHandler(store.New(), twin.Middleware()).Routes(twin.Router)

	verbs := map[string]map[string]bool{}
	err := chi.Walk(twin.Router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/api/") {
			if verbs[route] == nil {
				verbs[route] = map[string]bool{}
			}
			verbs[route][method] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(verbs) < 85 {
		t.Fatalf("expected the whole Web API surface under /api, found %d methods", len(verbs))
	}
	for route, got := range verbs {
		if !got["GET"] || !got["POST"] {
			t.Errorf("%s must answer both GET and POST, has %v", route, got)
		}
	}
}

// Slack adjusts a limit it cannot use and never rejects it, so a limit that is
// not an integer is not an invalid argument (apis/web-api/pagination).
func TestArgsUnusableLimitIsNotAnError(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")
	for _, limit := range []string{"abc", "1.5", "", "-1"} {
		status, m := call(t, srv, "GET", "/api/conversations.history?channel="+ch+"&limit="+limit, "", "", true)
		mustOK(t, status, m)
	}
}

func TestArgsUnusableJSONLimitIsNotAnError(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")
	for _, limit := range []string{`"abc"`, `"50"`, `1.5`, `null`, `true`, `[1]`} {
		body := `{"channel":"` + ch + `","limit":` + limit + `}`
		status, m := call(t, srv, "POST", "/api/conversations.history", "application/json", body, true)
		mustOK(t, status, m)
	}
	_, m := call(t, srv, "POST", "/api/conversations.history", "application/json", `{"channel":{"a":1}}`, true)
	if m["ok"] != false || m["error"] != "invalid_arguments" {
		t.Fatalf("type error on another field = %v", m)
	}
}
