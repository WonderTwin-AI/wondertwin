package events

import (
	"context"
	"sync"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
)

func newTestEngine(opts ...Option) *Engine {
	clock := state.NewClock()
	return NewEngine(append([]Option{WithClock(clock)}, opts...)...)
}

func ctx() context.Context {
	return context.Background()
}

// --- Track ---

func TestTrack_Basic(t *testing.T) {
	e := newTestEngine()

	err := e.Track(ctx(), &Event{
		EventName:  "page_view",
		DistinctID: "user_1",
		Properties: map[string]any{"url": "/home"},
	})
	if err != nil {
		t.Fatalf("track: %v", err)
	}

	if e.EventCount() != 1 {
		t.Errorf("count = %d, want 1", e.EventCount())
	}

	events := e.GetEvents("user_1")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventName != "page_view" {
		t.Errorf("name = %q, want page_view", events[0].EventName)
	}
	if events[0].ID == "" {
		t.Error("expected ID to be assigned")
	}
}

func TestTrack_RequiresFields(t *testing.T) {
	e := newTestEngine()

	err := e.Track(ctx(), &Event{DistinctID: "user_1"})
	if err == nil {
		t.Error("expected error for missing event name")
	}

	err = e.Track(ctx(), &Event{EventName: "click"})
	if err == nil {
		t.Error("expected error for missing distinct_id")
	}
}

func TestTrack_CreatesProfile(t *testing.T) {
	e := newTestEngine()

	e.Track(ctx(), &Event{EventName: "signup", DistinctID: "user_1"})

	profile, err := e.GetProfile("user_1")
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if profile.DistinctID != "user_1" {
		t.Errorf("distinct_id = %q, want user_1", profile.DistinctID)
	}
}

func TestTrack_PreservesProvidedID(t *testing.T) {
	e := newTestEngine()
	evt := &Event{ID: "custom-evt-id", EventName: "click", DistinctID: "u1"}
	e.Track(ctx(), evt)
	if evt.ID != "custom-evt-id" {
		t.Errorf("ID = %q, want custom-evt-id", evt.ID)
	}
}

// --- Identify ---

func TestIdentify_SetsProperties(t *testing.T) {
	e := newTestEngine()

	profile, err := e.Identify(ctx(), &IdentifyCall{
		DistinctID: "user_1",
		Properties: map[string]any{"email": "alice@test.com", "plan": "pro"},
	})
	if err != nil {
		t.Fatalf("identify: %v", err)
	}

	if profile.Properties["email"] != "alice@test.com" {
		t.Errorf("email = %v, want alice@test.com", profile.Properties["email"])
	}
	if profile.Properties["plan"] != "pro" {
		t.Errorf("plan = %v, want pro", profile.Properties["plan"])
	}
}

func TestIdentify_MergesProperties(t *testing.T) {
	e := newTestEngine()

	// First identify sets email.
	e.Identify(ctx(), &IdentifyCall{
		DistinctID: "user_1",
		Properties: map[string]any{"email": "alice@test.com"},
	})

	// Second identify adds plan without losing email.
	profile, _ := e.Identify(ctx(), &IdentifyCall{
		DistinctID: "user_1",
		Properties: map[string]any{"plan": "enterprise"},
	})

	if profile.Properties["email"] != "alice@test.com" {
		t.Error("email should be preserved")
	}
	if profile.Properties["plan"] != "enterprise" {
		t.Error("plan should be updated")
	}
}

func TestIdentify_RequiresDistinctID(t *testing.T) {
	e := newTestEngine()
	_, err := e.Identify(ctx(), &IdentifyCall{})
	if err == nil {
		t.Error("expected error for missing distinct_id")
	}
}

// --- Identity Merge ---

func TestIdentify_MergesAnonymousToKnown(t *testing.T) {
	e := newTestEngine()

	// Track events as anonymous user.
	e.Track(ctx(), &Event{EventName: "page_view", DistinctID: "anon_abc"})
	e.Track(ctx(), &Event{EventName: "click", DistinctID: "anon_abc"})

	// Set a property on anonymous profile.
	e.Identify(ctx(), &IdentifyCall{
		DistinctID: "anon_abc",
		Properties: map[string]any{"referrer": "google"},
	})

	// Identify: merge anonymous into known user.
	profile, err := e.Identify(ctx(), &IdentifyCall{
		DistinctID:  "user_1",
		AnonymousID: "anon_abc",
		Properties:  map[string]any{"email": "alice@test.com"},
	})
	if err != nil {
		t.Fatalf("identify merge: %v", err)
	}

	// Profile should have both properties.
	if profile.Properties["email"] != "alice@test.com" {
		t.Error("email missing")
	}
	if profile.Properties["referrer"] != "google" {
		t.Error("referrer from anonymous profile should be merged")
	}

	// Anonymous IDs should be tracked.
	if len(profile.AnonymousIDs) != 1 || profile.AnonymousIDs[0] != "anon_abc" {
		t.Errorf("anonymous_ids = %v, want [anon_abc]", profile.AnonymousIDs)
	}

	// Events should be re-attributed.
	events := e.GetEvents("user_1")
	if len(events) != 2 {
		t.Errorf("expected 2 re-attributed events, got %d", len(events))
	}

	// Looking up by anonymous ID should resolve to the known profile.
	resolvedProfile, err := e.GetProfile("anon_abc")
	if err != nil {
		t.Fatalf("anon lookup should resolve to known profile: %v", err)
	}
	if resolvedProfile.DistinctID != "user_1" {
		t.Errorf("resolved distinct_id = %q, want user_1", resolvedProfile.DistinctID)
	}

	// Future events with anonymous ID should resolve to known user.
	e.Track(ctx(), &Event{EventName: "purchase", DistinctID: "anon_abc"})
	events = e.GetEvents("user_1")
	if len(events) != 3 {
		t.Errorf("expected 3 events after anon tracking, got %d", len(events))
	}
}

