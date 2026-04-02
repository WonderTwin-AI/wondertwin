package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// --- Releases extra ---

// UpdateRelease handles PATCH /repos/{owner}/{repo}/releases/{release_id}
func (h *Handler) UpdateRelease(w http.ResponseWriter, r *http.Request) {
	releaseID, _ := strconv.ParseInt(chi.URLParam(r, "release_id"), 10, 64)

	ids, releases := h.store.Releases.FilterWithIDs(func(_ string, rel store.Release) bool {
		return rel.ID == releaseID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}

	var req map[string]any
	json.NewDecoder(r.Body).Decode(&req)

	rel := releases[0]
	if name, ok := req["name"].(string); ok {
		rel.Name = name
	}
	if body, ok := req["body"].(string); ok {
		rel.Body = body
	}
	if draft, ok := req["draft"].(bool); ok {
		rel.Draft = draft
	}
	if pre, ok := req["prerelease"].(bool); ok {
		rel.Prerelease = pre
	}

	h.store.Releases.Set(ids[0], rel)
	ghJSON(w, 200, rel)
}

// GetReleaseByTag handles GET /repos/{owner}/{repo}/releases/tags/{tag}
func (h *Handler) GetReleaseByTag(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	tag := chi.URLParam(r, "tag")

	releases := h.store.ListRepoReleases(owner, repo)
	for _, rel := range releases {
		if rel.TagName == tag {
			ghJSON(w, 200, rel)
			return
		}
	}
	ghError(w, 404, "Not Found")
}

// GetReleaseAsset handles GET /repos/{owner}/{repo}/releases/assets/{asset_id}
func (h *Handler) GetReleaseAsset(w http.ResponseWriter, r *http.Request) {
	assetID, _ := strconv.ParseInt(chi.URLParam(r, "asset_id"), 10, 64)
	_, assets := h.store.ReleaseAssets.FilterWithIDs(func(_ string, a store.ReleaseAsset) bool {
		return a.ID == assetID
	})
	if len(assets) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, assets[0])
}

