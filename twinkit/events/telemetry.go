package events

// EventIngestedContext builds the canonical Context map for OnEventIngested.
func EventIngestedContext(eventName string, hasProperties bool, propertyCount int) map[string]any {
	return map[string]any{
		"event_name":     eventName,
		"has_properties": hasProperties,
		"property_count": propertyCount,
	}
}

// IdentifyContext builds the canonical Context map for OnIdentify.
func IdentifyContext(hasAnonymousIDs bool, propertyCount int) map[string]any {
	return map[string]any{
		"has_anonymous_ids": hasAnonymousIDs,
		"property_count":    propertyCount,
	}
}

// MergeContext builds the canonical Context map for OnMerge.
func MergeContext(anonymousIDCount int) map[string]any {
	return map[string]any{
		"anonymous_id_count": anonymousIDCount,
	}
}

// ValidateEventContext builds the canonical Context map for ValidateEvent.
func ValidateEventContext(eventName string, valid bool, errorType string) map[string]any {
	ctx := map[string]any{
		"event_name": eventName,
		"valid":      valid,
	}
	if errorType != "" {
		ctx["error_type"] = errorType
	}
	return ctx
}
