package store

import (
	"encoding/base64"
	"math"
	"strings"
	"testing"
)

func TestIDsAreLargeAndPerKind(t *testing.T) {
	s := New()
	issue := s.NewID(KindIssue)
	if issue <= math.MaxInt32 {
		t.Errorf("issue IDs should exceed int32 like GitHub's do, got %d", issue)
	}
	if next := s.NewID(KindIssue); next != issue+1 {
		t.Errorf("expected consecutive issue IDs, got %d then %d", issue, next)
	}
	if repo := s.NewID(KindRepo); repo >= issue {
		t.Errorf("repo IDs come from their own space, got %d", repo)
	}
}

func TestNumbersArePerRepository(t *testing.T) {
	s := New()
	if n := s.NextIssueNumber("a", "one"); n != 1 {
		t.Errorf("expected 1, got %d", n)
	}
	if n := s.NextIssueNumber("a", "one"); n != 2 {
		t.Errorf("expected 2, got %d", n)
	}
	if n := s.NextIssueNumber("a", "two"); n != 1 {
		t.Errorf("a second repository starts at 1, got %d", n)
	}
	s.Reset()
	if n := s.NextIssueNumber("a", "one"); n != 1 {
		t.Errorf("reset restarts numbering, got %d", n)
	}
}

func TestNodeIDShape(t *testing.T) {
	id := NodeID("R", 1_050_000_001)
	if !strings.HasPrefix(id, "R_kgDO") {
		t.Errorf("a repository id under 2^32 encodes as R_kgDO..., got %s", id)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(id, "R_"))
	if err != nil {
		t.Fatalf("node id is not URL-safe base64: %v", err)
	}
	want := []byte{0x92, 0x00, 0xce, 0x3e, 0x95, 0xba, 0x81}
	if string(raw) != string(want) {
		t.Errorf("msgpack payload = % x, want % x", raw, want)
	}
	if big := NodeID("CR", 51_000_000_001); !strings.HasPrefix(big, "CR_kwDP") && !strings.HasPrefix(big, "CR_kgDP") {
		t.Errorf("an id over 2^32 uses the 8-byte encoding, got %s", big)
	}
}
