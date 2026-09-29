package api_test

import "testing"

// requiredFields lists, per component schema, the properties GitHub's REST
// API description for calendar version 2026-03-10 marks required
// (github/rest-api-description@2f44eacae7f3,
// descriptions/api.github.com/api.github.com.2026-03-10.json). Tests assert
// that the app emulator's responses carry every one of them.
var requiredFields = map[string][]string{
	"full-repository": {
		"archive_url", "assignees_url", "blobs_url", "branches_url", "collaborators_url", "comments_url",
		"commits_url", "compare_url", "contents_url", "contributors_url", "deployments_url",
		"description", "downloads_url", "events_url", "fork", "forks_url", "full_name",
		"git_commits_url", "git_refs_url", "git_tags_url", "hooks_url", "html_url", "id", "node_id",
		"issue_comment_url", "issue_events_url", "issues_url", "keys_url", "labels_url", "languages_url",
		"merges_url", "milestones_url", "name", "notifications_url", "owner", "private", "pulls_url",
		"releases_url", "stargazers_url", "statuses_url", "subscribers_url", "subscription_url",
		"tags_url", "teams_url", "trees_url", "url", "clone_url", "default_branch", "forks",
		"forks_count", "git_url", "has_issues", "has_projects", "has_wiki", "has_pages",
		"has_discussions", "homepage", "language", "archived", "disabled", "mirror_url", "open_issues",
		"open_issues_count", "license", "pushed_at", "size", "ssh_url", "stargazers_count", "svn_url",
		"watchers", "watchers_count", "created_at", "updated_at", "network_count", "subscribers_count",
	},
	"repository": {
		"archive_url", "assignees_url", "blobs_url", "branches_url", "collaborators_url", "comments_url",
		"commits_url", "compare_url", "contents_url", "contributors_url", "deployments_url",
		"description", "downloads_url", "events_url", "fork", "forks_url", "full_name",
		"git_commits_url", "git_refs_url", "git_tags_url", "hooks_url", "html_url", "id", "node_id",
		"issue_comment_url", "issue_events_url", "issues_url", "keys_url", "labels_url", "languages_url",
		"merges_url", "milestones_url", "name", "notifications_url", "owner", "private", "pulls_url",
		"releases_url", "stargazers_url", "statuses_url", "subscribers_url", "subscription_url",
		"tags_url", "teams_url", "trees_url", "url", "clone_url", "default_branch", "forks",
		"forks_count", "git_url", "has_issues", "has_projects", "has_wiki", "has_pages", "homepage",
		"language", "archived", "disabled", "mirror_url", "open_issues", "open_issues_count", "license",
		"pushed_at", "size", "ssh_url", "stargazers_count", "svn_url", "watchers", "watchers_count",
		"created_at", "updated_at",
	},
	"minimal-repository": {
		"archive_url", "assignees_url", "blobs_url", "branches_url", "collaborators_url", "comments_url",
		"commits_url", "compare_url", "contents_url", "contributors_url", "deployments_url",
		"description", "downloads_url", "events_url", "fork", "forks_url", "full_name",
		"git_commits_url", "git_refs_url", "git_tags_url", "hooks_url", "html_url", "id", "node_id",
		"issue_comment_url", "issue_events_url", "issues_url", "keys_url", "labels_url", "languages_url",
		"merges_url", "milestones_url", "name", "notifications_url", "owner", "private", "pulls_url",
		"releases_url", "stargazers_url", "statuses_url", "subscribers_url", "subscription_url",
		"tags_url", "teams_url", "trees_url", "url",
	},
	"simple-user": {
		"avatar_url", "events_url", "followers_url", "following_url", "gists_url", "gravatar_id",
		"html_url", "id", "node_id", "login", "organizations_url", "received_events_url", "repos_url",
		"site_admin", "starred_url", "subscriptions_url", "type", "url",
	},
	"issue": {
		"closed_at", "comments", "comments_url", "events_url", "html_url", "id", "node_id", "labels",
		"labels_url", "milestone", "number", "repository_url", "state", "locked", "title", "url", "user",
		"created_at", "updated_at",
	},
	"label": {
		"id", "node_id", "url", "name", "description", "color", "default", "archived_at", "archived_by",
	},
	"issue-comment": {
		"id", "node_id", "html_url", "issue_url", "user", "url", "created_at", "updated_at",
	},
	"pull-request": {
		"_links", "labels", "base", "body", "closed_at", "comments_url", "commits_url", "created_at",
		"diff_url", "head", "html_url", "id", "node_id", "issue_url", "merged_at", "milestone", "number",
		"patch_url", "review_comment_url", "review_comments_url", "statuses_url", "state", "locked",
		"title", "updated_at", "url", "user", "author_association", "auto_merge", "additions",
		"changed_files", "comments", "commits", "deletions", "mergeable", "mergeable_state", "merged",
		"maintainer_can_modify", "merged_by", "review_comments",
	},
	"pull-request-simple": {
		"_links", "labels", "base", "body", "closed_at", "comments_url", "commits_url", "created_at",
		"diff_url", "head", "html_url", "id", "node_id", "issue_url", "merged_at", "milestone", "number",
		"patch_url", "review_comment_url", "review_comments_url", "statuses_url", "state", "locked",
		"title", "updated_at", "url", "user", "author_association", "auto_merge",
	},
	"pull-request-review": {
		"id", "node_id", "user", "body", "state", "commit_id", "html_url", "pull_request_url", "_links",
		"author_association",
	},
	"status": {
		"url", "avatar_url", "id", "node_id", "state", "description", "target_url", "context",
		"created_at", "updated_at", "creator",
	},
	"combined-commit-status": {
		"state", "sha", "total_count", "statuses", "repository", "commit_url", "url",
	},
	"simple-commit-status": {
		"description", "id", "node_id", "state", "context", "target_url", "avatar_url", "url",
		"created_at", "updated_at",
	},
	"check-run": {
		"id", "node_id", "head_sha", "name", "url", "html_url", "details_url", "status", "conclusion",
		"started_at", "completed_at", "external_id", "check_suite", "output", "app", "pull_requests",
	},
	"integration": {
		"id", "node_id", "owner", "name", "description", "external_url", "html_url", "created_at",
		"updated_at", "permissions", "events",
	},
	"installation": {
		"id", "app_id", "app_slug", "target_id", "target_type", "single_file_name",
		"repository_selection", "access_tokens_url", "html_url", "repositories_url", "events", "account",
		"permissions", "created_at", "updated_at", "suspended_by", "suspended_at",
	},
	"installation-token": {
		"token", "expires_at",
	},
	"release": {
		"assets_url", "upload_url", "tarball_url", "zipball_url", "created_at", "published_at", "draft",
		"id", "node_id", "author", "html_url", "name", "prerelease", "tag_name", "target_commitish",
		"assets", "url",
	},
	"release-asset": {
		"id", "name", "content_type", "size", "digest", "state", "url", "node_id", "download_count",
		"label", "uploader", "browser_download_url", "created_at", "updated_at",
	},
	"workflow-run": {
		"id", "node_id", "head_branch", "run_number", "display_title", "event", "status", "conclusion",
		"head_sha", "path", "workflow_id", "url", "html_url", "created_at", "updated_at", "head_commit",
		"head_repository", "repository", "jobs_url", "logs_url", "check_suite_url", "cancel_url",
		"rerun_url", "artifacts_url", "workflow_url", "pull_requests",
	},
	"workflow-dispatch-response": {
		"workflow_run_id", "run_url", "html_url",
	},
	"hook": {
		"id", "url", "type", "name", "active", "events", "config", "ping_url", "created_at",
		"updated_at", "last_response", "test_url",
	},
	"hook-delivery-item": {
		"id", "guid", "delivered_at", "redelivery", "duration", "status", "status_code", "event",
		"action", "installation_id", "repository_id",
	},
	"git-ref": {
		"ref", "node_id", "url", "object",
	},
	"content-file": {
		"_links", "git_url", "html_url", "download_url", "name", "path", "sha", "size", "type", "url",
		"content", "encoding",
	},
	"short-branch": {
		"name", "commit", "protected",
	},
	"commit": {
		"url", "sha", "node_id", "html_url", "comments_url", "commit", "author", "committer", "parents",
	},
	"file-commit": {
		"content", "commit",
	},
	"private-user": {
		"avatar_url", "events_url", "followers_url", "following_url", "gists_url", "gravatar_id",
		"html_url", "id", "node_id", "login", "organizations_url", "received_events_url", "repos_url",
		"site_admin", "starred_url", "subscriptions_url", "type", "url", "bio", "blog", "company",
		"email", "followers", "following", "hireable", "location", "name", "public_gists",
		"public_repos", "created_at", "updated_at", "collaborators", "disk_usage", "owned_private_repos",
		"private_gists", "total_private_repos", "two_factor_authentication",
	},
}

// assertRequired fails the test for each required property of schema that
// obj lacks.
func assertRequired(t *testing.T, schema string, obj map[string]any) {
	t.Helper()
	fields, ok := requiredFields[schema]
	if !ok {
		t.Fatalf("no required-field list for %s", schema)
	}
	for _, f := range fields {
		if _, present := obj[f]; !present {
			t.Errorf("%s: missing required field %q", schema, f)
		}
	}
}
