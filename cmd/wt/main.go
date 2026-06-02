// wt is the WonderTwin CLI — a process manager and admin client for twin binaries.
//
// Usage:
//
//	wt up                         Start all twins from wondertwin.yaml
//	wt down                       Stop all running twins
//	wt status                     Health check all running twins
//	wt reset                      Reset state on all running twins
//	wt seed <twin> <file>         POST seed data to a twin's /admin/state
//	wt logs <twin>                Tail stdout/stderr of a running twin
//	wt inspect <twin> [res]       Query a running twin's internal state
//	wt test [path]                Run YAML test scenarios against running twins
//	wt install                    Install all twins from wondertwin.yaml
//	wt install <twin>@<version>   Install a specific twin at a version
//	wt ci                         Install twins from lock file (frozen)
//	wt verify                     Assert lockfile twins are covered by org entitlements
//	wt auth login                 Activate a license key
//	wt auth status                Show current license tier
//	wt auth logout                Clear license key
//	wt registry add <n> <url>     Add a named registry
//	wt registry remove <name>     Remove a named registry
//	wt registry list              List configured registries
//	wt conformance <binary>       Run conformance tests against a twin
//	wt replay show <path>         Pretty-print a replay artifact (JSONL v1)
//	wt login --org <slug>         Authenticate with WonderTwin platform
//	wt catalog                    Browse the full twin catalog
//	wt scan                       Detect project dependencies, match against catalog
//	wt subscribe <twin>           Subscribe to a twin (trial, paid, or entitled)
//	wt request <service>          Request a twin that doesn't exist yet
//	wt request --list             List your org's twin requests
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"context"

	"github.com/wondertwin-ai/wondertwin/internal/auth"
	"github.com/wondertwin-ai/wondertwin/internal/cache"
	"github.com/wondertwin-ai/wondertwin/internal/client"
	"github.com/wondertwin-ai/wondertwin/internal/config"
	"github.com/wondertwin-ai/wondertwin/internal/conformance"
	"github.com/wondertwin-ai/wondertwin/internal/content"
	"github.com/wondertwin-ai/wondertwin/internal/lockfile"
	"github.com/wondertwin-ai/wondertwin/internal/manifest"
	"github.com/wondertwin-ai/wondertwin/internal/mcp"
	"github.com/wondertwin-ai/wondertwin/internal/platform"
	"github.com/wondertwin-ai/wondertwin/internal/procmgr"
	"github.com/wondertwin-ai/wondertwin/internal/registry"
	"github.com/wondertwin-ai/wondertwin/internal/scenario/v2"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

const defaultManifest = "wondertwin.json"

// resolveManifestPath returns the manifest path to use. If the given path
// is the default JSON and it doesn't exist, fall back to YAML variants.
func resolveManifestPath(path string) string {
	if path == "wondertwin.json" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		// Fall back to YAML if JSON doesn't exist
		if _, err := os.Stat("wondertwin.yaml"); err == nil {
			return "wondertwin.yaml"
		}
		if _, err := os.Stat("wondertwin.yml"); err == nil {
			return "wondertwin.yml"
		}
	}
	return path
}

