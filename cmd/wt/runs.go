package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runsHelp is printed when the user runs `wt runs` with no verb or
// asks for `--help`. The examples are intentionally copy-pasteable.
const runsHelp = `wt runs — drive a twin's /admin/runs/* lifecycle

Usage:
  wt runs start   [--twin URL] [--seed N] [--fixtures FILE] [--run-id ID] [--json]
  wt runs finish  [--twin URL] [--run-id ID] [--export PATH] [--gzip] [--json]
  wt runs current [--twin URL] [--json]
  wt runs wrap    [--twin URL] [--seed N] [--fixtures FILE] [--run-id ID] [--export PATH] [--gzip] -- <command> [args...]

Examples:
  wt runs start --twin http://127.0.0.1:4111 --seed 42 --run-id ci-123
  wt runs wrap --seed 42 --export /tmp/run.jsonl.gz --gzip -- go test ./...
  wt runs finish --run-id ci-123 --export /tmp/run.jsonl.gz

The twin must already be running. wt runs wrap does NOT spawn the twin
— customers manage twin lifecycle externally (docker-compose, &, GitHub
Actions service container).

For raw bundle access without lifecycle, see wt replay export.
`

// cmdRuns dispatches `wt runs <verb>`.
func cmdRuns(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Print(runsHelp)
		if len(args) == 0 {
			return fmt.Errorf("usage: wt runs <start|finish|current|wrap> [flags]")
		}
		return nil
	}
	switch args[0] {
	case "start":
		return cmdRunsStart(args[1:])
	case "finish":
		return cmdRunsFinish(args[1:])
	case "current":
		return cmdRunsCurrent(args[1:])
	case "wrap":
		return cmdRunsWrap(args[1:])
	default:
		return fmt.Errorf("unknown runs subcommand %q (want: start, finish, current, wrap)", args[0])
	}
}

// resolveTwinURL returns the twin base URL. Precedence:
// flag → WT_TWIN_URL env → http://127.0.0.1:4111.
func resolveTwinURL(flagVal string) string {
	if flagVal != "" {
		return strings.TrimRight(flagVal, "/")
	}
	if v := os.Getenv("WT_TWIN_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://127.0.0.1:4111"
}

// startRunRequest mirrors the admin handler's body shape.
type startRunRequest struct {
	RunID    string          `json:"run_id,omitempty"`
	Seed     int64           `json:"seed,omitempty"`
	Fixtures json.RawMessage `json:"fixtures,omitempty"`
}

func loadFixtures(path string) (json.RawMessage, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixtures: %w", err)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("fixtures file %s is not valid JSON", path)
	}
	return json.RawMessage(data), nil
}