// UpdateReleaseAsset handles PATCH /repos/{owner}/{repo}/releases/assets/{asset_id}
func (h *Handler) UpdateReleaseAsset(w http.ResponseWriter, r *http.Request) {
	assetID, _ := strconv.ParseInt(chi.URLParam(r, "asset_id"), 10, 64)
	ids, assets := h.store.ReleaseAssets.FilterWithIDs(func(_ string, a store.ReleaseAsset) bool {
		return a.ID == assetID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}

	var req map[string]any
	json.NewDecoder(r.Body).Decode(&req)

	a := assets[0]
	if name, ok := req["name"].(string); ok {
		a.Name = name
	}
	if label, ok := req["label"].(string); ok {
		a.Label = label
	}
	h.store.ReleaseAssets.Set(ids[0], a)
	ghJSON(w, 200, a)
}

// DeleteReleaseAsset handles DELETE /repos/{owner}/{repo}/releases/assets/{asset_id}
func (h *Handler) DeleteReleaseAsset(w http.ResponseWriter, r *http.Request) {
	assetID, _ := strconv.ParseInt(chi.URLParam(r, "asset_id"), 10, 64)
	ids, _ := h.store.ReleaseAssets.FilterWithIDs(func(_ string, a store.ReleaseAsset) bool {
		return a.ID == assetID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.ReleaseAssets.Delete(ids[0])
	w.WriteHeader(204)
}

// --- Branches extra ---

// GetBranchProtection handles GET /repos/{owner}/{repo}/branches/{branch}/protection
func (h *Handler) GetBranchProtection(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{
		"required_status_checks":        nil,
		"enforce_admins":                map[string]any{"enabled": false},
		"required_pull_request_reviews": nil,
		"restrictions":                  nil,
	})
}

// UpdateBranchProtection handles PUT /repos/{owner}/{repo}/branches/{branch}/protection
func (h *Handler) UpdateBranchProtection(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{
		"required_status_checks":        nil,
		"enforce_admins":                map[string]any{"enabled": false},
		"required_pull_request_reviews": nil,
		"restrictions":                  nil,
	})
}

// DeleteBranchProtection handles DELETE /repos/{owner}/{repo}/branches/{branch}/protection
func (h *Handler) DeleteBranchProtection(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(204)
}

// RenameBranch handles POST /repos/{owner}/{repo}/branches/{branch}/rename
func (h *Handler) RenameBranch(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	oldName := chi.URLParam(r, "branch")

	var req struct {
		NewName string `json:"new_name"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	ids, branches := h.store.Branches.FilterWithIDs(func(_ string, b store.Branch) bool {
		return b.RepoOwner == owner && b.RepoName == repo && b.Name == oldName
	})
	if len(ids) > 0 {
		b := branches[0]
		b.Name = req.NewName
		h.store.Branches.Set(ids[0], b)
		ghJSON(w, 201, b)
		return
	}
	ghJSON(w, 201, store.Branch{Name: req.NewName, Commit: store.BranchCommit{SHA: store.DefaultSHA()}})
}

// --- Check Runs/Suites extra ---

// ListCheckRunsInSuite handles GET /repos/{owner}/{repo}/check-suites/{check_suite_id}/check-runs
func (h *Handler) ListCheckRunsInSuite(w http.ResponseWriter, r *http.Request) {
	// Simplified: return all check runs for the repo
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	runs := h.store.CheckRuns.Filter(func(_ string, cr store.CheckRun) bool {
		return cr.RepoOwner == owner && cr.RepoName == repo
	})
	ghJSON(w, 200, map[string]any{"total_count": len(runs), "check_runs": runs})
}

// ListCheckRunAnnotations handles GET /repos/{owner}/{repo}/check-runs/{check_run_id}/annotations
func (h *Handler) ListCheckRunAnnotations(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []any{})
}

// RerequestCheckRun handles POST /repos/{owner}/{repo}/check-runs/{check_run_id}/rerequest
func (h *Handler) RerequestCheckRun(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(201)
}

// RerequestCheckSuite handles POST /repos/{owner}/{repo}/check-suites/{check_suite_id}/rerequest
func (h *Handler) RerequestCheckSuite(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(201)
}

// UpdateCheckSuitePreferences handles PATCH /repos/{owner}/{repo}/check-suites/preferences
func (h *Handler) UpdateCheckSuitePreferences(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{"preferences": map[string]any{"auto_trigger_checks": []any{}}})
}

// --- Contents extra ---

// GetReadmeForDir handles GET /repos/{owner}/{repo}/readme/{dir}
func (h *Handler) GetReadmeForDir(w http.ResponseWriter, r *http.Request) {
	h.GetReadme(w, r) // Simplified: same as readme
}

// DownloadTarball handles GET /repos/{owner}/{repo}/tarball/{ref}
func (h *Handler) DownloadTarball(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-gzip")
	w.WriteHeader(200)
	w.Write([]byte{}) // Empty tarball placeholder
}

// DownloadZipball handles GET /repos/{owner}/{repo}/zipball/{ref}
func (h *Handler) DownloadZipball(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/zip")
	w.WriteHeader(200)
	w.Write([]byte{}) // Empty zipball placeholder
}

// --- Webhooks extra ---

// PingWebhook handles POST /repos/{owner}/{repo}/hooks/{hook_id}/pings
func (h *Handler) PingWebhook(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(204)
}

// TestWebhook handles POST /repos/{owner}/{repo}/hooks/{hook_id}/tests
func (h *Handler) TestWebhook(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(204)
}

// ListWebhookDeliveries handles GET /repos/{owner}/{repo}/hooks/{hook_id}/deliveries
func (h *Handler) ListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []any{})
}

// --- Orgs extra ---

// UpdateOrg handles PATCH /orgs/{org}
func (h *Handler) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	orgLogin := chi.URLParam(r, "org")
	org, ok := h.store.Orgs.Get(orgLogin)
	if !ok {
		org = store.Organization{ID: h.store.NextID(), Login: orgLogin, Type: "Organization"}
	}

	var req map[string]any
	json.NewDecoder(r.Body).Decode(&req)

	if name, ok := req["name"].(string); ok {
		org.Name = name
	}
	if desc, ok := req["description"].(string); ok {
		org.Description = desc
	}
	h.store.Orgs.Set(orgLogin, org)
	ghJSON(w, 200, org)
}

// CheckOrgMembership handles GET /orgs/{org}/members/{username}
func (h *Handler) CheckOrgMembership(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(204) // Member
}

// RemoveOrgMember handles DELETE /orgs/{org}/members/{username}
func (h *Handler) RemoveOrgMember(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(204)
}

// ListOrgPublicMembers handles GET /orgs/{org}/public_members
func (h *Handler) ListOrgPublicMembers(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []any{})
}

// --- Teams extra ---

// ListTeamRepos handles GET /orgs/{org}/teams/{team_slug}/repos
func (h *Handler) ListTeamRepos(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []any{})
}

// ListTeamMembers handles GET /orgs/{org}/teams/{team_slug}/members
func (h *Handler) ListTeamMembers(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []any{})
}

// ListChildTeams handles GET /orgs/{org}/teams/{team_slug}/teams
func (h *Handler) ListChildTeams(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []any{})
}

// --- Collaborators extra ---

// CheckCollaborator handles GET /repos/{owner}/{repo}/collaborators/{username}
func (h *Handler) CheckCollaborator(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(204)
}

// --- Deployments extra ---

// DeleteDeployment handles DELETE /repos/{owner}/{repo}/deployments/{deployment_id}
func (h *Handler) DeleteDeployment(w http.ResponseWriter, r *http.Request) {
	deployID, _ := strconv.ParseInt(chi.URLParam(r, "deployment_id"), 10, 64)
	ids, _ := h.store.Deployments.FilterWithIDs(func(_ string, d store.Deployment) bool {
		return d.ID == deployID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Deployments.Delete(ids[0])
	w.WriteHeader(204)
}

// GetDeploymentStatus handles GET /repos/{owner}/{repo}/deployments/{deployment_id}/statuses/{status_id}
func (h *Handler) GetDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	statusID, _ := strconv.ParseInt(chi.URLParam(r, "status_id"), 10, 64)
	_, statuses := h.store.DeployStatuses.FilterWithIDs(func(_ string, ds store.DeploymentStatus) bool {
		return ds.ID == statusID
	})
	if len(statuses) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, statuses[0])
}

// --- Actions extra ---

// GetWorkflowRunLogs handles GET /repos/{owner}/{repo}/actions/runs/{run_id}/logs
func (h *Handler) GetWorkflowRunLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/zip")
	w.WriteHeader(200)
}

// GetWorkflowRunTiming handles GET /repos/{owner}/{repo}/actions/runs/{run_id}/timing
func (h *Handler) GetWorkflowRunTiming(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{
		"billable": map[string]any{
			"UBUNTU": map[string]any{"total_ms": 1000, "jobs": 1},
		},
		"run_duration_ms": 1000,
	})
}

// ApproveWorkflowRun handles POST /repos/{owner}/{repo}/actions/runs/{run_id}/approve
func (h *Handler) ApproveWorkflowRun(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(201)
}

// ListWorkflowRunApprovals handles GET /repos/{owner}/{repo}/actions/runs/{run_id}/approvals
func (h *Handler) ListWorkflowRunApprovals(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []any{})
}

// GetJobLogs handles GET /repos/{owner}/{repo}/actions/jobs/{job_id}/logs
func (h *Handler) GetJobLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(200)
}

// RerunJob handles POST /repos/{owner}/{repo}/actions/jobs/{job_id}/rerun
func (h *Handler) RerunJob(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(201)
}

// DownloadArtifact handles GET /repos/{owner}/{repo}/actions/artifacts/{artifact_id}/{archive_format}
func (h *Handler) DownloadArtifact(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/zip")
	w.WriteHeader(200)
}

// --- Org-level Actions Secrets ---

// ListOrgSecrets handles GET /orgs/{org}/actions/secrets
func (h *Handler) ListOrgSecrets(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{"total_count": 0, "secrets": []any{}})
}

// GetOrgPublicKey handles GET /orgs/{org}/actions/secrets/public-key
func (h *Handler) GetOrgPublicKey(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{
		"key_id": "012345678912345678",
		"key":    "2Sg8iYjAxxmI2LvUXpJjkYrMxURPc8r+dB7TJyvv1234",
	})
}

// GetOrgSecret handles GET /orgs/{org}/actions/secrets/{secret_name}
func (h *Handler) GetOrgSecret(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "secret_name")
	ghJSON(w, 200, map[string]any{
		"name":       name,
		"created_at": h.store.Now(),
		"updated_at": h.store.Now(),
	})
}

// CreateOrUpdateOrgSecret handles PUT /orgs/{org}/actions/secrets/{secret_name}
func (h *Handler) CreateOrUpdateOrgSecret(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(204)
}

// DeleteOrgSecret handles DELETE /orgs/{org}/actions/secrets/{secret_name}
func (h *Handler) DeleteOrgSecret(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(204)
}

// --- Search extra ---

// SearchCode handles GET /search/code
func (h *Handler) SearchCode(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{"total_count": 0, "incomplete_results": false, "items": []any{}})
}

// SearchCommits handles GET /search/commits
func (h *Handler) SearchCommits(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{"total_count": 0, "incomplete_results": false, "items": []any{}})
}

// SearchTopics handles GET /search/topics
func (h *Handler) SearchTopics(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{"total_count": 0, "incomplete_results": false, "items": []any{}})
}

// SearchLabels handles GET /search/labels
func (h *Handler) SearchLabels(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{"total_count": 0, "incomplete_results": false, "items": []any{}})
}

// --- Reactions extra (PR comments, commit comments) ---

// ListPRCommentReactions handles GET /repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions
func (h *Handler) ListPRCommentReactions(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	commentID := chi.URLParam(r, "comment_id")
	reactions := h.store.ListSubjectReactions(owner, repo, "pr_comment:"+commentID)
	ghJSON(w, 200, reactions)
}

// CreatePRCommentReaction handles POST /repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions
func (h *Handler) CreatePRCommentReaction(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	commentID := chi.URLParam(r, "comment_id")

	var req struct {
		Content string `json:"content"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	rx := store.Reaction{
		ID: h.store.NextID(), User: store.User{ID: 1, Login: "twin-bot", Type: "User"},
		Content: req.Content, CreatedAt: h.store.Now(),
		RepoOwner: owner, RepoName: repo, Subject: "pr_comment:" + commentID,
	}
	id := h.store.Reactions.NextID()
	h.store.Reactions.Set(id, rx)
	ghJSON(w, 201, rx)
}

// DeletePRCommentReaction handles DELETE /repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions/{reaction_id}
func (h *Handler) DeletePRCommentReaction(w http.ResponseWriter, r *http.Request) {
	reactionID, _ := strconv.ParseInt(chi.URLParam(r, "reaction_id"), 10, 64)
	ids, _ := h.store.Reactions.FilterWithIDs(func(_ string, rx store.Reaction) bool { return rx.ID == reactionID })
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Reactions.Delete(ids[0])
	w.WriteHeader(204)
}

// --- GitHub App endpoints ---

// GetApp handles GET /app
func (h *Handler) GetApp(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{
		"id":    1,
		"slug":  "wondertwin-app",
		"name":  "WonderTwin App",
		"owner": store.User{ID: 1, Login: "twin-bot", Type: "User"},
	})
}

// ListAppInstallations handles GET /app/installations
func (h *Handler) ListAppInstallations(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []map[string]any{
		{
			"id":         1,
			"app_id":     1,
			"target_type": "Organization",
			"account":    store.User{ID: 1, Login: "twin-bot", Type: "Organization"},
		},
	})
}

