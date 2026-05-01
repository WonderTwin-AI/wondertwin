package cimode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Environment variables consulted by LoadLicense.
const (
	// EnvLicenseFile is an explicit license file path. When set, the
	// path must exist and parse cleanly; LoadLicense returns an error
	// otherwise.
	EnvLicenseFile = "WONDERTWIN_LICENSE_FILE"

	// EnvHome overrides the default $HOME/.wondertwin location used
	// for license discovery and installation.
	EnvHome = "WONDERTWIN_HOME"
)

// DefaultLicenseFilename is the file name LoadLicense searches for
// inside $WONDERTWIN_HOME (or $HOME/.wondertwin).
const DefaultLicenseFilename = "license.json"

// LoadLicense reads the license file from the standard locations.
//
// Resolution order:
//
//  1. WONDERTWIN_LICENSE_FILE env var. The path must exist and parse;
//     a missing file returns an error.
//  2. $WONDERTWIN_HOME/license.json (or $HOME/.wondertwin/license.json
//     when WONDERTWIN_HOME is unset). A missing file returns nil, nil
//     — the soft-gate path.
//
// Malformed JSON returns nil + an error in both cases.
func LoadLicense() (*License, error) {
	if path := os.Getenv(EnvLicenseFile); path != "" {
		return LoadLicenseFromPath(path)
	}
	home, err := licenseHome()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, DefaultLicenseFilename)
	lic, err := LoadLicenseFromPath(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return lic, nil
}

// LoadLicenseFromPath reads and parses a license file at path. The
// returned License is structurally well-formed; callers must invoke
// ValidateLicense to check the signature and expiry.
func LoadLicenseFromPath(path string) (*License, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read license %s: %w", path, err)
	}
	var lic License
	if err := json.Unmarshal(data, &lic); err != nil {
		return nil, fmt.Errorf("parse license %s: %w", path, err)
	}
	return &lic, nil
}

// LicenseHome returns the resolved license home directory. Exported
// for the wt CLI; honors WONDERTWIN_HOME, then $HOME/.wondertwin.
func LicenseHome() (string, error) {
	return licenseHome()
}

func licenseHome() (string, error) {
	if h := os.Getenv(EnvHome); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".wondertwin"), nil
}
