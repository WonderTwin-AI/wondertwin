// Package project loads the project-scope assertion file
// `.wondertwin/project.json`. Per adr-ci-twin-verification, this file
// is the auditable, in-repo declaration of which WonderTwin org a
// codebase belongs to — `wt verify` and `wt status` use it to bridge
// between local working state and the org's entitlements API.
//
// The file is intentionally narrow: one field today (account_id),
// room to grow without breaking existing readers. Linters / CI
// tooling can require the file's presence on repos that need verify;
// developers experimenting locally without one are unaffected
// (everything is permissive when project.json is absent).
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Filename is the relative path of the project scope file inside a
// repo. Lives under .wondertwin/ alongside future per-project state
// (cache directories, captured fixtures, etc.).
const Filename = ".wondertwin/project.json"

// Project is the decoded contents of .wondertwin/project.json.
// AccountID is the WonderTwin org account ID this codebase belongs
// to. SourcePath is the absolute path the file was loaded from
// (informational, used in warning messages).
type Project struct {
	AccountID  string `json:"account_id"`
	SourcePath string `json:"-"`
}

// Find walks up from startDir looking for the project file. Returns
// the parsed Project, or (nil, nil) when no file is found — that's a
// valid state (developer running outside a project-scoped repo);
// the caller decides whether absence is fatal.
//
// The walk stops at the filesystem root or at any directory
// containing a .git subdirectory — repo boundaries are natural stops
// because finding a project.json above a repo boundary would be
// misleading. Errors reading or parsing the file when it IS found
// are surfaced.
func Find(startDir string) (*Project, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", startDir, err)
	}

	for {
		candidate := filepath.Join(dir, Filename)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return load(candidate)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat %s: %w", candidate, err)
		}

		// Stop at the repo root (any directory containing .git, file
		// or dir — covers worktrees and git submodules).
		if _, gitErr := os.Stat(filepath.Join(dir, ".git")); gitErr == nil {
			return nil, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root; no project.json on the way up.
			return nil, nil
		}
		dir = parent
	}
}

// LoadFile reads and parses the project file at the explicit path.
// Used by tests and by callers that don't want the auto-walking
// behavior. Returns (nil, nil) for a missing file so callers can
// branch on absence cleanly.
func LoadFile(path string) (*Project, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory, not a file", path)
	}
	return load(path)
}

func load(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var p Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if p.AccountID == "" {
		return nil, fmt.Errorf("%s: account_id is required", path)
	}
	p.SourcePath = path
	return &p, nil
}
