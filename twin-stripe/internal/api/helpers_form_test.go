package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

// formResp is a response from stripeForm.
type formResp struct {
	t      *testing.T
	status int
	body   []byte
	header http.Header
}

func (r formResp) assertStatus(want int) formResp {
	r.t.Helper()
	if r.status != want {
		r.t.Fatalf("expected status %d, got %d\nbody: %s", want, r.status, r.body)
	}
	return r
}

func (r formResp) json() map[string]any {
	r.t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		r.t.Fatalf("decode body: %v\nbody: %s", err, r.body)
	}
	return m
}

// stripeForm sends a request the way the official SDKs do: a form-encoded
// body on POST, the query string otherwise, a test secret key, and any extra
// headers given.
func stripeForm(t *testing.T, tc *testutil.TwinClient, method, path string, form url.Values, headers map[string]string) formResp {
	t.Helper()
	var body io.Reader
	target := tc.BaseURL + path
	if method == http.MethodPost {
		body = strings.NewReader(form.Encode())
	} else if len(form) > 0 {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		target += sep + form.Encode()
	}
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer sk_test_sim_123")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := tc.HTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return formResp{t: t, status: resp.StatusCode, body: b, header: resp.Header}
}
