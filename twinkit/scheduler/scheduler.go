// Package scheduler provides time-triggered callback execution for WonderTwin
// twins. Callbacks fire when the simulated clock advances past their scheduled
// time. Supports one-shot and recurring schedules.
package scheduler

import (
	"fmt"
	"sync"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// Task is a scheduled callback.
type Task struct {
	ID       string
	FireAt   time.Time
	Interval time.Duration // 0 = one-shot, >0 = recurring
	Callback func()
	fired    bool
}

// Scheduler manages scheduled tasks and fires them when the clock advances.
type Scheduler struct {
	mu        sync.Mutex
	tasks     []*Task
	clock     *state.Clock
	lastCheck time.Time
	idCounter int
}

// New creates a new Scheduler using the given clock.
func New(clock *state.Clock) *Scheduler {
	return &Scheduler{
		clock:     clock,
		tasks:     make([]*Task, 0),
		lastCheck: clock.Now(),
	}
}

// Schedule registers a one-shot callback at a specific time.
// Returns the task ID.
func (s *Scheduler) Schedule(at time.Time, callback func()) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idCounter++
	id := fmt.Sprintf("task_%06d", s.idCounter)
	s.tasks = append(s.tasks, &Task{
		ID:       id,
		FireAt:   at,
		Callback: callback,
	})
	return id
}

// ScheduleAfter registers a one-shot callback after a delay from now.
func (s *Scheduler) ScheduleAfter(delay time.Duration, callback func()) string {
	return s.Schedule(s.clock.Now().Add(delay), callback)
}

// ScheduleRecurring registers a recurring callback at the given interval.
// First fires at now + interval.
func (s *Scheduler) ScheduleRecurring(interval time.Duration, callback func()) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idCounter++
	id := fmt.Sprintf("task_%06d", s.idCounter)
	s.tasks = append(s.tasks, &Task{
		ID:       id,
		FireAt:   s.clock.Now().Add(interval),
		Interval: interval,
		Callback: callback,
	})
	return id
}

// Cancel removes a scheduled task by ID. Returns true if found.
func (s *Scheduler) Cancel(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.tasks {
		if t.ID == id {
			s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
			return true
		}
	}
	return false
}

// Tick checks all pending tasks and fires those whose time has come.
// Returns the number of tasks fired.
// Call this after advancing the clock or on a regular polling interval.
func (s *Scheduler) Tick() int {
	s.mu.Lock()
	now := s.clock.Now()

	// Collect tasks to fire.
	var toFire []func()
	var keep []*Task

	for _, t := range s.tasks {
		if now.Before(t.FireAt) {
			keep = append(keep, t)
			continue
		}

		toFire = append(toFire, t.Callback)

		if t.Interval > 0 {
			// Recurring: reschedule.
			keep = append(keep, &Task{
				ID:       t.ID,
				FireAt:   t.FireAt.Add(t.Interval),
				Interval: t.Interval,
				Callback: t.Callback,
			})
		}
		// One-shot tasks are not kept.
	}

	s.tasks = keep
	s.lastCheck = now
	s.mu.Unlock()

	// Fire callbacks outside the lock to avoid deadlocks.
	for _, cb := range toFire {
		cb()
	}

	return len(toFire)
}

// PendingCount returns the number of pending (unfired) tasks.
func (s *Scheduler) PendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tasks)
}

// NextFireTime returns the earliest scheduled fire time, or zero if no tasks.
func (s *Scheduler) NextFireTime() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tasks) == 0 {
		return time.Time{}
	}
	earliest := s.tasks[0].FireAt
	for _, t := range s.tasks[1:] {
		if t.FireAt.Before(earliest) {
			earliest = t.FireAt
		}
	}
	return earliest
}

// Reset clears all scheduled tasks.
func (s *Scheduler) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = make([]*Task, 0)
}
