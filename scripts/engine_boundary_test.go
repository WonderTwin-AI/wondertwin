// Package scripts_test covers the repo's CI gate scripts by running them.
//
// check-engine-boundary is the durable fix for CTO-46, so the thing that
// must not rot is the gate's own behaviour: a refactor that collapsed a
// nested path to its top-level component, or an extra name appended to
// ALLOWED_PACKAGES, would reopen the hole while CI still printed green.
// These cases execute the real script against a throwaway git index
// rather than reading its source, because a gate is only worth what it
// exits with.
package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// allowlistedKit mirrors ALLOWED_PACKAGES. A clean kit of these must pass;
// anything else must not.
var allowlistedKit = []string{
	"admin", "twincore", "state", "sim", "webhook", "testutil",
	"pagination", "identity", "graphql", "grpc", "soap", "ws",
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	// Isolate from the developer's global config: a global excludesfile
	// would hide the very files under test.
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newKit builds a throwaway repo holding a clean, fully allowlisted kit and
// a copy of the gate, then stages it. The gate resolves twinkit/ relative to
// its own parent's parent, so the copy has to sit at <root>/scripts/.
func newKit(t *testing.T, extra map[string]string) string {
	t.Helper()
	root := t.TempDir()

	src, err := os.ReadFile("check-engine-boundary")
	if err != nil {
		t.Fatal(err)
	}
	gate := filepath.Join(root, "scripts", "check-engine-boundary")
	write(t, gate, string(src))
	if err := os.Chmod(gate, 0o755); err != nil {
		t.Fatal(err)
	}

	write(t, filepath.Join(root, "twinkit", "go.mod"),
		"module github.com/wondertwin-ai/wondertwin/twinkit\n\ngo 1.26.6\n")
	for _, pkg := range allowlistedKit {
		write(t, filepath.Join(root, "twinkit", pkg, "doc.go"), "package "+pkg+"\n")
	}
	for path, content := range extra {
		write(t, filepath.Join(root, filepath.FromSlash(path)), content)
	}

	git(t, root, "init", "-q")
	git(t, root, "add", "-A")
	return root
}

func runGate(t *testing.T, root string) int {
	t.Helper()
	cmd := exec.Command(filepath.Join(root, "scripts", "check-engine-boundary"))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	t.Logf("gate output:\n%s", out)

	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		return exit.ExitCode()
	default:
		t.Fatalf("running the gate: %v", err)
		return -1
	}
}

func TestEngineBoundary(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]string
		want  int
	}{
		{
			name: "a clean allowlisted kit passes",
			want: 0,
		},
		{
			name:  "an engine re-added as its own package fails",
			extra: map[string]string{"twinkit/ledger/engine.go": "package ledger\n"},
			want:  1,
		},
		{
			name:  "an engine nested under an allowlisted package fails",
			extra: map[string]string{"twinkit/state/journal/journal.go": "package journal\n"},
			want:  1,
		},
		{
			name:  "a package under a name on no list fails",
			extra: map[string]string{"twinkit/billing/engine.go": "package billing\n"},
			want:  1,
		},
		{
			name:  "a go file at the kit root fails",
			extra: map[string]string{"twinkit/ledger.go": "package twinkit\n"},
			want:  1,
		},
		{
			name:  "a testdata directory is not exempt",
			extra: map[string]string{"twinkit/testutil/testdata/fixture.go": "package testdata\n"},
			want:  1,
		},
		{
			// The documented residual gap, asserted so it stays a known
			// gap rather than quietly widening: the gate is a path rule,
			// so it is package-granular, not file-granular.
			name:  "a go file inside an allowlisted package passes",
			extra: map[string]string{"twinkit/state/journal.go": "package state\n"},
			want:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := runGate(t, newKit(t, tc.extra)); got != tc.want {
				t.Errorf("gate exit = %d, want %d", got, tc.want)
			}
		})
	}
}

// An unstaged engine is invisible to the gate, which is the cost the
// docstring names: it reads the index, not the worktree. Asserted so the
// claim and the behaviour cannot drift apart again.
func TestEngineBoundaryOnlySeesTheIndex(t *testing.T) {
	root := newKit(t, nil)
	write(t, filepath.Join(root, "twinkit", "ledger", "engine.go"), "package ledger\n")

	if got := runGate(t, root); got != 0 {
		t.Errorf("unstaged engine: gate exit = %d, want 0", got)
	}
	git(t, root, "add", "-A")
	if got := runGate(t, root); got != 1 {
		t.Errorf("staged engine: gate exit = %d, want 1", got)
	}
}