func main() {
	cmd, args, manifestPath := parseArgs()
	manifestPath = resolveManifestPath(manifestPath)

	if cmd == "" || cmd == "help" || cmd == "--help" || cmd == "-h" {
		printUsage()
		if cmd == "" {
			os.Exit(1)
		}
		return
	}

	var err error
	switch cmd {
	case "version", "--version", "-v":
		fmt.Printf("wt version %s\n", version)
		return
	case "up":
		err = cmdUp(manifestPath)
	case "down":
		err = cmdDown()
	case "status":
		err = cmdStatus(manifestPath)
	case "reset":
		err = cmdReset(manifestPath)
	case "seed":
		err = cmdSeed(manifestPath, args)
	case "logs":
		err = cmdLogs(manifestPath, args)
	case "inspect":
		err = cmdInspect(manifestPath, args)
	case "mcp":
		err = cmdMcp(manifestPath)
	case "test":
		err = cmdTest(manifestPath, args)
	case "install":
		err = cmdInstall(manifestPath, args)
	case "ci":
		err = cmdCI(manifestPath)
	case "verify":
		err = cmdVerify(args)
	case "login":
		err = cmdLogin(args)
	case "catalog":
		err = cmdCatalog(args)
	case "scan":
		err = cmdScan(args)
	case "subscribe":
		err = cmdSubscribe(args)
	case "request":
		err = cmdRequest(args)
	case "auth":
		err = cmdAuth(args)
	case "registry":
		err = cmdRegistry(args)
	case "cache":
		err = cmdCache(manifestPath, args)
	case "conformance":
		err = cmdConformance(args)
	case "replay":
		err = cmdReplay(args)
	default:
		fmt.Fprintf(os.Stderr, "wt: unknown command %q\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "wt: %v\n", err)
		os.Exit(1)
	}
}

// parseArgs extracts the subcommand, positional args, and --config path from os.Args.
func parseArgs() (command string, args []string, manifestPath string) {
	manifestPath = defaultManifest
	if p := os.Getenv("WT_CONFIG"); p != "" {
		manifestPath = p
	}

	raw := os.Args[1:]
	var filtered []string
	for i := 0; i < len(raw); i++ {
		if raw[i] == "--config" && i+1 < len(raw) {
			manifestPath = raw[i+1]
			i++
			continue
		}
		filtered = append(filtered, raw[i])
	}

	if len(filtered) == 0 {
		return "", nil, manifestPath
	}
	return filtered[0], filtered[1:], manifestPath
}

func printUsage() {
	fmt.Printf(`wt — WonderTwin CLI %s

Usage:
  wt [--config <path>] <command> [arguments]

Commands:
  up                         Start all twins defined in wondertwin.json (or .yaml)
  down                       Stop all running twins
  status                     Health check all running twins
  reset                      Reset state on all running twins
  seed <twin> <file>         POST seed data to a twin
  logs <twin>                Tail logs of a running twin
  inspect <twin> [res]       Query twin state (res: state|requests|faults|time)
  mcp                        Start MCP server over stdio (for AI agents)
  test [path]                Run JSON test scenarios (default: ./scenarios/)
  install                    Install all twins from manifest
  install <twin>@<version>   Install a specific twin at a version
  ci                         Install twins from lock file (frozen, reproducible)
  verify                     Assert lockfile twins are covered by org entitlements (CI gate)
  auth login                 Activate a license key
  auth status                Show current license tier and org
  auth logout                Clear license key
  registry add <n> <url>     Add a named registry (--token <t> for auth)
  registry remove <name>     Remove a named registry
  registry list              List configured registries
  cache warm [twin...]       Fetch and cache content for manifest twins
  cache clear [twin...]      Remove cached content
  conformance <binary>       Run conformance tests against a twin binary
  replay show <path>         Pretty-print a replay artifact (JSONL, format v1)
  version                    Print the wt version

Options:
  --config <path>   Path to manifest (default: ./wondertwin.json, falls back to .yaml)

Environment:
  WT_CONFIG              Override default manifest path
  WT_REGISTRY_URL        Override registry URL
  WONDERTWIN_CONTENT_URL Override content API URL (default: https://api.wondertwin.ai)
`, version)
}

// ---------------------------------------------------------------------------
// wt up
// ---------------------------------------------------------------------------

func cmdUp(manifestPath string) error {
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	// Ensure twins are installed before starting
	manifestDir := filepath.Dir(manifestPath)
	if manifestDir == "" || manifestDir == "." {
		manifestDir, _ = os.Getwd()
	}

	if lockfile.Exists(manifestDir) {
		fmt.Println("Using locked versions from wondertwin-lock.json")
		if err := installFromLockFile(manifestDir, m); err != nil {
			return fmt.Errorf("installing from lock file: %w", err)
		}
	} else {
		fmt.Println("No lock file found, resolving from registry...")
		if err := cmdInstall(manifestPath, nil); err != nil {
			return err
		}
	}

	// Resolve content for commercial twins.
	// When wt-auth is configured, auth failures are fatal — the twin does not start.
	if err := resolveContent(m); err != nil {
		return fmt.Errorf("content resolution failed: %w", err)
	}

	pids, _ := procmgr.LoadPids()
	ac := client.New()

	fmt.Println("Starting twins...")
	fmt.Println()

	names := m.TwinNames()
	for _, name := range names {
		twin := m.Twins[name]

		// Skip if already running
		if entry, ok := pids[name]; ok && procmgr.IsRunning(entry.PID) {
			fmt.Printf("  %-20s already running (pid %d)\n", name, entry.PID)
			continue
		}

		pid, err := procmgr.Start(name, twin, m.Settings.LogDir, m.Settings.Verbose)
		if err != nil {
			fmt.Printf("  %-20s FAILED — %v\n", name, err)
			continue
		}

		pids[name] = procmgr.PidEntry{
			PID:    pid,
			Port:   twin.Port,
			Binary: twin.Binary,
		}
		fmt.Printf("  %-20s started (pid %d, port %d)\n", name, pid, twin.Port)
	}

	if err := procmgr.SavePids(pids); err != nil {
		return fmt.Errorf("saving pid state: %w", err)
	}

	// Wait for twins to pass health checks (retry with backoff, up to 5s per twin).
	fmt.Println()
	fmt.Println("Waiting for health checks...")
	time.Sleep(500 * time.Millisecond) // initial grace period
	fmt.Println()

	allHealthy := true
	for _, name := range names {
		twin := m.Twins[name]
		healthy := false
		for attempt := 0; attempt < 10; attempt++ {
			ok, _ := ac.Health(twin.AdminPort)
			if ok {
				healthy = true
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if healthy {
			fmt.Printf("  %-20s healthy    http://localhost:%d\n", name, twin.Port)
		} else {
			fmt.Printf("  %-20s unhealthy  http://localhost:%d\n", name, twin.Port)
			allHealthy = false
		}
	}

	// Load cached content into healthy twins via /admin/state
	loadContentIntoTwins(m, ac, names)

	fmt.Println()
	if allHealthy {
		fmt.Println("All twins up and healthy.")
	} else {
		fmt.Println("Some twins failed health check. Use 'wt logs <twin>' to investigate.")
	}
	return nil
}

// ---------------------------------------------------------------------------
// wt down
// ---------------------------------------------------------------------------

func cmdDown() error {
	pids, err := procmgr.LoadPids()
	if err != nil {
		return fmt.Errorf("loading pid state: %w", err)
	}

	if len(pids) == 0 {
		fmt.Println("No twins running.")
		return nil
	}

	fmt.Println("Stopping twins...")
	fmt.Println()

	for name, entry := range pids {
		if !procmgr.IsRunning(entry.PID) {
			fmt.Printf("  %-20s already stopped\n", name)
			continue
		}
		if err := procmgr.Stop(name, entry); err != nil {
			fmt.Printf("  %-20s FAILED — %v\n", name, err)
		} else {
			fmt.Printf("  %-20s stopped (was pid %d)\n", name, entry.PID)
		}
	}

	procmgr.RemovePidFile()

	fmt.Println()
	fmt.Println("All twins stopped.")
	return nil
}

// ---------------------------------------------------------------------------
// wt status
// ---------------------------------------------------------------------------

func cmdStatus(manifestPath string) error {
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	pids, _ := procmgr.LoadPids()
	ac := client.New()

	fmt.Println()
	fmt.Printf("  %-20s %-8s %-7s %-11s %s\n", "TWIN", "PID", "PORT", "HEALTH", "URL")
	fmt.Printf("  %-20s %-8s %-7s %-11s %s\n", "----", "---", "----", "------", "---")

	for _, name := range m.TwinNames() {
		twin := m.Twins[name]
		pidStr := "-"
		health := "stopped"

		if entry, ok := pids[name]; ok && procmgr.IsRunning(entry.PID) {
			pidStr = fmt.Sprintf("%d", entry.PID)
			ok, _ := ac.Health(twin.AdminPort)
			if ok {
				health = "healthy"
			} else {
				health = "unhealthy"
			}
		}

		fmt.Printf("  %-20s %-8s %-7d %-11s http://localhost:%d\n",
			name, pidStr, twin.Port, health, twin.Port)
	}

	fmt.Println()
	return nil
}

// ---------------------------------------------------------------------------
// wt reset
// ---------------------------------------------------------------------------

func cmdReset(manifestPath string) error {
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	pids, _ := procmgr.LoadPids()
	ac := client.New()

	fmt.Println("Resetting twins...")
	fmt.Println()

	for _, name := range m.TwinNames() {
		twin := m.Twins[name]
		if entry, ok := pids[name]; !ok || !procmgr.IsRunning(entry.PID) {
			fmt.Printf("  %-20s skipped (not running)\n", name)
			continue
		}

		resp, err := ac.Reset(twin.AdminPort)
		if err != nil {
			fmt.Printf("  %-20s FAILED — %v\n", name, err)
		} else {
			fmt.Printf("  %-20s reset   %s\n", name, resp)
		}
	}

	fmt.Println()
	return nil
}

// ---------------------------------------------------------------------------
// wt seed <twin> <file>
// ---------------------------------------------------------------------------

func cmdSeed(manifestPath string, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: wt seed <twin> <file>")
	}

	twinName := args[0]
	seedFile := args[1]

	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	twin, err := m.Twin(twinName)
	if err != nil {
		return err
	}

	ac := client.New()
	resp, err := ac.Seed(twin.AdminPort, seedFile)
	if err != nil {
		return fmt.Errorf("seeding %s: %w", twinName, err)
	}

	fmt.Printf("Seeded %s: %s\n", twinName, resp)
	return nil
}

// ---------------------------------------------------------------------------
// wt logs <twin>
// ---------------------------------------------------------------------------

func cmdLogs(manifestPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: wt logs <twin>")
	}

	twinName := args[0]

	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	// Validate twin exists in manifest
	if _, err := m.Twin(twinName); err != nil {
		return err
	}

	logPath := fmt.Sprintf("%s/%s.log", m.Settings.LogDir, twinName)

	if _, err := os.Stat(logPath); err != nil {
		return fmt.Errorf("no logs found for %s (expected %s)", twinName, logPath)
	}

	// tail -f the log file
	cmd := exec.Command("tail", "-f", "-n", "100", logPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Forward Ctrl+C to kill tail cleanly
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	return cmd.Run()
}

// ---------------------------------------------------------------------------
// wt inspect <twin> [resource]
// ---------------------------------------------------------------------------

func cmdInspect(manifestPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: wt inspect <twin> [state|requests|faults|time]")
	}

	twinName := args[0]
	resource := "state"
	if len(args) >= 2 {
		resource = args[1]
	}

	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	twin, err := m.Twin(twinName)
	if err != nil {
		return err
	}

	ac := client.New()

	var raw string
	switch resource {
	case "state":
		raw, err = ac.Inspect(twin.AdminPort)
	case "requests":
		raw, err = ac.InspectRequests(twin.AdminPort)
	case "faults":
		raw, err = ac.InspectFaults(twin.AdminPort)
	case "time":
		raw, err = ac.InspectTime(twin.AdminPort)
	default:
		return fmt.Errorf("unknown resource %q (expected state, requests, faults, or time)", resource)
	}
	if err != nil {
		return fmt.Errorf("inspecting %s/%s: %w", twinName, resource, err)
	}

	// Pretty-print the JSON response
	pretty, err := prettyJSON(raw)
	if err != nil {
		// Not valid JSON — print raw
		fmt.Print(raw)
		return nil
	}
	fmt.Println(pretty)
	return nil
}

// prettyJSON re-formats a JSON string with indentation.
func prettyJSON(raw string) (string, error) {
	var parsed json.RawMessage
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", err
	}
	indented, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return "", err
	}
	return string(indented), nil
}

