// Package scripts_test covers the repo's CI gate scripts by running them.
//
// check-engine-boundary is the durable fix for CTO-46, so the thing that
// must not rot is the gate's own behaviour. These cases execute the real
// script against a throwaway git index rather than reading its source,
// because a gate is only worth what it exits with.
//
// What they pin: that a nested path is not collapsed to its top-level
// component, that the kit root is covered, that testdata is not exempt,
// and that an empty read fails instead of reporting clean.
//
// What they do not pin is the allowlist's contents. They probe four
// names, so an appended entry is caught only where it collides with one
// of those. That is by design rather than a gap to close: adding a name
// is the gate's deliberate escape hatch, and what forces the decision
// into review is the commit that adds it, not this suite.
package scripts_test

import (
	"errors"
	"maps"
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

const kitGoMod = "module github.com/wondertwin-ai/wondertwin/twinkit\n\ngo 1.26.6\n"

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

// newRepo builds a throwaway repo holding the given files and a copy of the
// gate, then stages it. The gate resolves twinkit/ relative to its own
// parent's parent, so the copy has to sit at <root>/scripts/.
func newRepo(t *testing.T, files map[string]string) string {
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

	for path, content := range files {
		write(t, filepath.Join(root, filepath.FromSlash(path)), content)
	}

	git(t, root, "init", "-q")
	git(t, root, "add", "-A")
	return root
}

// newKit builds a repo whose kit is clean and fully allowlisted, plus extra.
func newKit(t *testing.T, extra map[string]string) string {
	t.Helper()
	files := map[string]string{"twinkit/go.mod": kitGoMod}
	for _, pkg := range allowlistedKit {
		files["twinkit/"+pkg+"/doc.go"] = "package " + pkg + "\n"
	}
	maps.Copy(files, extra)
	return newRepo(t, files)
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := runGate(t, newKit(t, tc.extra)); got != tc.want {
				t.Errorf("gate exit = %d, want %d", got, tc.want)
			}
		})
	}
}

// A kit that yields no Go packages means the gate did not read what it
// claims to have read, so it must not report clean. Without this floor the
// gate prints "no domain engines in the public kit" on zero evidence.
func TestEngineBoundaryFailsOnAnEmptyRead(t *testing.T) {
	t.Run("kit present but holding no Go package", func(t *testing.T) {
		if got := runGate(t, newRepo(t, map[string]string{"twinkit/go.mod": kitGoMod})); got != 1 {
			t.Errorf("gate exit = %d, want 1", got)
		}
	})

	t.Run("no kit at all", func(t *testing.T) {
		if got := runGate(t, newRepo(t, map[string]string{"go.mod": kitGoMod})); got != 1 {
			t.Errorf("gate exit = %d, want 1", got)
		}
	})
}
