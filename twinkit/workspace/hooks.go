package workspace

import "context"

// WorkspaceHooks defines extension points for platform-specific behavior.
// The engine calls these at behavioral inflection points. Implementations
// must not call back into the engine.
type WorkspaceHooks interface {
	// ValidateEntity is called before create and update. Return error to reject.
	ValidateEntity(ctx context.Context, entity *Entity) error

	// OnEntityCreated is called after an entity is created and stored.
	OnEntityCreated(ctx context.Context, entity *Entity) error

	// OnEntityUpdated is called after an entity is updated.
	OnEntityUpdated(ctx context.Context, entity *Entity, changes map[string]any) error

	// OnEntityDeleted is called after an entity is deleted.
	OnEntityDeleted(ctx context.Context, entityType, entityID string) error

	// OnStatusTransition is called after a workflow status transition.
	OnStatusTransition(ctx context.Context, entity *Entity, from, to string) error

	// OnCommentAdded is called after a comment is added to an entity.
	OnCommentAdded(ctx context.Context, entity *Entity, comment *Comment) error

	// OnMemberAdded is called after a member is added to a container.
	OnMemberAdded(ctx context.Context, containerID, userID, role string) error

	// OnMemberRemoved is called after a member is removed from a container.
	OnMemberRemoved(ctx context.Context, containerID, userID string) error

	// FormatEvent converts a MutationEvent into a platform-specific webhook payload.
	FormatEvent(event *MutationEvent) any
}

// NoOpWorkspaceHooks provides default no-op implementations.
// Embed this in the twin's hook implementation to avoid implementing unused hooks.
type NoOpWorkspaceHooks struct{}

func (NoOpWorkspaceHooks) ValidateEntity(_ context.Context, _ *Entity) error  { return nil }
func (NoOpWorkspaceHooks) OnEntityCreated(_ context.Context, _ *Entity) error { return nil }
func (NoOpWorkspaceHooks) OnEntityUpdated(_ context.Context, _ *Entity, _ map[string]any) error {
	return nil
}
func (NoOpWorkspaceHooks) OnEntityDeleted(_ context.Context, _, _ string) error { return nil }
func (NoOpWorkspaceHooks) OnStatusTransition(_ context.Context, _ *Entity, _, _ string) error {
	return nil
}
func (NoOpWorkspaceHooks) OnCommentAdded(_ context.Context, _ *Entity, _ *Comment) error {
	return nil
}
func (NoOpWorkspaceHooks) OnMemberAdded(_ context.Context, _, _, _ string) error { return nil }
func (NoOpWorkspaceHooks) OnMemberRemoved(_ context.Context, _, _ string) error  { return nil }
func (NoOpWorkspaceHooks) FormatEvent(event *MutationEvent) any                  { return event }
