package twincore

import (
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/cimode"
)

func TestNewExposesRandAndIDs(t *testing.T) {
	t.Parallel()
	twin := New(&Config{Name: "twin-test"})
	if twin.Rand == nil {
		t.Fatal("Twin.Rand must be non-nil after New")
	}
	if twin.IDs == nil {
		t.Fatal("Twin.IDs must be non-nil after New")
	}
	if got := twin.IDs.Sequential("inv"); got == "" {
		t.Error("Twin.IDs.Sequential returned empty id")
	}
}

func TestNewSetsModeFromEnv(t *testing.T) {
	t.Parallel()
	twin := New(&Config{Name: "twin-test"})
	// Detect from the real environment may be local or CI depending on
	// where the test runs; both are valid. Just confirm parity with a
	// fresh Detect call.
	want := cimode.Detect().Mode
	if twin.Mode != want {
		t.Errorf("Twin.Mode = %v, want %v", twin.Mode, want)
	}
}

func TestSetRunID(t *testing.T) {
	t.Parallel()
	twin := New(&Config{Name: "twin-test"})
	if twin.RunID != "" {
		t.Errorf("RunID should default to empty, got %q", twin.RunID)
	}
	twin.SetRunID("run-1")
	if twin.RunID != "run-1" {
		t.Errorf("RunID after SetRunID = %q, want run-1", twin.RunID)
	}
	twin.SetRunID("")
	if twin.RunID != "" {
		t.Errorf("RunID after clear = %q, want empty", twin.RunID)
	}
}

func TestSeedFromEnvFallback(t *testing.T) {
	t.Setenv(envSeed, "")
	if r := seedFromEnv(); r == nil {
		t.Fatal("seedFromEnv() returned nil")
	}
	t.Setenv(envSeed, "garbage")
	if r := seedFromEnv(); r == nil {
		t.Fatal("seedFromEnv() returned nil for invalid input")
	}
	t.Setenv(envSeed, "12345")
	if r := seedFromEnv(); r == nil {
		t.Fatal("seedFromEnv() returned nil for valid input")
	}
}
