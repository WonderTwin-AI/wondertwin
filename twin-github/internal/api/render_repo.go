package api

import (
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// account renders the simple-user for a repository owner, which may be an
// organization.
func (x renderer) account(login string) map[string]any {
	if org, ok := x.h.store.Orgs.Get(login); ok {
		return x.userObj(store.User{ID: org.ID, Login: org.Login, Type: "Organization"})
	}
	return x.user(login)
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// openIssueCount counts open issues and pull requests, which GitHub reports
// together as open_issues_count.
func (x renderer) openIssueCount(owner, repo string) int {
	n := len(x.h.store.ListRepoIssues(owner, repo, "open"))
	n += len(x.h.store.ListRepoPRs(owner, repo, "open"))
	return n
}

// repoMinimal renders the fields every repository shape shares (the
// minimal-repository required set).
func (x renderer) repoMinimal(rp store.Repository) map[string]any {
	owner, name := rp.Owner.Login, rp.Name
	base := x.api("/repos/%s/%s", owner, name)
	visibility := "public"
	if rp.Private {
		visibility = "private"
	}
	open := x.openIssueCount(owner, name)
	m := map[string]any{
		"id":                rp.ID,
		"node_id":           store.NodeID("R", rp.ID),
		"name":              name,
		"full_name":         owner + "/" + name,
		"owner":             x.account(owner),
		"private":           rp.Private,
		"html_url":          x.web("/%s/%s", owner, name),
		"description":       nullable(rp.Description),
		"fork":              rp.Fork,
		"url":               base,
		"archive_url":       base + "/{archive_format}{/ref}",
		"assignees_url":     base + "/assignees{/user}",
		"blobs_url":         base + "/git/blobs{/sha}",
		"branches_url":      base + "/branches{/branch}",
		"collaborators_url": base + "/collaborators{/collaborator}",
		"comments_url":      base + "/comments{/number}",
		"commits_url":       base + "/commits{/sha}",
		"compare_url":       base + "/compare/{base}...{head}",
		"contents_url":      base + "/contents/{+path}",
		"contributors_url":  base + "/contributors",
		"deployments_url":   base + "/deployments",
		"downloads_url":     base + "/downloads",
		"events_url":        base + "/events",
		"forks_url":         base + "/forks",
		"git_commits_url":   base + "/git/commits{/sha}",
		"git_refs_url":      base + "/git/refs{/sha}",
		"git_tags_url":      base + "/git/tags{/sha}",
		"git_url":           "git://github.com/" + owner + "/" + name + ".git",
		"issue_comment_url": base + "/issues/comments{/number}",
		"issue_events_url":  base + "/issues/events{/number}",
		"issues_url":        base + "/issues{/number}",
		"keys_url":          base + "/keys{/key_id}",
		"labels_url":        base + "/labels{/name}",
		"languages_url":     base + "/languages",
		"merges_url":        base + "/merges",
		"milestones_url":    base + "/milestones{/number}",
		"notifications_url": base + "/notifications{?since,all,participating}",
		"pulls_url":         base + "/pulls{/number}",
		"releases_url":      base + "/releases{/id}",
		"ssh_url":           "git@github.com:" + owner + "/" + name + ".git",
		"stargazers_url":    base + "/stargazers",
		"statuses_url":      base + "/statuses/{sha}",
		"subscribers_url":   base + "/subscribers",
		"subscription_url":  base + "/subscription",
		"tags_url":          base + "/tags",
		"teams_url":         base + "/teams",
		"trees_url":         base + "/git/trees{/sha}",
		"clone_url":         x.web("/%s/%s.git", owner, name),
		"mirror_url":        nil,
		"hooks_url":         base + "/hooks",
		"svn_url":           x.web("/%s/%s", owner, name),
		"homepage":          nullable(rp.Homepage),
		"language":          nullable(rp.Language),
		"forks_count":       rp.ForksCount,
		"stargazers_count":  rp.StarCount,
		"watchers_count":    rp.StarCount,
		"size":              0,
		"default_branch":    rp.DefaultBranch,
		"open_issues_count": open,
		"is_template":       rp.IsTemplate,
		"topics":            nonNil(rp.Topics),
		"has_issues":        rp.HasIssues,
		"has_projects":      rp.HasProjects,
		"has_wiki":          rp.HasWiki,
		"has_pages":         false,
		"has_discussions":   false,
		"archived":          rp.Archived,
		"disabled":          rp.Disabled,
		"visibility":        visibility,
		"pushed_at":         rp.PushedAt,
		"created_at":        rp.CreatedAt,
		"updated_at":        rp.UpdatedAt,
		"allow_forking":     true,
		"forks":             rp.ForksCount,
		"open_issues":       open,
		"watchers":          rp.StarCount,
		"license":           nil,

		"web_commit_signoff_required": false,
	}
	return m
}

// repo renders the repository schema (the shape embedded in pull requests
// and listed by the repository list routes).
func (x renderer) repo(rp store.Repository, viewer string) map[string]any {
	m := x.repoMinimal(rp)
	m["permissions"] = x.permissions(rp, viewer)
	m["allow_squash_merge"] = boolOr(rp.AllowSquashMerge, true)
	m["allow_merge_commit"] = boolOr(rp.AllowMergeCommit, true)
	m["allow_rebase_merge"] = boolOr(rp.AllowRebaseMerge, true)
	m["allow_auto_merge"] = false
	m["allow_update_branch"] = false
	m["delete_branch_on_merge"] = rp.DeleteBranchOnMerge
	m["squash_merge_commit_title"] = "COMMIT_OR_PR_TITLE"
	m["squash_merge_commit_message"] = "COMMIT_MESSAGES"
	m["merge_commit_title"] = "MERGE_MESSAGE"
	m["merge_commit_message"] = "PR_TITLE"
	return m
}

// repoFull renders full-repository, what GET /repos/{owner}/{repo} and the
// create routes return. The 2026-03-10 shape has no has_downloads,
// use_squash_pr_title_as_default or master_branch.
func (x renderer) repoFull(rp store.Repository, viewer string) map[string]any {
	m := x.repo(rp, viewer)
	m["subscribers_count"] = 0
	m["network_count"] = rp.ForksCount
	m["temp_clone_token"] = nil
	if org, ok := x.h.store.Orgs.Get(rp.Owner.Login); ok {
		m["organization"] = x.userObj(store.User{ID: org.ID, Login: org.Login, Type: "Organization"})
	}
	m["security_and_analysis"] = map[string]any{
		"secret_scanning":                 map[string]any{"status": "disabled"},
		"secret_scanning_push_protection": map[string]any{"status": "disabled"},
	}
	m["custom_properties"] = map[string]any{}
	return m
}

// permissions reports what viewer may do. The owner administers; anyone
// else authenticated gets push, which is what a test collaborator needs.
func (x renderer) permissions(rp store.Repository, viewer string) map[string]any {
	admin := viewer == rp.Owner.Login
	if org, ok := x.h.store.Orgs.Get(rp.Owner.Login); ok && org.Login != "" {
		admin = viewer != ""
	}
	return map[string]any{"admin": admin, "maintain": admin, "push": viewer != "", "triage": viewer != "", "pull": true}
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
