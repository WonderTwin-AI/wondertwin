package testutil

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// Request describes a single HTTP call in a determinism scenario.
//
// Method, Path, Headers and Body are forwarded verbatim to the
// twin's TwinClient. Responses are read but otherwise ignored —
// determinism is asserted via the captured replay JSONL bundle, not
// per-request.
type Request struct {
	Method  string
	Path    string
	Headers map[string]string
	Body    string
}

// AssertDeterministic runs the given request sequence twice against a
// fresh twin, both seeded with `seed`, and asserts the captured
// replay JSONL bundles match modulo fields that are inherently
// non-deterministic.
//
// Canonicalization rules (applied to each line before comparison):
//
//   - Per-entry: `timestamp`, `duration_ns`, `sim_time` are removed.
//   - Manifest: `started_at`, `finished_at`, `run_id` are removed.
//   - Headers maps: keys with `X-Request-Id` (case-insensitive) are
//     dropped; the remaining keys are emitted in sorted order so
//     map-iteration noise can't cause spurious diffs.
//   - Entries whose `path` starts with `/admin/` are filtered. Admin
//     control-plane operations are not part of the customer-facing
//     determinism contract; their captured response bodies embed
//     freshly-generated `started_at` timestamps that legitimately
//     differ across runs.
//
// On mismatch, AssertDeterministic prints the first diverging line
// (with both versions side by side) and dumps both canonicalized
// bundles via t.Logf so authors can locate the leaky source of
// randomness.
//
// newClient must construct a fresh twin instance bound to the given
// seed. The returned cleanup runs after each iteration.
func AssertDeterministic(
	t *testing.T,
	newClient func(seed int64) (tc *TwinClient, ac *AdminClient, cleanup func()),
	seed int64,
	requests []Request,
) {
	t.Helper()
	a := captureBundle(t, newClient, seed, requests)
	b := captureBundle(t, newClient, seed, requests)

	canonA := canonicalizeBundle(a)
	canonB := canonicalizeBundle(b)
	if canonA == canonB {
		return
	}
	linesA := strings.Split(canonA, "\n")
	linesB := strings.Split(canonB, "\n")
	first := -1
	for i := 0; i < len(linesA) && i < len(linesB); i++ {
		if linesA[i] != linesB[i] {
			first = i
			break
		}
	}
	if first < 0 {
		first = min(len(linesA), len(linesB))
	}
	t.Errorf("determinism violation: seeded runs diverge at canonicalized line %d", first)
	if first < len(linesA) {
		t.Logf("A: %s", linesA[first])
	}
	if first < len(linesB) {
		t.Logf("B: %s", linesB[first])
	}
	t.Logf("--- full bundle A ---\n%s", canonA)
	t.Logf("--- full bundle B ---\n%s", canonB)
}

func captureBundle(
	t *testing.T,
	newClient func(seed int64) (*TwinClient, *AdminClient, func()),
	seed int64,
	requests []Request,
) string {
	t.Helper()
	tc, ac, cleanup := newClient(seed)
	defer cleanup()

	// Drive the lifecycle inline rather than via WithRun's t.Cleanup
	// so the run is finished while the test server is still alive.
	started := ac.StartRun(WithSeed(seed))

	for _, r := range requests {
		hdrs := r.Headers
		if hdrs == nil {
			hdrs = map[string]string{}
		}
		var body any
		if r.Body != "" {
			body = r.Body
		}
		tc.DoWithHeaders(r.Method, r.Path, body, hdrs)
	}

	resp, err := http.Get(tc.BaseURL + "/admin/replay")
	if err != nil {
		t.Fatalf("fetch replay bundle: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	ac.FinishRun(started.RunID)
	return string(raw)
}

func canonicalizeBundle(jsonl string) string {
	var out bytes.Buffer
	sc := bufio.NewScanner(strings.NewReader(jsonl))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		canon, ok := canonicalizeLine(line)
		if !ok {
			continue
		}
		out.Write(canon)
		out.WriteByte('\n')
	}
	return out.String()
}

func canonicalizeLine(line []byte) ([]byte, bool) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return append([]byte(nil), line...), true
	}
	if path, ok := raw["path"].(string); ok && strings.HasPrefix(path, "/admin/") {
		return nil, false
	}
	for _, k := range []string{"timestamp", "duration_ns", "sim_time", "started_at", "finished_at", "run_id"} {
		delete(raw, k)
	}
	for _, key := range []string{"req_headers", "res_headers"} {
		if h, ok := raw[key].(map[string]any); ok {
			for hk := range h {
				if strings.EqualFold(hk, "X-Request-Id") {
					delete(h, hk)
				}
			}
			raw[key] = sortedHeaderMap(h)
		}
	}
	out, _ := jsonStable(raw)
	return out, true
}

// jsonStable encodes v with sorted keys so map-iteration order can't
// produce spurious diffs.
func jsonStable(v any) ([]byte, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var buf bytes.Buffer
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			buf.Write(kb)
			buf.WriteByte(':')
			vb, err := jsonStable(t[k])
			if err != nil {
				return nil, err
			}
			buf.Write(vb)
		}
		buf.WriteByte('}')
		return buf.Bytes(), nil
	case []any:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			eb, err := jsonStable(e)
			if err != nil {
				return nil, err
			}
			buf.Write(eb)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	default:
		return json.Marshal(v)
	}
}

func sortedHeaderMap(h map[string]any) map[string]any {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		out[k] = h[k]
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
