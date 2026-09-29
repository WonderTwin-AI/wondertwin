package api

import (
	"net/url"

	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

func (x renderer) repoID(owner, repo string) int64 {
	if rp, ok := x.h.store.GetRepo(owner, repo); ok {
		return rp.ID
	}
	return 0
}

// authorAssociation is OWNER for the repository owner and NONE otherwise.
func authorAssociation(owner, login string) string {
	if owner == login {
		return "OWNER"
	}
	return "NONE"
}

// reactions renders a reaction-rollup for the subject key.
func (x renderer) reactions(owner, repo, subject, u string) map[string]any {
	m := map[string]any{"url": u + "/reactions", "total_count": 0, "+1": 0, "-1": 0, "laugh": 0,
		"confused": 0, "heart": 0, "hooray": 0, "eyes": 0, "rocket": 0}
	for _, re := range x.h.store.ListSubjectReactions(owner, repo, subject) {
		if n, ok := m[re.Content].(int); ok {
			m[re.Content] = n + 1
			m["total_count"] = m["total_count"].(int) + 1
		}
	}
	return m
}

// label renders a label, reading its current definition from the
// repository when the stored copy on an issue is stale.
func (x renderer) label(owner, repo string, l store.Label) map[string]any {
	if cur, _, ok := x.h.store.GetLabel(owner, repo, l.Name); ok {
		l = *cur
	}
	return map[string]any{
		"id":          l.ID,
		"node_id":     store.NodeID("LA", x.repoID(owner, repo), l.ID),
		"url":         x.api("/repos/%s/%s/labels/%s", owner, repo, url.PathEscape(l.Name)),
		"name":        l.Name,
		"description": nullable(l.Description),
		"color":       l.Color,
		"default":     l.Default,
		"archived_at": nil,
		"archived_by": nil,
	}
}

func (x renderer) labels(owner, repo string, ls []store.Label) []map[string]any {
	out := make([]map[string]any, 0, len(ls))
	for _, l := range ls {
		out = append(out, x.label(owner, repo, l))
	}
	return out
}

func (x renderer) users(us []store.User) []map[string]any {
	out := make([]map[string]any, 0, len(us))
	for _, u := range us {
		out = append(out, x.user(u.Login))
	}
	return out
}

func (x renderer) milestone(owner, repo string, m *store.Milestone) any {
	if m == nil {
		return nil
	}
	base := x.api("/repos/%s/%s/milestones/%d", owner, repo, m.Number)
	open, closed := 0, 0
	for _, i := range x.h.store.ListRepoIssues(owner, repo, "all") {
		if i.Milestone != nil && i.Milestone.Number == m.Number {
			if i.State == "open" {
				open++
			} else {
				closed++
			}
		}
	}
	return map[string]any{
		"url": base, "html_url": x.web("/%s/%s/milestone/%d", owner, repo, m.Number), "labels_url": base + "/labels",
		"id": m.ID, "node_id": store.NodeID("MI", x.repoID(owner, repo), m.ID), "number": m.Number,
		"state": m.State, "title": m.Title, "description": nullable(m.Description),
		"creator": x.user(store.DefaultLogin), "open_issues": open, "closed_issues": closed,
		"created_at": m.CreatedAt, "updated_at": m.UpdatedAt, "closed_at": nil, "due_on": nil,
	}
}

// issue renders the issue schema.
func (x renderer) issue(i store.Issue) map[string]any {
	owner, repo := i.RepoOwner, i.RepoName
	u := x.api("/repos/%s/%s/issues/%d", owner, repo, i.Number)
	var closedBy any
	if i.ClosedBy != "" {
		closedBy = x.user(i.ClosedBy)
	}
	var reason any
	if i.StateReason != "" {
		reason = i.StateReason
	}
	return map[string]any{
		"id":                         i.ID,
		"node_id":                    store.NodeID("I", x.repoID(owner, repo), i.ID),
		"url":                        u,
		"repository_url":             x.api("/repos/%s/%s", owner, repo),
		"labels_url":                 u + "/labels{/name}",
		"comments_url":               u + "/comments",
		"events_url":                 u + "/events",
		"html_url":                   x.web("/%s/%s/issues/%d", owner, repo, i.Number),
		"timeline_url":               u + "/timeline",
		"number":                     i.Number,
		"state":                      i.State,
		"state_reason":               reason,
		"title":                      i.Title,
		"body":                       nullable(i.Body),
		"user":                       x.user(i.User.Login),
		"labels":                     x.labels(owner, repo, i.Labels),
		"assignees":                  x.users(i.Assignees),
		"milestone":                  x.milestone(owner, repo, i.Milestone),
		"locked":                     i.Locked,
		"active_lock_reason":         nil,
		"comments":                   i.Comments,
		"closed_at":                  nullable(i.ClosedAt),
		"created_at":                 i.CreatedAt,
		"updated_at":                 i.UpdatedAt,
		"closed_by":                  closedBy,
		"author_association":         authorAssociation(owner, i.User.Login),
		"reactions":                  x.reactions(owner, repo, "issue:"+itoa(i.Number), u),
		"performed_via_github_app":   nil,
		"sub_issues_summary":         map[string]any{"total": 0, "completed": 0, "percent_completed": 0},
		"issue_dependencies_summary": map[string]any{"blocked_by": 0, "blocking": 0, "total_blocked_by": 0, "total_blocking": 0},
	}
}

// comment renders an issue-comment.
func (x renderer) comment(c store.Comment) map[string]any {
	owner, repo := c.RepoOwner, c.RepoName
	u := x.api("/repos/%s/%s/issues/comments/%d", owner, repo, c.ID)
	return map[string]any{
		"id":                       c.ID,
		"node_id":                  store.NodeID("IC", x.repoID(owner, repo), c.ID),
		"url":                      u,
		"html_url":                 x.web("/%s/%s/issues/%d#issuecomment-%d", owner, repo, c.IssueNumber, c.ID),
		"body":                     c.Body,
		"user":                     x.user(c.User.Login),
		"created_at":               c.CreatedAt,
		"updated_at":               c.UpdatedAt,
		"issue_url":                x.api("/repos/%s/%s/issues/%d", owner, repo, c.IssueNumber),
		"author_association":       authorAssociation(owner, c.User.Login),
		"reactions":                x.reactions(owner, repo, "comment:"+itoa64(c.ID), u),
		"performed_via_github_app": nil,
	}
}

// pullAsIssue renders a pull request in its issue form, as the issues API
// lists it.
func (x renderer) pullAsIssue(p store.PullRequest) map[string]any {
	i := store.Issue{
		ID: p.ID, Number: p.Number, Title: p.Title, Body: p.Body, State: p.State, User: p.User,
		Labels: p.Labels, Assignees: p.Assignees, Comments: p.Comments,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, ClosedAt: p.ClosedAt,
		RepoOwner: p.RepoOwner, RepoName: p.RepoName,
	}
	m := x.issue(i)
	m["node_id"] = store.NodeID("PR", x.repoID(p.RepoOwner, p.RepoName), p.ID)
	m["draft"] = p.Draft
	m["pull_request"] = map[string]any{
		"url":       x.api("/repos/%s/%s/pulls/%d", p.RepoOwner, p.RepoName, p.Number),
		"html_url":  x.web("/%s/%s/pull/%d", p.RepoOwner, p.RepoName, p.Number),
		"diff_url":  x.web("/%s/%s/pull/%d.diff", p.RepoOwner, p.RepoName, p.Number),
		"patch_url": x.web("/%s/%s/pull/%d.patch", p.RepoOwner, p.RepoName, p.Number),
		"merged_at": nullable(p.MergedAt),
	}
	return m
}
