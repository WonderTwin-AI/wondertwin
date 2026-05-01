package sim

import "testing"

func TestReseedResetsSequence(t *testing.T) {
	t.Parallel()
	a := NewRand(42)
	first := []int{a.Intn(1_000_000), a.Intn(1_000_000), a.Intn(1_000_000)}

	a.Reseed(42)
	second := []int{a.Intn(1_000_000), a.Intn(1_000_000), a.Intn(1_000_000)}

	for i := range first {
		if first[i] != second[i] {
			t.Errorf("seq[%d] differs after Reseed: %d vs %d", i, first[i], second[i])
		}
	}
}

func TestReseedDifferentSeedDifferentSequence(t *testing.T) {
	t.Parallel()
	r := NewRand(1)
	r.Reseed(2)
	first := r.Intn(1_000_000)

	r.Reseed(3)
	second := r.Intn(1_000_000)
	if first == second {
		t.Errorf("different seeds produced identical first value: %d", first)
	}
}
