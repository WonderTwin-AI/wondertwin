package messaging

import "context"

// MessagingHooks defines extension points for platform-specific behavior.
// The engine calls these at behavioral inflection points. Implementations
// must not call back into the engine.
type MessagingHooks interface {
	// ValidateMessage is called before accepting a message. Return error to reject.
	ValidateMessage(ctx context.Context, msg *Message) error

	// OnMessageCreated is called after a message is created and queued.
	OnMessageCreated(ctx context.Context, msg *Message) error

	// OnStatusChange is called after a message transitions state.
	OnStatusChange(ctx context.Context, msg *Message, from, to MessageStatus) error

	// FormatDeliveryEvent converts a DeliveryEvent into a platform-specific payload.
	FormatDeliveryEvent(event *DeliveryEvent) any
}

// NoOpMessagingHooks provides default no-op implementations.
// Embed this in the twin's hook implementation to avoid implementing unused hooks.
type NoOpMessagingHooks struct{}

func (NoOpMessagingHooks) ValidateMessage(_ context.Context, _ *Message) error        { return nil }
func (NoOpMessagingHooks) OnMessageCreated(_ context.Context, _ *Message) error        { return nil }
func (NoOpMessagingHooks) OnStatusChange(_ context.Context, _ *Message, _, _ MessageStatus) error {
	return nil
}
func (NoOpMessagingHooks) FormatDeliveryEvent(event *DeliveryEvent) any { return event }
