// wt is the WonderTwin CLI — a process manager and admin client for twin binaries.
//
// Usage:
//
//	wt up                     Start all twins from wondertwin.yaml
//	wt down                   Stop all running twins
//	wt status                 Health check all running twins
//	wt reset                  Reset state on all running twins
//	wt seed <twin> <file>     POST seed data to a twin's /admin/state
//	wt logs <twin>            Tail stdout/stderr of a running twin
//	wt inspect <twin> [res]   Query a running twin's internal state
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/client"
	"github.com/wondertwin-ai/wondertwin/internal/manifest"
	"github.com/wondertwin-ai/wondertwin/internal/mcp"
	"github.com/wondertwin-ai/wondertwin/internal/procmgr"
)

const defaultManifest = "wondertwin.yaml"

func main() {
	cmd, args, manifestPath := parseArgs()

	if cmd == "" || cmd == "help" || cmd == "--help" || cmd == "-h" {
		printUsage()
		if cmd == "" {
			os.Exit(1)
		}
		return
	}

	var err error
	switch cmd {
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
	fmt.Print(`wt — WonderTwin CLI v0.1

Usage:
  wt [--config <path>] <command> [arguments]

Commands:
  up                     Start all twins defined in wondertwin.yaml
  down                   Stop all running twins
  status                 Health check all running twins
  reset                  Reset state on all running twins
  seed <twin> <file>     POST seed data to a twin
  logs <twin>            Tail logs of a running twin
  inspect <twin> [res]   Query twin state (res: state|requests|faults|time)
  mcp                    Start MCP server over stdio (for AI agents)

Options:
  --config <path>   Path to wondertwin.yaml (default: ./wondertwin.yaml)

Environment:
  WT_CONFIG         Override default manifest path
`)
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
