package workspace

import (
	"context"

	"github.com/wondertwin-ai/wondertwin/twinkit/domainevents"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// WithTelemetry returns an Option that wraps the engine's hooks to automatically
// emit DomainEvent telemetry when hooks fire.
func WithTelemetry(emitter domainevents.Emitter) Option {
	return func(e *Engine) {
		inner := e.hooks
		if inner == nil {
			inner = NoOpWorkspaceHooks{}
		}
		e.hooks = &telemetryBridge{emitter: emitter, inner: inner}
	}
}

type telemetryBridge struct {
	emitter domainevents.Emitter
	inner   WorkspaceHooks
}

func (b *telemetryBridge) ValidateEntity(ctx context.Context, entity *Entity) error {
	return b.inner.ValidateEntity(ctx, entity)
}

func (b *telemetryBridge) OnEntityCreated(ctx context.Context, entity *Entity) error {
	propCount := 0
	if entity.Properties != nil {
		propCount = len(entity.Properties)
	}
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "workspace",
			Hook:       "OnEntityCreated",
			Resource:   entity.Type,
			Action:     "create",
			EntityType: entity.Type,
			EntityID:   entity.ID,
			Context:    EntityCreatedContext(entity.Type, entity.ParentID != "", propCount),
		},
	)
	return b.inner.OnEntityCreated(ctx, entity)
}

func (b *telemetryBridge) OnEntityUpdated(ctx context.Context, entity *Entity, changes map[string]any) error {
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "workspace",
			Hook:       "OnEntityUpdated",
			Resource:   entity.Type,
			Action:     "update",
			EntityType: entity.Type,
			EntityID:   entity.ID,
			Context:    EntityUpdatedContext(entity.Type, len(changes)),
		},
	)
	return b.inner.OnEntityUpdated(ctx, entity, changes)
}

func (b *telemetryBridge) OnEntityDeleted(ctx context.Context, entityType, entityID string) error {
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "workspace",
			Hook:       "OnEntityDeleted",
			Resource:   entityType,
			Action:     "delete",
			EntityType: entityType,
			EntityID:   entityID,
		},
	)
	return b.inner.OnEntityDeleted(ctx, entityType, entityID)
}

func (b *telemetryBridge) OnStatusTransition(ctx context.Context, entity *Entity, from, to string) error {
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "workspace",
			Hook:       "OnStatusTransition",
			Resource:   entity.Type,
			Action:     "transition",
			EntityType: entity.Type,
			EntityID:   entity.ID,
			StateFrom:  from,
			StateTo:    to,
			Context:    StatusTransitionContext(from, to, entity.Type),
		},
	)
	return b.inner.OnStatusTransition(ctx, entity, from, to)
}

func (b *telemetryBridge) OnCommentAdded(ctx context.Context, entity *Entity, comment *Comment) error {
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "workspace",
			Hook:       "OnCommentAdded",
			Resource:   entity.Type,
			Action:     "create",
			EntityType: "comment",
			EntityID:   comment.ID,
			Context:    CommentAddedContext(entity.Type, false),
		},
	)
	return b.inner.OnCommentAdded(ctx, entity, comment)
}

func (b *telemetryBridge) OnMemberAdded(ctx context.Context, containerID, userID, role string) error {
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "workspace",
			Hook:       "OnMemberAdded",
			Resource:   "members",
			Action:     "create",
			EntityType: "member",
			EntityID:   userID,
			Context:    MemberAddedContext(role),
		},
	)
	return b.inner.OnMemberAdded(ctx, containerID, userID, role)
}

func (b *telemetryBridge) OnMemberRemoved(ctx context.Context, containerID, userID string) error {
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "workspace",
			Hook:       "OnMemberRemoved",
			Resource:   "members",
			Action:     "delete",
			EntityType: "member",
			EntityID:   userID,
		},
	)
	return b.inner.OnMemberRemoved(ctx, containerID, userID)
}

func (b *telemetryBridge) FormatEvent(event *MutationEvent) any {
	return b.inner.FormatEvent(event)
}