// ---------------------------------------------------------------------------
// wt mcp
// ---------------------------------------------------------------------------

func cmdMcp(manifestPath string) error {
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	srv := mcp.NewServer(m)
	return srv.Serve()
}

// ---------------------------------------------------------------------------
// wt test [path]
// ---------------------------------------------------------------------------

func cmdTest(manifestPath string, args []string) error {
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	// Determine what to load: a specific file, a directory, or the default ./scenarios/
	path := "./scenarios/"
	if len(args) > 0 {
		path = args[0]
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("scenario path %s: %w", path, err)
	}

	var totalPassed, totalFailed, totalSteps int

	if info.IsDir() {
		scenarios, err := v2.LoadDir(path)
		if err != nil {
			return fmt.Errorf("loading scenarios: %w", err)
		}
		runner := v2.NewRunner(m)
		for _, s := range scenarios {
			result, runErr := runner.Run(s)
			p, f, st := printScenarioResult(s.Name, s.Description, result, runErr)
			totalPassed += p
			totalFailed += f
			totalSteps += st
		}
	} else {
		s, err := v2.LoadScenario(path)
		if err != nil {
			return err
		}
		runner := v2.NewRunner(m)
		result, runErr := runner.Run(s)
		p, f, st := printScenarioResult(s.Name, s.Description, result, runErr)
		totalPassed += p
		totalFailed += f
		totalSteps += st
	}

	// Summary
	fmt.Println()
	fmt.Printf("Results: %d passed, %d failed, %d total steps\n", totalPassed, totalFailed, totalSteps)

	if totalFailed > 0 {
		os.Exit(1)
	}
	return nil
}

// printScenarioResult prints scenario results and returns counts.
func printScenarioResult(name, description string, result *v2.Result, err error) (passed, failed, steps int) {
	fmt.Printf("\n--- %s ---\n", name)
	if description != "" {
		fmt.Printf("    %s\n", description)
	}
	fmt.Println()

	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return 0, 1, 0
	}

	for _, sr := range result.Steps {
		steps++
		if sr.Passed {
			fmt.Printf("  PASS  %-50s (%s)\n", sr.Name, sr.Duration.Round(time.Millisecond))
			passed++
		} else {
			fmt.Printf("  FAIL  %-50s (%s)\n", sr.Name, sr.Duration.Round(time.Millisecond))
			fmt.Printf("        %s\n", sr.Error)
			failed++
		}
	}

	fmt.Printf("\n  Scenario: %s (%s)\n", passFailLabel(result.Passed), result.Duration.Round(time.Millisecond))
	return
}

func passFailLabel(passed bool) string {
	if passed {
		return "PASSED"
	}
	return "FAILED"
}

// ---------------------------------------------------------------------------
// wt install
// ---------------------------------------------------------------------------

