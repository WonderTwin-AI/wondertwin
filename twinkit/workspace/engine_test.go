package workspace

import (
	"context"
	"sync"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
)

func newTestEngine(opts ...Option) *Engine {
	clock := state.NewClock()
	defaults := []Option{WithClock(clock)}
	return NewEngine(append(defaults, opts...)...)
}

func ctx() context.Context {
	return context.Background()
}

// --- CRUD + Events ---

func TestCreateEntity(t *testing.T) {
	e := newTestEngine()

	entity := &Entity{Type: "channel", Properties: map[string]any{"name": "general"}}
	event, err := e.CreateEntity(ctx(), entity)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if entity.ID == "" {
		t.Error("expected ID to be assigned")
	}
	if event.Action != "created" {
		t.Errorf("action = %q, want created", event.Action)
	}
	if event.EntityType != "channel" {
		t.Errorf("entity_type = %q, want channel", event.EntityType)
	}
	if event.EntityID != entity.ID {
		t.Errorf("entity_id = %q, want %q", event.EntityID, entity.ID)
	}

	// Verify stored.
	got, err := e.GetEntity("channel", entity.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Properties["name"] != "general" {
		t.Errorf("name = %v, want general", got.Properties["name"])
	}
}

func TestCreateEntity_RequiresType(t *testing.T) {
	e := newTestEngine()
	_, err := e.CreateEntity(ctx(), &Entity{})
	if err == nil {
		t.Error("expected error for missing type")
	}
}

func TestCreateEntity_PreservesProvidedID(t *testing.T) {
	e := newTestEngine()
	entity := &Entity{ID: "custom-id", Type: "issue"}
	_, err := e.CreateEntity(ctx(), entity)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if entity.ID != "custom-id" {
		t.Errorf("ID = %q, want custom-id", entity.ID)
	}
}

func TestUpdateEntity(t *testing.T) {
	e := newTestEngine()

	entity := &Entity{Type: "issue", Properties: map[string]any{"title": "Bug", "priority": "high"}}
	e.CreateEntity(ctx(), entity)

	updated, event, err := e.UpdateEntity(ctx(), "issue", entity.ID, map[string]any{"title": "Feature"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Properties["title"] != "Feature" {
		t.Errorf("title = %v, want Feature", updated.Properties["title"])
	}
	if updated.Properties["priority"] != "high" {
		t.Errorf("priority = %v, want high (unchanged)", updated.Properties["priority"])
	}
	if event.Action != "updated" {
		t.Errorf("action = %q, want updated", event.Action)
	}
}

func TestUpdateEntity_ChangeDetection(t *testing.T) {
	e := newTestEngine()

	entity := &Entity{Type: "issue", Properties: map[string]any{"title": "Old", "status": "open"}}
	e.CreateEntity(ctx(), entity)

	_, event, err := e.UpdateEntity(ctx(), "issue", entity.ID, map[string]any{"title": "New"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	// Changes should only contain "title", not "status".
	if _, ok := event.Changes["title"]; !ok {
		t.Error("expected title in changes")
	}
	if _, ok := event.Changes["status"]; ok {
		t.Error("status should not be in changes (unchanged)")
	}
}

func TestUpdateEntity_NotFound(t *testing.T) {
	e := newTestEngine()
	_, _, err := e.UpdateEntity(ctx(), "issue", "nonexistent", map[string]any{})
	if err == nil {
		t.Error("expected error for not found")
	}
}

func TestDeleteEntity(t *testing.T) {
	e := newTestEngine()

	entity := &Entity{Type: "channel", Properties: map[string]any{"name": "temp"}}
	e.CreateEntity(ctx(), entity)

	event, err := e.DeleteEntity(ctx(), "channel", entity.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if event.Action != "deleted" {
		t.Errorf("action = %q, want deleted", event.Action)
	}

	_, err = e.GetEntity("channel", entity.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestDeleteEntity_CleansUpCommentsAndMembers(t *testing.T) {
	e := newTestEngine()

	entity := &Entity{Type: "channel", Properties: map[string]any{"name": "temp"}}
	e.CreateEntity(ctx(), entity)

	e.AddComment(ctx(), "channel", entity.ID, &Comment{AuthorID: "u1", Body: "hello"})
	e.AddMember(ctx(), entity.ID, "u1", "member")

	e.DeleteEntity(ctx(), "channel", entity.ID)

	if len(e.ListComments(entity.ID)) != 0 {
		t.Error("expected comments to be cleaned up")
	}
	if len(e.ListMembers(entity.ID)) != 0 {
		t.Error("expected members to be cleaned up")
	}
}

// --- Workflow ---

func TestWorkflow_EnforcesTransitions(t *testing.T) {
	e := newTestEngine(WithWorkflow(WorkflowConfig{
		EntityType:    "issue",
		InitialStatus: "open",
		Transitions: map[string][]string{
			"open":   {"closed"},
			"closed": {"open"},
		},
	}))

	entity := &Entity{Type: "issue", Properties: map[string]any{"title": "Test"}}
	e.CreateEntity(ctx(), entity)

	if entity.Status != "open" {
		t.Errorf("initial status = %q, want open", entity.Status)
	}

	// Valid transition: open → closed.
	updated, event, err := e.TransitionStatus(ctx(), "issue", entity.ID, "closed")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if updated.Status != "closed" {
		t.Errorf("status = %q, want closed", updated.Status)
	}
	if event.Action != "status_changed" {
		t.Errorf("action = %q, want status_changed", event.Action)
	}
	if event.OldStatus != "open" || event.NewStatus != "closed" {
		t.Errorf("status change = %q→%q, want open→closed", event.OldStatus, event.NewStatus)
	}

	// Invalid transition: closed → in_progress (not defined).
	_, _, err = e.TransitionStatus(ctx(), "issue", entity.ID, "in_progress")
	if err == nil {
		t.Error("expected error for invalid transition")
	}

	// Valid reopen: closed → open.
	_, _, err = e.TransitionStatus(ctx(), "issue", entity.ID, "open")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
}

func TestWorkflow_TerminalState(t *testing.T) {
	e := newTestEngine(WithWorkflow(WorkflowConfig{
		EntityType:    "deal",
		InitialStatus: "open",
		Transitions: map[string][]string{
			"open": {"won", "lost"},
		},
		TerminalStates: []string{"won", "lost"},
	}))

	entity := &Entity{Type: "deal", Properties: map[string]any{}}
	e.CreateEntity(ctx(), entity)
	e.TransitionStatus(ctx(), "deal", entity.ID, "won")

	// Cannot transition from terminal state.
	_, _, err := e.TransitionStatus(ctx(), "deal", entity.ID, "open")
	if err == nil {
		t.Error("expected error transitioning from terminal state")
	}
}

func TestWorkflow_NoWorkflowAllowsAnyStatus(t *testing.T) {
	e := newTestEngine() // No workflow registered

	entity := &Entity{Type: "task", Properties: map[string]any{}}
	e.CreateEntity(ctx(), entity)

	// Should accept any status when no workflow is configured.
	_, _, err := e.TransitionStatus(ctx(), "task", entity.ID, "whatever")
	if err != nil {
		t.Fatalf("expected no error with no workflow, got: %v", err)
	}
}

// --- Container Hierarchy ---

func TestListByParent(t *testing.T) {
	e := newTestEngine()

	e.CreateEntity(ctx(), &Entity{ID: "ws1", Type: "workspace"})
	e.CreateEntity(ctx(), &Entity{ID: "ch1", Type: "channel", ParentID: "ws1"})
	e.CreateEntity(ctx(), &Entity{ID: "ch2", Type: "channel", ParentID: "ws1"})
	e.CreateEntity(ctx(), &Entity{ID: "ch3", Type: "channel", ParentID: "ws2"}) // different parent

	children := e.ListByParent("channel", "ws1")
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}

	children = e.ListChildren("workspace", "ws1", "channel")
	if len(children) != 2 {
		t.Errorf("ListChildren: expected 2, got %d", len(children))
	}
}

func TestMoveEntity(t *testing.T) {
	e := newTestEngine()

	e.CreateEntity(ctx(), &Entity{ID: "ch1", Type: "channel", ParentID: "ws1", ParentType: "workspace"})

	event, err := e.MoveEntity(ctx(), "channel", "ch1", "workspace", "ws2")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if event.Action != "updated" {
		t.Errorf("action = %q, want updated", event.Action)
	}

	got, _ := e.GetEntity("channel", "ch1")
	if got.ParentID != "ws2" {
		t.Errorf("ParentID = %q, want ws2", got.ParentID)
	}
}

// --- Comments ---

func TestComments_CRUD(t *testing.T) {
	e := newTestEngine()

	e.CreateEntity(ctx(), &Entity{ID: "issue1", Type: "issue"})

	// Add comment.
	event, err := e.AddComment(ctx(), "issue", "issue1", &Comment{AuthorID: "user1", Body: "looks good"})
	if err != nil {
		t.Fatalf("add comment: %v", err)
	}
	if event.Action != "comment_created" {
		t.Errorf("action = %q, want comment_created", event.Action)
	}

	comments := e.ListComments("issue1")
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Body != "looks good" {
		t.Errorf("body = %v, want 'looks good'", comments[0].Body)
	}
	commentID := comments[0].ID

	// Update comment.
	event, err = e.UpdateComment(ctx(), commentID, "actually, needs changes")
	if err != nil {
		t.Fatalf("update comment: %v", err)
	}
	if event.Action != "comment_updated" {
		t.Errorf("action = %q, want comment_updated", event.Action)
	}

	comments = e.ListComments("issue1")
	if comments[0].Body != "actually, needs changes" {
		t.Errorf("body after update = %v", comments[0].Body)
	}

	// Delete comment.
	event, err = e.DeleteComment(ctx(), commentID)
	if err != nil {
		t.Fatalf("delete comment: %v", err)
	}
	if event.Action != "comment_deleted" {
		t.Errorf("action = %q, want comment_deleted", event.Action)
	}

	comments = e.ListComments("issue1")
	if len(comments) != 0 {
		t.Errorf("expected 0 comments after delete, got %d", len(comments))
	}
}

func TestComments_NotFoundEntity(t *testing.T) {
	e := newTestEngine()
	_, err := e.AddComment(ctx(), "issue", "nonexistent", &Comment{Body: "hello"})
	if err == nil {
		t.Error("expected error for nonexistent entity")
	}
}

// --- Membership ---

func TestMembership_AddRemoveList(t *testing.T) {
	e := newTestEngine()

	e.CreateEntity(ctx(), &Entity{ID: "ch1", Type: "channel"})

	// Add members.
	event, err := e.AddMember(ctx(), "ch1", "user1", "admin")
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
	if event.Action != "member_added" {
		t.Errorf("action = %q, want member_added", event.Action)
	}

	e.AddMember(ctx(), "ch1", "user2", "member")

	// List.
	members := e.ListMembers("ch1")
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	// IsMember.
	if !e.IsMember("ch1", "user1") {
		t.Error("user1 should be a member")
	}
	if e.IsMember("ch1", "user3") {
		t.Error("user3 should not be a member")
	}

	// Remove.
	event, err = e.RemoveMember(ctx(), "ch1", "user1")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if event.Action != "member_removed" {
		t.Errorf("action = %q, want member_removed", event.Action)
	}

	if e.IsMember("ch1", "user1") {
		t.Error("user1 should no longer be a member")
	}
	if len(e.ListMembers("ch1")) != 1 {
		t.Error("expected 1 member remaining")
	}
}

func TestMembership_DuplicateRejected(t *testing.T) {
	e := newTestEngine()
	e.AddMember(ctx(), "ch1", "user1", "member")
	_, err := e.AddMember(ctx(), "ch1", "user1", "admin")
	if err == nil {
		t.Error("expected error for duplicate membership")
	}
}

func TestMembership_RemoveNonMember(t *testing.T) {
	e := newTestEngine()
	_, err := e.RemoveMember(ctx(), "ch1", "user1")
	if err == nil {
		t.Error("expected error removing non-member")
	}
}

// --- Hooks ---

type testHooks struct {
	NoOpWorkspaceHooks
	created    int
	updated    int
	deleted    int
	transitioned int
	commented  int
}

func (h *testHooks) OnEntityCreated(_ context.Context, _ *Entity) error {
	h.created++
	return nil
}
func (h *testHooks) OnEntityUpdated(_ context.Context, _ *Entity, _ map[string]any) error {
	h.updated++
	return nil
}
func (h *testHooks) OnEntityDeleted(_ context.Context, _, _ string) error {
	h.deleted++
	return nil
}
func (h *testHooks) OnStatusTransition(_ context.Context, _ *Entity, _, _ string) error {
	h.transitioned++
	return nil
}
func (h *testHooks) OnCommentAdded(_ context.Context, _ *Entity, _ *Comment) error {
	h.commented++
	return nil
}

func TestHooks_CalledOnOperations(t *testing.T) {
	hooks := &testHooks{}
	e := newTestEngine(
		WithHooks(hooks),
		WithWorkflow(WorkflowConfig{
			EntityType:    "issue",
			InitialStatus: "open",
			Transitions:   map[string][]string{"open": {"closed"}},
		}),
	)

	entity := &Entity{Type: "issue", Properties: map[string]any{"title": "Test"}}
	e.CreateEntity(ctx(), entity)
	if hooks.created != 1 {
		t.Errorf("created hooks = %d, want 1", hooks.created)
	}

	e.UpdateEntity(ctx(), "issue", entity.ID, map[string]any{"title": "Updated"})
	if hooks.updated != 1 {
		t.Errorf("updated hooks = %d, want 1", hooks.updated)
	}

	e.TransitionStatus(ctx(), "issue", entity.ID, "closed")
	if hooks.transitioned != 1 {
		t.Errorf("transitioned hooks = %d, want 1", hooks.transitioned)
	}

	e.AddComment(ctx(), "issue", entity.ID, &Comment{Body: "test"})
	if hooks.commented != 1 {
		t.Errorf("commented hooks = %d, want 1", hooks.commented)
	}

	e.DeleteEntity(ctx(), "issue", entity.ID)
	if hooks.deleted != 1 {
		t.Errorf("deleted hooks = %d, want 1", hooks.deleted)
	}
}

// --- Filter ---

func TestFilterEntities(t *testing.T) {
	e := newTestEngine()

	e.CreateEntity(ctx(), &Entity{Type: "issue", Properties: map[string]any{"priority": "high"}})
	e.CreateEntity(ctx(), &Entity{Type: "issue", Properties: map[string]any{"priority": "low"}})
	e.CreateEntity(ctx(), &Entity{Type: "issue", Properties: map[string]any{"priority": "high"}})

	high := e.FilterEntities("issue", func(ent *Entity) bool {
		return ent.Properties["priority"] == "high"
	})
	if len(high) != 2 {
		t.Errorf("expected 2 high priority issues, got %d", len(high))
	}
}

// --- Clock ---

func TestClock_UsedForTimestamps(t *testing.T) {
	clock := state.NewClock()
	e := NewEngine(WithClock(clock))

	entity := &Entity{Type: "task", Properties: map[string]any{}}
	e.CreateEntity(ctx(), entity)

	createdAt := entity.CreatedAt

	clock.Advance(60_000_000_000) // 1 minute

	e.UpdateEntity(ctx(), "task", entity.ID, map[string]any{"done": true})

	got, _ := e.GetEntity("task", entity.ID)
	if !got.UpdatedAt.After(createdAt) {
		t.Error("UpdatedAt should be after CreatedAt when clock advances")
	}
}

// --- Thread Safety ---

func TestConcurrentCRUD(t *testing.T) {
	e := newTestEngine()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entity := &Entity{Type: "task", Properties: map[string]any{"n": 1}}
			e.CreateEntity(ctx(), entity)
		}()
	}
	wg.Wait()

	tasks := e.ListEntities("task")
	if len(tasks) != 100 {
		t.Errorf("expected 100 tasks, got %d", len(tasks))
	}
}
