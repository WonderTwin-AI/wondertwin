// Package cimode detects the execution environment of a twin (local
// development vs. CI) and the specific CI platform when applicable.
//
// Detection is intentionally low-cost: it inspects environment variables
// only. Platform-specific run identifiers and organization hints are
// surfaced for downstream telemetry consumers, but cimode itself never
// performs network I/O.
//
// License validation is deliberately not part of this package; that
// concern is layered on top in a later iteration.
package cimode

import (
	"os"
	"strings"
)

// Mode classifies the runtime environment of a twin.
type Mode int

const (
	// ModeLocal indicates a local developer machine — no CI signals present.
	ModeLocal Mode = iota
	// ModeCI indicates execution inside a continuous-integration runner.
	ModeCI
)

// String returns a stable lower-case name for the mode, suitable for
// telemetry payloads.
func (m Mode) String() string {
	switch m {
	case ModeCI:
		return "ci"
	default:
		return "local"
	}
}

// Detection captures the result of inspecting the process environment.
//
// Platform is one of:
//   - "github-actions"
//   - "circleci"
//   - "buildkite"
//   - "gitlab"
//   - "generic"  (CI=true with no recognized platform variable)
//   - ""         (no CI signals detected)
//
// RunID is the platform-native run identifier when present (e.g.
// GITHUB_RUN_ID). OrgHint is the platform's organization or namespace
// identifier (e.g. GITHUB_REPOSITORY_OWNER) and is intended for
// downstream hashing into telemetry payloads — never logged verbatim.
type Detection struct {
	Mode     Mode
	Platform string
	RunID    string
	OrgHint  string
}

// Detect reads the current process environment and returns a Detection.
// It is safe to call multiple times and never returns an error: an
// unrecognized environment yields Detection{Mode: ModeLocal}.
func Detect() Detection {
	return DetectFromEnv(os.Environ())
}

// DetectFromEnv is the testable variant of Detect. It accepts an
// os.Environ()-style slice of "KEY=VALUE" entries and returns a
// Detection without consulting the real process environment.
func DetectFromEnv(env []string) Detection {
	lookup := make(map[string]string, len(env))
	for _, kv := range env {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			continue
		}
		lookup[kv[:i]] = kv[i+1:]
	}

	get := func(k string) string { return lookup[k] }
	truthy := func(k string) bool {
		v := strings.ToLower(strings.TrimSpace(get(k)))
		return v == "true" || v == "1" || v == "yes"
	}

	switch {
	case truthy("GITHUB_ACTIONS"):
		return Detection{
			Mode:     ModeCI,
			Platform: "github-actions",
			RunID:    get("GITHUB_RUN_ID"),
			OrgHint:  get("GITHUB_REPOSITORY_OWNER"),
		}
	case truthy("CIRCLECI"):
		return Detection{
			Mode:     ModeCI,
			Platform: "circleci",
			RunID:    get("CIRCLE_WORKFLOW_ID"),
			OrgHint:  get("CIRCLE_PROJECT_USERNAME"),
		}
	case truthy("BUILDKITE"):
		return Detection{
			Mode:     ModeCI,
			Platform: "buildkite",
			RunID:    get("BUILDKITE_BUILD_ID"),
			OrgHint:  get("BUILDKITE_ORGANIZATION_SLUG"),
		}
	case truthy("GITLAB_CI"):
		return Detection{
			Mode:     ModeCI,
			Platform: "gitlab",
			RunID:    get("CI_PIPELINE_ID"),
			OrgHint:  get("CI_PROJECT_NAMESPACE"),
		}
	case truthy("CI"):
		return Detection{Mode: ModeCI, Platform: "generic"}
	default:
		return Detection{Mode: ModeLocal}
	}
}
