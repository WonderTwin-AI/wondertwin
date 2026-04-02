// Package api implements the Linear GraphQL API-compatible handler for the twin.
package api

import (
	"context"
	"fmt"

	"github.com/wondertwin-ai/wondertwin/twinkit/graphql"
	"github.com/wondertwin-ai/wondertwin/twin-linear/internal/store"
)

// RegisterResolvers registers all Linear API queries and mutations on the schema.
func RegisterResolvers(schema *graphql.Schema, s *store.MemoryStore) {
	registerQueries(schema, s)
	registerMutations(schema, s)
}

func registerQueries(schema *graphql.Schema, s *store.MemoryStore) {
	// viewer — authenticated user
	schema.Query("viewer", func(ctx context.Context, args map[string]any) (any, error) {
		users := s.Users.List()
		if len(users) > 0 {
			return users[0], nil
		}
		return store.User{ID: "user_viewer", Name: "Twin Bot", Email: "bot@wondertwin.dev", Active: true}, nil
	})

	// organization
	schema.Query("organization", func(ctx context.Context, args map[string]any) (any, error) {
		return s.Org, nil
	})

	// issue(id)
	schema.Query("issue", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		iss, ok := s.Issues.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		return iss, nil
	})

	// issues(filter, first, after)
	schema.Query("issues", func(ctx context.Context, args map[string]any) (any, error) {
		issues := s.Issues.List()
		return store.Connection(issues), nil
	})

	// issueSearch(query)
	schema.Query("issueSearch", func(ctx context.Context, args map[string]any) (any, error) {
		issues := s.Issues.List()
		return store.Connection(issues), nil
	})

	// team(id)
	schema.Query("team", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		t, ok := s.Teams.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		return t, nil
	})

	// teams
	schema.Query("teams", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.Teams.List()), nil
	})

	// project(id)
	schema.Query("project", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		p, ok := s.Projects.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		return p, nil
	})

	// projects
	schema.Query("projects", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.Projects.List()), nil
	})

	// cycle(id)
	schema.Query("cycle", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		c, ok := s.Cycles.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		return c, nil
	})

	// cycles
	schema.Query("cycles", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.Cycles.List()), nil
	})

	// user(id)
	schema.Query("user", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		u, ok := s.Users.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		return u, nil
	})

	// users
	schema.Query("users", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.Users.List()), nil
	})

	// workflowStates
	schema.Query("workflowStates", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.WorkflowStates.List()), nil
	})

	// issueLabels
	schema.Query("issueLabels", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.Labels.List()), nil
	})

	// comments
	schema.Query("comments", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.Comments.List()), nil
	})

	// issueRelations
	schema.Query("issueRelations", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.IssueRelations.List()), nil
	})

	// attachments
	schema.Query("attachments", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.Attachments.List()), nil
	})

	// documents
	schema.Query("documents", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.Documents.List()), nil
	})

	// projectUpdates
	schema.Query("projectUpdates", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.ProjectUpdates.List()), nil
	})

	// webhooks
	schema.Query("webhooks", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.Webhooks.List()), nil
	})

	// notifications
	schema.Query("notifications", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.Notifications.List()), nil
	})

	// favorites
	schema.Query("favorites", func(ctx context.Context, args map[string]any) (any, error) {
		return store.Connection(s.Favorites.List()), nil
	})
}

