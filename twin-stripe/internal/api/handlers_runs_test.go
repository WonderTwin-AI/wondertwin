package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

// TestDeterministicSeedSmoke is the Wave 2B determinism smoke. Two
// runs of identical traffic against twin-stripe with the same seed
// should produce identical replay JSONL once timestamps are stripped.
//
// The full per-twin determinism contract is session 2A; this test
// just proves the WithRun + reseed plumbing works end-to-end on one
// twin.
func TestDeterministicSeedSmoke(t *testing.T) {
	bundle1 := captureRunBundle(t)
	bundle2 := captureRunBundle(t)

	canon1 := stripTimestampsAndDurations(bundle1)
	canon2 := stripTimestampsAndDurations(bundle2)
	if canon1 != canon2 {
		t.Errorf("seeded runs diverged after timestamp/duration removal:\nA:\n%s\nB:\n%s", canon1, canon2)
	}
}

func captureRunBundle(t *testing.T) string {
	t.Helper()
	srv, tc := setupStripe(t)
	ac := testutil.NewAdminClient(tc)

	_ = testutil.WithRun(t, ac, testutil.WithSeed(42), testutil.WithRunID("seed-smoke"))

	stripePost(tc, "/v1/customers", `email=alice@example.com&name=Alice`).AssertStatus(200)
	stripePost(tc, "/v1/customers", `email=bob@example.com&name=Bob`).AssertStatus(200)

	resp, err := http.Get(srv.URL + "/admin/replay")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

// stripTimestampsAndDurations rewrites each JSONL line to drop fields
// that are intrinsically non-deterministic: timestamp, duration_ns,
// started_at, finished_at. The remaining shape is what determinism
// asserts on.
func stripTimestampsAndDurations(jsonl string) string {
	var out bytes.Buffer
	for _, line := range strings.Split(strings.TrimRight(jsonl, "\n"), "\n") {
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		for _, k := range []string{"timestamp", "duration_ns", "started_at", "finished_at", "sim_time"} {
			delete(raw, k)
		}
		// Normalize header maps that include nondeterministic
		// per-request values (RequestID/CORS).
		for _, key := range []string{"req_headers", "res_headers"} {
			if h, ok := raw[key].(map[string]any); ok {
				for hk := range h {
					if strings.EqualFold(hk, "X-Request-Id") {
						delete(h, hk)
					}
				}
			}
		}
		b, _ := json.Marshal(raw)
		out.Write(b)
		out.WriteByte('\n')
	}
	return out.String()
}
