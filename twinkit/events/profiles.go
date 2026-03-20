package events

// SetProfileProperty sets a single property on a user profile.
func (e *Engine) SetProfileProperty(distinctID, key string, value any) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Resolve anonymous ID.
	if resolved, ok := e.anonMap[distinctID]; ok {
		distinctID = resolved
	}

	profile := e.ensureProfileLocked(distinctID)
	profile.Properties[key] = value
	profile.UpdatedAt = e.clock.Now()
	return nil
}

// IncrementProfileProperty increments a numeric property by the given amount.
// If the property doesn't exist, it starts at the increment value.
func (e *Engine) IncrementProfileProperty(distinctID, key string, amount float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if resolved, ok := e.anonMap[distinctID]; ok {
		distinctID = resolved
	}

	profile := e.ensureProfileLocked(distinctID)
	current, ok := profile.Properties[key]
	if !ok {
		profile.Properties[key] = amount
	} else {
		switch v := current.(type) {
		case float64:
			profile.Properties[key] = v + amount
		case int:
			profile.Properties[key] = float64(v) + amount
		case int64:
			profile.Properties[key] = float64(v) + amount
		default:
			profile.Properties[key] = amount
		}
	}
	profile.UpdatedAt = e.clock.Now()
	return nil
}

// DeleteProfile removes a user profile and all associated events.
func (e *Engine) DeleteProfile(distinctID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if resolved, ok := e.anonMap[distinctID]; ok {
		distinctID = resolved
	}

	if _, ok := e.profiles[distinctID]; !ok {
		return false
	}
	delete(e.profiles, distinctID)

	// Remove associated anonymous ID mappings.
	for anonID, knownID := range e.anonMap {
		if knownID == distinctID {
			delete(e.anonMap, anonID)
		}
	}

	return true
}
