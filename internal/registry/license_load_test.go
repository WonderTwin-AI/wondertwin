package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadLocalLicenseInfo_MissingFileReturnsNilNil(t *testing.T) {
	t.Parallel()
	info, err := ReadLocalLicenseInfo(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Errorf("missing file: want nil err, got %v", err)
	}
	if info != nil {
		t.Errorf("missing file: want nil info, got %+v", info)
	}
}

func TestReadLocalLicenseInfo_HappyPathDecodes(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "license.json")
	body := `{
		"format_version": "2.0",
		"account_id": "org_t",
		"issued_at": "2026-06-01T12:00:00Z",
		"not_after": "2026-07-01T12:00:00Z",
		"signature": "abc"
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	info, err := ReadLocalLicenseInfo(path)
	if err != nil {
		t.Fatalf("ReadLocalLicenseInfo: %v", err)
	}
	if info.AccountID != "org_t" {
		t.Errorf("account_id: got %q", info.AccountID)
	}
	want := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if !info.IssuedAt.Equal(want) {
		t.Errorf("issued_at: want %v, got %v", want, info.IssuedAt)
	}
}

func TestReadLocalLicenseInfo_MalformedJSONErrors(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "license.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := ReadLocalLicenseInfo(path)
	if err == nil {
		t.Fatal("want error on malformed JSON")
	}
}

func TestIsLocalLicenseStale_NilLocalReturnsFalse(t *testing.T) {
	t.Parallel()
	if IsLocalLicenseStale(nil, "2026-06-01T12:00:00Z") {
		t.Errorf("nil local should never be stale (no comparison possible)")
	}
}

func TestIsLocalLicenseStale_EmptyServerValueReturnsFalse(t *testing.T) {
	t.Parallel()
	local := &LocalLicenseInfo{IssuedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)}
	if IsLocalLicenseStale(local, "") {
		t.Errorf("empty server value: don't trigger refresh")
	}
}

func TestIsLocalLicenseStale_MalformedServerValueReturnsFalse(t *testing.T) {
	t.Parallel()
	local := &LocalLicenseInfo{IssuedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)}
	if IsLocalLicenseStale(local, "not-a-timestamp") {
		t.Errorf("malformed server value: should be conservative and not trigger refresh")
	}
}

func TestIsLocalLicenseStale_ServerNewerReturnsTrue(t *testing.T) {
	t.Parallel()
	local := &LocalLicenseInfo{IssuedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)}
	if !IsLocalLicenseStale(local, "2026-06-01T12:00:00Z") {
		t.Errorf("server newer: want stale")
	}
}

func TestIsLocalLicenseStale_ServerSameReturnsFalse(t *testing.T) {
	t.Parallel()
	local := &LocalLicenseInfo{IssuedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}
	if IsLocalLicenseStale(local, "2026-06-01T12:00:00Z") {
		t.Errorf("server equal to local: don't trigger refresh")
	}
}

func TestIsLocalLicenseStale_ServerOlderReturnsFalse(t *testing.T) {
	t.Parallel()
	local := &LocalLicenseInfo{IssuedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}
	if IsLocalLicenseStale(local, "2026-05-01T12:00:00Z") {
		t.Errorf("server older: don't trigger refresh (the runtime has the newer license)")
	}
}

func TestIsLocalLicenseStale_SubSecondDifferenceReturnsFalse(t *testing.T) {
	t.Parallel()
	// Server reports 100ms after local. Should NOT be stale —
	// we truncate to second-level granularity to avoid false positives
	// from server-time vs local-time-of-write drift.
	local := &LocalLicenseInfo{
		IssuedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
	}
	if IsLocalLicenseStale(local, "2026-06-01T12:00:00.100Z") {
		t.Errorf("sub-second diff: don't trigger refresh (truncation defense)")
	}
}
