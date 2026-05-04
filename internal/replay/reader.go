// Package replay reads the public-contract JSONL artifact format produced
// by pro twins. The format spec is documented at
// wondertwin-docs/product/replay/artifact-format.md; this package
// implements the reader side independently of the producer (which lives
// behind the MIT/BSL wall in wondertwin-pro/twinkit-pro/replay/) so the
// community CLI stays MIT.
package replay

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// FormatVersionSupported is the highest format_version this reader
// understands. Producers shipping a higher version surface a clear
// error so operators can pin tooling.
const FormatVersionSupported = "1"

// Manifest mirrors the producer's Manifest. Header lines populate
// FormatVersion; trailer lines populate EntryCount/EndedAt.
type Manifest struct {
	FormatVersion string    `json:"format_version,omitempty"`
	TwinName      string    `json:"twin_name,omitempty"`
	TwinVersion   string    `json:"twin_version,omitempty"`
	RunID         string    `json:"run_id,omitempty"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	EndedAt       time.Time `json:"ended_at,omitempty"`
	EntryCount    int       `json:"entry_count,omitempty"`
	DroppedCount  int64     `json:"dropped_count,omitempty"`
}

// Entry mirrors the producer's Entry. The producer-side struct embeds
// twincore.RequestLogEntry; on the wire the embedded fields are flat
// (Method/Path/StatusCode/Duration/RequestID/Headers/Timestamp), so
// this package declares them flat too. That keeps the community
// reader free of any coupling to the producer's import graph while
// still consuming the same wire format.
type Entry struct {
	Timestamp       time.Time         `json:"timestamp"`
	Method          string            `json:"method"`
	Path            string            `json:"path"`
	Headers         map[string]string `json:"headers,omitempty"`
	StatusCode      int               `json:"status_code"`
	Duration        time.Duration     `json:"duration_ms"` // ns on the wire; field tag is inherited drift
	RequestID       string            `json:"request_id,omitempty"`
	Seq             int64             `json:"seq"`
	RunID           string            `json:"run_id,omitempty"`
	RequestSummary  map[string]any    `json:"request_summary,omitempty"`
	ResponseSummary map[string]any    `json:"response_summary,omitempty"`
}

// Artifact is the parsed result of one replay file.
type Artifact struct {
	Header  Manifest
	Entries []Entry
	Trailer Manifest
}

// ErrInvalidArtifact wraps every artifact-shape failure.
var ErrInvalidArtifact = errors.New("invalid replay artifact")

// ErrUnsupportedFormat is returned when format_version exceeds
// FormatVersionSupported.
var ErrUnsupportedFormat = errors.New("unsupported format_version")

// Read parses a replay artifact from r. Enforces the public contract
// documented at wondertwin-docs/product/replay/artifact-format.md:
//
//   - line 1 has format_version (header)
//   - last line has entry_count or ended_at (trailer)
//   - format_version == FormatVersionSupported
//   - parsed entry-line count == trailer.entry_count
func Read(r io.Reader) (Artifact, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 64<<20) // 64 MiB max line — generous

	var (
		art      Artifact
		gotHdr   bool
		gotTrl   bool
		lastLine int
	)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := append([]byte(nil), scanner.Bytes()...)
		var m Manifest
		if err := json.Unmarshal(raw, &m); err == nil && m.FormatVersion != "" {
			if gotHdr {
				return Artifact{}, fmt.Errorf("%w: extra header at line %d", ErrInvalidArtifact, lineNum)
			}
			art.Header = m
			gotHdr = true
			continue
		}
		// Try trailer (no format_version, has entry_count or ended_at).
		var t Manifest
		if err := json.Unmarshal(raw, &t); err == nil && (t.EntryCount > 0 || !t.EndedAt.IsZero()) && t.FormatVersion == "" {
			art.Trailer = t
			gotTrl = true
			lastLine = lineNum
			continue
		}
		if !gotHdr {
			return Artifact{}, fmt.Errorf("%w: first line must be a manifest header", ErrInvalidArtifact)
		}
		if gotTrl {
			return Artifact{}, fmt.Errorf("%w: entry at line %d after trailer", ErrInvalidArtifact, lineNum)
		}
		var e Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			return Artifact{}, fmt.Errorf("%w: line %d not a valid entry: %v", ErrInvalidArtifact, lineNum, err)
		}
		art.Entries = append(art.Entries, e)
	}
	if err := scanner.Err(); err != nil {
		return Artifact{}, fmt.Errorf("%w: scan: %v", ErrInvalidArtifact, err)
	}
	if !gotHdr {
		return Artifact{}, fmt.Errorf("%w: missing manifest header", ErrInvalidArtifact)
	}
	if art.Header.FormatVersion != FormatVersionSupported {
		return Artifact{}, fmt.Errorf("%w: artifact is format_version=%q, reader supports %q", ErrUnsupportedFormat, art.Header.FormatVersion, FormatVersionSupported)
	}
	if !gotTrl {
		return Artifact{}, fmt.Errorf("%w: missing manifest trailer", ErrInvalidArtifact)
	}
	if lastLine != lineNum {
		return Artifact{}, fmt.Errorf("%w: trailer is not the last line", ErrInvalidArtifact)
	}
	if art.Trailer.EntryCount != len(art.Entries) {
		return Artifact{}, fmt.Errorf("%w: entry_count=%d but parsed %d entries", ErrInvalidArtifact, art.Trailer.EntryCount, len(art.Entries))
	}
	return art, nil
}
