package replay

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// Show renders an Artifact in human-readable form. Output shape is
// stable enough for shell scripting (header line, dashed separator,
// one line per entry, trailer line) but is not a machine format —
// `wt replay show --json <path>` (future) would be the right path for
// programmatic consumers.
func Show(w io.Writer, a Artifact) error {
	h := a.Header
	fmt.Fprintf(w, "replay artifact (format_version=%s)\n", h.FormatVersion)
	fmt.Fprintf(w, "  twin:        %s", h.TwinName)
	if h.TwinVersion != "" {
		fmt.Fprintf(w, " (%s)", h.TwinVersion)
	}
	fmt.Fprintln(w)
	if h.RunID != "" {
		fmt.Fprintf(w, "  run_id:      %s\n", h.RunID)
	}
	fmt.Fprintf(w, "  started_at:  %s\n", h.StartedAt.Format(time.RFC3339))
	if !a.Trailer.EndedAt.IsZero() {
		fmt.Fprintf(w, "  ended_at:    %s\n", a.Trailer.EndedAt.Format(time.RFC3339))
	}
	fmt.Fprintf(w, "  entries:     %d\n", a.Trailer.EntryCount)
	if a.Trailer.DroppedCount > 0 {
		fmt.Fprintf(w, "  dropped:     %d (capacity pressure — see DefaultMaxEntries)\n", a.Trailer.DroppedCount)
	}

	fmt.Fprintln(w, strings.Repeat("─", 72))
	fmt.Fprintf(w, "%-4s  %-8s  %-7s  %-30s  %s\n", "seq", "method", "status", "path", "duration")
	fmt.Fprintln(w, strings.Repeat("─", 72))
	for _, e := range a.Entries {
		path := truncate(e.Path, 30)
		// Duration is on the wire as the embedded twincore.RequestLogEntry's
		// time.Duration (nanoseconds), tagged duration_ms. Render in ms.
		ms := float64(e.Duration) / float64(time.Millisecond)
		fmt.Fprintf(w, "%-4d  %-8s  %-7d  %-30s  %.1fms\n",
			e.Seq, e.Method, e.StatusCode, path, ms,
		)
	}
	fmt.Fprintln(w, strings.Repeat("─", 72))
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
