# twinkit/workspace — Specification

## Overview

The workspace engine provides the shared behavioral core for twins that simulate **stateful collaboration platforms**: Slack, GitHub, Linear, HubSpot, Jira, Notion, Discord, and similar. It handles entity CRUD with automatic event emission, hierarchical containment, configurable workflow state machines, and threaded conversations.

This primitive is the most leveraged in the twinkit architecture — it also serves commerce (Shopify), people tech (Rippling, BambooHR), and security tech (Okta, PagerDuty, Vanta) twins.

## Design principles

1. **Generic entities, typed APIs.** The engine works with `Entity` (a property bag). Each twin maps its domain types (Channel, Issue, Contact) to/from Entity in its handlers. This keeps the engine simple while supporting wildly different platform shapes.

2. **Every mutation emits an event.** This is the engine's core invariant. Create, update, delete, and status transitions all produce a `MutationEvent` that flows to the webhook dispatcher. Twins format these events into platform-specific payloads via hooks.

3. **Follow the ledger engine's conventions.** Functional options for configuration. Hooks interface with NoOp embed. Thread-safe with mutex. Shared `state.Clock`. Context-passing on all operations.

4. **Don't model permissions.** Most integrations run as service accounts with full access. The engine tracks membership (who's in what container) but does not enforce authorization. Twins that need permission checks can do so in their hooks.

5. **Don't model content.** Block Kit (Slack), Markdown (GitHub), and rich text (Notion) are platform-specific. The engine stores content as opaque `any` — each twin handles its own content format.

## Package structure

```
twinkit/workspace/
├── interface.go       # Types: Entity, MutationEvent, Comment, Member, WorkflowConfig
├── engine.go          # Engine: CRUD operations, event emission, options
├── workflow.go        # State machine: transition validation, configuration
├── container.go       # Parent-child containment: hierarchy, cascading operations
├── comments.go        # Threaded conversations attached to entities
├── membership.go      # Container membership tracking
├── hooks.go           # WorkspaceHooks interface + NoOpWorkspaceHooks
└── engine_test.go     # Core engine tests
```

## Types

### Entity

The core data type. Every workspace object (channel, issue, contact, PR, deal) is an Entity.

```go
type Entity struct {
    ID         string         `json:"id"`
    Type       string         `json:"type"`          // "channel", "issue", "contact", etc.
    ParentID   string         `json:"parent_id"`     // container this entity belongs to
    ParentType string         `json:"parent_type"`   // type of the parent entity
    Status     string         `json:"status"`        // workflow state (if applicable)
    Properties map[string]any `json:"properties"`    // platform-specific fields
    CreatedAt  time.Time      `json:"created_at"`
    UpdatedAt  time.Time      `json:"updated_at"`
    CreatedBy  string         `json:"created_by,omitempty"`
}
```

**Why `Properties map[string]any` instead of typed fields?**

A Slack channel has `name`, `topic`, `purpose`, `is_archived`. A GitHub issue has `title`, `body`, `labels`, `assignees`, `milestone`. A HubSpot deal has `dealname`, `amount`, `pipeline`, `dealstage`, `closedate`. These share no common fields. The engine doesn't need to understand them — it needs to store them, detect changes, and emit events.

Each twin wraps this with typed structs:
```go
// In twin-slack's handlers:
func channelToEntity(ch SlackChannel) workspace.Entity {
    return workspace.Entity{
        ID:   ch.ID,
        Type: "channel",
        Properties: map[string]any{
            "name":    ch.Name,
            "topic":   ch.Topic,
            "purpose": ch.Purpose,
        },
    }
}
```

### MutationEvent

Emitted on every entity mutation. This is the engine's primary output — it feeds into the webhook dispatcher.

```go
type MutationEvent struct {
    Action     string         `json:"action"`      // "created", "updated", "deleted", "status_changed"
    EntityType string         `json:"entity_type"` // "channel", "issue", etc.
    EntityID   string         `json:"entity_id"`
    Changes    map[string]any `json:"changes,omitempty"`     // for updates: old → new values
    OldStatus  string         `json:"old_status,omitempty"`  // for status changes
    NewStatus  string         `json:"new_status,omitempty"`  // for status changes
    Actor      string         `json:"actor,omitempty"`
    Timestamp  time.Time      `json:"timestamp"`
}
```

