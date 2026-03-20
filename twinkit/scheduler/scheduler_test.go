package scheduler

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
)

func TestSchedule_OneShot(t *testing.T) {
	clock := state.NewClock()
	s := New(clock)

	var fired int32
	s.ScheduleAfter(time.Minute, func() {
		atomic.AddInt32(&fired, 1)
	})

	// Not yet.
	n := s.Tick()
	if n != 0 {
		t.Errorf("fired %d before time", n)
	}

	// Advance past the scheduled time.
	clock.Advance(2 * time.Minute)
	n = s.Tick()
	if n != 1 {
		t.Errorf("expected 1 fired, got %d", n)
	}
	if atomic.LoadInt32(&fired) != 1 {
		t.Error("callback not called")
	}

	// Should not fire again.
	clock.Advance(time.Minute)
	n = s.Tick()
	if n != 0 {
		t.Errorf("one-shot fired again: %d", n)
	}
}

func TestSchedule_Recurring(t *testing.T) {
	clock := state.NewClock()
	s := New(clock)

	var count int32
	s.ScheduleRecurring(time.Hour, func() {
		atomic.AddInt32(&count, 1)
	})

	// Advance 3.5 hours — should fire 3 times.
	for i := 0; i < 7; i++ {
		clock.Advance(30 * time.Minute)
		s.Tick()
	}

	if atomic.LoadInt32(&count) != 3 {
		t.Errorf("count = %d, want 3", atomic.LoadInt32(&count))
	}
}

func TestSchedule_Cancel(t *testing.T) {
	clock := state.NewClock()
	s := New(clock)

	var fired bool
	id := s.ScheduleAfter(time.Minute, func() { fired = true })

	if !s.Cancel(id) {
		t.Error("cancel should return true")
	}

	clock.Advance(2 * time.Minute)
	s.Tick()

	if fired {
		t.Error("cancelled task should not fire")
	}
}

func TestSchedule_CancelNonexistent(t *testing.T) {
	clock := state.NewClock()
	s := New(clock)
	if s.Cancel("nonexistent") {
		t.Error("should return false for unknown ID")
	}
}

func TestSchedule_Multiple(t *testing.T) {
	clock := state.NewClock()
	s := New(clock)

	var order []string
	s.ScheduleAfter(2*time.Minute, func() { order = append(order, "B") })
	s.ScheduleAfter(1*time.Minute, func() { order = append(order, "A") })
	s.ScheduleAfter(3*time.Minute, func() { order = append(order, "C") })

	clock.Advance(4 * time.Minute)
	s.Tick()

	if len(order) != 3 {
		t.Fatalf("expected 3, got %d", len(order))
	}
}

func TestPendingCount(t *testing.T) {
	clock := state.NewClock()
	s := New(clock)

	s.ScheduleAfter(time.Minute, func() {})
	s.ScheduleAfter(time.Minute, func() {})

	if s.PendingCount() != 2 {
		t.Errorf("pending = %d, want 2", s.PendingCount())
	}

	clock.Advance(2 * time.Minute)
	s.Tick()

	if s.PendingCount() != 0 {
		t.Errorf("pending = %d, want 0", s.PendingCount())
	}
}

func TestNextFireTime(t *testing.T) {
	clock := state.NewClock()
	s := New(clock)

	if !s.NextFireTime().IsZero() {
		t.Error("should be zero with no tasks")
	}

	now := clock.Now()
	s.Schedule(now.Add(5*time.Minute), func() {})
	s.Schedule(now.Add(2*time.Minute), func() {})

	next := s.NextFireTime()
	expected := now.Add(2 * time.Minute)
	if !next.Equal(expected) {
		t.Errorf("next = %v, want %v", next, expected)
	}
}

func TestReset(t *testing.T) {
	clock := state.NewClock()
	s := New(clock)

	s.ScheduleAfter(time.Minute, func() {})
	s.Reset()

	if s.PendingCount() != 0 {
		t.Error("reset should clear all tasks")
	}
}

func TestSchedule_AtSpecificTime(t *testing.T) {
	clock := state.NewClock()
	s := New(clock)

	target := clock.Now().Add(10 * time.Minute)
	var fired bool
	s.Schedule(target, func() { fired = true })

	clock.Advance(9 * time.Minute)
	s.Tick()
	if fired {
		t.Error("should not fire before target time")
	}

	clock.Advance(2 * time.Minute)
	s.Tick()
	if !fired {
		t.Error("should fire after target time")
	}
}
