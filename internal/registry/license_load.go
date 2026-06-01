package registry

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// WriteLicenseFromB64 decodes a base64-encoded license file and
// atomic-writes it to path. Sister to CommitInstallBundle's license
// branch, but standalone — for the refresh-on-MCP-call path where
// only the license is being updated (no binary).
//
// Uses the same temp-then-rename strategy as writeLicenseAtomic so a
// partial write never bricks the runtime: the runtime continues
// running against the prior license until the rename commits.
func WriteLicenseFromB64(path, licenseB64 string) error {
	licenseBytes, err := base64.StdEncoding.DecodeString(licenseB64)
	if err != nil {
		return fmt.Errorf("decode license: %w", err)
	}
	return writeLicenseAtomic(path, licenseBytes)
}

// LocalLicenseInfo is the subset of the offline license file the
// runtime needs for freshness comparison. Decoded from
// ~/.wondertwin/license.json (or wherever Path points) — minimal
// shape so the runtime doesn't have to depend on the full
// twinkit-pro/license types (which live in a separate module under
// the BSL license boundary).
type LocalLicenseInfo struct {
	AccountID string    `json:"account_id"`
	IssuedAt  time.Time `json:"issued_at"`
	NotAfter  time.Time `json:"not_after"`
}

// ReadLocalLicenseInfo reads the license file at path (typically
// ~/.wondertwin/license.json) and decodes the IssuedAt /
// AccountID / NotAfter fields. Used for the refresh-on-MCP-call
// freshness comparison per adr-license-delivery-via-mcp §3:
// runtime compares the local IssuedAt against the
// mcp_metadata.license_current_issued_at on each MCP response and
// triggers wt_license_refresh when newer server-side.
//
// Returns (nil, nil) when the file doesn't exist — that's a valid
// state (fresh install before first wt_install) and the caller
// treats it as "no comparison possible; skip the freshness check."
// Other errors (malformed JSON, permission denied) bubble up.
func ReadLocalLicenseInfo(path string) (*LocalLicenseInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read license file: %w", err)
	}
	var info LocalLicenseInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("decode license file: %w", err)
	}
	return &info, nil
}

// IsLocalLicenseStale reports whether the server's most-recent
// license issued_at is newer than what the runtime has locally.
// Returns false when the local license is missing (no comparison
// possible) or when the server's value is empty/malformed (don't
// trigger refresh on a server that didn't tell us its state).
//
// The comparison is at second-level granularity since license
// IssuedAt is written with second-level precision; sub-second
// differences would always show stale otherwise.
func IsLocalLicenseStale(local *LocalLicenseInfo, serverIssuedAt string) bool {
	if local == nil || serverIssuedAt == "" {
		return false
	}
	server, err := time.Parse(time.RFC3339, serverIssuedAt)
	if err != nil {
		return false
	}
	return server.Truncate(time.Second).After(local.IssuedAt.Truncate(time.Second))
}