**Change tracking:** On updates, the engine compares old and new Properties maps and records which fields changed. This enables twins to format targeted webhook payloads (e.g., GitHub's `issues.labeled` vs `issues.assigned` based on what changed).

### Comment

A threaded reply attached to an entity.

```go
type Comment struct {
    ID        string    `json:"id"`
    EntityID  string    `json:"entity_id"`   // parent entity
    AuthorID  string    `json:"author_id"`
    Body      any       `json:"body"`         // opaque — string, markdown, blocks
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

Comments are a first-class concept because they exist on every workspace platform and always follow the same pattern: ordered list attached to a parent entity, with CRUD that emits events.

### Member

Membership in a container.

```go
type Member struct {
    UserID      string    `json:"user_id"`
    ContainerID string    `json:"container_id"`
    Role        string    `json:"role,omitempty"` // "admin", "member", "guest", etc.
    JoinedAt    time.Time `json:"joined_at"`
}
```

### WorkflowConfig

Defines the state machine for an entity type.

```go
type WorkflowConfig struct {
    EntityType     string              `json:"entity_type"`
    InitialStatus  string              `json:"initial_status"`
    Transitions    map[string][]string `json:"transitions"`      // from → [allowed to states]
    TerminalStates []string            `json:"terminal_states"`  // states with no outgoing transitions
}
```

Example configurations:
```go
// GitHub issue workflow
var IssueWorkflow = WorkflowConfig{
    EntityType:     "issue",
    InitialStatus:  "open",
    Transitions:    map[string][]string{
        "open":   {"closed"},
        "closed": {"open"},  // can reopen
    },
}

// Linear issue workflow (customizable per team)
var LinearWorkflow = WorkflowConfig{
    EntityType:     "issue",
    InitialStatus:  "backlog",
    Transitions:    map[string][]string{
        "backlog":     {"todo", "cancelled"},
        "todo":        {"in_progress", "cancelled"},
        "in_progress": {"done", "todo", "cancelled"},
        "done":        {"todo"},  // can reopen
        "cancelled":   {"backlog"},
    },
    TerminalStates: []string{},  // Linear has no truly terminal states
}

// HubSpot deal pipeline
var DealWorkflow = WorkflowConfig{
    EntityType:     "deal",
    InitialStatus:  "appointmentscheduled",
    Transitions:    map[string][]string{
        "appointmentscheduled": {"qualifiedtobuy", "closedlost"},
        "qualifiedtobuy":      {"presentationscheduled", "closedlost"},
        "presentationscheduled": {"decisionmakerboughtin", "closedlost"},
        "decisionmakerboughtin": {"closedwon", "closedlost"},
    },
    TerminalStates: []string{"closedwon", "closedlost"},
}
```

## Engine

### Construction

```go
type Engine struct {
    mu         sync.RWMutex
    entities   map[string]map[string]*Entity   // type → id → entity
    comments   map[string][]*Comment           // entityID → comments
    members    map[string][]*Member            // containerID → members
    workflows  map[string]*WorkflowConfig      // entityType → workflow
    hooks      WorkspaceHooks
    clock      *state.Clock
    idCounter  int
}

type Option func(*Engine)

func WithHooks(h WorkspaceHooks) Option
func WithClock(c *state.Clock) Option
func WithWorkflow(cfg WorkflowConfig) Option  // can be called multiple times

func NewEngine(opts ...Option) *Engine
```

### Operations

All operations are thread-safe and emit events.

#### CreateEntity

```go
func (e *Engine) CreateEntity(ctx context.Context, entity *Entity) (*MutationEvent, error)
```

- Assigns ID if empty (using `{type}_{counter}` format)
- Sets `CreatedAt` and `UpdatedAt` from clock
- Sets initial status from workflow config (if one exists for this entity type)
- Validates via hooks (`ValidateEntity`)
- Stores the entity
- Calls `hooks.OnEntityCreated`
- Returns a `MutationEvent` with action "created"

#### UpdateEntity

```go
func (e *Engine) UpdateEntity(ctx context.Context, entityType, entityID string, changes map[string]any) (*Entity, *MutationEvent, error)
```

- Fetches existing entity (error if not found)
- Computes diff between old and new Properties
- Applies changes to Properties
- Updates `UpdatedAt`
- Validates via hooks
- Calls `hooks.OnEntityUpdated`
- Returns updated entity and `MutationEvent` with action "updated" and changes map

**Why `changes map[string]any` instead of a full Entity?** This enables sparse updates (only change specified fields) and accurate change detection for webhook payloads. The twin's handler converts the incoming request to a changes map.

#### DeleteEntity

```go
func (e *Engine) DeleteEntity(ctx context.Context, entityType, entityID string) (*MutationEvent, error)
```

- Fetches existing entity (error if not found)
- Removes entity from store
- Removes associated comments and memberships
- Calls `hooks.OnEntityDeleted`
- Returns `MutationEvent` with action "deleted"

Some platforms soft-delete (archive) instead of hard-delete. This is a twin concern — the twin can update the status to "archived" via `TransitionStatus` instead of calling `DeleteEntity`.

#### TransitionStatus

```go
func (e *Engine) TransitionStatus(ctx context.Context, entityType, entityID, toStatus string) (*Entity, *MutationEvent, error)
```

- Fetches existing entity
- Validates transition against workflow config (error if not allowed)
- Updates Status field
- Calls `hooks.OnStatusTransition`
- Returns `MutationEvent` with action "status_changed", old_status, new_status

If no workflow is configured for the entity type, any status value is accepted (no validation).

#### GetEntity / ListEntities

```go
func (e *Engine) GetEntity(entityType, entityID string) (*Entity, error)
func (e *Engine) ListEntities(entityType string) []*Entity
func (e *Engine) ListByParent(entityType, parentID string) []*Entity
func (e *Engine) FilterEntities(entityType string, predicate func(*Entity) bool) []*Entity
```

Read operations. No events emitted, no hooks called. Thread-safe via RLock.

### Container operations

```go
// ListChildren returns all entities whose ParentID matches the given container.
func (e *Engine) ListChildren(parentType, parentID, childType string) []*Entity

// MoveEntity changes an entity's parent container.
func (e *Engine) MoveEntity(ctx context.Context, entityType, entityID, newParentType, newParentID string) (*MutationEvent, error)
```

No dedicated container type — any entity can be a container. Containment is expressed via `ParentID`/`ParentType` on child entities. This avoids a separate container registry while still supporting hierarchy queries.

### Comment operations

```go
func (e *Engine) AddComment(ctx context.Context, entityType, entityID string, comment *Comment) (*MutationEvent, error)
func (e *Engine) ListComments(entityID string) []*Comment
func (e *Engine) UpdateComment(ctx context.Context, commentID string, body any) (*MutationEvent, error)
func (e *Engine) DeleteComment(ctx context.Context, commentID string) (*MutationEvent, error)
```

Comment events use action "comment_created", "comment_updated", "comment_deleted" with the parent entity's type and ID in the event.

### Membership operations

```go
func (e *Engine) AddMember(ctx context.Context, containerID, userID, role string) (*MutationEvent, error)
func (e *Engine) RemoveMember(ctx context.Context, containerID, userID string) (*MutationEvent, error)
func (e *Engine) ListMembers(containerID string) []*Member
func (e *Engine) IsMember(containerID, userID string) bool
```

Membership events use action "member_added", "member_removed".

## Hooks

```go
type WorkspaceHooks interface {
    // ValidateEntity is called before create and update. Return error to reject.
    ValidateEntity(ctx context.Context, entity *Entity) error

    // OnEntityCreated is called after an entity is created and stored.
    OnEntityCreated(ctx context.Context, entity *Entity) error

    // OnEntityUpdated is called after an entity is updated.
    OnEntityUpdated(ctx context.Context, entity *Entity, changes map[string]any) error

    // OnEntityDeleted is called after an entity is deleted.
    OnEntityDeleted(ctx context.Context, entityType, entityID string) error

    // OnStatusTransition is called after a workflow status transition.
    OnStatusTransition(ctx context.Context, entity *Entity, from, to string) error

    // OnCommentAdded is called after a comment is added to an entity.
    OnCommentAdded(ctx context.Context, entity *Entity, comment *Comment) error

    // OnMemberAdded is called after a member is added to a container.
    OnMemberAdded(ctx context.Context, containerID, userID, role string) error

    // OnMemberRemoved is called after a member is removed from a container.
    OnMemberRemoved(ctx context.Context, containerID, userID string) error

    // FormatEvent converts a MutationEvent into a platform-specific webhook payload.
    // This is the primary hook for customizing webhook delivery.
    FormatEvent(event *MutationEvent) any
}

// NoOpWorkspaceHooks provides default no-op implementations.
type NoOpWorkspaceHooks struct{}
```

### Hook usage pattern (example: twin-slack)

```go
type SlackHooks struct {
    workspace.NoOpWorkspaceHooks
    Dispatcher *webhook.Dispatcher
}

func (h *SlackHooks) FormatEvent(event *workspace.MutationEvent) any {
    // Convert generic MutationEvent to Slack Events API format.
    switch event.EntityType {
    case "message":
        return map[string]any{
            "type":    "event_callback",
            "event":   map[string]any{
                "type":    "message",
                "subtype": eventActionToSlackSubtype(event.Action),
                "channel": event.Properties["channel_id"],
                "user":    event.Actor,
                "text":    event.Properties["text"],
                "ts":      fmt.Sprintf("%d.%06d", event.Timestamp.Unix(), event.Timestamp.Nanosecond()/1000),
            },
        }
    // ... other entity types
    }
    return event
}

func (h *SlackHooks) OnEntityCreated(ctx context.Context, entity *workspace.Entity) error {
    if h.Dispatcher == nil {
        return nil
    }
    event := &workspace.MutationEvent{
        Action:     "created",
        EntityType: entity.Type,
        EntityID:   entity.ID,
        Timestamp:  entity.CreatedAt,
    }
    h.Dispatcher.Enqueue(entity.Type+".created", h.FormatEvent(event))
    return nil
}
```

## How twins wire in

```go
// twin-slack/cmd/twin-slack/main.go
func main() {
    cfg := twincore.ParseFlags("twin-slack")
    twin := twincore.New(cfg)
    memStore := store.New()

    dispatcher := webhook.NewDispatcher(webhook.Config{ ... })

    slackHooks := &hooks.SlackHooks{Dispatcher: dispatcher}
    engine := workspace.NewEngine(
        workspace.WithHooks(slackHooks),
        workspace.WithClock(memStore.Clock),
        workspace.WithWorkflow(workspace.WorkflowConfig{
            EntityType:    "channel",
            InitialStatus: "active",
            Transitions:   map[string][]string{
                "active":   {"archived"},
                "archived": {"active"},
            },
        }),
    )

    apiHandler := api.NewHandler(memStore, engine, dispatcher, twin.Middleware())
    apiHandler.Routes(twin.Router)
    // ... admin, seed, serve
}
```

## What the engine does NOT do

1. **Content rendering** — no Block Kit, Markdown, or rich text processing. Content is stored as opaque `any`.
2. **Permission enforcement** — membership is tracked but not enforced. Twins that need auth checks do so in handlers or hooks.
3. **Search/query language** — the engine provides `ListEntities`, `ListByParent`, `FilterEntities`. Platform-specific query languages (JQL, GraphQL filters) are implemented in twin handlers.
4. **Pagination** — the engine returns full lists. Twins handle pagination in their API handlers using the existing `twinkit/state` pagination helpers or their own cursor logic.
5. **ID format** — the engine generates fallback IDs (`{type}_{counter}`) but twins typically assign their own IDs in the format their platform uses (UUIDs for Slack, numeric for GitHub, nanoids for Linear).

## Comparison with ledger engine

| Dimension | `twinkit/ledger` | `twinkit/workspace` |
|-----------|-----------------|---------------------|
| Data model | Strongly typed (Account, Document, Payment) | Generic (Entity with Properties map) |
| Core invariant | Balanced journal entries | Every mutation emits an event |
| State machine | Fixed (DRAFT→SUBMITTED→AUTHORISED→PAID) | Configurable per entity type |
| Domain logic | Debit/credit rules, payment application, reports | Container hierarchy, membership, change detection |
| Hook surface | 3 hooks (journal created, state transition, payment) | 8 hooks (validate, created, updated, deleted, status, comment, member add/remove) |
| Complexity | Accounting rules | Event fidelity and format |

## Test plan

The engine tests should verify:

1. **CRUD + events**: Create/update/delete entity → correct MutationEvent emitted
2. **Change detection**: Update entity with subset of Properties → only changed fields appear in event
3. **Workflow enforcement**: Status transition against configured workflow → allowed/rejected correctly
4. **Workflow bypass**: Entity type with no workflow → any status accepted
5. **Container hierarchy**: ListChildren returns correct subset; MoveEntity emits event
6. **Comments**: Add/update/delete comments → events emitted with parent entity context
7. **Membership**: Add/remove members → events emitted; IsMember returns correct result
8. **Hooks called**: All hooks invoked at correct points; hook errors propagate
9. **Thread safety**: Concurrent CRUD operations don't corrupt state
10. **Clock**: All timestamps use the shared clock, not real time
