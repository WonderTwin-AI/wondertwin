package events

import "context"

// EventsHooks defines extension points for platform-specific behavior.
type EventsHooks interface {
	// ValidateEvent is called before ingesting an event. Return error to reject.
	ValidateEvent(ctx context.Context, event *Event) error

	// OnEventIngested is called after an event is appended to the stream.
	OnEventIngested(ctx context.Context, event *Event) error

	// OnIdentify is called after a user profile is created or updated via identify.
	OnIdentify(ctx context.Context, profile *UserProfile) error

	// OnMerge is called after an anonymous ID is merged into a known user.
	OnMerge(ctx context.Context, profile *UserProfile, anonymousID string) error
}

// NoOpEventsHooks provides default no-op implementations.
type NoOpEventsHooks struct{}

func (NoOpEventsHooks) ValidateEvent(_ context.Context, _ *Event) error           { return nil }
func (NoOpEventsHooks) OnEventIngested(_ context.Context, _ *Event) error         { return nil }
func (NoOpEventsHooks) OnIdentify(_ context.Context, _ *UserProfile) error        { return nil }
func (NoOpEventsHooks) OnMerge(_ context.Context, _ *UserProfile, _ string) error { return nil }
