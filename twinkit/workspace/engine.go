package workspace

import (
	"context"
	"fmt"
	"sync"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// Engine manages workspace entities, their lifecycles, and event emission.
type Engine struct {
	mu        sync.RWMutex
	entities  map[string]map[string]*Entity // type → id → entity
	comments  map[string][]*Comment         // entityID → comments
	members   map[string][]*Member          // containerID → members
	workflows map[string]*WorkflowConfig    // entityType → workflow
	hooks     WorkspaceHooks
	clock     *state.Clock
	idCounter int
}

// Option configures the Engine.
type Option func(*Engine)

// WithHooks sets the workspace hooks implementation.
func WithHooks(h WorkspaceHooks) Option {
	return func(e *Engine) { e.hooks = h }
}

// WithClock sets the clock for timestamps.
func WithClock(c *state.Clock) Option {
	return func(e *Engine) { e.clock = c }
}

// WithWorkflow registers a workflow state machine for an entity type.
// Can be called multiple times for different entity types.
func WithWorkflow(cfg WorkflowConfig) Option {
	return func(e *Engine) {
		e.workflows[cfg.EntityType] = &cfg
	}
}

// NewEngine creates a new workspace engine with the given options.
func NewEngine(opts ...Option) *Engine {
	e := &Engine{
		entities:  make(map[string]map[string]*Entity),
		comments:  make(map[string][]*Comment),
		members:   make(map[string][]*Member),
		workflows: make(map[string]*WorkflowConfig),
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.clock == nil {
		e.clock = state.NewClock()
	}
	return e
}

// CreateEntity creates a new entity, stores it, and emits a "created" event.
func (e *Engine) CreateEntity(ctx context.Context, entity *Entity) (*MutationEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if entity.Type == "" {
		return nil, fmt.Errorf("entity type is required")
	}

	// Assign ID if empty.
	if entity.ID == "" {
		e.idCounter++
		entity.ID = fmt.Sprintf("%s_%06d", entity.Type, e.idCounter)
	}

	// Set timestamps.
	now := e.clock.Now()
	entity.CreatedAt = now
	entity.UpdatedAt = now

	// Set initial status from workflow.
	if wf, ok := e.workflows[entity.Type]; ok {
		if entity.Status == "" {
			entity.Status = wf.InitialStatus
		}
	}

	// Initialize properties map if nil.
	if entity.Properties == nil {
		entity.Properties = make(map[string]any)
	}

	// Validate via hooks.
	if e.hooks != nil {
		if err := e.hooks.ValidateEntity(ctx, entity); err != nil {
			return nil, fmt.Errorf("validation failed: %w", err)
		}
	}

	// Store entity.
	if e.entities[entity.Type] == nil {
		e.entities[entity.Type] = make(map[string]*Entity)
	}
	e.entities[entity.Type][entity.ID] = entity

	// Notify hooks.
	if e.hooks != nil {
		if err := e.hooks.OnEntityCreated(ctx, entity); err != nil {
			return nil, fmt.Errorf("hook OnEntityCreated: %w", err)
		}
	}

	event := &MutationEvent{
		Action:     "created",
		EntityType: entity.Type,
		EntityID:   entity.ID,
		Actor:      entity.CreatedBy,
		Timestamp:  now,
	}
	return event, nil
}

// UpdateEntity applies changes to an existing entity and emits an "updated" event.
// Only the specified keys in changes are modified (sparse update).
func (e *Engine) UpdateEntity(ctx context.Context, entityType, entityID string, changes map[string]any) (*Entity, *MutationEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	entity, err := e.getEntityLocked(entityType, entityID)
	if err != nil {
		return nil, nil, err
	}

	// Compute diff.
	diff := make(map[string]any)
	for k, newVal := range changes {
		oldVal, exists := entity.Properties[k]
		if !exists || oldVal != newVal {
			diff[k] = map[string]any{"old": oldVal, "new": newVal}
		}
		entity.Properties[k] = newVal
	}

	now := e.clock.Now()
	entity.UpdatedAt = now

	// Validate via hooks.
	if e.hooks != nil {
		if err := e.hooks.ValidateEntity(ctx, entity); err != nil {
			return nil, nil, fmt.Errorf("validation failed: %w", err)
		}
	}

	// Notify hooks.
	if e.hooks != nil {
		if err := e.hooks.OnEntityUpdated(ctx, entity, diff); err != nil {
			return nil, nil, fmt.Errorf("hook OnEntityUpdated: %w", err)
		}
	}

	event := &MutationEvent{
		Action:     "updated",
		EntityType: entityType,
		EntityID:   entityID,
		Changes:    diff,
		Timestamp:  now,
	}
	return entity, event, nil
}

// DeleteEntity removes an entity and its associated comments and memberships.
func (e *Engine) DeleteEntity(ctx context.Context, entityType, entityID string) (*MutationEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, err := e.getEntityLocked(entityType, entityID); err != nil {
		return nil, err
	}

	// Remove entity.
	delete(e.entities[entityType], entityID)

	// Remove associated comments.
	delete(e.comments, entityID)

	// Remove associated memberships.
	delete(e.members, entityID)

	now := e.clock.Now()

	// Notify hooks.
	if e.hooks != nil {
		if err := e.hooks.OnEntityDeleted(ctx, entityType, entityID); err != nil {
			return nil, fmt.Errorf("hook OnEntityDeleted: %w", err)
		}
	}

	event := &MutationEvent{
		Action:     "deleted",
		EntityType: entityType,
		EntityID:   entityID,
		Timestamp:  now,
	}
	return event, nil
}

// TransitionStatus changes an entity's workflow status.
func (e *Engine) TransitionStatus(ctx context.Context, entityType, entityID, toStatus string) (*Entity, *MutationEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	entity, err := e.getEntityLocked(entityType, entityID)
	if err != nil {
		return nil, nil, err
	}

	fromStatus := entity.Status

	// Validate against workflow if one is configured.
	if wf, ok := e.workflows[entityType]; ok {
		if err := validateWorkflowTransition(wf, fromStatus, toStatus); err != nil {
			return nil, nil, err
		}
	}

	now := e.clock.Now()
	entity.Status = toStatus
	entity.UpdatedAt = now

	// Notify hooks.
	if e.hooks != nil {
		if err := e.hooks.OnStatusTransition(ctx, entity, fromStatus, toStatus); err != nil {
			return nil, nil, fmt.Errorf("hook OnStatusTransition: %w", err)
		}
	}

	event := &MutationEvent{
		Action:     "status_changed",
		EntityType: entityType,
		EntityID:   entityID,
		OldStatus:  fromStatus,
		NewStatus:  toStatus,
		Timestamp:  now,
	}
	return entity, event, nil
}

// GetEntity returns an entity by type and ID.
func (e *Engine) GetEntity(entityType, entityID string) (*Entity, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.getEntityLocked(entityType, entityID)
}

// ListEntities returns all entities of the given type.
func (e *Engine) ListEntities(entityType string) []*Entity {
	e.mu.RLock()
	defer e.mu.RUnlock()
	typeMap := e.entities[entityType]
	result := make([]*Entity, 0, len(typeMap))
	for _, ent := range typeMap {
		result = append(result, ent)
	}
	return result
}

// ListByParent returns all entities of the given type with the specified parent.
func (e *Engine) ListByParent(entityType, parentID string) []*Entity {
	e.mu.RLock()
	defer e.mu.RUnlock()
	typeMap := e.entities[entityType]
	var result []*Entity
	for _, ent := range typeMap {
		if ent.ParentID == parentID {
			result = append(result, ent)
		}
	}
	return result
}

// FilterEntities returns entities of the given type matching a predicate.
func (e *Engine) FilterEntities(entityType string, predicate func(*Entity) bool) []*Entity {
	e.mu.RLock()
	defer e.mu.RUnlock()
	typeMap := e.entities[entityType]
	var result []*Entity
	for _, ent := range typeMap {
		if predicate(ent) {
			result = append(result, ent)
		}
	}
	return result
}

// ListChildren returns all entities whose ParentID matches the given container.
func (e *Engine) ListChildren(parentType, parentID, childType string) []*Entity {
	return e.ListByParent(childType, parentID)
}

// MoveEntity changes an entity's parent container.
func (e *Engine) MoveEntity(ctx context.Context, entityType, entityID, newParentType, newParentID string) (*MutationEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	entity, err := e.getEntityLocked(entityType, entityID)
	if err != nil {
		return nil, err
	}

	oldParentID := entity.ParentID
	entity.ParentID = newParentID
	entity.ParentType = newParentType

	now := e.clock.Now()
	entity.UpdatedAt = now

	if e.hooks != nil {
		changes := map[string]any{
			"parent_id": map[string]any{"old": oldParentID, "new": newParentID},
		}
		if err := e.hooks.OnEntityUpdated(ctx, entity, changes); err != nil {
			return nil, fmt.Errorf("hook OnEntityUpdated: %w", err)
		}
	}

	event := &MutationEvent{
		Action:     "updated",
		EntityType: entityType,
		EntityID:   entityID,
		Changes:    map[string]any{"parent_id": map[string]any{"old": oldParentID, "new": newParentID}},
		Timestamp:  now,
	}
	return event, nil
}

// getEntityLocked returns an entity. Caller must hold at least a read lock.
func (e *Engine) getEntityLocked(entityType, entityID string) (*Entity, error) {
	typeMap, ok := e.entities[entityType]
	if !ok {
		return nil, fmt.Errorf("%s %s not found", entityType, entityID)
	}
	entity, ok := typeMap[entityID]
	if !ok {
		return nil, fmt.Errorf("%s %s not found", entityType, entityID)
	}
	return entity, nil
}
