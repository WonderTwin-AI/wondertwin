package events

import (
	"context"

	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// WithTelemetry returns an Option that wraps the engine's hooks to automatically
// emit DomainEvent telemetry when hooks fire.
func WithTelemetry(emitter *telemetry.Emitter) Option {
	return func(e *Engine) {
		inner := e.hooks
		if inner == nil {
			inner = NoOpEventsHooks{}
		}
		e.hooks = &telemetryBridge{emitter: emitter, inner: inner}
	}
}

type telemetryBridge struct {
	emitter *telemetry.Emitter
	inner   EventsHooks
}

func (b *telemetryBridge) ValidateEvent(ctx context.Context, event *Event) error {
	err := b.inner.ValidateEvent(ctx, event)
	errType := ""
	if err != nil {
		errType = err.Error()
	}
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		telemetry.DomainEvent{
			Engine:     "events",
			Hook:       "ValidateEvent",
			Resource:   "events",
			Action:     "validate",
			EntityType: event.EventName,
			EntityID:   event.ID,
			Context:    ValidateEventContext(event.EventName, err == nil, errType),
		},
	)
	return err
}

func (b *telemetryBridge) OnEventIngested(ctx context.Context, event *Event) error {
	propCount := 0
	if event.Properties != nil {
		propCount = len(event.Properties)
	}
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		telemetry.DomainEvent{
			Engine:     "events",
			Hook:       "OnEventIngested",
			Resource:   "events",
			Action:     "create",
			EntityType: event.EventName,
			EntityID:   event.ID,
			Context:    EventIngestedContext(event.EventName, propCount > 0, propCount),
		},
	)
	return b.inner.OnEventIngested(ctx, event)
}

func (b *telemetryBridge) OnIdentify(ctx context.Context, profile *UserProfile) error {
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		telemetry.DomainEvent{
			Engine:     "events",
			Hook:       "OnIdentify",
			Resource:   "profiles",
			Action:     "update",
			EntityType: "user_profile",
			EntityID:   profile.DistinctID,
			Context:    IdentifyContext(len(profile.AnonymousIDs) > 0, len(profile.Properties)),
		},
	)
	return b.inner.OnIdentify(ctx, profile)
}

func (b *telemetryBridge) OnMerge(ctx context.Context, profile *UserProfile, anonymousID string) error {
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		telemetry.DomainEvent{
			Engine:     "events",
			Hook:       "OnMerge",
			Resource:   "profiles",
			Action:     "update",
			EntityType: "user_profile",
			EntityID:   profile.DistinctID,
			Context:    MergeContext(len(profile.AnonymousIDs)),
		},
	)
	return b.inner.OnMerge(ctx, profile, anonymousID)
}
