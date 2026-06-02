package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/config"
	"github.com/wondertwin-ai/wondertwin/internal/lockfile"
	"github.com/wondertwin-ai/wondertwin/internal/platform"
	"github.com/wondertwin-ai/wondertwin/internal/project"
)

// Exit codes for `wt verify`, per adr-ci-twin-verification.
//
//	0 — every twin in the lock file is covered by the asserted org's
//	    entitlements.
//	1 — one or more twins are missing entitlements (the conversion
//	    signal — CI fails and the human is told to subscribe).
//	2 — usage / configuration error (no lockfile, no account, network
//	    failure). Distinct from 1 so CI dashboards can tell "missing
//	    coverage" apart from "tool didn't run."
const (
	verifyExitOK         = 0
	verifyExitNotCovered = 1
	verifyExitUsage      = 2
)

// cmdVerify implements `wt verify` — the CI gate that asserts every
// twin in wondertwin-lock.json is covered by the entitlements of the
// account-of-record. The account is resolved from (in order):
//
//  1. --org / --account flag
//  2. WONDERTWIN_ACCOUNT_ID env (CI-friendly override)
//  3. .wondertwin/project.json#account_id (committed, walks up)
//  4. ~/.wondertwin/config.yaml's org_id (developer fallback)
//
// Per adr-ci-twin-verification we want the gate to be loud-but-fair:
// missing entitlements get printed with a human-readable next step.
func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	orgOverride := fs.String("org", "", "override account_id (e.g. for CI)")
	accountAlias := fs.String("account", "", "alias for --org")
	dir := fs.String("dir", "", "directory containing wondertwin-lock.json (default: cwd)")
	if err := fs.Parse(args); err != nil {
		// Treat flag-parse errors as usage failures (exit 2).
		os.Exit(verifyExitUsage)
	}

	workDir := *dir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			os.Exit(verifyExitUsage)
		}
	}

	// 1. Lockfile is the source of truth for what to check.
	if !lockfile.Exists(workDir) {
		fmt.Fprintf(os.Stderr, "wt verify: no %s found in %s\n", lockfile.Filename, workDir)
		fmt.Fprintln(os.Stderr, "Run `wt install` first to produce a lock file.")
		os.Exit(verifyExitUsage)
	}
	lf, err := lockfile.Load(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wt verify: reading %s: %v\n", lockfile.Filename, err)
		os.Exit(verifyExitUsage)
	}
	twins := sortedTwinNames(lf)

	// 2. Resolve account.
	accountID, source, err := resolveAccountID(*orgOverride, *accountAlias, workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wt verify: %v\n", err)
		os.Exit(verifyExitUsage)
	}

	// 3. Need an API key to call the server.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wt verify: loading config: %v\n", err)
		os.Exit(verifyExitUsage)
	}
	apiKey := cfg.APIKey
	if k := os.Getenv("WONDERTWIN_API_KEY"); k != "" {
		apiKey = k
	}
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "wt verify: no API key configured")
		fmt.Fprintln(os.Stderr, "Set WONDERTWIN_API_KEY (CI) or run `wt login` (local).")
		os.Exit(verifyExitUsage)
	}

	// 4. Call the server.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := platform.New(platformBaseURL(), apiKey)
	resp, err := client.EntitlementsCover(ctx, accountID, twins)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wt verify: %v\n", err)
		os.Exit(verifyExitUsage)
	}

	// 5. Report.
	fmt.Printf("Verifying %d twins against account %s (%s)\n",
		len(twins), accountID, source)
	fmt.Println()

	if len(resp.Missing) == 0 {
		fmt.Printf("  All %d twins covered.\n", len(resp.Covered))
		return nil
	}

	fmt.Printf("  %d twin(s) NOT covered:\n", len(resp.Missing))
	for _, m := range resp.Missing {
		fmt.Printf("    - %-24s %s\n", m.TwinName, humanReason(m.ReasonCode))
	}
	fmt.Println()
	fmt.Println("To resolve:")
	fmt.Println("  • Run `wt subscribe <twin>` to add the missing entitlements, or")
	fmt.Println("  • Remove unused twins from wondertwin.yaml and rerun `wt install`.")
	os.Exit(verifyExitNotCovered)
	return nil
}

// resolveAccountID picks the account_id and returns (id, source-label, err).
// The source label is printed back to the user so they can tell at a
// glance which input won — useful when CI overrides a project.json.
func resolveAccountID(orgFlag, accountFlag, workDir string) (id, source string, err error) {
	if orgFlag != "" {
		return orgFlag, "--org flag", nil
	}
	if accountFlag != "" {
		return accountFlag, "--account flag", nil
	}
	if env := os.Getenv("WONDERTWIN_ACCOUNT_ID"); env != "" {
		return env, "WONDERTWIN_ACCOUNT_ID env", nil
	}
	p, perr := project.Find(workDir)
	if perr != nil {
		return "", "", fmt.Errorf("reading project.json: %w", perr)
	}
	if p != nil {
		return p.AccountID, ".wondertwin/project.json", nil
	}
	// Developer fallback.
	cfg, cerr := config.Load()
	if cerr == nil && cfg.OrgID != "" {
		return cfg.OrgID, "~/.wondertwin/config.yaml", nil
	}
	return "", "", fmt.Errorf("no account_id configured (set --org, WONDERTWIN_ACCOUNT_ID, or commit .wondertwin/project.json)")
}

// printProjectMismatchWarning emits a soft warning to stderr when the
// signed-in account_id doesn't match the .wondertwin/project.json
// committed in the current directory tree.
//
// This is the "you're about to push code that CI is going to reject"
// hint — strictly informational, never an error, since the developer
// may legitimately be testing locally against their own account
// before opening a PR (the friction-as-conversion path from
// adr-ci-twin-verification).
func printProjectMismatchWarning(localOrgID string) {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	p, err := project.Find(cwd)
	if err != nil || p == nil {
		return
	}
	if p.AccountID == localOrgID {
		return
	}
	fmt.Fprintf(os.Stderr, "\nNote: .wondertwin/project.json#account_id is %s\n", p.AccountID)
	fmt.Fprintf(os.Stderr, "      Local login is %s. `wt verify` will assert against %s.\n",
		localOrgID, p.AccountID)
}

func sortedTwinNames(lf *lockfile.LockFile) []string {
	names := make([]string, 0, len(lf.Twins))
	for n := range lf.Twins {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// humanReason turns a wire-level reason_code into a one-line
// explanation. Unknown codes pass through verbatim — forward-compat
// per the same ADR pattern as install outcomes (clients should never
// hide a server message even if they don't recognize the code).
func humanReason(code string) string {
	switch code {
	case "not_entitled":
		return "not entitled — run `wt subscribe`"
	case "entitlement_lapsed":
		return "entitlement lapsed — renew via `wt subscribe`"
	case "entitlement_cancelled":
		return "entitlement cancelled — run `wt subscribe` to re-add"
	case "entitlement_unknown_status":
		return "entitlement in unknown state — contact support"
	default:
		return code
	}
}
