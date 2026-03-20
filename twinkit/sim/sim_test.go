package sim

import (
	"testing"
	"time"
)

// --- Deterministic Rand ---

func TestRand_Deterministic(t *testing.T) {
	r1 := NewRand(42)
	r2 := NewRand(42)

	for i := 0; i < 100; i++ {
		if r1.Float64() != r2.Float64() {
			t.Fatalf("same seed should produce same sequence at step %d", i)
		}
	}
}

func TestRand_DifferentSeeds(t *testing.T) {
	r1 := NewRand(1)
	r2 := NewRand(2)

	same := true
	for i := 0; i < 10; i++ {
		if r1.Float64() != r2.Float64() {
			same = false
			break
		}
	}
	if same {
		t.Error("different seeds should produce different sequences")
	}
}

// --- Weighted Outcomes ---

func TestPickWeighted_SingleOutcome(t *testing.T) {
	r := DefaultRand()
	result := r.PickWeighted([]WeightedOutcome{{Value: "only", Weight: 1}})
	if result != "only" {
		t.Errorf("got %q, want only", result)
	}
}

func TestPickWeighted_Distribution(t *testing.T) {
	r := NewRand(123)
	outcomes := []WeightedOutcome{
		{Value: "A", Weight: 90},
		{Value: "B", Weight: 10},
	}

	counts := map[string]int{}
	for i := 0; i < 1000; i++ {
		counts[r.PickWeighted(outcomes)]++
	}

	// A should be ~900, B should be ~100 (with some variance).
	if counts["A"] < 800 || counts["A"] > 950 {
		t.Errorf("A count = %d, expected ~900", counts["A"])
	}
	if counts["B"] < 50 || counts["B"] > 200 {
		t.Errorf("B count = %d, expected ~100", counts["B"])
	}
}

func TestPickWeighted_Empty(t *testing.T) {
	r := DefaultRand()
	if r.PickWeighted(nil) != "" {
		t.Error("empty should return empty string")
	}
}

// --- Jitter ---

func TestJitter(t *testing.T) {
	r := DefaultRand()
	base := time.Second

	for i := 0; i < 100; i++ {
		result := r.Jitter(base, 0.2)
		if result < 800*time.Millisecond || result > 1200*time.Millisecond {
			t.Errorf("jitter ±20%% produced %v (out of 800ms-1200ms range)", result)
		}
	}
}

func TestJitter_ZeroFactor(t *testing.T) {
	r := DefaultRand()
	result := r.Jitter(time.Second, 0)
	if result != time.Second {
		t.Errorf("zero factor should return original: %v", result)
	}
}

// --- Normal Distribution ---

func TestNormalDuration(t *testing.T) {
	r := NewRand(99)
	mean := 500 * time.Millisecond
	stddev := 100 * time.Millisecond

	var sum time.Duration
	n := 1000
	for i := 0; i < n; i++ {
		d := r.NormalDuration(mean, stddev)
		sum += d
		if d < 0 {
			t.Error("should not produce negative durations")
		}
	}

	avg := sum / time.Duration(n)
	// Average should be close to mean (within 50ms for 1000 samples).
	if avg < 450*time.Millisecond || avg > 550*time.Millisecond {
		t.Errorf("average = %v, expected ~500ms", avg)
	}
}

// --- Exponential Distribution ---

func TestExponentialDuration(t *testing.T) {
	r := NewRand(77)
	mean := time.Second

	var sum time.Duration
	n := 1000
	for i := 0; i < n; i++ {
		d := r.ExponentialDuration(mean)
		sum += d
		if d < 0 {
			t.Error("should not produce negative durations")
		}
	}

	avg := sum / time.Duration(n)
	// Average of exponential distribution = mean (within 200ms for 1000 samples).
	if avg < 800*time.Millisecond || avg > 1200*time.Millisecond {
		t.Errorf("average = %v, expected ~1s", avg)
	}
}

// --- Bool ---

func TestBoolWithProbability(t *testing.T) {
	r := NewRand(55)
	trueCount := 0
	for i := 0; i < 1000; i++ {
		if r.BoolWithProbability(0.3) {
			trueCount++
		}
	}
	if trueCount < 200 || trueCount > 400 {
		t.Errorf("true count = %d, expected ~300", trueCount)
	}
}

// --- ID Generation ---

func TestIDGenerator_Sequential(t *testing.T) {
	gen := NewIDGenerator(DefaultRand())
	id1 := gen.Sequential("inv")
	id2 := gen.Sequential("inv")
	if id1 == id2 {
		t.Error("sequential IDs should be unique")
	}
	if id1 != "inv_000001" {
		t.Errorf("first ID = %q, want inv_000001", id1)
	}
}

func TestIDGenerator_UUID(t *testing.T) {
	gen := NewIDGenerator(NewRand(42))
	id := gen.UUID()
	if len(id) != 36 {
		t.Errorf("UUID length = %d, want 36", len(id))
	}
	// Should contain hyphens in correct positions.
	if id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		t.Errorf("UUID format invalid: %q", id)
	}
	// Version nibble should be 4.
	if id[14] != '4' {
		t.Errorf("UUID version = %c, want 4", id[14])
	}
}

func TestIDGenerator_UUID_Deterministic(t *testing.T) {
	gen1 := NewIDGenerator(NewRand(42))
	gen2 := NewIDGenerator(NewRand(42))
	if gen1.UUID() != gen2.UUID() {
		t.Error("same seed should produce same UUID")
	}
}

func TestIDGenerator_Prefixed(t *testing.T) {
	gen := NewIDGenerator(DefaultRand())
	id := gen.Prefixed("pi_", 24)
	if len(id) != 27 { // 3 (prefix) + 24 (random)
		t.Errorf("length = %d, want 27", len(id))
	}
	if id[:3] != "pi_" {
		t.Errorf("prefix = %q, want pi_", id[:3])
	}
}

func TestIDGenerator_Nanoid(t *testing.T) {
	gen := NewIDGenerator(DefaultRand())
	id := gen.Nanoid(21)
	if len(id) != 21 {
		t.Errorf("length = %d, want 21", len(id))
	}
}

func TestIDGenerator_Hex(t *testing.T) {
	gen := NewIDGenerator(DefaultRand())
	id := gen.Hex(16)
	if len(id) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("length = %d, want 32", len(id))
	}
}
