package events

import (
	"context"
	"fmt"
	"sync"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// Engine manages the event stream, user profiles, and identity resolution.
type Engine struct {
	mu        sync.RWMutex
	events    []*Event                // append-only event stream
	profiles  map[string]*UserProfile // distinctID → profile
	anonMap   map[string]string       // anonymousID → distinctID (for identity merge)
	hooks     EventsHooks
	clock     *state.Clock
	idCounter int
}

// Option configures the Engine.
type Option func(*Engine)

// WithHooks sets the events hooks implementation.
func WithHooks(h EventsHooks) Option {
	return func(e *Engine) { e.hooks = h }
}

// WithClock sets the clock for timestamps.
func WithClock(c *state.Clock) Option {
	return func(e *Engine) { e.clock = c }
}

// NewEngine creates a new events engine with the given options.
func NewEngine(opts ...Option) *Engine {
	e := &Engine{
		events:   make([]*Event, 0),
		profiles: make(map[string]*UserProfile),
		anonMap:  make(map[string]string),
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.clock == nil {
		e.clock = state.NewClock()
	}
	return e
}

// Track ingests an event into the stream. The event is appended to the
// append-only stream and associated with the user's distinct_id.
func (e *Engine) Track(ctx context.Context, event *Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if event.EventName == "" {
		return fmt.Errorf("event name is required")
	}
	if event.DistinctID == "" {
		return fmt.Errorf("distinct_id is required")
	}

	// Resolve anonymous ID if mapped.
	if resolved, ok := e.anonMap[event.DistinctID]; ok {
		event.DistinctID = resolved
	}

	// Assign ID and timestamp.
	if event.ID == "" {
		e.idCounter++
		event.ID = fmt.Sprintf("evt_%06d", e.idCounter)
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = e.clock.Now()
	}

	// Initialize properties if nil.
	if event.Properties == nil {
		event.Properties = make(map[string]any)
	}

	// Validate via hooks.
	if e.hooks != nil {
		if err := e.hooks.ValidateEvent(ctx, event); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}

	// Append to stream.
	e.events = append(e.events, event)

	// Ensure profile exists.
	e.ensureProfileLocked(event.DistinctID)

	// Notify hooks.
	if e.hooks != nil {
		if err := e.hooks.OnEventIngested(ctx, event); err != nil {
			return fmt.Errorf("hook OnEventIngested: %w", err)
		}
	}

	return nil
}

// Identify creates or updates a user profile with the given properties.
// If AnonymousID is provided, it merges the anonymous identity into the
// known user identified by DistinctID.
func (e *Engine) Identify(ctx context.Context, call *IdentifyCall) (*UserProfile, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if call.DistinctID == "" {
		return nil, fmt.Errorf("distinct_id is required")
	}

	now := e.clock.Now()
	profile := e.ensureProfileLocked(call.DistinctID)

	// Merge properties.
	for k, v := range call.Properties {
		profile.Properties[k] = v
	}
	profile.UpdatedAt = now

	// Handle anonymous → known merge.
	if call.AnonymousID != "" && call.AnonymousID != call.DistinctID {
		e.anonMap[call.AnonymousID] = call.DistinctID

		// Track the anonymous ID on the profile.
		found := false
		for _, aid := range profile.AnonymousIDs {
			if aid == call.AnonymousID {
				found = true
				break
			}
		}
		if !found {
			profile.AnonymousIDs = append(profile.AnonymousIDs, call.AnonymousID)
		}

		// Re-attribute events from anonymous ID to known ID.
		for _, evt := range e.events {
			if evt.DistinctID == call.AnonymousID {
				evt.DistinctID = call.DistinctID
			}
		}

		// Merge anonymous profile if it exists.
		if anonProfile, ok := e.profiles[call.AnonymousID]; ok {
			for k, v := range anonProfile.Properties {
				if _, exists := profile.Properties[k]; !exists {
					profile.Properties[k] = v
				}
			}
			delete(e.profiles, call.AnonymousID)
		}

		// Notify hooks about merge.
		if e.hooks != nil {
			if err := e.hooks.OnMerge(ctx, profile, call.AnonymousID); err != nil {
				return nil, fmt.Errorf("hook OnMerge: %w", err)
			}
		}
	}

	// Notify hooks about identify.
	if e.hooks != nil {
		if err := e.hooks.OnIdentify(ctx, profile); err != nil {
			return nil, fmt.Errorf("hook OnIdentify: %w", err)
		}
	}

	return profile, nil
}

// GetProfile returns a user profile by distinct ID.
func (e *Engine) GetProfile(distinctID string) (*UserProfile, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Resolve anonymous ID if mapped.
	if resolved, ok := e.anonMap[distinctID]; ok {
		distinctID = resolved
	}

	profile, ok := e.profiles[distinctID]
	if !ok {
		return nil, fmt.Errorf("profile %s not found", distinctID)
	}
	return profile, nil
}

// ListProfiles returns all user profiles.
func (e *Engine) ListProfiles() []*UserProfile {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]*UserProfile, 0, len(e.profiles))
	for _, p := range e.profiles {
		result = append(result, p)
	}
	return result
}

// GetEvents returns all events, optionally filtered by distinct_id.
func (e *Engine) GetEvents(distinctID string) []*Event {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if distinctID == "" {
		result := make([]*Event, len(e.events))
		copy(result, e.events)
		return result
	}
	var result []*Event
	for _, evt := range e.events {
		if evt.DistinctID == distinctID {
			result = append(result, evt)
		}
	}
	return result
}

// GetEventsByName returns all events with the given name.
func (e *Engine) GetEventsByName(eventName string) []*Event {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var result []*Event
	for _, evt := range e.events {
		if evt.EventName == eventName {
			result = append(result, evt)
		}
	}
	return result
}

// EventCount returns the total number of events in the stream.
func (e *Engine) EventCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.events)
}

// ensureProfileLocked creates a profile if it doesn't exist. Caller must hold lock.
func (e *Engine) ensureProfileLocked(distinctID string) *UserProfile {
	if p, ok := e.profiles[distinctID]; ok {
		return p
	}
	now := e.clock.Now()
	p := &UserProfile{
		DistinctID: distinctID,
		Properties: make(map[string]any),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	e.profiles[distinctID] = p
	return p
}
