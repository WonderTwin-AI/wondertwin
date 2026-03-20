package workspace

// StatusTransitionContext builds the canonical Context map for OnStatusTransition.
func StatusTransitionContext(from, to, entityType string) map[string]any {
	return map[string]any{
		"from":        from,
		"to":          to,
		"entity_type": entityType,
	}
}

// EntityCreatedContext builds the canonical Context map for OnEntityCreated.
func EntityCreatedContext(entityType string, hasParent bool, propertyCount int) map[string]any {
	return map[string]any{
		"entity_type":    entityType,
		"has_parent":     hasParent,
		"property_count": propertyCount,
	}
}

// EntityUpdatedContext builds the canonical Context map for OnEntityUpdated.
func EntityUpdatedContext(entityType string, changedFieldCount int) map[string]any {
	return map[string]any{
		"entity_type":         entityType,
		"changed_field_count": changedFieldCount,
	}
}

// CommentAddedContext builds the canonical Context map for OnCommentAdded.
func CommentAddedContext(entityType string, isReply bool) map[string]any {
	return map[string]any{
		"entity_type": entityType,
		"is_reply":    isReply,
	}
}

// MemberAddedContext builds the canonical Context map for OnMemberAdded.
func MemberAddedContext(role string) map[string]any {
	return map[string]any{
		"role": role,
	}
}