func cmdInstall(manifestPath string, args []string) error {
	// Load config for tier enforcement and registry lookup
	cfg, _ := config.Load()

	// wt install <twin>@<version> — install a single twin
	if len(args) > 0 {
		spec := args[0]
		twinName, versionSpec := parseInstallSpec(spec)

		if versionSpec == "" {
			versionSpec = "latest"
		}

		// Use public registry for ad-hoc installs (no manifest context)
		regEntry := cfg.Registries["public"]
		if u := os.Getenv("WT_REGISTRY_URL"); u != "" {
			regEntry.URL = u
		}

		fmt.Println("Fetching twin registry...")
		reg, err := registry.FetchRegistry(regEntry.URL, regEntry.Token)
		if err != nil {
			return err
		}

		resolvedVersion, ver, err := reg.ResolveVersion(twinName, versionSpec)
		if err != nil {
			return err
		}

		// Tier enforcement
		if err := registry.CheckTierAccess(twinName, resolvedVersion, ver, cfg); err != nil {
			return err
		}

		binaryDir := registry.ExpandPath("~/.wondertwin/bin")

		// Skip if already installed
		if registry.IsAlreadyInstalled(twinName, resolvedVersion, binaryDir) {
			fmt.Printf("  twin-%s v%s already installed, skipping.\n", twinName, resolvedVersion)
			return nil
		}

		return registry.Install(twinName, resolvedVersion, ver, binaryDir)
	}

	// wt install — install all twins from manifest
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	binaryDir := registry.ExpandPath(m.Settings.BinaryDir)
	platform := runtime.GOOS + "-" + runtime.GOARCH

	// Group twins by registry so we fetch each registry at most once
	registryCache := map[string]*registry.Registry{}
	registryFetchedAt := time.Time{}

	// Collect lock file entries as we resolve
	lockedTwins := map[string]lockfile.LockedTwin{}

	fmt.Println()
	names := m.TwinNames()
	var failed []string
	for _, name := range names {
		twin := m.Twins[name]
		versionSpec := twin.Version
		if versionSpec == "" {
			fmt.Printf("  %-20s skipped (no version specified, using binary path)\n", name)
			continue
		}

		// Resolve which registry this twin uses
		regName := twin.Registry
		reg, ok := registryCache[regName]
		if !ok {
			regEntry, entryOk := cfg.Registries[regName]
			if !entryOk {
				fmt.Printf("  %-20s FAILED — registry %q not configured (run `wt registry add %s <url>`)\n", name, regName, regName)
				failed = append(failed, name)
				continue
			}
			if u := os.Getenv("WT_REGISTRY_URL"); u != "" && regName == "public" {
				regEntry.URL = u
			}
			fmt.Printf("  Fetching registry %q...\n", regName)
			var fetchErr error
			reg, fetchErr = registry.FetchRegistry(regEntry.URL, regEntry.Token)
			if fetchErr != nil {
				fmt.Printf("  %-20s FAILED — %v\n", name, fetchErr)
				failed = append(failed, name)
				continue
			}
			registryCache[regName] = reg
			registryFetchedAt = time.Now().UTC()
		}

		resolvedVersion, ver, err := reg.ResolveVersion(name, versionSpec)
		if err != nil {
			fmt.Printf("  %-20s FAILED — %v\n", name, err)
			failed = append(failed, name)
			continue
		}

		// Record lock entry
		lockedTwins[name] = lockfile.LockedTwin{
			Version:      resolvedVersion,
			ResolvedFrom: versionSpec,
			SDKPackage:   ver.SDKPackage,
			SDKVersion:   ver.SDKVersion,
			Checksum:     ver.Checksums[platform],
			BinaryURL:    ver.BinaryURLs[platform],
		}

		// Tier enforcement
		if err := registry.CheckTierAccess(name, resolvedVersion, ver, cfg); err != nil {
			fmt.Printf("  %-20s BLOCKED — %v\n", name, err)
			failed = append(failed, name)
			continue
		}

		// Skip if already installed
		if registry.IsAlreadyInstalled(name, resolvedVersion, binaryDir) {
			fmt.Printf("  %-20s v%s already installed, skipping.\n", name, resolvedVersion)
			continue
		}

		if err := registry.Install(name, resolvedVersion, ver, binaryDir); err != nil {
			fmt.Printf("  %-20s FAILED — %v\n", name, err)
			failed = append(failed, name)
			continue
		}
	}

	fmt.Println()
	if len(failed) > 0 {
		return fmt.Errorf("failed to install: %s", strings.Join(failed, ", "))
	}

	// Write lock file
	if len(lockedTwins) > 0 {
		now := time.Now().UTC()
		if registryFetchedAt.IsZero() {
			registryFetchedAt = now
		}
		lf := &lockfile.LockFile{
			GeneratedAt:       now,
			RegistryFetchedAt: registryFetchedAt,
			Twins:             lockedTwins,
		}
		manifestDir := filepath.Dir(manifestPath)
		if manifestDir == "" || manifestDir == "." {
			manifestDir, _ = os.Getwd()
		}
		if err := lockfile.Save(manifestDir, lf); err != nil {
			return fmt.Errorf("writing lock file: %w", err)
		}
		fmt.Printf("Wrote %s\n", lockfile.Filename)
	}

	fmt.Println("All twins installed.")
	return nil
}

// parseInstallSpec parses "twin@version" into (twin, version).
// If no @ is present, returns (spec, "").
func parseInstallSpec(spec string) (string, string) {
	if i := strings.LastIndex(spec, "@"); i >= 0 {
		return spec[:i], spec[i+1:]
	}
	return spec, ""
}

// ---------------------------------------------------------------------------
// wt registry add|remove|list
// ---------------------------------------------------------------------------

func cmdRegistry(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: wt registry <add|remove|list>")
	}

	switch args[0] {
	case "add":
		return cmdRegistryAdd(args[1:])
	case "remove":
		return cmdRegistryRemove(args[1:])
	case "list":
		return cmdRegistryList()
	default:
		return fmt.Errorf("unknown registry subcommand %q (expected add, remove, or list)", args[0])
	}
}