// CreateInstallationAccessToken handles POST /app/installations/{installation_id}/access_tokens
func (h *Handler) CreateInstallationAccessToken(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 201, map[string]any{
		"token":      fmt.Sprintf("ghs_%s", store.MakeSHA(h.store.Now())[:20]),
		"expires_at": "2099-01-01T00:00:00Z",
		"permissions": map[string]any{
			"issues":        "write",
			"pull_requests": "write",
			"contents":      "read",
		},
	})
}

// --- Misc ---

// ListEmojis handles GET /emojis
func (h *Handler) ListEmojis(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{"+1": "https://github.githubassets.com/images/icons/emoji/unicode/1f44d.png"})
}

// ListGitignoreTemplates handles GET /gitignore/templates
func (h *Handler) ListGitignoreTemplates(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []string{"Go", "Node", "Python", "Java", "Ruby"})
}

// ListLicenses handles GET /licenses
func (h *Handler) ListLicenses(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []map[string]any{
		{"key": "mit", "name": "MIT License", "spdx_id": "MIT"},
		{"key": "apache-2.0", "name": "Apache License 2.0", "spdx_id": "Apache-2.0"},
	})
}

// RepoDispatch handles POST /repos/{owner}/{repo}/dispatches
func (h *Handler) RepoDispatch(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(204)
}
