package workspace

import (
	"context"
	"fmt"
)

// AddComment adds a comment to an entity and emits a "comment_created" event.
func (e *Engine) AddComment(ctx context.Context, entityType, entityID string, comment *Comment) (*MutationEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	entity, err := e.getEntityLocked(entityType, entityID)
	if err != nil {
		return nil, err
	}

	now := e.clock.Now()

	// Assign ID if empty.
	if comment.ID == "" {
		e.idCounter++
		comment.ID = fmt.Sprintf("comment_%06d", e.idCounter)
	}

	comment.EntityID = entityID
	comment.CreatedAt = now
	comment.UpdatedAt = now

	e.comments[entityID] = append(e.comments[entityID], comment)

	// Notify hooks.
	if e.hooks != nil {
		if err := e.hooks.OnCommentAdded(ctx, entity, comment); err != nil {
			return nil, fmt.Errorf("hook OnCommentAdded: %w", err)
		}
	}

	event := &MutationEvent{
		Action:     "comment_created",
		EntityType: entityType,
		EntityID:   entityID,
		Actor:      comment.AuthorID,
		Timestamp:  now,
	}
	return event, nil
}

// ListComments returns all comments on an entity, in creation order.
func (e *Engine) ListComments(entityID string) []*Comment {
	e.mu.RLock()
	defer e.mu.RUnlock()
	comments := e.comments[entityID]
	result := make([]*Comment, len(comments))
	copy(result, comments)
	return result
}

// UpdateComment updates the body of an existing comment.
func (e *Engine) UpdateComment(ctx context.Context, commentID string, body any) (*MutationEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	comment, entityID, err := e.findCommentLocked(commentID)
	if err != nil {
		return nil, err
	}

	now := e.clock.Now()
	comment.Body = body
	comment.UpdatedAt = now

	event := &MutationEvent{
		Action:     "comment_updated",
		EntityType: "comment",
		EntityID:   entityID,
		Timestamp:  now,
		Actor:      comment.AuthorID,
	}
	return event, nil
}

// DeleteComment removes a comment by ID.
func (e *Engine) DeleteComment(ctx context.Context, commentID string) (*MutationEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	_, entityID, err := e.findCommentLocked(commentID)
	if err != nil {
		return nil, err
	}

	// Remove from the list.
	comments := e.comments[entityID]
	for i, c := range comments {
		if c.ID == commentID {
			e.comments[entityID] = append(comments[:i], comments[i+1:]...)
			break
		}
	}

	now := e.clock.Now()
	event := &MutationEvent{
		Action:     "comment_deleted",
		EntityType: "comment",
		EntityID:   entityID,
		Timestamp:  now,
	}
	return event, nil
}

// findCommentLocked finds a comment by ID across all entities. Caller must hold lock.
func (e *Engine) findCommentLocked(commentID string) (*Comment, string, error) {
	for entityID, comments := range e.comments {
		for _, c := range comments {
			if c.ID == commentID {
				return c, entityID, nil
			}
		}
	}
	return nil, "", fmt.Errorf("comment %s not found", commentID)
}
