package messaging

// StatusChangeContext builds the canonical Context map for OnStatusChange.
func StatusChangeContext(from, to MessageStatus, channel ChannelType, attemptCount int) map[string]any {
	return map[string]any{
		"from":          string(from),
		"to":            string(to),
		"channel":       string(channel),
		"attempt_count": attemptCount,
	}
}

// MessageCreatedContext builds the canonical Context map for OnMessageCreated.
func MessageCreatedContext(channel ChannelType, recipientCount int, hasAttachments bool) map[string]any {
	return map[string]any{
		"channel":         string(channel),
		"recipient_count": recipientCount,
		"has_attachments": hasAttachments,
	}
}

// ValidateMessageContext builds the canonical Context map for ValidateMessage.
func ValidateMessageContext(channel ChannelType, valid bool, errorType string) map[string]any {
	ctx := map[string]any{
		"channel": string(channel),
		"valid":   valid,
	}
	if errorType != "" {
		ctx["error_type"] = errorType
	}
	return ctx
}

// FormatDeliveryEventContext builds the canonical Context map for FormatDeliveryEvent.
func FormatDeliveryEventContext(eventType string, channel ChannelType) map[string]any {
	return map[string]any{
		"event_type": eventType,
		"channel":    string(channel),
	}
}
