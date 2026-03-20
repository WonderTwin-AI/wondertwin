package workspace

import (
	"context"
	"fmt"
)

// AddMember adds a member to a container and emits a "member_added" event.
func (e *Engine) AddMember(ctx context.Context, containerID, userID, role string) (*MutationEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check for duplicate.
	for _, m := range e.members[containerID] {
		if m.UserID == userID {
			return nil, fmt.Errorf("user %s is already a member of %s", userID, containerID)
		}
	}

	now := e.clock.Now()
	member := &Member{
		UserID:      userID,
		ContainerID: containerID,
		Role:        role,
		JoinedAt:    now,
	}
	e.members[containerID] = append(e.members[containerID], member)

	// Notify hooks.
	if e.hooks != nil {
		if err := e.hooks.OnMemberAdded(ctx, containerID, userID, role); err != nil {
			return nil, fmt.Errorf("hook OnMemberAdded: %w", err)
		}
	}

	event := &MutationEvent{
		Action:     "member_added",
		EntityType: "member",
		EntityID:   containerID,
		Actor:      userID,
		Timestamp:  now,
	}
	return event, nil
}

// RemoveMember removes a member from a container and emits a "member_removed" event.
func (e *Engine) RemoveMember(ctx context.Context, containerID, userID string) (*MutationEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	members := e.members[containerID]
	found := false
	for i, m := range members {
		if m.UserID == userID {
			e.members[containerID] = append(members[:i], members[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("user %s is not a member of %s", userID, containerID)
	}

	now := e.clock.Now()

	// Notify hooks.
	if e.hooks != nil {
		if err := e.hooks.OnMemberRemoved(ctx, containerID, userID); err != nil {
			return nil, fmt.Errorf("hook OnMemberRemoved: %w", err)
		}
	}

	event := &MutationEvent{
		Action:     "member_removed",
		EntityType: "member",
		EntityID:   containerID,
		Actor:      userID,
		Timestamp:  now,
	}
	return event, nil
}

// ListMembers returns all members of a container.
func (e *Engine) ListMembers(containerID string) []*Member {
	e.mu.RLock()
	defer e.mu.RUnlock()
	members := e.members[containerID]
	result := make([]*Member, len(members))
	copy(result, members)
	return result
}

// IsMember returns true if the user is a member of the container.
func (e *Engine) IsMember(containerID, userID string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, m := range e.members[containerID] {
		if m.UserID == userID {
			return true
		}
	}
	return false
}