func cmdRunsStart(args []string) error {
	fs := flag.NewFlagSet("wt runs start", flag.ContinueOnError)
	twin := fs.String("twin", "", "Twin base URL (default: $WT_TWIN_URL or http://127.0.0.1:4111)")
	seed := fs.Int64("seed", 0, "Deterministic seed (0 = unset)")
	runID := fs.String("run-id", "", "Run identifier (auto-generated when empty)")
	fixturesPath := fs.String("fixtures", "", "Path to a JSON file forwarded as the run's fixtures blob")
	asJSON := fs.Bool("json", false, "Emit the server response as JSON to stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	url := resolveTwinURL(*twin)
	body := startRunRequest{RunID: *runID, Seed: *seed}
	if fx, err := loadFixtures(*fixturesPath); err != nil {
		return err
	} else if fx != nil {
		body.Fixtures = fx
	}
	raw, err := postJSON(url+"/admin/runs/start", body)
	if err != nil {
		return err
	}

	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	id, _ := resp["run_id"].(string)
	if *asJSON {
		os.Stdout.Write(raw)
		fmt.Println()
	} else {
		fmt.Fprintf(os.Stderr, "WT_RUN_ID=%s\n", id)
		fmt.Fprintf(os.Stderr, "started run %s on %s (seed=%v seed_source=%v)\n", id, url, resp["seeded"], resp["seed_source"])
	}

	writeGitHubEnv("WT_RUN_ID", id)
	writeGitHubEnv("WT_TWIN_URL", url)
	return nil
}

type finishResp struct {
	RunID      string    `json:"run_id"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	EntryCount int       `json:"entry_count,omitempty"`
}

func cmdRunsFinish(args []string) error {
	fs := flag.NewFlagSet("wt runs finish", flag.ContinueOnError)
	twin := fs.String("twin", "", "Twin base URL (default: $WT_TWIN_URL or http://127.0.0.1:4111)")
	runID := fs.String("run-id", os.Getenv("WT_RUN_ID"), "Run identifier (default: $WT_RUN_ID)")
	export := fs.String("export", "", "Write the JSONL replay bundle to PATH after finish")
	gz := fs.Bool("gzip", false, "Gzip-compress --export output (also auto-detected from .gz suffix)")
	asJSON := fs.Bool("json", false, "Emit the manifest as JSON to stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	url := resolveTwinURL(*twin)
	status, raw, err := postJSONStatus(url+"/admin/runs/finish", map[string]string{"run_id": *runID})
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK:
	case http.StatusNotFound:
		fmt.Fprintln(os.Stderr, "wt runs finish: no run active; skipping export")
		return nil
	case http.StatusConflict:
		return fmt.Errorf("run_id mismatch: %s", strings.TrimSpace(string(raw)))
	default:
		return fmt.Errorf("twin returned HTTP %d: %s", status, strings.TrimSpace(string(raw)))
	}

	var manifest finishResp
	_ = json.Unmarshal(raw, &manifest)
	if *asJSON {
		os.Stdout.Write(raw)
		fmt.Println()
	} else {
		fmt.Fprintf(os.Stderr, "finished run %s (entries=%d)\n", manifest.RunID, manifest.EntryCount)
	}

	if *export != "" {
		if err := exportBundle(url, *export, *gz || strings.HasSuffix(*export, ".gz")); err != nil {
			return err
		}
		writeStepSummary(url, manifest, *export)
	}
	return nil
}

func cmdRunsCurrent(args []string) error {
	fs := flag.NewFlagSet("wt runs current", flag.ContinueOnError)
	twin := fs.String("twin", "", "Twin base URL (default: $WT_TWIN_URL or http://127.0.0.1:4111)")
	asJSON := fs.Bool("json", false, "Emit the response as JSON to stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	url := resolveTwinURL(*twin)
	resp, err := http.Get(url + "/admin/runs/current")
	if err != nil {
		return fmt.Errorf("fetch current run: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("twin returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if *asJSON {
		os.Stdout.Write(raw)
		fmt.Println()
		return nil
	}
	var c map[string]any
	_ = json.Unmarshal(raw, &c)
	if id, _ := c["run_id"].(string); id == "" {
		fmt.Println("no run active")
		return nil
	}
	fmt.Printf("run_id:      %v\n", c["run_id"])
	if v, ok := c["started_at"]; ok {
		fmt.Printf("started_at:  %v\n", v)
	}
	fmt.Printf("entry_count: %v\n", c["entry_count"])
	if v, ok := c["seed_source"]; ok && v != "" {
		fmt.Printf("seed_source: %v\n", v)
	}
	return nil
}

// cmdRunsWrap starts a run, runs the child command, and always
// finishes on exit. Returns the child's exit code.
func cmdRunsWrap(args []string) error {
	fs := flag.NewFlagSet("wt runs wrap", flag.ContinueOnError)
	twin := fs.String("twin", "", "Twin base URL (default: $WT_TWIN_URL or http://127.0.0.1:4111)")
	seed := fs.Int64("seed", 0, "Deterministic seed (0 = unset)")
	runID := fs.String("run-id", "", "Run identifier (auto-generated when empty)")
	fixturesPath := fs.String("fixtures", "", "Path to a JSON fixtures file")
	export := fs.String("export", "", "Write the JSONL replay bundle to PATH after finish")
	gz := fs.Bool("gzip", false, "Gzip-compress --export output (also auto-detected from .gz suffix)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("usage: wt runs wrap [flags] -- <command> [args...]")
	}

	url := resolveTwinURL(*twin)
	body := startRunRequest{RunID: *runID, Seed: *seed}
	if fx, err := loadFixtures(*fixturesPath); err != nil {
		return err
	} else if fx != nil {
		body.Fixtures = fx
	}
	startRaw, err := postJSON(url+"/admin/runs/start", body)
	if err != nil {
		return err
	}
	var startResp map[string]any
	_ = json.Unmarshal(startRaw, &startResp)
	id, _ := startResp["run_id"].(string)
	fmt.Fprintf(os.Stderr, "wt runs wrap: started run %s\n", id)

	defer func() {
		status, finishRaw, err := postJSONStatus(url+"/admin/runs/finish", map[string]string{"run_id": id})
		if err != nil {
			fmt.Fprintf(os.Stderr, "wt runs wrap: finish failed: %v\n", err)
			return
		}
		if status != http.StatusOK && status != http.StatusNotFound {
			fmt.Fprintf(os.Stderr, "wt runs wrap: finish returned HTTP %d: %s\n", status, finishRaw)
			return
		}
		var manifest finishResp
		_ = json.Unmarshal(finishRaw, &manifest)
		fmt.Fprintf(os.Stderr, "wt runs wrap: finished run %s (entries=%d)\n", manifest.RunID, manifest.EntryCount)
		if *export != "" {
			if err := exportBundle(url, *export, *gz || strings.HasSuffix(*export, ".gz")); err != nil {
				fmt.Fprintf(os.Stderr, "wt runs wrap: export failed: %v\n", err)
				return
			}
			writeStepSummary(url, manifest, *export)
		}
	}()

	cmd := exec.Command(rest[0], rest[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "WT_RUN_ID="+id, "WT_TWIN_URL="+url)
	err = cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if asExitErr(&ee, err) {
			os.Exit(ee.ExitCode())
		}
		return fmt.Errorf("exec: %w", err)
	}
	return nil
}

// asExitErr is a tiny helper to avoid importing errors just for As.
func asExitErr(target **exec.ExitError, err error) bool {
	if e, ok := err.(*exec.ExitError); ok {
		*target = e
		return true
	}
	return false
}

// postJSON wraps postJSONStatus and treats non-2xx as an error.
func postJSON(url string, body any) ([]byte, error) {
	status, raw, err := postJSONStatus(url, body)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("twin returned HTTP %d: %s", status, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

func postJSONStatus(url string, body any) (int, []byte, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		return 0, nil, fmt.Errorf("post %s: %w", url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, nil
}

// exportBundle GETs /admin/replay and writes the response to path.
func exportBundle(twinURL, path string, useGzip bool) error {
	resp, err := http.Get(twinURL + "/admin/replay")
	if err != nil {
		return fmt.Errorf("fetch replay bundle: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("twin returned HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	var w io.Writer = f
	var gzCloser io.Closer
	if useGzip {
		gzw := gzip.NewWriter(f)
		gzCloser = gzw
		w = gzw
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		f.Close()
		return err
	}
	if gzCloser != nil {
		if err := gzCloser.Close(); err != nil {
			f.Close()
			return err
		}
	}
	return f.Close()
}

// writeGitHubEnv appends `KEY=value` to $GITHUB_ENV when that env var
// is set and the file is writable. Standard GitHub Actions
// convention. Silent no-op outside of GitHub Actions.
func writeGitHubEnv(key, value string) {
	path := os.Getenv("GITHUB_ENV")
	if path == "" || value == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s=%s\n", key, value)
}

// writeStepSummary appends a markdown table to $GITHUB_STEP_SUMMARY
// when set. Customers see the summary in the GitHub Actions UI
// without opening the artifact.
func writeStepSummary(twinURL string, manifest finishResp, exportPath string) {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "### WonderTwin replay bundle\n\n")
	fmt.Fprintf(f, "| field | value |\n| --- | --- |\n")
	fmt.Fprintf(f, "| run_id | `%s` |\n", manifest.RunID)
	fmt.Fprintf(f, "| twin | `%s` |\n", twinURL)
	fmt.Fprintf(f, "| entry_count | %d |\n", manifest.EntryCount)
	if !manifest.StartedAt.IsZero() {
		fmt.Fprintf(f, "| started_at | %s |\n", manifest.StartedAt.UTC().Format(time.RFC3339))
	}
	if !manifest.FinishedAt.IsZero() {
		fmt.Fprintf(f, "| finished_at | %s |\n", manifest.FinishedAt.UTC().Format(time.RFC3339))
	}
	fmt.Fprintf(f, "| export | `%s` |\n\n", exportPath)
}
