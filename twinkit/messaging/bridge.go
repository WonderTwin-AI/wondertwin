package messaging

import (
	"context"

	"github.com/wondertwin-ai/wondertwin/twinkit/domainevents"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// WithTelemetry returns an Option that wraps the engine's hooks to automatically
// emit DomainEvent telemetry when hooks fire.
func WithTelemetry(emitter domainevents.Emitter) Option {
	return func(e *Engine) {
		inner := e.hooks
		if inner == nil {
			inner = NoOpMessagingHooks{}
		}
		e.hooks = &telemetryBridge{emitter: emitter, inner: inner}
	}
}

type telemetryBridge struct {
	emitter domainevents.Emitter
	inner   MessagingHooks
}

func (b *telemetryBridge) ValidateMessage(ctx context.Context, msg *Message) error {
	err := b.inner.ValidateMessage(ctx, msg)
	errType := ""
	if err != nil {
		errType = err.Error()
	}
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "messaging",
			Hook:       "ValidateMessage",
			Resource:   "messages",
			Action:     "validate",
			EntityType: string(msg.Channel),
			EntityID:   msg.ID,
			Context:    ValidateMessageContext(msg.Channel, err == nil, errType),
		},
	)
	return err
}

func (b *telemetryBridge) OnMessageCreated(ctx context.Context, msg *Message) error {
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "messaging",
			Hook:       "OnMessageCreated",
			Resource:   "messages",
			Action:     "create",
			EntityType: string(msg.Channel),
			EntityID:   msg.ID,
			Context:    MessageCreatedContext(msg.Channel, len(msg.To), len(msg.Attachments) > 0),
		},
	)
	return b.inner.OnMessageCreated(ctx, msg)
}

func (b *telemetryBridge) OnStatusChange(ctx context.Context, msg *Message, from, to MessageStatus) error {
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "messaging",
			Hook:       "OnStatusChange",
			Resource:   "messages",
			Action:     "transition",
			EntityType: string(msg.Channel),
			EntityID:   msg.ID,
			StateFrom:  string(from),
			StateTo:    string(to),
			Context:    StatusChangeContext(from, to, msg.Channel, 0),
		},
	)
	return b.inner.OnStatusChange(ctx, msg, from, to)
}

func (b *telemetryBridge) FormatDeliveryEvent(event *DeliveryEvent) any {
	return b.inner.FormatDeliveryEvent(event)
}