func cmdRegistryAdd(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: wt registry add <name> <url> [--token <token>]")
	}

	name := args[0]
	url := args[1]

	if name == "public" {
		return fmt.Errorf("cannot override the built-in public registry")
	}

	var token string
	for i := 2; i < len(args); i++ {
		if args[i] == "--token" && i+1 < len(args) {
			token = args[i+1]
			i++
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	cfg.Registries[name] = config.RegistryEntry{
		URL:   url,
		Token: token,
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Registry %q added (%s)\n", name, url)
	if token != "" {
		fmt.Println("  Token: configured")
	}
	return nil
}

func cmdRegistryRemove(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: wt registry remove <name>")
	}

	name := args[0]
	if name == "public" {
		return fmt.Errorf("cannot remove the built-in public registry")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if _, ok := cfg.Registries[name]; !ok {
		return fmt.Errorf("registry %q not found", name)
	}

	delete(cfg.Registries, name)

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Registry %q removed.\n", name)
	return nil
}

func cmdRegistryList() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("  %-20s %-60s %s\n", "NAME", "URL", "AUTH")
	fmt.Printf("  %-20s %-60s %s\n", "----", "---", "----")

	for name, entry := range cfg.Registries {
		auth := "-"
		if entry.Token != "" {
			auth = "token"
		}
		fmt.Printf("  %-20s %-60s %s\n", name, entry.URL, auth)
	}

	fmt.Println()
	return nil
}

// ---------------------------------------------------------------------------
// wt auth login|status|logout
// ---------------------------------------------------------------------------

// platformBaseURL returns the wondertwin-app API URL, using the WT_PLATFORM_URL
// env var if set, otherwise the default.
func platformBaseURL() string {
	if u := os.Getenv("WT_PLATFORM_URL"); u != "" {
		return u
	}
	return platform.DefaultBaseURL
}

func cmdLogin(args []string) error {
	// Parse --org flag.
	var orgSlug string
	for i, a := range args {
		if a == "--org" && i+1 < len(args) {
			orgSlug = args[i+1]
			break
		}
		if strings.HasPrefix(a, "--org=") {
			orgSlug = strings.TrimPrefix(a, "--org=")
			break
		}
	}

	if orgSlug == "" {
		return fmt.Errorf("usage: wt login --org <org-slug>")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	fmt.Printf("Logging into org %q.\n", orgSlug)
	fmt.Print("Enter API key: ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return fmt.Errorf("no input received")
	}

	apiKey := strings.TrimSpace(scanner.Text())
	if apiKey == "" {
		return fmt.Errorf("no API key provided")
	}

	// Validate the key against the platform.
	client := platform.New(platformBaseURL(), "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := client.ValidateKey(ctx, apiKey)
	if err != nil {
		return fmt.Errorf("API key validation failed: %w", err)
	}

	cfg.OrgSlug = result.OrgSlug
	cfg.OrgID = result.OrgID
	cfg.APIKey = apiKey

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Authenticated as org %q (ID: %s).\n", result.OrgSlug, result.OrgID[:8]+"...")
	return nil
}

func cmdCatalog(args []string) error {
	// Parse flags.
	var category, tier, search string
	var mine bool
	var twinName string

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--category" && i+1 < len(args):
			category = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--category="):
			category = strings.TrimPrefix(args[i], "--category=")
		case args[i] == "--tier" && i+1 < len(args):
			tier = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--tier="):
			tier = strings.TrimPrefix(args[i], "--tier=")
		case args[i] == "--search" && i+1 < len(args):
			search = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--search="):
			search = strings.TrimPrefix(args[i], "--search=")
		case args[i] == "--mine":
			mine = true
		case !strings.HasPrefix(args[i], "-"):
			twinName = args[i]
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := platform.New(platformBaseURL(), "")

	// Detail view: wt catalog <name>
	if twinName != "" {
		return catalogDetail(ctx, client, twinName)
	}

	// --mine: org-scoped catalog.
	if mine {
		return catalogMine(ctx, category, search)
	}

	// Public catalog.
	twins, err := client.ListTwins(ctx, category, tier, "", search)
	if err != nil {
		return fmt.Errorf("fetching catalog: %w", err)
	}

	if len(twins) == 0 {
		fmt.Println("No twins found.")
		return nil
	}

	printCatalogTable(twins)
	return nil
}

func catalogDetail(ctx context.Context, client *platform.Client, name string) error {
	twins, err := client.ListTwins(ctx, "", "", "", name)
	if err != nil {
		return fmt.Errorf("fetching twin detail: %w", err)
	}

	// Filter to exact match.
	var matches []platform.CatalogEntry
	for _, t := range twins {
		if t.Name == name {
			matches = append(matches, t)
		}
	}

	if len(matches) == 0 {
		return fmt.Errorf("twin %q not found", name)
	}

	for i, t := range matches {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("Name:       %s\n", t.DisplayName)
		fmt.Printf("Tier:       %s\n", t.Tier)
		fmt.Printf("Category:   %s\n", t.Category)
		fmt.Printf("Status:     %s\n", t.Status)
		fmt.Printf("Coverage:   %d%%\n", t.Coverage.EstimatedPct)
		fmt.Printf("Resources:  %d\n", t.Coverage.ResourceCount)
		fmt.Printf("Auth:       %s\n", t.Coverage.AuthPattern)
		fmt.Printf("Webhooks:   %v\n", t.Coverage.WebhookSupport)
		if t.SDKTargets.Primary.Package != "" {
			fmt.Printf("SDK:        %s (%s)\n", t.SDKTargets.Primary.Package, t.SDKTargets.Primary.Language)
		}
		if t.Description != "" {
			fmt.Printf("Desc:       %s\n", t.Description)
		}
	}
	return nil
}

func catalogMine(ctx context.Context, category, search string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasOrgContext() {
		fmt.Println("Not logged in. Run `wt login --org <slug>` or sign up at https://app.wondertwin.ai/signup")
		return nil
	}

	client := platform.New(platformBaseURL(), cfg.APIKey)
	twins, err := client.ListOrgCatalog(ctx, cfg.OrgID, category, search)
	if err != nil {
		return fmt.Errorf("fetching org catalog: %w", err)
	}

	if len(twins) == 0 {
		fmt.Println("No twins found.")
		return nil
	}

	printOrgCatalogTable(twins)
	return nil
}

func printCatalogTable(twins []platform.CatalogEntry) {
	fmt.Printf("%-20s %-16s %-12s %-8s\n", "NAME", "CATEGORY", "TIER", "STATUS")
	for _, t := range twins {
		cat := t.Category
		if cat == "" {
			cat = "—"
		}
		fmt.Printf("%-20s %-16s %-12s %-8s\n", t.Name, cat, t.Tier, t.Status)
	}
}

func printOrgCatalogTable(twins []platform.OrgCatalogEntry) {
	fmt.Printf("%-20s %-16s %-12s %-12s %-16s\n", "NAME", "CATEGORY", "TIER", "STATE", "ACTION")
	for _, t := range twins {
		cat := t.Category
		if cat == "" {
			cat = "—"
		}
		fmt.Printf("%-20s %-16s %-12s %-12s %-16s\n", t.Name, cat, t.Tier, t.EntitlementState, t.AvailableAction)
	}
}

func cmdScan(args []string) error {
	var projectPath string
	var recommend bool

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--path" && i+1 < len(args):
			projectPath = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--path="):
			projectPath = strings.TrimPrefix(args[i], "--path=")
		case args[i] == "--recommend":
			recommend = true
		}
	}

	if projectPath == "" {
		projectPath = "."
	}

	if recommend {
		fmt.Println("Recommendations coming soon (Phase 2).")
	}

	// Detect SDKs from project files.
	detected := detectSDKs(projectPath)
	if len(detected) == 0 {
		fmt.Println("No SDK dependencies detected.")
		return nil
	}

	// Fetch catalog to match against.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := platform.New(platformBaseURL(), "")
	catalog, err := client.ListTwins(ctx, "", "", "", "")
	if err != nil {
		return fmt.Errorf("fetching catalog: %w", err)
	}

	// Build SDK → twin lookup from catalog.
	type twinMatch struct {
		Name string
		Tier string
	}
	sdkIndex := make(map[string]twinMatch)
	for _, entry := range catalog {
		if pkg := entry.SDKTargets.Primary.Package; pkg != "" {
			sdkIndex[pkg] = twinMatch{Name: entry.Name, Tier: entry.Tier}
		}
		for _, a := range entry.SDKTargets.Additional {
			if a.Package != "" {
				sdkIndex[a.Package] = twinMatch{Name: entry.Name, Tier: entry.Tier}
			}
		}
	}

	// Match detected SDKs against catalog.
	fmt.Printf("%-45s %-14s %-12s %-16s\n", "DETECTED SDK", "TWIN", "TIER", "ACTION")
	for _, sdk := range detected {
		match, found := sdkIndex[sdk.Package]
		if !found {
			// Check for prefix match (Go module versioning: github.com/stripe/stripe-go/v81).
			for catalogPkg, m := range sdkIndex {
				if strings.HasPrefix(sdk.Package, catalogPkg) || strings.HasPrefix(catalogPkg, sdk.Package) {
					match = m
					found = true
					break
				}
			}
		}

		label := sdk.Package
		if sdk.Version != "" {
			label += " " + sdk.Version
		}

		if found {
			action := "subscribe"
			if match.Tier == "community" {
				action = "install"
			}
			fmt.Printf("%-45s %-14s %-12s %-16s\n", label, match.Name, match.Tier, action)
		} else {
			fmt.Printf("%-45s %-14s %-12s %-16s\n", label, "—", "—", "no twin available")
		}
	}

	return nil
}

type detectedSDK struct {
	Package  string
	Version  string
	Language string
}

func detectSDKs(projectPath string) []detectedSDK {
	var results []detectedSDK
	results = append(results, detectGoSDKs(projectPath)...)
	results = append(results, detectNodeSDKs(projectPath)...)
	return results
}

func detectGoSDKs(projectPath string) []detectedSDK {
	goModPath := filepath.Join(projectPath, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return nil
	}

	var results []detectedSDK
	inRequire := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "require (" {
			inRequire = true
			continue
		}
		if line == ")" {
			inRequire = false
			continue
		}
		if !inRequire {
			continue
		}
		// Skip indirect deps.
		if strings.Contains(line, "// indirect") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			results = append(results, detectedSDK{
				Package:  parts[0],
				Version:  parts[1],
				Language: "go",
			})
		}
	}
	return results
}

