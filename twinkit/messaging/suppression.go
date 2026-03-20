package messaging

import "context"

// Suppress adds an address to the suppression list for a channel.
func (e *Engine) Suppress(address string, channel ChannelType, reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := suppressionKey(channel, address)
	e.suppressions[key] = &Suppression{
		Address:   address,
		Channel:   channel,
		Reason:    reason,
		CreatedAt: e.clock.Now(),
	}
}

// Unsuppress removes an address from the suppression list.
func (e *Engine) Unsuppress(address string, channel ChannelType) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := suppressionKey(channel, address)
	if _, ok := e.suppressions[key]; !ok {
		return false
	}
	delete(e.suppressions, key)
	return true
}

// IsSuppressed checks if an address is suppressed for a channel.
func (e *Engine) IsSuppressed(address string, channel ChannelType) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.suppressions[suppressionKey(channel, address)]
	return ok
}

// ListSuppressions returns all suppressions for a channel.
// If channel is empty, returns all suppressions.
func (e *Engine) ListSuppressions(channel ChannelType) []*Suppression {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var result []*Suppression
	for _, sup := range e.suppressions {
		if channel == "" || sup.Channel == channel {
			result = append(result, sup)
		}
	}
	return result
}

// SendOrDrop is a convenience method that checks suppression before sending.
// If the recipient is suppressed, returns a DROPPED event without error.
// This is the same as Send (which already checks suppression) but makes
// the intent explicit for twin handlers.
func (e *Engine) SendOrDrop(ctx context.Context, msg *Message) (*DeliveryEvent, error) {
	return e.Send(ctx, msg)
}
