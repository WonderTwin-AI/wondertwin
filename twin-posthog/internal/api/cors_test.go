package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// TestCORS holds the preflight answer: the headers posthog-js and the PostHog
// SDKs send are allowed, and the headers carried over from another emulator
// are not advertised (D14).
func TestCORS(t *testing.T) {
	srv, _ := setupPostHog(t)

	for _, path := range []string{"/e/", "/batch/", "/i/v1/analytics/events", "/flags/"} {
		t.Run(path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodOptions, srv.URL+path, nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("Origin", "http://localhost:3000")
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", "content-type,posthog-sdk-info")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("status %d, want 204", resp.StatusCode)
			}
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
				t.Errorf("allow origin %q, want *", got)
			}
			allow := strings.ToLower(resp.Header.Get("Access-Control-Allow-Headers"))
			for _, want := range []string{
				"content-type", "content-encoding", "authorization", "x-posthog-api-key",
				"posthog-sdk-info", "posthog-attempt", "posthog-request-id", "posthog-request-timestamp",
			} {
				if !strings.Contains(allow, want) {
					t.Errorf("allow headers %q lack %s", allow, want)
				}
			}
			for _, residue := range []string{"stripe-account", "idempotency-key"} {
				if strings.Contains(allow, residue) {
					t.Errorf("allow headers %q still name %s", allow, residue)
				}
			}
		})
	}
}
