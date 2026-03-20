package messaging

import (
	"context"
	"fmt"
	"math/rand"
	"sync"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// Engine processes messages through their lifecycle.
type Engine struct {
	mu           sync.RWMutex
	messages     map[string]*Message          // id → message
	lifecycles   map[ChannelType]*Lifecycle
	suppressions map[string]*Suppression      // "channel:address" → suppression
	senders      map[string]*Sender           // id → sender
	hooks        MessagingHooks
	clock        *state.Clock
	idCounter    int
	rng          *rand.Rand // deterministic random for weighted outcomes
}

// Option configures the Engine.
type Option func(*Engine)

// WithHooks sets the messaging hooks implementation.
func WithHooks(h MessagingHooks) Option {
	return func(e *Engine) { e.hooks = h }
}

// WithClock sets the clock for timestamps.
func WithClock(c *state.Clock) Option {
	return func(e *Engine) { e.clock = c }
}

// WithLifecycle registers a lifecycle state machine for a channel type.
func WithLifecycle(channel ChannelType, lc *Lifecycle) Option {
	return func(e *Engine) { e.lifecycles[channel] = lc }
}

// WithSeed sets the random seed for deterministic weighted outcomes.
func WithSeed(seed int64) Option {
	return func(e *Engine) { e.rng = rand.New(rand.NewSource(seed)) }
}

// NewEngine creates a new messaging engine with the given options.
func NewEngine(opts ...Option) *Engine {
	e := &Engine{
		messages:     make(map[string]*Message),
		lifecycles:   make(map[ChannelType]*Lifecycle),
		suppressions: make(map[string]*Suppression),
		senders:      make(map[string]*Sender),
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.clock == nil {
		e.clock = state.NewClock()
	}
	if e.rng == nil {
		e.rng = rand.New(rand.NewSource(42))
	}
	return e
}

// Send validates a message, checks suppression, and queues it.
// The message enters the lifecycle at the channel's initial status.
func (e *Engine) Send(ctx context.Context, msg *Message) (*DeliveryEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if msg.Channel == "" {
		return nil, fmt.Errorf("channel is required")
	}
	if len(msg.To) == 0 {
		return nil, fmt.Errorf("at least one recipient is required")
	}
	if msg.From == "" {
		return nil, fmt.Errorf("from address is required")
	}

	// Assign ID if empty.
	if msg.ID == "" {
		e.idCounter++
		msg.ID = fmt.Sprintf("msg_%06d", e.idCounter)
	}

	now := e.clock.Now()
	msg.CreatedAt = now

	// Validate via hooks.
	if e.hooks != nil {
		if err := e.hooks.ValidateMessage(ctx, msg); err != nil {
			return nil, fmt.Errorf("validation failed: %w", err)
		}
	}

	// Check suppression for all recipients.
	for _, addr := range msg.To {
		key := suppressionKey(msg.Channel, addr)
		if sup, ok := e.suppressions[key]; ok {
			msg.Status = StatusDropped
			e.messages[msg.ID] = msg
			return &DeliveryEvent{
				MessageID: msg.ID,
				Status:    StatusDropped,
				Channel:   msg.Channel,
				Recipient: addr,
				Timestamp: now,
				Details:   fmt.Sprintf("suppressed: %s", sup.Reason),
			}, nil
		}
	}

	// Check sender verification.
	if !e.isSenderVerified(msg.Channel, msg.From) {
		return nil, fmt.Errorf("sender %q is not verified for channel %s", msg.From, msg.Channel)
	}

	// Set initial status from lifecycle.
	lc, ok := e.lifecycles[msg.Channel]
	if ok {
		msg.Status = lc.InitialStatus
	} else {
		msg.Status = StatusQueued
	}

	e.messages[msg.ID] = msg

	// Notify hooks.
	if e.hooks != nil {
		if err := e.hooks.OnMessageCreated(ctx, msg); err != nil {
			return nil, fmt.Errorf("hook OnMessageCreated: %w", err)
		}
	}

	event := &DeliveryEvent{
		MessageID: msg.ID,
		Status:    msg.Status,
		Channel:   msg.Channel,
		Timestamp: now,
	}
	return event, nil
}

// Advance moves a message to the next state in its lifecycle via auto-advance.
// Returns the delivery event, or nil if no auto-advance is configured for the
// message's current status.
func (e *Engine) Advance(ctx context.Context, msgID string) (*DeliveryEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	msg, ok := e.messages[msgID]
	if !ok {
		return nil, fmt.Errorf("message %s not found", msgID)
	}

	lc, ok := e.lifecycles[msg.Channel]
	if !ok {
		return nil, fmt.Errorf("no lifecycle configured for channel %s", msg.Channel)
	}

	aa, ok := lc.AutoAdvance[msg.Status]
	if !ok {
		return nil, nil // no auto-advance from this status
	}

	// Pick target status.
	target := aa.To
	if len(aa.Outcomes) > 0 {
		target = e.pickWeightedStatus(aa.Outcomes)
	}

	return e.transitionLocked(ctx, msg, target)
}

// Transition explicitly moves a message to a specific status.
// Validates the transition against the channel's lifecycle.
func (e *Engine) Transition(ctx context.Context, msgID string, to MessageStatus) (*DeliveryEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	msg, ok := e.messages[msgID]
	if !ok {
		return nil, fmt.Errorf("message %s not found", msgID)
	}

	// Validate transition.
	lc, ok := e.lifecycles[msg.Channel]
	if ok {
		allowed := lc.Transitions[msg.Status]
		valid := false
		for _, s := range allowed {
			if s == to {
				valid = true
				break
			}
		}
		if !valid {
			return nil, fmt.Errorf("transition from %s to %s is not allowed", msg.Status, to)
		}
	}

	return e.transitionLocked(ctx, msg, to)
}

// transitionLocked performs the actual status change. Caller must hold the lock.
func (e *Engine) transitionLocked(ctx context.Context, msg *Message, to MessageStatus) (*DeliveryEvent, error) {
	from := msg.Status
	now := e.clock.Now()

	msg.Status = to

	// Track timing milestones.
	switch to {
	case StatusSent:
		msg.SentAt = now
	case StatusDelivered:
		msg.DeliveredAt = now
	}

	// Auto-suppress on bounce/complaint.
	if to == StatusBounced || to == StatusComplained {
		for _, addr := range msg.To {
			reason := "bounce"
			if to == StatusComplained {
				reason = "complaint"
			}
			e.suppressions[suppressionKey(msg.Channel, addr)] = &Suppression{
				Address:   addr,
				Channel:   msg.Channel,
				Reason:    reason,
				CreatedAt: now,
			}
		}
	}

	// Notify hooks.
	if e.hooks != nil {
		if err := e.hooks.OnStatusChange(ctx, msg, from, to); err != nil {
			return nil, fmt.Errorf("hook OnStatusChange: %w", err)
		}
	}

	event := &DeliveryEvent{
		MessageID: msg.ID,
		Status:    to,
		Channel:   msg.Channel,
		Timestamp: now,
	}
	return event, nil
}

// AdvanceAll auto-advances all messages that have a configured auto-advance
// from their current status. Returns the delivery events produced.
func (e *Engine) AdvanceAll(ctx context.Context) ([]*DeliveryEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var events []*DeliveryEvent
	for _, msg := range e.messages {
		lc, ok := e.lifecycles[msg.Channel]
		if !ok {
			continue
		}
		if _, ok := lc.AutoAdvance[msg.Status]; !ok {
			continue
		}
		aa := lc.AutoAdvance[msg.Status]
		target := aa.To
		if len(aa.Outcomes) > 0 {
			target = e.pickWeightedStatus(aa.Outcomes)
		}
		event, err := e.transitionLocked(ctx, msg, target)
		if err != nil {
			return events, err
		}
		if event != nil {
			events = append(events, event)
		}
	}
	return events, nil
}

func (e *Engine) pickWeightedStatus(outcomes []WeightedStatus) MessageStatus {
	var total float64
	for _, o := range outcomes {
		total += o.Weight
	}
	roll := e.rng.Float64() * total
	var cumulative float64
	for _, o := range outcomes {
		cumulative += o.Weight
		if roll < cumulative {
			return o.Status
		}
	}
	return outcomes[len(outcomes)-1].Status
}

// GetMessage returns a message by ID.
func (e *Engine) GetMessage(msgID string) (*Message, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	msg, ok := e.messages[msgID]
	if !ok {
		return nil, fmt.Errorf("message %s not found", msgID)
	}
	return msg, nil
}

// ListMessages returns all messages.
func (e *Engine) ListMessages() []*Message {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]*Message, 0, len(e.messages))
	for _, msg := range e.messages {
		result = append(result, msg)
	}
	return result
}

// isSenderVerified checks if a sender is verified for the given channel.
// If no senders are registered, all senders are accepted (open mode).
func (e *Engine) isSenderVerified(channel ChannelType, from string) bool {
	if len(e.senders) == 0 {
		return true // open mode: no senders registered = accept all
	}
	for _, s := range e.senders {
		if s.Channel == channel && s.Verified {
			// For email, match domain. For SMS, match exact number.
			if channel == ChannelEmail {
				if domainMatch(from, s.Address) {
					return true
				}
			} else {
				if from == s.Address {
					return true
				}
			}
		}
	}
	return false
}

// domainMatch checks if an email address matches a domain.
// "user@example.com" matches domain "example.com".
func domainMatch(email, domain string) bool {
	for i := len(email) - 1; i >= 0; i-- {
		if email[i] == '@' {
			return email[i+1:] == domain
		}
	}
	return false
}

func suppressionKey(channel ChannelType, address string) string {
	return string(channel) + ":" + address
}