func detectNodeSDKs(projectPath string) []detectedSDK {
	pkgPath := filepath.Join(projectPath, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil
	}

	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	var results []detectedSDK
	for name, version := range pkg.Dependencies {
		results = append(results, detectedSDK{
			Package:  name,
			Version:  version,
			Language: "typescript",
		})
	}
	for name, version := range pkg.DevDependencies {
		results = append(results, detectedSDK{
			Package:  name,
			Version:  version,
			Language: "typescript",
		})
	}
	return results
}

func cmdAuth(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: wt auth <login|status|logout>")
	}

	switch args[0] {
	case "login":
		return cmdAuthLogin()
	case "status":
		return cmdAuthStatus()
	case "logout":
		return cmdAuthLogout()
	default:
		return fmt.Errorf("unknown auth subcommand %q (expected login, status, or logout)", args[0])
	}
}

func cmdAuthLogin() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	fmt.Print("Enter license key: ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return fmt.Errorf("no input received")
	}

	key := strings.TrimSpace(scanner.Text())
	if key == "" {
		return fmt.Errorf("no license key provided")
	}

	info := config.ParseLicenseKey(key)
	if info == nil {
		return fmt.Errorf("invalid license key format")
	}

	cfg.LicenseKey = key
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	tierName := config.TierName(info.Tier)
	if info.Org == "ind" {
		fmt.Printf("Activated %s license (individual).\n", tierName)
	} else {
		fmt.Printf("Activated %s license for org %q.\n", tierName, info.Org)
	}

	return nil
}

func cmdAuthStatus() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Show org context if present.
	if cfg.HasOrgContext() {
		fmt.Printf("Org:     %s\n", cfg.OrgSlug)
		fmt.Printf("Org ID:  %s\n", cfg.OrgID)
		maskedKey := cfg.APIKey[:8] + "..." + cfg.APIKey[len(cfg.APIKey)-4:]
		fmt.Printf("API Key: %s\n", maskedKey)
		printProjectMismatchWarning(cfg.OrgID)
		return nil
	}

	// Fall back to legacy license key display.
	if cfg.LicenseKey == "" {
		fmt.Println("Not authenticated. Run `wt login --org <slug>` to connect.")
		return nil
	}

	info := config.ParseLicenseKey(cfg.LicenseKey)
	if info == nil {
		fmt.Println("Tier: free (invalid license key)")
		return nil
	}

	tierName := config.TierName(info.Tier)
	fmt.Printf("Tier: %s\n", tierName)
	if info.Org != "ind" {
		fmt.Printf("Org:  %s\n", info.Org)
	}
	fmt.Printf("Key:  %s...%s\n", info.Raw[:6], info.Raw[len(info.Raw)-4:])

	return nil
}

func cmdAuthLogout() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	hadOrg := cfg.HasOrgContext()
	hadLicense := cfg.LicenseKey != ""

	if !hadOrg && !hadLicense {
		fmt.Println("Not authenticated.")
		return nil
	}

	cfg.ClearOrgContext()
	cfg.LicenseKey = ""

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("Logged out.")
	return nil
}

// ---------------------------------------------------------------------------
// wt conformance <binary> [--port <port>]
// ---------------------------------------------------------------------------

func cmdConformance(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: wt conformance <binary> [--port <port>]")
	}

	binaryPath := args[0]
	port := 19876 // default conformance test port

	// Parse optional --port flag
	for i := 1; i < len(args); i++ {
		if args[i] == "--port" && i+1 < len(args) {
			p, err := strconv.Atoi(args[i+1])
			if err != nil {
				return fmt.Errorf("invalid port: %s", args[i+1])
			}
			port = p
			i++
		}
	}

	// Resolve binary path
	absPath, err := filepath.Abs(binaryPath)
	if err != nil {
		return fmt.Errorf("resolving binary path: %w", err)
	}

	fmt.Printf("Running conformance suite against %s on port %d...\n\n", binaryPath, port)

	report, err := conformance.Run(absPath, port)
	if err != nil {
		return err
	}

	for _, r := range report.Results {
		if r.Passed {
			fmt.Printf("  PASS  %s\n", r.Name)
		} else {
			fmt.Printf("  FAIL  %s\n", r.Name)
			fmt.Printf("        %s\n", r.Detail)
		}
	}

	fmt.Printf("\nResults: %d passed, %d failed, %d total\n", report.Passed, report.Failed, report.Passed+report.Failed)

	if report.Failed > 0 {
		os.Exit(1)
	}
	return nil
}

// ---------------------------------------------------------------------------
// wt ci
// ---------------------------------------------------------------------------

