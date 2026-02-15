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
//	wt auth login                 Activate a license key
//	wt auth status                Show current license tier
//	wt auth logout                Clear license key
//	wt registry add <n> <url>     Add a named registry
//	wt registry remove <name>     Remove a named registry
//	wt registry list              List configured registries
//	wt conformance <binary>       Run conformance tests against a twin
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/client"
	"github.com/wondertwin-ai/wondertwin/internal/config"
	"github.com/wondertwin-ai/wondertwin/internal/conformance"
	"github.com/wondertwin-ai/wondertwin/internal/manifest"
	"github.com/wondertwin-ai/wondertwin/internal/mcp"
	"github.com/wondertwin-ai/wondertwin/internal/procmgr"
	"github.com/wondertwin-ai/wondertwin/internal/registry"
	"github.com/wondertwin-ai/wondertwin/internal/scenario"
	scenariov2 "github.com/wondertwin-ai/wondertwin/internal/scenario/v2"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

const defaultManifest = "wondertwin.yaml"

// resolveManifestPath returns the manifest path to use. If the given path
// is the default YAML and a wondertwin.json exists alongside it, prefer JSON.
func resolveManifestPath(path string) string {
	base := filepath.Base(path)
	if base == "wondertwin.yaml" || base == "wondertwin.yml" {
		jsonPath := filepath.Join(filepath.Dir(path), "wondertwin.json")
		if _, err := os.Stat(jsonPath); err == nil {
			return jsonPath
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
	case "auth":
		err = cmdAuth(args)
	case "registry":
		err = cmdRegistry(args)
	case "conformance":
		err = cmdConformance(args)
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
  test [path]                Run YAML test scenarios (default: ./scenarios/)
  install                    Install all twins from manifest
  install <twin>@<version>   Install a specific twin at a version
  auth login                 Activate a license key
  auth status                Show current license tier and org
  auth logout                Clear license key
  registry add <n> <url>     Add a named registry (--token <t> for auth)
  registry remove <name>     Remove a named registry
  registry list              List configured registries
  conformance <binary>       Run conformance tests against a twin binary
  version                    Print the wt version

Options:
  --config <path>   Path to manifest (default: ./wondertwin.json or ./wondertwin.yaml)

Environment:
  WT_CONFIG         Override default manifest path
  WT_REGISTRY_URL   Override registry URL
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

	// Give twins a moment to bind their ports
	fmt.Println()
	fmt.Println("Waiting for health checks...")
	time.Sleep(1500 * time.Millisecond)
	fmt.Println()

	allHealthy := true
	for _, name := range names {
		twin := m.Twins[name]
		ok, _ := ac.Health(twin.AdminPort)
		if ok {
			fmt.Printf("  %-20s healthy    http://localhost:%d\n", name, twin.Port)
		} else {
			fmt.Printf("  %-20s unhealthy  http://localhost:%d\n", name, twin.Port)
			allHealthy = false
		}
	}

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

	totalPassed := 0
	totalFailed := 0
	totalSteps := 0

	if info.IsDir() {
		// Run both YAML and JSON scenarios from the directory
		p, f, s := runV1Dir(m, path)
		totalPassed += p
		totalFailed += f
		totalSteps += s

		p, f, s = runV2Dir(m, path)
		totalPassed += p
		totalFailed += f
		totalSteps += s
	} else {
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".json":
			p, f, s := runV2File(m, path)
			totalPassed += p
			totalFailed += f
			totalSteps += s
		case ".yaml", ".yml":
			p, f, s := runV1File(m, path)
			totalPassed += p
			totalFailed += f
			totalSteps += s
		default:
			return fmt.Errorf("unsupported scenario format %q", ext)
		}
	}

	// Summary
	fmt.Println()
	fmt.Printf("Results: %d passed, %d failed, %d total\n", totalPassed, totalFailed, totalPassed+totalFailed)

	if totalFailed > 0 {
		os.Exit(1)
	}
	return nil
}

// runV1Dir runs all YAML scenarios from a directory using the v1 runner.
// JSON files are skipped here because they are handled by the v2 runner.
func runV1Dir(m *manifest.Manifest, dir string) (passed, failed, steps int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, 0
	}

	runner := scenario.NewRunner(m)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		s, err := scenario.LoadScenario(filepath.Join(dir, entry.Name()))
		if err != nil {
			fmt.Printf("\n  ERROR loading %s: %v\n", entry.Name(), err)
			failed++
			continue
		}
		result, runErr := runner.Run(s)
		p, f, st := printScenarioResult(s.Name, s.Description, result, runErr)
		passed += p
		failed += f
		steps += st
	}
	return
}

// runV2Dir runs all JSON scenarios from a directory using the v2 runner.
func runV2Dir(m *manifest.Manifest, dir string) (passed, failed, steps int) {
	scenarios, err := scenariov2.LoadDir(dir)
	if err != nil {
		fmt.Printf("\n  ERROR loading scenarios: %v\n", err)
		return 0, 1, 0
	}

	runner := scenariov2.NewRunner(m)
	for _, s := range scenarios {
		result, runErr := runner.Run(s)
		p, f, st := printV2ScenarioResult(s.Name, s.Description, result, runErr)
		passed += p
		failed += f
		steps += st
	}
	return
}

// runV1File runs a single YAML scenario file.
func runV1File(m *manifest.Manifest, path string) (passed, failed, steps int) {
	s, err := scenario.LoadScenario(path)
	if err != nil {
		fmt.Printf("\n  ERROR: %v\n", err)
		return 0, 1, 0
	}
	runner := scenario.NewRunner(m)
	result, runErr := runner.Run(s)
	return printScenarioResult(s.Name, s.Description, result, runErr)
}

// runV2File runs a single JSON scenario file using the v2 runner.
func runV2File(m *manifest.Manifest, path string) (passed, failed, steps int) {
	s, err := scenariov2.LoadScenario(path)
	if err != nil {
		fmt.Printf("\n  ERROR: %v\n", err)
		return 0, 1, 0
	}
	runner := scenariov2.NewRunner(m)
	result, runErr := runner.Run(s)
	return printV2ScenarioResult(s.Name, s.Description, result, runErr)
}

// printScenarioResult prints v1 scenario results and returns counts.
func printScenarioResult(name, description string, result *scenario.Result, err error) (passed, failed, steps int) {
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

// printV2ScenarioResult prints v2 scenario results and returns counts.
func printV2ScenarioResult(name, description string, result *scenariov2.Result, err error) (passed, failed, steps int) {
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

	// Group twins by registry so we fetch each registry at most once
	registryCache := map[string]*registry.Registry{}

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
		}

		resolvedVersion, ver, err := reg.ResolveVersion(name, versionSpec)
		if err != nil {
			fmt.Printf("  %-20s FAILED — %v\n", name, err)
			failed = append(failed, name)
			continue
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

	if cfg.LicenseKey == "" {
		fmt.Println("Tier: free (no license key)")
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

	if cfg.LicenseKey == "" {
		fmt.Println("No license key configured.")
		return nil
	}

	cfg.LicenseKey = ""
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("License key removed.")
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
