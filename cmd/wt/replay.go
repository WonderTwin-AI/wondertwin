package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// cmdReplay dispatches `wt replay <subcommand>`. Currently supported:
//
//	wt replay export   Fetch /admin/replay from a running twin and write JSONL
//	wt replay view     Pretty-print a JSONL bundle
func cmdReplay(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: wt replay <export|view> [flags]")
	}
	switch args[0] {
	case "export":
		return cmdReplayExport(args[1:])
	case "view":
		return cmdReplayView(args[1:])
	default:
		return fmt.Errorf("unknown replay subcommand %q (want: export, view)", args[0])
	}
}

func cmdReplayExport(args []string) error {
	fs := flag.NewFlagSet("wt replay export", flag.ContinueOnError)
	twinURL := fs.String("twin", os.Getenv("WT_TWIN_URL"), "Twin base URL (default: $WT_TWIN_URL or http://localhost:4111)")
	output := fs.String("output", "-", "Output path (- for stdout)")
	gz := fs.Bool("gzip", false, "Gzip-compress the output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	url := *twinURL
	if url == "" {
		url = "http://localhost:4111"
	}
	url = strings.TrimRight(url, "/") + "/admin/replay"

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("fetching replay bundle: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("twin returned HTTP %d", resp.StatusCode)
	}

	var w io.Writer = os.Stdout
	var fileCloser io.Closer
	if *output != "-" {
		f, err := os.Create(*output)
		if err != nil {
			return err
		}
		fileCloser = f
		w = f
	}
	var gzCloser io.Closer
	if *gz {
		gzw := gzip.NewWriter(w)
		gzCloser = gzw
		w = gzw
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("writing bundle: %w", err)
	}
	// Close gzip first so its trailer is flushed, then close the file.
	if gzCloser != nil {
		if err := gzCloser.Close(); err != nil {
			return fmt.Errorf("gzip close: %w", err)
		}
	}
	if fileCloser != nil {
		if err := fileCloser.Close(); err != nil {
			return fmt.Errorf("file close: %w", err)
		}
	}
	return nil
}

func cmdReplayView(args []string) error {
	fs := flag.NewFlagSet("wt replay view", flag.ContinueOnError)
	full := fs.Bool("full", false, "Print each entry as full JSON instead of a one-line summary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: wt replay view [--full] <path>")
	}
	path := fs.Arg(0)

	r, closer, err := openMaybeGzip(path)
	if err != nil {
		return err
	}
	defer closer.Close()

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)

	if !sc.Scan() {
		return fmt.Errorf("bundle is empty")
	}
	var manifest replayManifestView
	if err := json.Unmarshal(sc.Bytes(), &manifest); err != nil {
		return fmt.Errorf("manifest line: %w", err)
	}
	printManifest(manifest)

	count := 0
	for sc.Scan() {
		count++
		if *full {
			fmt.Println(string(sc.Bytes()))
			continue
		}
		var e replayEntryView
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping malformed entry on line %d: %v\n", count+1, err)
			continue
		}
		dur := time.Duration(e.DurationNs) * time.Nanosecond
		fmt.Printf("%4d  %s  %-6s %-32s %3d  %6s\n",
			e.Seq, e.Timestamp.Format("15:04:05.000"), e.Method, truncate(e.Path, 32), e.ResStatus, formatDuration(dur))
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	if count == 0 {
		fmt.Println("(no entries)")
	}
	return nil
}

// openMaybeGzip opens a file and returns a Reader that transparently
// decompresses gzip-compressed bundles (detected by .gz suffix).
func openMaybeGzip(path string) (io.Reader, io.Closer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, nil, fmt.Errorf("gzip: %w", err)
		}
		return gz, multiCloser{gz, f}, nil
	}
	return f, f, nil
}

type multiCloser []io.Closer

func (m multiCloser) Close() error {
	var firstErr error
	for _, c := range m {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

type replayManifestView struct {
	TwinName      string    `json:"twin_name"`
	TwinVersion   string    `json:"twin_version"`
	RunID         string    `json:"run_id"`
	Seed          int64     `json:"seed"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	EntryCount    int       `json:"entry_count"`
	SchemaVersion int       `json:"schema_version"`
}

type replayEntryView struct {
	Seq        uint64    `json:"seq"`
	Timestamp  time.Time `json:"timestamp"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	ResStatus  int       `json:"res_status"`
	DurationNs int64     `json:"duration_ns"`
}

func printManifest(m replayManifestView) {
	fmt.Printf("# WonderTwin replay bundle (schema v%d)\n", m.SchemaVersion)
	if m.TwinName != "" {
		fmt.Printf("# Twin:    %s %s\n", m.TwinName, m.TwinVersion)
	}
	if m.RunID != "" {
		fmt.Printf("# Run:     %s\n", m.RunID)
	}
	if !m.StartedAt.IsZero() {
		fmt.Printf("# Started: %s\n", m.StartedAt.Format(time.RFC3339))
	}
	if !m.FinishedAt.IsZero() {
		fmt.Printf("# Finished: %s\n", m.FinishedAt.Format(time.RFC3339))
	}
	fmt.Printf("# Entries: %d\n", m.EntryCount)
	fmt.Println()
	fmt.Println(" seq  time         method path                              status  dur")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func formatDuration(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Truncate(time.Millisecond).String()
}