func registerMutations(schema *graphql.Schema, s *store.MemoryStore) {
	// --- Issues ---
	schema.Mutation("issueCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		now := s.Now()
		id := s.NextID()
		teamID, _ := input["teamId"].(string)
		team, _ := s.Teams.Get(teamID)

		num := s.NextIssueNumber()
		identifier := team.Key + "-" + fmt.Sprintf("%d", num)

		iss := store.Issue{
			ID:          id,
			Identifier:  identifier,
			Title:       getString(input, "title"),
			Description: getString(input, "description"),
			Priority:    getInt(input, "priority"),
			TeamID:      teamID,
			StateID:     getString(input, "stateId"),
			AssigneeID:  getString(input, "assigneeId"),
			ProjectID:   getString(input, "projectId"),
			CycleID:     getString(input, "cycleId"),
			ParentID:    getString(input, "parentId"),
			DueDate:     getString(input, "dueDate"),
			Number:      num,
			URL:         fmt.Sprintf("https://linear.app/wondertwin/issue/%s", identifier),
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		s.Issues.Set(id, iss)
		return store.MutationResult(true, iss, "issue"), nil
	})

	schema.Mutation("issueUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		input := extractInput(args)
		iss, ok := s.Issues.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		if v, ok := input["title"].(string); ok {
			iss.Title = v
		}
		if v, ok := input["description"].(string); ok {
			iss.Description = v
		}
		if v, ok := input["stateId"].(string); ok {
			iss.StateID = v
		}
		if v, ok := input["assigneeId"].(string); ok {
			iss.AssigneeID = v
		}
		if v, ok := input["priority"].(int); ok {
			iss.Priority = v
		}
		if v, ok := input["projectId"].(string); ok {
			iss.ProjectID = v
		}
		if v, ok := input["cycleId"].(string); ok {
			iss.CycleID = v
		}
		iss.UpdatedAt = s.Now()
		s.Issues.Set(id, iss)
		return store.MutationResult(true, iss, "issue"), nil
	})

	schema.Mutation("issueArchive", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		iss, ok := s.Issues.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		iss.ArchivedAt = s.Now()
		iss.UpdatedAt = s.Now()
		s.Issues.Set(id, iss)
		return store.MutationResult(true, iss, "issue"), nil
	})

	schema.Mutation("issueUnarchive", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		iss, ok := s.Issues.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		iss.ArchivedAt = ""
		iss.UpdatedAt = s.Now()
		s.Issues.Set(id, iss)
		return store.MutationResult(true, iss, "issue"), nil
	})

	schema.Mutation("issueDelete", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.Issues.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Comments ---
	schema.Mutation("commentCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		now := s.Now()
		id := s.NextID()
		c := store.Comment{
			ID:        id,
			Body:      getString(input, "body"),
			IssueID:   getString(input, "issueId"),
			UserID:    "user_viewer",
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.Comments.Set(id, c)
		return store.MutationResult(true, c, "comment"), nil
	})

	schema.Mutation("commentUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		input := extractInput(args)
		c, ok := s.Comments.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		if v, ok := input["body"].(string); ok {
			c.Body = v
		}
		c.UpdatedAt = s.Now()
		s.Comments.Set(id, c)
		return store.MutationResult(true, c, "comment"), nil
	})

	schema.Mutation("commentDelete", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.Comments.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Labels ---
	schema.Mutation("issueLabelCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		now := s.Now()
		id := s.NextID()
		l := store.IssueLabel{
			ID:        id,
			Name:      getString(input, "name"),
			Color:     getString(input, "color"),
			TeamID:    getString(input, "teamId"),
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.Labels.Set(id, l)
		return store.MutationResult(true, l, "issueLabel"), nil
	})

	schema.Mutation("issueLabelUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		input := extractInput(args)
		l, ok := s.Labels.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		if v, ok := input["name"].(string); ok {
			l.Name = v
		}
		if v, ok := input["color"].(string); ok {
			l.Color = v
		}
		l.UpdatedAt = s.Now()
		s.Labels.Set(id, l)
		return store.MutationResult(true, l, "issueLabel"), nil
	})

	schema.Mutation("issueLabelArchive", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.Labels.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Projects ---
	schema.Mutation("projectCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		now := s.Now()
		id := s.NextID()
		p := store.Project{
			ID:        id,
			Name:      getString(input, "name"),
			Description: getString(input, "description"),
			State:     "planned",
			SlugID:    id,
			URL:       "https://linear.app/wondertwin/project/" + id,
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.Projects.Set(id, p)
		return store.MutationResult(true, p, "project"), nil
	})

	schema.Mutation("projectUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		input := extractInput(args)
		p, ok := s.Projects.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		if v, ok := input["name"].(string); ok {
			p.Name = v
		}
		if v, ok := input["state"].(string); ok {
			p.State = v
		}
		p.UpdatedAt = s.Now()
		s.Projects.Set(id, p)
		return store.MutationResult(true, p, "project"), nil
	})

	schema.Mutation("projectArchive", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		p, ok := s.Projects.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		p.ArchivedAt = s.Now()
		s.Projects.Set(id, p)
		return store.MutationResult(true, nil, ""), nil
	})

	schema.Mutation("projectDelete", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.Projects.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Project Updates ---
	schema.Mutation("projectUpdateCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		now := s.Now()
		id := s.NextID()
		pu := store.ProjectUpdate{
			ID:        id,
			Body:      getString(input, "body"),
			Health:    getString(input, "health"),
			ProjectID: getString(input, "projectId"),
			UserID:    "user_viewer",
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.ProjectUpdates.Set(id, pu)
		return store.MutationResult(true, pu, "projectUpdate"), nil
	})

	schema.Mutation("projectUpdateDelete", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.ProjectUpdates.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Cycles ---
	schema.Mutation("cycleCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		now := s.Now()
		id := s.NextID()
		c := store.Cycle{
			ID:        id,
			Number:    s.NextCycleNumber(),
			Name:      getString(input, "name"),
			StartsAt:  getString(input, "startsAt"),
			EndsAt:    getString(input, "endsAt"),
			TeamID:    getString(input, "teamId"),
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.Cycles.Set(id, c)
		return store.MutationResult(true, c, "cycle"), nil
	})

	schema.Mutation("cycleUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		input := extractInput(args)
		c, ok := s.Cycles.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		if v, ok := input["name"].(string); ok {
			c.Name = v
		}
		c.UpdatedAt = s.Now()
		s.Cycles.Set(id, c)
		return store.MutationResult(true, c, "cycle"), nil
	})

	schema.Mutation("cycleArchive", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.Cycles.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Workflow States ---
	schema.Mutation("workflowStateCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		now := s.Now()
		id := s.NextID()
		ws := store.WorkflowState{
			ID:     id,
			Name:   getString(input, "name"),
			Type:   getString(input, "type"),
			Color:  getString(input, "color"),
			TeamID: getString(input, "teamId"),
		}
		_ = now
		s.WorkflowStates.Set(id, ws)
		return store.MutationResult(true, ws, "workflowState"), nil
	})

	schema.Mutation("workflowStateUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		input := extractInput(args)
		ws, ok := s.WorkflowStates.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		if v, ok := input["name"].(string); ok {
			ws.Name = v
		}
		if v, ok := input["color"].(string); ok {
			ws.Color = v
		}
		s.WorkflowStates.Set(id, ws)
		return store.MutationResult(true, ws, "workflowState"), nil
	})

	schema.Mutation("workflowStateArchive", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.WorkflowStates.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Teams ---
	schema.Mutation("teamCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		now := s.Now()
		id := s.NextID()
		t := store.Team{
			ID:        id,
			Name:      getString(input, "name"),
			Key:       getString(input, "key"),
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.Teams.Set(id, t)
		return store.MutationResult(true, t, "team"), nil
	})

	schema.Mutation("teamUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		input := extractInput(args)
		t, ok := s.Teams.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		if v, ok := input["name"].(string); ok {
			t.Name = v
		}
		t.UpdatedAt = s.Now()
		s.Teams.Set(id, t)
		return store.MutationResult(true, t, "team"), nil
	})

	schema.Mutation("teamDelete", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.Teams.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Users ---
	schema.Mutation("userUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		input := extractInput(args)
		u, ok := s.Users.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		if v, ok := input["name"].(string); ok {
			u.Name = v
		}
		if v, ok := input["displayName"].(string); ok {
			u.DisplayName = v
		}
		u.UpdatedAt = s.Now()
		s.Users.Set(id, u)
		return store.MutationResult(true, u, "user"), nil
	})

	// --- Issue Relations ---
	schema.Mutation("issueRelationCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		now := s.Now()
		id := s.NextID()
		r := store.IssueRelation{
			ID:        id,
			Type:      getString(input, "type"),
			IssueID:   getString(input, "issueId"),
			RelatedID: getString(input, "relatedIssueId"),
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.IssueRelations.Set(id, r)
		return store.MutationResult(true, r, "issueRelation"), nil
	})

	schema.Mutation("issueRelationUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		input := extractInput(args)
		r, ok := s.IssueRelations.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		if v, ok := input["type"].(string); ok {
			r.Type = v
		}
		r.UpdatedAt = s.Now()
		s.IssueRelations.Set(id, r)
		return store.MutationResult(true, r, "issueRelation"), nil
	})

	schema.Mutation("issueRelationDelete", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.IssueRelations.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Attachments ---
	schema.Mutation("attachmentCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		now := s.Now()
		id := s.NextID()
		a := store.Attachment{
			ID:        id,
			Title:     getString(input, "title"),
			Subtitle:  getString(input, "subtitle"),
			URL:       getString(input, "url"),
			IssueID:   getString(input, "issueId"),
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.Attachments.Set(id, a)
		return store.MutationResult(true, a, "attachment"), nil
	})

	schema.Mutation("attachmentUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		input := extractInput(args)
		a, ok := s.Attachments.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		if v, ok := input["title"].(string); ok {
			a.Title = v
		}
		if v, ok := input["url"].(string); ok {
			a.URL = v
		}
		a.UpdatedAt = s.Now()
		s.Attachments.Set(id, a)
		return store.MutationResult(true, a, "attachment"), nil
	})

	schema.Mutation("attachmentDelete", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.Attachments.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Documents ---
	schema.Mutation("documentCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		now := s.Now()
		id := s.NextID()
		d := store.Document{
			ID:        id,
			Title:     getString(input, "title"),
			Content:   getString(input, "content"),
			ProjectID: getString(input, "projectId"),
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.Documents.Set(id, d)
		return store.MutationResult(true, d, "document"), nil
	})

	schema.Mutation("documentUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		input := extractInput(args)
		d, ok := s.Documents.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		if v, ok := input["title"].(string); ok {
			d.Title = v
		}
		if v, ok := input["content"].(string); ok {
			d.Content = v
		}
		d.UpdatedAt = s.Now()
		s.Documents.Set(id, d)
		return store.MutationResult(true, d, "document"), nil
	})

	schema.Mutation("documentDelete", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.Documents.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Reactions ---
	schema.Mutation("reactionCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		id := s.NextID()
		rx := store.Reaction{
			ID:        id,
			Emoji:     getString(input, "emoji"),
			UserID:    "user_viewer",
			CommentID: getString(input, "commentId"),
			CreatedAt: s.Now(),
		}
		s.Reactions.Set(id, rx)
		return store.MutationResult(true, rx, "reaction"), nil
	})

	schema.Mutation("reactionDelete", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.Reactions.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Webhooks ---
	schema.Mutation("webhookCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		now := s.Now()
		id := s.NextID()
		wh := store.Webhook{
			ID:        id,
			Label:     getString(input, "label"),
			URL:       getString(input, "url"),
			Enabled:   true,
			TeamID:    getString(input, "teamId"),
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.Webhooks.Set(id, wh)
		return store.MutationResult(true, wh, "webhook"), nil
	})

	schema.Mutation("webhookUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		input := extractInput(args)
		wh, ok := s.Webhooks.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		if v, ok := input["url"].(string); ok {
			wh.URL = v
		}
		if v, ok := input["enabled"].(bool); ok {
			wh.Enabled = v
		}
		wh.UpdatedAt = s.Now()
		s.Webhooks.Set(id, wh)
		return store.MutationResult(true, wh, "webhook"), nil
	})

	schema.Mutation("webhookDelete", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.Webhooks.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Notifications ---
	schema.Mutation("notificationUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		input := extractInput(args)
		n, ok := s.Notifications.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		if v, ok := input["readAt"].(string); ok {
			n.ReadAt = v
		}
		n.UpdatedAt = s.Now()
		s.Notifications.Set(id, n)
		return store.MutationResult(true, n, "notification"), nil
	})

	schema.Mutation("notificationArchive", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.Notifications.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- Favorites ---
	schema.Mutation("favoriteCreate", func(ctx context.Context, args map[string]any) (any, error) {
		input := extractInput(args)
		id := s.NextID()
		now := s.Now()
		f := store.Favorite{
			ID:        id,
			Type:      getString(input, "type"),
			IssueID:   getString(input, "issueId"),
			ProjectID: getString(input, "projectId"),
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.Favorites.Set(id, f)
		return store.MutationResult(true, f, "favorite"), nil
	})

	schema.Mutation("favoriteUpdate", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		f, ok := s.Favorites.Get(id)
		if !ok {
			return nil, fmt.Errorf("entity not found")
		}
		f.UpdatedAt = s.Now()
		s.Favorites.Set(id, f)
		return store.MutationResult(true, f, "favorite"), nil
	})

	schema.Mutation("favoriteDelete", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		s.Favorites.Delete(id)
		return store.MutationResult(true, nil, ""), nil
	})

	// --- File Upload ---
	schema.Mutation("fileUpload", func(ctx context.Context, args map[string]any) (any, error) {
		return map[string]any{
			"uploadFile": map[string]any{
				"uploadUrl": "https://uploads.linear.app/mock-upload-url",
				"assetUrl":  "https://uploads.linear.app/mock-asset-url",
			},
		}, nil
	})
}

// --- Helpers ---

func extractInput(args map[string]any) map[string]any {
	if input, ok := args["input"].(map[string]any); ok {
		return input
	}
	return args
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]any, key string) int {
	if v, ok := m[key].(int); ok {
		return v
	}
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}
