package messaging

import (
	"context"
	"sync"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
)

func newTestEngine(opts ...Option) *Engine {
	clock := state.NewClock()
	defaults := []Option{
		WithClock(clock),
		WithLifecycle(ChannelEmail, EmailLifecycle()),
		WithLifecycle(ChannelSMS, SMSLifecycle()),
	}
	return NewEngine(append(defaults, opts...)...)
}

func ctx() context.Context {
	return context.Background()
}

// --- Send ---

func TestSend_Basic(t *testing.T) {
	e := newTestEngine()
	// Register a verified sender (open mode won't need this, but let's be explicit).
	e.RegisterSender(&Sender{Channel: ChannelEmail, Address: "example.com", Verified: true})

	msg := &Message{
		Channel: ChannelEmail,
		From:    "test@example.com",
		To:      []string{"user@other.com"},
		Subject: "Hello",
		Body:    "World",
	}
	event, err := e.Send(ctx(), msg)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if msg.ID == "" {
		t.Error("expected ID to be assigned")
	}
	if msg.Status != StatusQueued {
		t.Errorf("status = %q, want QUEUED", msg.Status)
	}
	if event.Status != StatusQueued {
		t.Errorf("event status = %q, want QUEUED", event.Status)
	}
	if event.MessageID != msg.ID {
		t.Errorf("event message_id = %q, want %q", event.MessageID, msg.ID)
	}
}

func TestSend_PreservesProvidedID(t *testing.T) {
	e := newTestEngine()
	msg := &Message{ID: "custom-123", Channel: ChannelEmail, From: "a@b.com", To: []string{"c@d.com"}}
	e.Send(ctx(), msg)
	if msg.ID != "custom-123" {
		t.Errorf("ID = %q, want custom-123", msg.ID)
	}
}

func TestSend_RequiresFields(t *testing.T) {
	e := newTestEngine()

	// Missing channel.
	_, err := e.Send(ctx(), &Message{From: "a@b.com", To: []string{"c@d.com"}})
	if err == nil {
		t.Error("expected error for missing channel")
	}

	// Missing recipients.
	_, err = e.Send(ctx(), &Message{Channel: ChannelEmail, From: "a@b.com"})
	if err == nil {
		t.Error("expected error for missing recipients")
	}

	// Missing from.
	_, err = e.Send(ctx(), &Message{Channel: ChannelEmail, To: []string{"c@d.com"}})
	if err == nil {
		t.Error("expected error for missing from")
	}
}

func TestSend_SMS(t *testing.T) {
	e := newTestEngine()
	msg := &Message{
		Channel: ChannelSMS,
		From:    "+15551234567",
		To:      []string{"+15559876543"},
		Body:    "Hello via SMS",
	}
	event, err := e.Send(ctx(), msg)
	if err != nil {
		t.Fatalf("send SMS: %v", err)
	}
	if msg.Status != StatusQueued {
		t.Errorf("status = %q, want QUEUED", msg.Status)
	}
	if event.Channel != ChannelSMS {
		t.Errorf("channel = %q, want SMS", event.Channel)
	}
}

// --- Lifecycle ---

func TestAdvance_EmailLifecycle(t *testing.T) {
	e := newTestEngine()
	msg := &Message{Channel: ChannelEmail, From: "a@b.com", To: []string{"c@d.com"}, Subject: "Test"}
	e.Send(ctx(), msg)

	// QUEUED → SENDING
	event, err := e.Advance(ctx(), msg.ID)
	if err != nil {
		t.Fatalf("advance 1: %v", err)
	}
	if event.Status != StatusSending {
		t.Errorf("status = %q, want SENDING", event.Status)
	}

	// SENDING → SENT
	event, err = e.Advance(ctx(), msg.ID)
	if err != nil {
		t.Fatalf("advance 2: %v", err)
	}
	if event.Status != StatusSent {
		t.Errorf("status = %q, want SENT", event.Status)
	}
	if msg.SentAt.IsZero() {
		t.Error("SentAt should be set")
	}

	// SENT → DELIVERED
	event, err = e.Advance(ctx(), msg.ID)
	if err != nil {
		t.Fatalf("advance 3: %v", err)
	}
	if event.Status != StatusDelivered {
		t.Errorf("status = %q, want DELIVERED", event.Status)
	}
	if msg.DeliveredAt.IsZero() {
		t.Error("DeliveredAt should be set")
	}

	// DELIVERED has no auto-advance.
	event, err = e.Advance(ctx(), msg.ID)
	if err != nil {
		t.Fatalf("advance 4: %v", err)
	}
	if event != nil {
		t.Error("expected nil event when no auto-advance configured")
	}
}

