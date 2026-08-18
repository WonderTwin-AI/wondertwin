// Package procmgr manages twin process lifecycle — starting, stopping,
// PID tracking, and log file management.
package procmgr

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/manifest"
)

const pidFileName = ".wt/pids.json"

// PidEntry tracks a running twin process.
type PidEntry struct {
	PID    int    `json:"pid"`
	Port   int    `json:"port"`
	Binary string `json:"binary"`
}

// PidMap maps twin names to their PID entries.
type PidMap map[string]PidEntry

// LoadPids reads the PID tracking file. Returns an empty map if the file doesn't exist.
func LoadPids() (PidMap, error) {
	data, err := os.ReadFile(pidFileName)
	if err != nil {
		if os.IsNotExist(err) {
			return PidMap{}, nil
		}
		return nil, err
	}
	var pids PidMap
	if err := json.Unmarshal(data, &pids); err != nil {
		return nil, err
	}
	return pids, nil
}

// SavePids writes the PID tracking file.
func SavePids(pids PidMap) error {
	if err := os.MkdirAll(filepath.Dir(pidFileName), 0o700); err != nil {
		return err
	}
	if err := tightenPerms(filepath.Dir(pidFileName), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pids, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(pidFileName, data, 0o600); err != nil {
		return err
	}
	return tightenPerms(pidFileName, 0o600)
}

// Start launches a twin binary as a background process with output redirected to a log file.
// Returns the process PID.
func Start(name string, twin manifest.Twin, logDir string, verbose bool) (int, error) {
	// Resolve binary to absolute path
	binary, err := filepath.Abs(twin.Binary)
	if err != nil {
		return 0, fmt.Errorf("resolving binary path: %w", err)
	}

	// Verify binary exists and is executable
	info, err := os.Stat(binary)
	if err != nil {
		return 0, fmt.Errorf("binary not found: %s", binary)
	}
	if info.IsDir() {
		return 0, fmt.Errorf("binary path is a directory: %s", binary)
	}

	// Build command arguments
	args := []string{
		"--port", strconv.Itoa(twin.Port),
	}
	if verbose {
		args = append(args, "--verbose")
	}
	if twin.Seed != "" {
		seedPath, err := filepath.Abs(twin.Seed)
		if err != nil {
			return 0, fmt.Errorf("resolving seed path: %w", err)
		}
		args = append(args, "--seed-file", seedPath)
	}

	//nolint:gosec // G204: launching a resolved twin binary is this package's purpose;
	// the path comes from the installer, not from untrusted input.
	cmd := exec.Command(binary, args...)

	// Inherit env and add twin-specific vars
	cmd.Env = os.Environ()
	for k, v := range twin.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Set up log file
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return 0, fmt.Errorf("creating log dir: %w", err)
	}
	if err := tightenPerms(logDir, 0o700); err != nil {
		return 0, fmt.Errorf("tightening log dir perms: %w", err)
	}
	logPath := filepath.Join(logDir, name+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, fmt.Errorf("creating log file: %w", err)
	}
	if err := tightenPerms(logPath, 0o600); err != nil {
		return 0, fmt.Errorf("tightening log file perms: %w", err)
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Start in its own process group on platforms that support it.
	setDetachedProcessAttrs(cmd)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, fmt.Errorf("starting %s: %w", name, err)
	}

	// Detach: let the process run independently
	go func() {
		cmd.Wait()
		logFile.Close()
	}()

	return cmd.Process.Pid, nil
}

// Stop sends SIGTERM to a twin process and waits for it to exit.
// Falls back to SIGKILL after 5 seconds.
func Stop(name string, entry PidEntry) error {
	if !IsRunning(entry.PID) {
		return nil
	}

	proc, err := os.FindProcess(entry.PID)
	if err != nil {
		return nil
	}

	// Graceful shutdown via SIGTERM
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return nil // already gone
	}

	// Poll for exit
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !IsRunning(entry.PID) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill after timeout
	proc.Signal(syscall.SIGKILL)
	time.Sleep(100 * time.Millisecond)
	return nil
}

// IsRunning checks if a process with the given PID is still alive.
func IsRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Signal 0 probes existence.
	return proc.Signal(syscall.Signal(0)) == nil
}

// RemovePidFile deletes the PID tracking file.
func RemovePidFile() {
	os.Remove(pidFileName)
}

// tightenPerms drops mode bits on a path that already exists. MkdirAll is a
// no-op on an existing directory and WriteFile/OpenFile do not chmod an
// existing file, so a path created by an older CLI version keeps its original,
// looser mode until something re-tightens it. Mirrors the follow-up chmod in
// internal/config. No-op on Windows, where Unix mode bits are inert.
func tightenPerms(path string, mode os.FileMode) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	//nolint:gosec // G302: callers pass 0700 for directories, which G302 reads
	// as a file mode; it is the tightest useful mode for a directory.
	return os.Chmod(path, mode)
}
