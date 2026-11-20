//go:build sdksmoke

// Command smoke runs the PostHog app emulator's capture flows through the
// official posthog-go SDK (v1.44.1, pinned by posthog-go.mod) against a running
// binary. It is test-only and sits outside the repository's Go module: run.sh
// copies it next to posthog-go.mod and posthog-go.sum and runs it.
//
//	POSTHOG_EMULATOR_URL=http://localhost:4114 go run -tags sdksmoke .
//
// Prints one JSON line per case ({"sdk","case","pass","detail"}) with the same
// case names as python/smoke.py. Each case runs on a freshly reset emulator and
// reads what the SDK sent back through /admin.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/posthog/posthog-go"
)

const sdk = "posthog-go v1.44.1"

var base = strings.TrimRight(os.Getenv("POSTHOG_EMULATOR_URL"), "/")

func admin(method, path string, out any) error {
	req, err := http.NewRequest(method, base+path, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, body)
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// newClient builds a client that flushes at once and, on a failed send, fails
// the case instead of retrying silently.
func newClient(errs *[]error) posthog.Client {
	c, err := posthog.NewWithConfig("phc_sdksmoke000000000000000000000000000000", posthog.Config{
		Endpoint:  base,
		BatchSize: 1,
		Callback:  failureRecorder{errs},
	})
	if err != nil {
		panic(err)
	}
	return c
}

type failureRecorder struct{ errs *[]error }

func (f failureRecorder) Success(posthog.APIMessage) {}
func (f failureRecorder) Failure(m posthog.APIMessage, err error) {
	*f.errs = append(*f.errs, err)
}

type eventsResp struct {
	Events []struct {
		Event      string         `json:"event"`
		DistinctID string         `json:"distinct_id"`
		Properties map[string]any `json:"properties"`
	} `json:"events"`
}

func main() {
	var results []map[string]any
	check := func(name string, fn func(c posthog.Client) error) {
		var errs []error
		err := admin("POST", "/admin/reset", nil)
		if err == nil {
			c := newClient(&errs)
			err = fn(c)
			_ = c.Close()
			if err == nil && len(errs) > 0 {
				err = errs[0]
			}
		}
		r := map[string]any{"sdk": sdk, "case": name, "pass": err == nil, "detail": ""}
		if err != nil {
			r["detail"] = err.Error()
		}
		results = append(results, r)
	}

	check("capture one event", func(c posthog.Client) error {
		if err := c.Enqueue(posthog.Capture{DistinctId: "u1", Event: "page_view"}); err != nil {
			return err
		}
		_ = c.Close()
		var got eventsResp
		if err := admin("GET", "/admin/events?event=page_view", &got); err != nil {
			return err
		}
		if len(got.Events) != 1 || got.Events[0].DistinctID != "u1" {
			return fmt.Errorf("want one page_view for u1, got %+v", got.Events)
		}
		return nil
	})

	check("capture with properties", func(c posthog.Client) error {
		props := posthog.NewProperties().Set("plan", "pro").Set("seats", 3)
		if err := c.Enqueue(posthog.Capture{DistinctId: "u2", Event: "upgraded", Properties: props}); err != nil {
			return err
		}
		_ = c.Close()
		var got eventsResp
		if err := admin("GET", "/admin/events?event=upgraded", &got); err != nil {
			return err
		}
		if len(got.Events) != 1 || got.Events[0].Properties["plan"] != "pro" {
			return fmt.Errorf("want plan=pro on the stored event, got %+v", got.Events)
		}
		return nil
	})

	check("capture several events", func(c posthog.Client) error {
		for _, e := range []string{"a", "b", "c"} {
			if err := c.Enqueue(posthog.Capture{DistinctId: "u3", Event: "evt_" + e}); err != nil {
				return err
			}
		}
		_ = c.Close()
		var got eventsResp
		if err := admin("GET", "/admin/events?distinct_id=u3", &got); err != nil {
			return err
		}
		if len(got.Events) != 3 {
			return fmt.Errorf("want 3 events for u3, got %d", len(got.Events))
		}
		return nil
	})

	check("identify stores person properties", func(c posthog.Client) error {
		props := posthog.NewProperties().Set("email", "u4@example.com")
		if err := c.Enqueue(posthog.Identify{DistinctId: "u4", Properties: props}); err != nil {
			return err
		}
		_ = c.Close()
		var got struct {
			Persons []struct {
				DistinctID string         `json:"distinct_id"`
				Properties map[string]any `json:"properties"`
			} `json:"persons"`
		}
		if err := admin("GET", "/admin/persons?distinct_id=u4", &got); err != nil {
			return err
		}
		if len(got.Persons) != 1 || got.Persons[0].Properties["email"] != "u4@example.com" {
			return fmt.Errorf("want person u4 with email, got %+v", got.Persons)
		}
		return nil
	})

	check("alias links two ids", func(c posthog.Client) error {
		if err := c.Enqueue(posthog.Alias{DistinctId: "u5", Alias: "u5-alias"}); err != nil {
			return err
		}
		_ = c.Close()
		var got struct {
			Aliases []struct {
				Alias      string `json:"alias"`
				DistinctID string `json:"distinct_id"`
			} `json:"aliases"`
		}
		if err := admin("GET", "/admin/aliases", &got); err != nil {
			return err
		}
		if len(got.Aliases) != 1 || got.Aliases[0].Alias != "u5-alias" {
			return fmt.Errorf("want alias u5-alias, got %+v", got.Aliases)
		}
		return nil
	})

	enc := json.NewEncoder(os.Stdout)
	failed := false
	for _, r := range results {
		_ = enc.Encode(r)
		if r["pass"] == false {
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}