func TestAdvanceAll(t *testing.T) {
	e := newTestEngine()

	// Send 3 messages.
	for i := 0; i < 3; i++ {
		e.Send(ctx(), &Message{Channel: ChannelEmail, From: "a@b.com", To: []string{"c@d.com"}})
	}

	// Advance all: QUEUED → SENDING.
	events, err := e.AdvanceAll(ctx())
	if err != nil {
		t.Fatalf("advance all: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
	for _, ev := range events {
		if ev.Status != StatusSending {
			t.Errorf("status = %q, want SENDING", ev.Status)
		}
	}
}

func TestTransition_Explicit(t *testing.T) {
	e := newTestEngine()
	msg := &Message{Channel: ChannelEmail, From: "a@b.com", To: []string{"c@d.com"}}
	e.Send(ctx(), msg)

	// Valid: QUEUED → SENDING.
	_, err := e.Transition(ctx(), msg.ID, StatusSending)
	if err != nil {
		t.Fatalf("transition: %v", err)
	}

	// Invalid: SENDING → DELIVERED (must go through SENT first).
	_, err = e.Transition(ctx(), msg.ID, StatusDelivered)
	if err == nil {
		t.Error("expected error for invalid transition SENDING → DELIVERED")
	}

	// Valid: SENDING → FAILED.
	_, err = e.Transition(ctx(), msg.ID, StatusFailed)
	if err != nil {
		t.Fatalf("transition to failed: %v", err)
	}
	if msg.Status != StatusFailed {
		t.Errorf("status = %q, want FAILED", msg.Status)
	}
}

func TestTransition_Bounce(t *testing.T) {
	e := newTestEngine()
	msg := &Message{Channel: ChannelEmail, From: "a@b.com", To: []string{"bounced@bad.com"}}
	e.Send(ctx(), msg)
	e.Advance(ctx(), msg.ID) // → SENDING
	e.Advance(ctx(), msg.ID) // → SENT

	// Bounce.
	e.Transition(ctx(), msg.ID, StatusBounced)

	if msg.Status != StatusBounced {
		t.Errorf("status = %q, want BOUNCED", msg.Status)
	}

	// Should auto-suppress the bounced address.
	if !e.IsSuppressed("bounced@bad.com", ChannelEmail) {
		t.Error("expected bounced address to be suppressed")
	}
}

func TestTransition_Complaint(t *testing.T) {
	e := newTestEngine()
	msg := &Message{Channel: ChannelEmail, From: "a@b.com", To: []string{"complainer@test.com"}}
	e.Send(ctx(), msg)
	e.Advance(ctx(), msg.ID) // → SENDING
	e.Advance(ctx(), msg.ID) // → SENT
	e.Advance(ctx(), msg.ID) // → DELIVERED

	e.Transition(ctx(), msg.ID, StatusComplained)

	if !e.IsSuppressed("complainer@test.com", ChannelEmail) {
		t.Error("expected complained address to be suppressed")
	}
}

// --- Suppression ---

func TestSuppression_DropsMessage(t *testing.T) {
	e := newTestEngine()
	e.Suppress("blocked@test.com", ChannelEmail, "manual")

	msg := &Message{Channel: ChannelEmail, From: "a@b.com", To: []string{"blocked@test.com"}}
	event, err := e.Send(ctx(), msg)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if msg.Status != StatusDropped {
		t.Errorf("status = %q, want DROPPED", msg.Status)
	}
	if event.Status != StatusDropped {
		t.Errorf("event status = %q, want DROPPED", event.Status)
	}
}

func TestSuppression_Unsuppress(t *testing.T) {
	e := newTestEngine()
	e.Suppress("temp@test.com", ChannelEmail, "bounce")

	if !e.IsSuppressed("temp@test.com", ChannelEmail) {
		t.Error("should be suppressed")
	}

	e.Unsuppress("temp@test.com", ChannelEmail)

	if e.IsSuppressed("temp@test.com", ChannelEmail) {
		t.Error("should no longer be suppressed")
	}
}

func TestSuppression_ListByChannel(t *testing.T) {
	e := newTestEngine()
	e.Suppress("a@test.com", ChannelEmail, "bounce")
	e.Suppress("b@test.com", ChannelEmail, "complaint")
	e.Suppress("+15551234567", ChannelSMS, "carrier_block")

	emailSups := e.ListSuppressions(ChannelEmail)
	if len(emailSups) != 2 {
		t.Errorf("expected 2 email suppressions, got %d", len(emailSups))
	}

	allSups := e.ListSuppressions("")
	if len(allSups) != 3 {
		t.Errorf("expected 3 total suppressions, got %d", len(allSups))
	}
}

// --- Sender Identity ---

func TestSender_Verification(t *testing.T) {
	e := newTestEngine()

	// Register unverified sender.
	err := e.RegisterSender(&Sender{Channel: ChannelEmail, Address: "newdomain.com", Verified: false})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Should reject send from unverified sender.
	_, err = e.Send(ctx(), &Message{
		Channel: ChannelEmail,
		From:    "user@newdomain.com",
		To:      []string{"dest@other.com"},
	})
	if err == nil {
		t.Error("expected error for unverified sender")
	}

	// Verify the sender.
	senders := e.ListSenders()
	e.VerifySender(senders[0].ID)

	// Should now accept.
	_, err = e.Send(ctx(), &Message{
		Channel: ChannelEmail,
		From:    "user@newdomain.com",
		To:      []string{"dest@other.com"},
	})
	if err != nil {
		t.Fatalf("send after verify: %v", err)
	}
}

func TestSender_OpenMode(t *testing.T) {
	e := newTestEngine() // No senders registered = open mode

	// Should accept any sender.
	_, err := e.Send(ctx(), &Message{
		Channel: ChannelEmail,
		From:    "anyone@anything.com",
		To:      []string{"dest@other.com"},
	})
	if err != nil {
		t.Fatalf("open mode should accept any sender: %v", err)
	}
}

func TestSender_DomainMatch(t *testing.T) {
	if !domainMatch("user@example.com", "example.com") {
		t.Error("should match")
	}
	if domainMatch("user@other.com", "example.com") {
		t.Error("should not match")
	}
	if domainMatch("nodomain", "example.com") {
		t.Error("should not match without @")
	}
}

func TestSender_DeleteSender(t *testing.T) {
	e := newTestEngine()
	e.RegisterSender(&Sender{Channel: ChannelEmail, Address: "example.com", Verified: true})
	senders := e.ListSenders()
	if len(senders) != 1 {
		t.Fatalf("expected 1 sender, got %d", len(senders))
	}
	e.DeleteSender(senders[0].ID)
	if len(e.ListSenders()) != 0 {
		t.Error("expected 0 senders after delete")
	}
}

// --- Hooks ---

type testHooks struct {
	NoOpMessagingHooks
	created     int
	transitions int
}

func (h *testHooks) OnMessageCreated(_ context.Context, _ *Message) error {
	h.created++
	return nil
}
func (h *testHooks) OnStatusChange(_ context.Context, _ *Message, _, _ MessageStatus) error {
	h.transitions++
	return nil
}

func TestHooks_CalledOnOperations(t *testing.T) {
	hooks := &testHooks{}
	e := newTestEngine(WithHooks(hooks))

	msg := &Message{Channel: ChannelEmail, From: "a@b.com", To: []string{"c@d.com"}}
	e.Send(ctx(), msg)
	if hooks.created != 1 {
		t.Errorf("created = %d, want 1", hooks.created)
	}

	e.Advance(ctx(), msg.ID) // QUEUED → SENDING
	if hooks.transitions != 1 {
		t.Errorf("transitions = %d, want 1", hooks.transitions)
	}

	e.Advance(ctx(), msg.ID) // SENDING → SENT
	if hooks.transitions != 2 {
		t.Errorf("transitions = %d, want 2", hooks.transitions)
	}
}

// --- Get/List ---

func TestGetMessage(t *testing.T) {
	e := newTestEngine()
	msg := &Message{Channel: ChannelEmail, From: "a@b.com", To: []string{"c@d.com"}}
	e.Send(ctx(), msg)

	got, err := e.GetMessage(msg.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != msg.ID {
		t.Errorf("ID mismatch")
	}

	_, err = e.GetMessage("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent message")
	}
}

func TestListMessages(t *testing.T) {
	e := newTestEngine()
	for i := 0; i < 5; i++ {
		e.Send(ctx(), &Message{Channel: ChannelEmail, From: "a@b.com", To: []string{"c@d.com"}})
	}
	msgs := e.ListMessages()
	if len(msgs) != 5 {
		t.Errorf("expected 5 messages, got %d", len(msgs))
	}
}

// --- Clock ---

func TestClock_UsedForTimestamps(t *testing.T) {
	clock := state.NewClock()
	e := NewEngine(WithClock(clock), WithLifecycle(ChannelEmail, EmailLifecycle()))

	msg := &Message{Channel: ChannelEmail, From: "a@b.com", To: []string{"c@d.com"}}
	e.Send(ctx(), msg)
	createdAt := msg.CreatedAt

	clock.Advance(60_000_000_000) // 1 minute
	e.Advance(ctx(), msg.ID) // → SENDING
	e.Advance(ctx(), msg.ID) // → SENT

	if !msg.SentAt.After(createdAt) {
		t.Error("SentAt should be after CreatedAt when clock advances")
	}
}

// --- Thread Safety ---

func TestConcurrentSend(t *testing.T) {
	e := newTestEngine()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.Send(ctx(), &Message{Channel: ChannelEmail, From: "a@b.com", To: []string{"c@d.com"}})
		}()
	}
	wg.Wait()

	msgs := e.ListMessages()
	if len(msgs) != 100 {
		t.Errorf("expected 100 messages, got %d", len(msgs))
	}
}