// --- Profile Operations ---

func TestSetProfileProperty(t *testing.T) {
	e := newTestEngine()
	e.Track(ctx(), &Event{EventName: "signup", DistinctID: "user_1"})

	e.SetProfileProperty("user_1", "tier", "gold")

	profile, _ := e.GetProfile("user_1")
	if profile.Properties["tier"] != "gold" {
		t.Errorf("tier = %v, want gold", profile.Properties["tier"])
	}
}

func TestIncrementProfileProperty(t *testing.T) {
	e := newTestEngine()
	e.Track(ctx(), &Event{EventName: "signup", DistinctID: "user_1"})

	e.IncrementProfileProperty("user_1", "login_count", 1)
	e.IncrementProfileProperty("user_1", "login_count", 1)
	e.IncrementProfileProperty("user_1", "login_count", 1)

	profile, _ := e.GetProfile("user_1")
	count, ok := profile.Properties["login_count"].(float64)
	if !ok || count != 3 {
		t.Errorf("login_count = %v, want 3", profile.Properties["login_count"])
	}
}

func TestDeleteProfile(t *testing.T) {
	e := newTestEngine()
	e.Identify(ctx(), &IdentifyCall{DistinctID: "user_1", Properties: map[string]any{"x": 1}})

	if !e.DeleteProfile("user_1") {
		t.Error("expected true for existing profile")
	}

	_, err := e.GetProfile("user_1")
	if err == nil {
		t.Error("profile should be deleted")
	}

	if e.DeleteProfile("user_1") {
		t.Error("expected false for already-deleted profile")
	}
}

// --- Segments ---

func TestSegment_PropertyFilter(t *testing.T) {
	e := newTestEngine()

	e.Identify(ctx(), &IdentifyCall{DistinctID: "u1", Properties: map[string]any{"plan": "pro", "age": 30}})
	e.Identify(ctx(), &IdentifyCall{DistinctID: "u2", Properties: map[string]any{"plan": "free", "age": 25}})
	e.Identify(ctx(), &IdentifyCall{DistinctID: "u3", Properties: map[string]any{"plan": "pro", "age": 40}})

	// Filter: plan = pro
	matched := e.MatchSegment(&SegmentFilter{
		PropertyFilters: []PropertyFilter{
			{Key: "plan", Operator: "eq", Value: "pro"},
		},
	})
	if len(matched) != 2 {
		t.Errorf("expected 2 pro users, got %d", len(matched))
	}

	// Filter: age > 28
	matched = e.MatchSegment(&SegmentFilter{
		PropertyFilters: []PropertyFilter{
			{Key: "age", Operator: "gt", Value: 28},
		},
	})
	if len(matched) != 2 {
		t.Errorf("expected 2 users age > 28, got %d", len(matched))
	}

	// Filter: plan = pro AND age >= 35
	matched = e.MatchSegment(&SegmentFilter{
		PropertyFilters: []PropertyFilter{
			{Key: "plan", Operator: "eq", Value: "pro"},
			{Key: "age", Operator: "gte", Value: 35},
		},
	})
	if len(matched) != 1 {
		t.Errorf("expected 1 user matching both filters, got %d", len(matched))
	}
}

func TestSegment_EventFilter(t *testing.T) {
	e := newTestEngine()

	e.Identify(ctx(), &IdentifyCall{DistinctID: "u1"})
	e.Identify(ctx(), &IdentifyCall{DistinctID: "u2"})
	e.Identify(ctx(), &IdentifyCall{DistinctID: "u3"})

	// u1 has 2 purchases, u2 has 1, u3 has 0.
	e.Track(ctx(), &Event{EventName: "purchase", DistinctID: "u1"})
	e.Track(ctx(), &Event{EventName: "purchase", DistinctID: "u1"})
	e.Track(ctx(), &Event{EventName: "purchase", DistinctID: "u2"})
	e.Track(ctx(), &Event{EventName: "page_view", DistinctID: "u3"})

	// Users who purchased at least once.
	matched := e.MatchSegment(&SegmentFilter{
		EventFilter: &EventFilter{EventName: "purchase"},
	})
	if len(matched) != 2 {
		t.Errorf("expected 2 users with purchases, got %d", len(matched))
	}

	// Users who purchased at least twice.
	matched = e.MatchSegment(&SegmentFilter{
		EventFilter: &EventFilter{EventName: "purchase", MinCount: 2},
	})
	if len(matched) != 1 {
		t.Errorf("expected 1 user with 2+ purchases, got %d", len(matched))
	}
}

