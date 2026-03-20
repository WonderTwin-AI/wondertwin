package messaging

import "fmt"

// RegisterSender adds a sender identity to the engine.
func (e *Engine) RegisterSender(sender *Sender) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if sender.ID == "" {
		e.idCounter++
		sender.ID = fmt.Sprintf("sender_%06d", e.idCounter)
	}
	if sender.Address == "" {
		return fmt.Errorf("sender address is required")
	}
	if sender.Channel == "" {
		return fmt.Errorf("sender channel is required")
	}

	e.senders[sender.ID] = sender
	return nil
}

// VerifySender marks a sender as verified.
func (e *Engine) VerifySender(senderID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.senders[senderID]
	if !ok {
		return fmt.Errorf("sender %s not found", senderID)
	}
	s.Verified = true
	return nil
}

// GetSender returns a sender by ID.
func (e *Engine) GetSender(senderID string) (*Sender, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.senders[senderID]
	if !ok {
		return nil, fmt.Errorf("sender %s not found", senderID)
	}
	return s, nil
}

// ListSenders returns all registered senders.
func (e *Engine) ListSenders() []*Sender {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]*Sender, 0, len(e.senders))
	for _, s := range e.senders {
		result = append(result, s)
	}
	return result
}

// DeleteSender removes a sender identity.
func (e *Engine) DeleteSender(senderID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.senders[senderID]; !ok {
		return fmt.Errorf("sender %s not found", senderID)
	}
	delete(e.senders, senderID)
	return nil
}
