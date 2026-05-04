package replay_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/internal/replay"
)

const validArtifact = `{"format_version":"1","twin_name":"stripe","twin_version":"0.5.0","run_id":"run-1","started_at":"2026-05-04T12:00:00Z"}
{"timestamp":"2026-05-04T12:00:01Z","method":"POST","path":"/v1/charges","status_code":200,"duration_ms":12000000,"seq":1,"run_id":"run-1","request_summary":{"amount":"number"},"response_summary":{"id":"string"}}
{"timestamp":"2026-05-04T12:00:01.5Z","method":"GET","path":"/v1/charges/{id}","status_code":200,"duration_ms":3000000,"seq":2,"run_id":"run-1"}
{"ended_at":"2026-05-04T12:00:02Z","entry_count":2}
`

func TestReadValidArtifact(t *testing.T) {
	a, err := replay.Read(strings.NewReader(validArtifact))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if a.Header.TwinName != "stripe" || a.Header.RunID != "run-1" {
		t.Errorf("header = %+v", a.Header)
	}
	if len(a.Entries) != 2 {
		t.Fatalf("entries=%d, want 2", len(a.Entries))
	}
	if a.Entries[0].Method != "POST" || a.Entries[1].Method != "GET" {
		t.Errorf("entry methods: %s, %s", a.Entries[0].Method, a.Entries[1].Method)
	}
	if a.Trailer.EntryCount != 2 {
		t.Errorf("trailer entry_count=%d, want 2", a.Trailer.EntryCount)
	}
}

func TestReadRejectsCountMismatch(t *testing.T) {
	bad := strings.Replace(validArtifact, `"entry_count":2`, `"entry_count":99`, 1)
	if _, err := replay.Read(strings.NewReader(bad)); !errors.Is(err, replay.ErrInvalidArtifact) {
		t.Errorf("err=%v, want ErrInvalidArtifact", err)
	}
}

func TestReadRejectsUnsupportedVersion(t *testing.T) {
	bad := strings.Replace(validArtifact, `"format_version":"1"`, `"format_version":"99"`, 1)
	if _, err := replay.Read(strings.NewReader(bad)); !errors.Is(err, replay.ErrUnsupportedFormat) {
		t.Errorf("err=%v, want ErrUnsupportedFormat", err)
	}
}

func TestReadRejectsMissingHeader(t *testing.T) {
	noHeader := `{"timestamp":"2026-05-04T12:00:01Z","method":"GET","path":"/","status_code":200,"duration_ms":1000000,"seq":1}` + "\n" +
		`{"ended_at":"2026-05-04T12:00:02Z","entry_count":1}` + "\n"
	if _, err := replay.Read(strings.NewReader(noHeader)); !errors.Is(err, replay.ErrInvalidArtifact) {
		t.Errorf("err=%v, want ErrInvalidArtifact", err)
	}
}

func TestReadRejectsMissingTrailer(t *testing.T) {
	noTrailer := `{"format_version":"1","twin_name":"stripe","started_at":"2026-05-04T12:00:00Z"}` + "\n" +
		`{"timestamp":"2026-05-04T12:00:01Z","method":"GET","path":"/","status_code":200,"duration_ms":1000000,"seq":1}` + "\n"
	if _, err := replay.Read(strings.NewReader(noTrailer)); !errors.Is(err, replay.ErrInvalidArtifact) {
		t.Errorf("err=%v, want ErrInvalidArtifact", err)
	}
}

func TestShowIncludesEntriesAndManifest(t *testing.T) {
	a, err := replay.Read(strings.NewReader(validArtifact))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var buf bytes.Buffer
	if err := replay.Show(&buf, a); err != nil {
		t.Fatalf("Show: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"format_version=1", "stripe", "run-1", "POST", "/v1/charges", "GET", "/v1/charges/{id}", "12.0ms"} {
		if !strings.Contains(out, want) {
			t.Errorf("Show output missing %q\nfull output:\n%s", want, out)
		}
	}
}
