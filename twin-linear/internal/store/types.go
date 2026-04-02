// Package store defines the Linear twin's state types and in-memory store.
package store

// Issue is the core work item in Linear.
type Issue struct {
	ID          string  `json:"id"`
	Identifier  string  `json:"identifier"` // "ENG-123"
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	Priority    int     `json:"priority"` // 0=none, 1=urgent, 2=high, 3=medium, 4=low
	Estimate    float64 `json:"estimate,omitempty"`
	SortOrder   float64 `json:"sortOrder"`
	BoardOrder  float64 `json:"boardOrder"`
	DueDate     string  `json:"dueDate,omitempty"`
	TeamID      string  `json:"teamId"`       // internal: team reference
	StateID     string  `json:"stateId"`      // internal: workflow state
	AssigneeID  string  `json:"assigneeId,omitempty"`
	ProjectID   string  `json:"projectId,omitempty"`
	CycleID     string  `json:"cycleId,omitempty"`
	ParentID    string  `json:"parentId,omitempty"`
	LabelIDs    []string `json:"labelIds,omitempty"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
	ArchivedAt  string  `json:"archivedAt,omitempty"`
	Number      int     `json:"number"`
	URL         string  `json:"url"`
}

// Team is an organizational unit. Issues belong to teams.
type Team struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Key         string `json:"key"` // e.g., "ENG"
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// WorkflowState represents an issue status.
type WorkflowState struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // "backlog", "unstarted", "started", "completed", "canceled", "triage"
	Color    string `json:"color"`
	Position float64 `json:"position"`
	TeamID   string `json:"teamId"`
}

// User represents a Linear workspace member.
type User struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	Active      bool   `json:"active"`
	Admin       bool   `json:"admin"`
	AvatarURL   string `json:"avatarUrl,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// Project is a cross-team initiative.
type Project struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	State       string  `json:"state"` // "backlog", "planned", "started", "paused", "completed", "canceled"
	Progress    float64 `json:"progress"`
	SlugID      string  `json:"slugId"`
	URL         string  `json:"url"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
	ArchivedAt  string  `json:"archivedAt,omitempty"`
}

// Cycle is a time-boxed iteration (sprint).
type Cycle struct {
	ID        string `json:"id"`
	Number    int    `json:"number"`
	Name      string `json:"name,omitempty"`
	StartsAt  string `json:"startsAt"`
	EndsAt    string `json:"endsAt"`
	TeamID    string `json:"teamId"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// IssueLabel is a color-coded label.
type IssueLabel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
	ParentID    string `json:"parentId,omitempty"` // for sub-labels
	TeamID      string `json:"teamId,omitempty"`   // team-scoped or workspace-global
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// Comment represents an issue comment.
type Comment struct {
	ID        string `json:"id"`
	Body      string `json:"body"` // markdown
	IssueID   string `json:"issueId"`
	UserID    string `json:"userId"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Attachment is an external link attached to an issue.
type Attachment struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle,omitempty"`
	URL       string `json:"url"`
	IssueID   string `json:"issueId"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// IssueRelation is a relation between two issues.
type IssueRelation struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // "blocks", "blocked_by", "related", "duplicate"
	IssueID   string `json:"issueId"`
	RelatedID string `json:"relatedIssueId"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Document is a long-form document.
type Document struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ProjectUpdate is a status update on a project.
type ProjectUpdate struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	Health    string `json:"health"` // "onTrack", "atRisk", "offTrack"
	ProjectID string `json:"projectId"`
	UserID    string `json:"userId"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Webhook represents a Linear webhook subscription.
type Webhook struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	URL       string `json:"url"`
	Enabled   bool   `json:"enabled"`
	TeamID    string `json:"teamId,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Reaction is an emoji reaction on a comment or project update.
type Reaction struct {
	ID        string `json:"id"`
	Emoji     string `json:"emoji"`
	UserID    string `json:"userId"`
	CommentID string `json:"commentId,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// Organization represents the workspace.
type Organization struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URLKey    string `json:"urlKey"`
	CreatedAt string `json:"createdAt"`
}

// Notification represents a user notification.
type Notification struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	ReadAt    string `json:"readAt,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Favorite represents a user bookmark.
type Favorite struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // "issue", "project", "cycle", "customView"
	IssueID   string `json:"issueId,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}
