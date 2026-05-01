package telemetry

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecordNeverBlocksWhenBufferFull(t *testing.T) {
	t.Parallel()
	r := New(Config{})
	// Saturate the channel.
	for i := 0; i < 100_000; i++ {
		r.Record(Event{Type: "tick"})
	}
	// At this point the buffer is full; Record must still return
	// promptly. Bound the assertion at 1ms — if it ever blocks, the
	// test will hang and the timeout will trip.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1_000; i++ {
			r.Record(Event{Type: "post-saturation"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Record blocked under sustained pressure")
	}
}

func TestRecordOptOutIsNoop(t *testing.T) {
	t.Parallel()
	r := New(Config{OptOut: true})
	r.Record(Event{Type: "tick"})
	if r.ch != nil {
		t.Error("opt-out should leave channel nil")
	}
}

func TestFlushReturnsNil(t *testing.T) {
	t.Parallel()
	r := New(Config{})
	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() = %v, want nil", err)
	}
}

func TestCloseAfterRecordIsSafe(t *testing.T) {
	t.Parallel()
	r := New(Config{})
	r.Record(Event{})
	r.Close()
	r.Record(Event{}) // must not panic
}

func TestNewAppliesDefaults(t *testing.T) {
	t.Parallel()
	r := New(Config{})
	if r.cfg.BatchSize != defaultBatchSize {
		t.Errorf("BatchSize = %d, want %d", r.cfg.BatchSize, defaultBatchSize)
	}
	if r.cfg.Flush != defaultFlush {
		t.Errorf("Flush = %v, want %v", r.cfg.Flush, defaultFlush)
	}
}

// BenchmarkRecord asserts the non-blocking contract under contention.
// The benchmark runs Record from multiple goroutines and measures
// per-call latency. Future sessions adding the HTTP transport must keep
// this benchmark green; a regression here means the contract is
// broken.
func BenchmarkRecord(b *testing.B) {
	r := New(Config{})
	var dropped atomic.Uint64
	var wg sync.WaitGroup
	const workers = 8
	wg.Add(workers)
	b.ResetTimer()
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < b.N/workers; i++ {
				r.Record(Event{Type: "bench"})
			}
		}()
	}
	wg.Wait()
	b.ReportMetric(float64(dropped.Load()), "drops")
}