func cmdCI(manifestPath string) error {
	manifestDir := filepath.Dir(manifestPath)
	if manifestDir == "" || manifestDir == "." {
		manifestDir, _ = os.Getwd()
	}

	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	if !lockfile.Exists(manifestDir) {
		return fmt.Errorf("no %s found, run 'wt install' first", lockfile.Filename)
	}

	lf, err := lockfile.Load(manifestDir)
	if err != nil {
		return fmt.Errorf("reading lock file: %w", err)
	}

	binaryDir := registry.ExpandPath(m.Settings.BinaryDir)

	fmt.Println("Installing from lock file (frozen)...")
	fmt.Println()

	var failed []string
	for name, locked := range lf.Twins {
		if registry.IsAlreadyInstalled(name, locked.Version, binaryDir) {
			fmt.Printf("  %-20s v%s already installed, skipping.\n", name, locked.Version)
			continue
		}

		if locked.BinaryURL == "" {
			fmt.Printf("  %-20s FAILED — no binary_url in lock file\n", name)
			failed = append(failed, name)
			continue
		}

		fmt.Printf("  Downloading twin-%s v%s...\n", name, locked.Version)
		if err := registry.InstallFromURL(name, locked.Version, locked.BinaryURL, locked.Checksum, binaryDir); err != nil {
			fmt.Printf("  %-20s FAILED — %v\n", name, err)
			failed = append(failed, name)
			continue
		}
	}

	fmt.Println()
	if len(failed) > 0 {
		return fmt.Errorf("failed to install: %s", strings.Join(failed, ", "))
	}
	fmt.Println("All twins installed from lock file.")
	return nil
}

// ---------------------------------------------------------------------------
// Content resolution helpers
// ---------------------------------------------------------------------------

const defaultContentURL = "https://api.wondertwin.ai"

// contentURL returns the Content API base URL, checking env override first.
func contentURL() string {
	if u := os.Getenv("WONDERTWIN_CONTENT_URL"); u != "" {
		return u
	}
	return defaultContentURL
}

// cacheDir returns the path to ~/.wondertwin/cache.
func cacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".wondertwin/cache"
	}
	return filepath.Join(home, ".wondertwin", "cache")
}

// authURL returns the wt-auth service URL, checking env override first.
func authURL() string {
	if u := os.Getenv("WT_AUTH_URL"); u != "" {
		return u
	}
	return ""
}

// resolveContent fetches and caches content for all twins in the manifest
// that have a version set. Returns nil if no license is configured.
//
// When wt-auth is configured (WT_AUTH_URL set), this function requests a JIT
// token for each twin before fetching content. Auth failures are FATAL — the
// twin does not start. No graceful degradation, no stale cache fallback.
func resolveContent(m *manifest.Manifest) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasValidLicense() {
		return nil
	}

	cacheStore := cache.NewStore(cacheDir(), cfg.LicenseKey)

	// If wt-auth is configured, use JIT tokens. Otherwise fall back to
	// direct license key auth (development mode).
	authEndpoint := authURL()
	var ac *auth.Client
	if authEndpoint != "" {
		ac = auth.NewClient(authEndpoint)
	}

	for _, name := range m.TwinNames() {
		twin := m.Twins[name]
		version := twin.Version
		if version == "" || version == "latest" {
			continue
		}

		if cacheStore.IsValid(name, version) {
			continue
		}

		// Determine the bearer token for the Content API.
		bearerToken := cfg.LicenseKey // development mode fallback
		if ac != nil {
			// Production mode: request JIT token from wt-auth.
			tokenResp, err := ac.RequestToken(cfg.LicenseKey, name)
			if err != nil {
				// Auth failures are fatal for licensed twins.
				// No graceful degradation. No stale cache. Twin does not start.
				if authErr, ok := err.(*auth.AuthError); ok {
					return fmt.Errorf("authorization failed for %s: %s", name, authErr.Error())
				}
				return fmt.Errorf("authorization failed for %s: %v", name, err)
			}
			bearerToken = tokenResp.Token
		}

		cc := content.NewClient(contentURL(), bearerToken)
		raw, _, err := cc.FetchContent(name, version)
		if err != nil {
			if ac != nil {
				// With auth configured, content fetch failures are also fatal.
				return fmt.Errorf("content fetch failed for %s@%s: %v", name, version, err)
			}
			// Development mode: graceful degradation with stale cache.
			_, meta, cacheErr := cacheStore.Get(name, version)
			if cacheErr == nil && meta != nil {
				fmt.Printf("  warn: content fetch failed for %s@%s, using stale cache: %v\n", name, version, err)
				continue
			}
			fmt.Printf("  warn: content fetch failed for %s@%s: %v\n", name, version, err)
			continue
		}

		if err := cacheStore.Put(name, version, raw, 24); err != nil {
			fmt.Printf("  warn: caching content for %s@%s: %v\n", name, version, err)
		}
	}

	return nil
}

// loadContentIntoTwins posts cached content payloads to healthy twins via /admin/state.
func loadContentIntoTwins(m *manifest.Manifest, ac *client.AdminClient, names []string) {
	cfg, _ := config.Load()
	if cfg == nil || !cfg.HasValidLicense() {
		return
	}

	store := cache.NewStore(cacheDir(), cfg.LicenseKey)

	for _, name := range names {
		twin := m.Twins[name]
		version := twin.Version
		if version == "" || version == "latest" {
			continue
		}

		payload, _, err := store.Get(name, version)
		if err != nil || payload == nil {
			continue
		}

		if _, err := ac.PostState(twin.AdminPort, payload); err != nil {
			fmt.Printf("  warn: loading content into %s: %v\n", name, err)
		}
	}
}

// ---------------------------------------------------------------------------
// wt cache warm|clear
// ---------------------------------------------------------------------------

func cmdCache(manifestPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: wt cache <warm|clear>")
	}

	switch args[0] {
	case "warm":
		return cmdCacheWarm(manifestPath, args[1:])
	case "clear":
		return cmdCacheClear(args[1:])
	default:
		return fmt.Errorf("unknown cache subcommand %q (expected warm or clear)", args[0])
	}
}

func cmdCacheWarm(manifestPath string, args []string) error {
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasValidLicense() {
		return fmt.Errorf("no valid license key — run 'wt auth login' first")
	}

	store := cache.NewStore(cacheDir(), cfg.LicenseKey)
	cc := content.NewClient(contentURL(), cfg.LicenseKey)

	// If specific twins given, use those; otherwise use manifest
	targets := args
	if len(targets) == 0 {
		targets = m.TwinNames()
	}

	fmt.Println("Warming content cache...")
	fmt.Println()

	for _, name := range targets {
		twin, err := m.Twin(name)
		if err != nil {
			fmt.Printf("  %-20s skipped (not in manifest)\n", name)
			continue
		}
		version := twin.Version
		if version == "" || version == "latest" {
			fmt.Printf("  %-20s skipped (no pinned version)\n", name)
			continue
		}

		if store.IsValid(name, version) {
			fmt.Printf("  %-20s cached (valid)\n", name)
			continue
		}

		raw, _, err := cc.FetchContent(name, version)
		if err != nil {
			fmt.Printf("  %-20s FAILED — %v\n", name, err)
			continue
		}

		if err := store.Put(name, version, raw, 24); err != nil {
			fmt.Printf("  %-20s FAILED — %v\n", name, err)
			continue
		}
		fmt.Printf("  %-20s cached %s@%s\n", name, name, version)
	}

	fmt.Println()
	return nil
}

