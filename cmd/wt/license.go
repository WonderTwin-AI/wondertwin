package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/cimode"
)

// cmdLicense dispatches `wt license <subcommand>`.
func cmdLicense(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: wt license <install|status|inspect> [flags]")
	}
	switch args[0] {
	case "install":
		return cmdLicenseInstall(args[1:])
	case "status":
		return cmdLicenseStatus(args[1:])
	case "inspect":
		return cmdLicenseInspect(args[1:])
	default:
		return fmt.Errorf("unknown license subcommand %q (want: install, status, inspect)", args[0])
	}
}

// licenseReport is the JSON shape emitted by `--json`.
type licenseReport struct {
	Path         string         `json:"path,omitempty"`
	Installed    bool           `json:"installed,omitempty"`
	License      *cimode.License `json:"license,omitempty"`
	Valid        bool           `json:"valid"`
	Reason       string         `json:"reason,omitempty"`
	ExpiresInSec float64        `json:"expires_in_seconds"`
}

// scopeName is the twin name used for license validation in the wt
// CLI. The CLI itself isn't a twin; "*" scope licenses validate
// trivially, scoped licenses surface their scope without failing
// validation by an arbitrary name.
const scopeName = "*"

func cmdLicenseInstall(args []string) error {
	fs := flag.NewFlagSet("wt license install", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "Emit JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: wt license install [--json] <file>")
	}
	src := fs.Arg(0)

	lic, err := cimode.LoadLicenseFromPath(src)
	if err != nil {
		return fmt.Errorf("read license: %w", err)
	}
	st := cimode.ValidateLicense(lic, validationScope(lic), time.Now())
	if !st.Valid {
		report(*asJSON, licenseReport{Path: src, License: lic, Valid: false, Reason: st.Reason, ExpiresInSec: st.ExpiresIn.Seconds()})
		return fmt.Errorf("refusing to install invalid license: %s", st.Reason)
	}

	home, err := cimode.LicenseHome()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", home, err)
	}
	dst := filepath.Join(home, cimode.DefaultLicenseFilename)
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	report(*asJSON, licenseReport{
		Path:         dst,
		Installed:    true,
		License:      lic,
		Valid:        true,
		ExpiresInSec: st.ExpiresIn.Seconds(),
	})
	return nil
}

func cmdLicenseStatus(args []string) error {
	fs := flag.NewFlagSet("wt license status", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "Emit JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	lic, err := cimode.LoadLicense()
	if err != nil {
		return fmt.Errorf("load license: %w", err)
	}
	st := cimode.ValidateLicense(lic, validationScope(lic), time.Now())
	r := licenseReport{License: lic, Valid: st.Valid, Reason: st.Reason, ExpiresInSec: st.ExpiresIn.Seconds()}
	report(*asJSON, r)
	if !st.Valid {
		os.Exit(1)
	}
	return nil
}

func cmdLicenseInspect(args []string) error {
	fs := flag.NewFlagSet("wt license inspect", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "Emit JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: wt license inspect [--json] <file>")
	}
	path := fs.Arg(0)

	lic, err := cimode.LoadLicenseFromPath(path)
	if err != nil {
		return err
	}
	st := cimode.ValidateLicense(lic, validationScope(lic), time.Now())
	report(*asJSON, licenseReport{Path: path, License: lic, Valid: st.Valid, Reason: st.Reason, ExpiresInSec: st.ExpiresIn.Seconds()})
	if !st.Valid {
		return fmt.Errorf("license invalid: %s", st.Reason)
	}
	return nil
}

// validationScope returns "*" for license validation by the CLI when
// the license itself is wildcard-scoped, or the first scoped twin
// name when it isn't. The CLI is not a twin, so we just need to pick
// any name the license covers; this avoids tripping the
// twin-not-in-scope rule.
func validationScope(lic *cimode.License) string {
	if lic == nil {
		return scopeName
	}
	for _, s := range lic.TwinScope {
		if s == "*" {
			return "*"
		}
	}
	if len(lic.TwinScope) > 0 {
		return lic.TwinScope[0]
	}
	return scopeName
}

func report(asJSON bool, r licenseReport) {
	if asJSON {
		out, _ := json.MarshalIndent(r, "", "  ")
		fmt.Println(string(out))
		return
	}
	if r.License == nil {
		fmt.Printf("No license loaded.\n")
		fmt.Printf("Status:  %s\n", reasonOrValid(r))
		return
	}
	fmt.Printf("License: %s\n", r.License.Key)
	fmt.Printf("Org:     %s\n", r.License.OrgID)
	fmt.Printf("Scope:   %v\n", r.License.TwinScope)
	if !r.License.IssuedAt.IsZero() {
		fmt.Printf("Issued:  %s\n", r.License.IssuedAt.Format(time.RFC3339))
	}
	if !r.License.NotAfter.IsZero() {
		fmt.Printf("Expires: %s\n", r.License.NotAfter.Format(time.RFC3339))
	}
	fmt.Printf("Status:  %s\n", reasonOrValid(r))
	if r.Installed {
		fmt.Printf("Installed at: %s\n", r.Path)
	}
}

func reasonOrValid(r licenseReport) string {
	if r.Valid {
		return fmt.Sprintf("valid (expires in %s)", time.Duration(r.ExpiresInSec*float64(time.Second)).Truncate(time.Minute))
	}
	if r.Reason == "" {
		return "invalid"
	}
	return "invalid: " + r.Reason
}
