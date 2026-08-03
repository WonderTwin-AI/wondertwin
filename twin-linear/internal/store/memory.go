package store

import (
	"encoding/json"
	"fmt"
	"sync/atomic"

	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all Linear twin state in memory.
type MemoryStore struct {
	Issues         *pkgstate.Store[Issue]
	Teams          *pkgstate.Store[Team]
	WorkflowStates *pkgstate.Store[WorkflowState]
	Users          *pkgstate.Store[User]
	Projects       *pkgstate.Store[Project]
	Cycles         *pkgstate.Store[Cycle]
	Labels         *pkgstate.Store[IssueLabel]
	Comments       *pkgstate.Store[Comment]
	Attachments    *pkgstate.Store[Attachment]
	IssueRelations *pkgstate.Store[IssueRelation]
	Documents      *pkgstate.Store[Document]
	ProjectUpdates *pkgstate.Store[ProjectUpdate]
	Webhooks       *pkgstate.Store[Webhook]
	Reactions      *pkgstate.Store[Reaction]
	Notifications  *pkgstate.Store[Notification]
	Favorites      *pkgstate.Store[Favorite]
	Clock          *pkgstate.Clock

	Org Organization

	idCounter    atomic.Int64
	issueCounter atomic.Int64
	cycleCounter atomic.Int64
}

func New() *MemoryStore {
	return &MemoryStore{
		Issues:         pkgstate.New[Issue]("iss"),
		Teams:          pkgstate.New[Team]("team"),
		WorkflowStates: pkgstate.New[WorkflowState]("ws"),
		Users:          pkgstate.New[User]("user"),
		Projects:       pkgstate.New[Project]("proj"),
		Cycles:         pkgstate.New[Cycle]("cycle"),
		Labels:         pkgstate.New[IssueLabel]("label"),
		Comments:       pkgstate.New[Comment]("comment"),
		Attachments:    pkgstate.New[Attachment]("attach"),
		IssueRelations: pkgstate.New[IssueRelation]("rel"),
		Documents:      pkgstate.New[Document]("doc"),
		ProjectUpdates: pkgstate.New[ProjectUpdate]("pu"),
		Webhooks:       pkgstate.New[Webhook]("wh"),
		Reactions:      pkgstate.New[Reaction]("rx"),
		Notifications:  pkgstate.New[Notification]("notif"),
		Favorites:      pkgstate.New[Favorite]("fav"),
		Clock:          pkgstate.NewClock(),
		Org: Organization{
			ID:     "org_001",
			Name:   "WonderTwin",
			URLKey: "wondertwin",
		},
	}
}

func (s *MemoryStore) NextID() string {
	n := s.idCounter.Add(1)
	return fmt.Sprintf("id_%d", n)
}

func (s *MemoryStore) NextIssueNumber() int {
	return int(s.issueCounter.Add(1))
}

func (s *MemoryStore) NextCycleNumber() int {
	return int(s.cycleCounter.Add(1))
}

func (s *MemoryStore) Now() string {
	return s.Clock.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// Connection returns a Relay-style connection response.
func Connection[T any](items []T) map[string]any {
	nodes := make([]any, len(items))
	for i, item := range items {
		nodes[i] = item
	}
	return map[string]any{
		"nodes": nodes,
		"pageInfo": map[string]any{
			"hasNextPage":     false,
			"hasPreviousPage": false,
			"startCursor":     "",
			"endCursor":       "",
		},
	}
}

// MutationResult returns a Linear-style mutation response.
func MutationResult(success bool, entity any, key string) map[string]any {
	result := map[string]any{
		"success": success,
	}
	if key != "" && entity != nil {
		result[key] = entity
	}
	return result
}

type stateSnapshot struct {
	Issues         map[string]Issue         `json:"issues,omitempty"`
	Teams          map[string]Team          `json:"teams,omitempty"`
	WorkflowStates map[string]WorkflowState `json:"workflow_states,omitempty"`
	Users          map[string]User          `json:"users,omitempty"`
	Projects       map[string]Project       `json:"projects,omitempty"`
	Cycles         map[string]Cycle         `json:"cycles,omitempty"`
	Labels         map[string]IssueLabel    `json:"labels,omitempty"`
}

func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Issues:         s.Issues.Snapshot(),
		Teams:          s.Teams.Snapshot(),
		WorkflowStates: s.WorkflowStates.Snapshot(),
		Users:          s.Users.Snapshot(),
		Projects:       s.Projects.Snapshot(),
		Cycles:         s.Cycles.Snapshot(),
		Labels:         s.Labels.Snapshot(),
	}
}

func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.Issues != nil {
		s.Issues.LoadSnapshot(snap.Issues)
	}
	if snap.Teams != nil {
		s.Teams.LoadSnapshot(snap.Teams)
	}
	if snap.WorkflowStates != nil {
		s.WorkflowStates.LoadSnapshot(snap.WorkflowStates)
	}
	if snap.Users != nil {
		s.Users.LoadSnapshot(snap.Users)
	}
	if snap.Projects != nil {
		s.Projects.LoadSnapshot(snap.Projects)
	}
	if snap.Cycles != nil {
		s.Cycles.LoadSnapshot(snap.Cycles)
	}
	if snap.Labels != nil {
		s.Labels.LoadSnapshot(snap.Labels)
	}
	return nil
}

func (s *MemoryStore) Reset() {
	s.Issues.Reset()
	s.Teams.Reset()
	s.WorkflowStates.Reset()
	s.Users.Reset()
	s.Projects.Reset()
	s.Cycles.Reset()
	s.Labels.Reset()
	s.Comments.Reset()
	s.Attachments.Reset()
	s.IssueRelations.Reset()
	s.Documents.Reset()
	s.ProjectUpdates.Reset()
	s.Webhooks.Reset()
	s.Reactions.Reset()
	s.Notifications.Reset()
	s.Favorites.Reset()
	s.Clock.Reset()
	s.idCounter.Store(0)
	s.issueCounter.Store(0)
	s.cycleCounter.Store(0)
	s.Org = Organization{ID: "org_001", Name: "WonderTwin", URLKey: "wondertwin"}
}