func cmdCacheClear(args []string) error {
	cfg, _ := config.Load()
	licenseKey := ""
	if cfg != nil {
		licenseKey = cfg.LicenseKey
	}

	store := cache.NewStore(cacheDir(), licenseKey)

	if len(args) == 0 {
		if err := store.Clear("", ""); err != nil {
			return fmt.Errorf("clearing cache: %w", err)
		}
		fmt.Println("Content cache cleared.")
		return nil
	}

	for _, name := range args {
		if err := store.Clear(name, ""); err != nil {
			fmt.Printf("  warn: clearing cache for %s: %v\n", name, err)
			continue
		}
		fmt.Printf("  Cleared cache for %s\n", name)
	}
	return nil
}

// installFromLockFile installs twins using versions from the lock file.
func installFromLockFile(manifestDir string, m *manifest.Manifest) error {
	lf, err := lockfile.Load(manifestDir)
	if err != nil {
		return err
	}

	binaryDir := registry.ExpandPath(m.Settings.BinaryDir)

	for name, locked := range lf.Twins {
		if registry.IsAlreadyInstalled(name, locked.Version, binaryDir) {
			fmt.Printf("  %-20s v%s (locked), already installed.\n", name, locked.Version)
			continue
		}

		if locked.BinaryURL == "" {
			return fmt.Errorf("twin %s: no binary_url in lock file", name)
		}

		fmt.Printf("  Downloading twin-%s v%s (locked)...\n", name, locked.Version)
		if err := registry.InstallFromURL(name, locked.Version, locked.BinaryURL, locked.Checksum, binaryDir); err != nil {
			return fmt.Errorf("twin %s: %w", name, err)
		}
	}

	return nil
}

const signupBaseURL = "https://app.wondertwin.ai/signup"

func cmdSubscribe(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: wt subscribe <twin-name>")
	}

	twinName := args[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// No-account path: surface signup URL.
	if cfg.APIKey == "" {
		fmt.Printf("%s requires a WonderTwin account.\n", twinName)
		fmt.Printf("Sign up: %s?ref=cli&twin=%s\n", signupBaseURL, twinName)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client := platform.New(platformBaseURL(), cfg.APIKey)
	resp, err := client.Subscribe(ctx, cfg.OrgID, twinName)
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	switch resp.Action {
	case "already_entitled":
		fmt.Printf("%s is already in your catalog. Run `wt install %s` to install.\n", twinName, twinName)
	case "subscribed":
		fmt.Printf("Subscribed to %s. Run `wt install %s` to install.\n", twinName, twinName)
	case "trial_started":
		fmt.Printf("Pro trial started (expires %s). %s is now available.\n", resp.TrialEndsAt, twinName)
		fmt.Printf("Run `wt install %s` to install.\n", twinName)
	case "upgrade_required":
		fmt.Printf("%s requires WonderTwin Pro.\n", twinName)
		if resp.CheckoutURL != "" {
			fmt.Printf("Start your subscription: %s\n", resp.CheckoutURL)
		} else {
			fmt.Printf("Upgrade at: %s?ref=cli&twin=%s\n", signupBaseURL, twinName)
		}
	case "approval_requested":
		fmt.Printf("Subscription request submitted to your org admin.")
		if resp.RequestID != "" {
			fmt.Printf(" (request %s)", resp.RequestID)
		}
		fmt.Println()
	case "signup_required":
		fmt.Printf("%s requires a WonderTwin account.\n", twinName)
		if resp.SignupURL != "" {
			fmt.Printf("Sign up: %s\n", resp.SignupURL)
		} else {
			fmt.Printf("Sign up: %s?ref=cli&twin=%s\n", signupBaseURL, twinName)
		}
	default:
		fmt.Printf("Response: %s\n", resp.Action)
		if resp.Message != "" {
			fmt.Println(resp.Message)
		}
	}

	return nil
}

func cmdRequest(args []string) error {
	// Parse flags.
	var listMode bool
	var serviceName string

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--list":
			listMode = true
		case !strings.HasPrefix(args[i], "-"):
			serviceName = args[i]
		}
	}

	if !listMode && serviceName == "" {
		return fmt.Errorf("usage: wt request <service-name>  or  wt request --list")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// --list: show org's requests.
	if listMode {
		if cfg.APIKey == "" {
			return fmt.Errorf("not logged in. Run `wt login --org <slug>` first")
		}

		client := platform.New(platformBaseURL(), cfg.APIKey)
		reqs, err := client.ListTwinRequests(ctx, cfg.OrgID)
		if err != nil {
			return fmt.Errorf("list requests: %w", err)
		}

		if len(reqs) == 0 {
			fmt.Println("No twin requests.")
			return nil
		}

		fmt.Printf("%-36s  %-20s  %-12s  %s\n", "ID", "SERVICE", "STATUS", "CREATED")
		fmt.Printf("%-36s  %-20s  %-12s  %s\n", strings.Repeat("-", 36), strings.Repeat("-", 20), strings.Repeat("-", 12), strings.Repeat("-", 19))
		for _, r := range reqs {
			created := r.CreatedAt
			if len(created) > 19 {
				created = created[:19]
			}
			fmt.Printf("%-36s  %-20s  %-12s  %s\n", r.ID, r.ServiceName, r.Status, created)
		}
		return nil
	}

	// No account: suggest GitHub issue.
	if cfg.APIKey == "" {
		fmt.Printf("No WonderTwin account. To request a twin for %s:\n", serviceName)
		fmt.Printf("  Create an issue: https://github.com/wondertwin-ai/wondertwin/issues/new?title=Twin+Request:+%s&labels=twin-request\n", serviceName)
		fmt.Printf("  Or sign up for Pro to get structured request tracking: %s?ref=cli\n", signupBaseURL)
		return nil
	}

	// Pro path: submit structured request.
	client := platform.New(platformBaseURL(), cfg.APIKey)
	result, err := client.SubmitTwinRequest(ctx, cfg.OrgID, serviceName, "", "")
	if err != nil {
		return fmt.Errorf("submit request: %w", err)
	}

	if result.Status == "submitted" {
		fmt.Printf("Twin request submitted for %s (request %s).\n", serviceName, result.ID)
		fmt.Println("We'll research this and get back to you.")
	} else {
		// Dedup: existing request returned.
		fmt.Printf("Existing request for %s (request %s, status: %s).\n", serviceName, result.ID, result.Status)
	}

	return nil
}