func TestSegment_ContainsOperator(t *testing.T) {
	e := newTestEngine()
	e.Identify(ctx(), &IdentifyCall{DistinctID: "u1", Properties: map[string]any{"email": "alice@example.com"}})
	e.Identify(ctx(), &IdentifyCall{DistinctID: "u2", Properties: map[string]any{"email": "bob@other.com"}})

	matched := e.MatchSegment(&SegmentFilter{
		PropertyFilters: []PropertyFilter{
			{Key: "email", Operator: "contains", Value: "example.com"},
		},
	})
	if len(matched) != 1 {
		t.Errorf("expected 1 match, got %d", len(matched))
	}
}

func TestSegment_ExistsOperator(t *testing.T) {
	e := newTestEngine()
	e.Identify(ctx(), &IdentifyCall{DistinctID: "u1", Properties: map[string]any{"phone": "+1234"}})
	e.Identify(ctx(), &IdentifyCall{DistinctID: "u2", Properties: map[string]any{"email": "b@c.com"}})

	matched := e.MatchSegment(&SegmentFilter{
		PropertyFilters: []PropertyFilter{
			{Key: "phone", Operator: "exists"},
		},
	})
	if len(matched) != 1 {
		t.Errorf("expected 1 user with phone, got %d", len(matched))
	}
}

// --- Get Events ---

func TestGetEventsByName(t *testing.T) {
	e := newTestEngine()
	e.Track(ctx(), &Event{EventName: "click", DistinctID: "u1"})
	e.Track(ctx(), &Event{EventName: "page_view", DistinctID: "u1"})
	e.Track(ctx(), &Event{EventName: "click", DistinctID: "u2"})

	clicks := e.GetEventsByName("click")
	if len(clicks) != 2 {
		t.Errorf("expected 2 clicks, got %d", len(clicks))
	}
}

func TestGetEvents_All(t *testing.T) {
	e := newTestEngine()
	e.Track(ctx(), &Event{EventName: "a", DistinctID: "u1"})
	e.Track(ctx(), &Event{EventName: "b", DistinctID: "u2"})

	all := e.GetEvents("")
	if len(all) != 2 {
		t.Errorf("expected 2 events, got %d", len(all))
	}
}

// --- Hooks ---

type testHooks struct {
	NoOpEventsHooks
	ingested   int
	identified int
	merged     int
}

func (h *testHooks) OnEventIngested(_ context.Context, _ *Event) error {
	h.ingested++
	return nil
}
func (h *testHooks) OnIdentify(_ context.Context, _ *UserProfile) error {
	h.identified++
	return nil
}
func (h *testHooks) OnMerge(_ context.Context, _ *UserProfile, _ string) error {
	h.merged++
	return nil
}

func TestHooks_CalledOnOperations(t *testing.T) {
	hooks := &testHooks{}
	e := newTestEngine(WithHooks(hooks))

	e.Track(ctx(), &Event{EventName: "click", DistinctID: "u1"})
	if hooks.ingested != 1 {
		t.Errorf("ingested = %d, want 1", hooks.ingested)
	}

	e.Identify(ctx(), &IdentifyCall{DistinctID: "u1", Properties: map[string]any{"x": 1}})
	if hooks.identified != 1 {
		t.Errorf("identified = %d, want 1", hooks.identified)
	}

	e.Identify(ctx(), &IdentifyCall{DistinctID: "u1", AnonymousID: "anon_1"})
	if hooks.merged != 1 {
		t.Errorf("merged = %d, want 1", hooks.merged)
	}
}

// --- Clock ---

func TestClock_UsedForTimestamps(t *testing.T) {
	clock := state.NewClock()
	e := NewEngine(WithClock(clock))

	e.Track(ctx(), &Event{EventName: "first", DistinctID: "u1"})
	first := e.GetEvents("u1")[0].Timestamp

	clock.Advance(60_000_000_000) // 1 minute

	e.Track(ctx(), &Event{EventName: "second", DistinctID: "u1"})
	second := e.GetEvents("u1")[1].Timestamp

	if !second.After(first) {
		t.Error("second event should be after first when clock advances")
	}
}

// --- Thread Safety ---

func TestConcurrentTrack(t *testing.T) {
	e := newTestEngine()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.Track(ctx(), &Event{EventName: "click", DistinctID: "u1"})
		}()
	}
	wg.Wait()

	if e.EventCount() != 100 {
		t.Errorf("expected 100 events, got %d", e.EventCount())
	}
}
