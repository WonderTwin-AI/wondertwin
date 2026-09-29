package api

import (
	"bytes"
	"sort"

	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// fileChange is one path that differs between two trees.
type fileChange struct {
	Path, Status, OldSHA, NewSHA string
	Additions, Deletions         int
}

func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := bytes.Count(b, []byte("\n"))
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

// diffTrees lists the paths that differ from old to new, with line counts
// that treat a modified file as wholly replaced.
func (h *Handler) diffTrees(owner, repo string, old, cur map[string]string) []fileChange {
	var out []fileChange
	for p, sha := range cur {
		prev, had := old[p]
		if had && prev == sha {
			continue
		}
		content, _ := h.store.GetBlob(owner, repo, sha)
		fc := fileChange{Path: p, NewSHA: sha, Status: "added", Additions: countLines(content)}
		if had {
			prevContent, _ := h.store.GetBlob(owner, repo, prev)
			fc.Status, fc.OldSHA, fc.Deletions = "modified", prev, countLines(prevContent)
		}
		out = append(out, fc)
	}
	for p, sha := range old {
		if _, still := cur[p]; !still {
			content, _ := h.store.GetBlob(owner, repo, sha)
			out = append(out, fileChange{Path: p, Status: "removed", OldSHA: sha, Deletions: countLines(content)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// prState is what a pull request's head, base and diff look like now.
type prState struct {
	headSHA, baseSHA string
	commits          []store.Commit
	changes          []fileChange
	conflicts        bool
}

func (h *Handler) prState(p store.PullRequest) prState {
	owner, repo := p.RepoOwner, p.RepoName
	st := prState{headSHA: p.Head.SHA, baseSHA: p.Base.SHA}
	if p.State == "open" {
		if b, ok := h.store.GetBranch(owner, repo, p.Head.Ref); ok {
			st.headSHA = b.Commit.SHA
		}
		if b, ok := h.store.GetBranch(owner, repo, p.Base.Ref); ok {
			st.baseSHA = b.Commit.SHA
		}
	}
	base := p.MergeBase
	if base == "" {
		base = st.baseSHA
	}
	st.commits = h.store.History(owner, repo, st.headSHA, base)
	head, _ := h.store.GetCommit(owner, repo, st.headSHA)
	fork, _ := h.store.GetCommit(owner, repo, base)
	st.changes = h.diffTrees(owner, repo, fork.Files, head.Files)
	if baseHead, ok := h.store.GetCommit(owner, repo, st.baseSHA); ok && st.baseSHA != base {
		// A path both sides changed, to different results, conflicts.
		for _, c := range st.changes {
			onBase := baseHead.Files[c.Path]
			if onBase != fork.Files[c.Path] && onBase != c.NewSHA {
				st.conflicts = true
			}
		}
	}
	return st
}

func (x renderer) prRef(owner, repo, ref, sha string) map[string]any {
	m := map[string]any{"label": owner + ":" + ref, "ref": ref, "sha": sha, "user": x.account(owner), "repo": nil}
	if rp, ok := x.h.store.GetRepo(owner, repo); ok {
		m["repo"] = x.repo(*rp, "")
	}
	return m
}

func href(u string) map[string]any { return map[string]any{"href": u} }

// pull renders the pull-request schema. The 2026-03-10 shape has no
// merge_commit_sha and no singular assignee.
func (x renderer) pull(p store.PullRequest) map[string]any {
	owner, repo := p.RepoOwner, p.RepoName
	st := x.h.prState(p)
	u := x.api("/repos/%s/%s/pulls/%d", owner, repo, p.Number)
	issueURL := x.api("/repos/%s/%s/issues/%d", owner, repo, p.Number)
	statuses := x.api("/repos/%s/%s/statuses/%s", owner, repo, st.headSHA)
	html := x.web("/%s/%s/pull/%d", owner, repo, p.Number)

	var reviewers []map[string]any
	for _, l := range p.RequestedReviewers {
		reviewers = append(reviewers, x.user(l))
	}
	var mergedBy any
	if p.MergedBy != "" {
		mergedBy = x.user(p.MergedBy)
	}
	additions, deletions := 0, 0
	for _, c := range st.changes {
		additions += c.Additions
		deletions += c.Deletions
	}
	var mergeable any = !st.conflicts
	state := "clean"
	switch {
	case p.Merged || p.State == "closed":
		mergeable, state = nil, "unknown"
	case st.conflicts:
		state = "dirty"
	case p.Draft:
		state = "draft"
	}
	reviewComments := len(x.h.store.PRReviewComments.Filter(func(_ string, c store.PRReviewComment) bool {
		return c.RepoOwner == owner && c.RepoName == repo && c.PRNumber == p.Number
	}))
	return map[string]any{
		"url":                 u,
		"id":                  p.ID,
		"node_id":             store.NodeID("PR", x.repoID(owner, repo), p.ID),
		"html_url":            html,
		"diff_url":            html + ".diff",
		"patch_url":           html + ".patch",
		"issue_url":           issueURL,
		"commits_url":         u + "/commits",
		"review_comments_url": u + "/comments",
		"review_comment_url":  x.api("/repos/%s/%s/pulls/comments{/number}", owner, repo),
		"comments_url":        issueURL + "/comments",
		"statuses_url":        statuses,
		"number":              p.Number,
		"state":               p.State,
		"locked":              false,
		"title":               p.Title,
		"user":                x.user(p.User.Login),
		"body":                nullable(p.Body),
		"labels":              x.labels(owner, repo, p.Labels),
		"milestone":           nil,
		"active_lock_reason":  nil,
		"created_at":          p.CreatedAt,
		"updated_at":          p.UpdatedAt,
		"closed_at":           nullable(p.ClosedAt),
		"merged_at":           nullable(p.MergedAt),
		"assignees":           x.users(p.Assignees),
		"requested_reviewers": nonNil(reviewers),
		"requested_teams":     []any{},
		"head":                x.prRef(owner, repo, p.Head.Ref, st.headSHA),
		"base":                x.prRef(owner, repo, p.Base.Ref, st.baseSHA),
		"_links": map[string]any{
			"self": href(u), "html": href(html), "issue": href(issueURL), "comments": href(issueURL + "/comments"),
			"review_comments": href(u + "/comments"), "review_comment": href(x.api("/repos/%s/%s/pulls/comments{/number}", owner, repo)),
			"commits": href(u + "/commits"), "statuses": href(statuses),
		},
		"author_association":    authorAssociation(owner, p.User.Login),
		"auto_merge":            nil,
		"draft":                 p.Draft,
		"merged":                p.Merged,
		"mergeable":             mergeable,
		"rebaseable":            mergeable,
		"mergeable_state":       state,
		"merged_by":             mergedBy,
		"comments":              p.Comments,
		"review_comments":       reviewComments,
		"maintainer_can_modify": p.MaintainerCanModify,
		"commits":               len(st.commits),
		"additions":             additions,
		"deletions":             deletions,
		"changed_files":         len(st.changes),
	}
}

// pullSimple renders pull-request-simple, the list shape.
func (x renderer) pullSimple(p store.PullRequest) map[string]any {
	m := x.pull(p)
	for _, k := range []string{"merged", "mergeable", "rebaseable", "mergeable_state", "merged_by", "comments",
		"review_comments", "maintainer_can_modify", "commits", "additions", "deletions", "changed_files"} {
		delete(m, k)
	}
	return m
}

// review renders a pull-request-review.
func (x renderer) review(rv store.PRReview) map[string]any {
	owner, repo := rv.RepoOwner, rv.RepoName
	html := x.web("/%s/%s/pull/%d#pullrequestreview-%d", owner, repo, rv.PRNumber, rv.ID)
	prURL := x.api("/repos/%s/%s/pulls/%d", owner, repo, rv.PRNumber)
	m := map[string]any{
		"id":                 rv.ID,
		"node_id":            store.NodeID("PRR", x.repoID(owner, repo), rv.ID),
		"user":               x.user(rv.User.Login),
		"body":               rv.Body,
		"state":              rv.State,
		"html_url":           html,
		"pull_request_url":   prURL,
		"_links":             map[string]any{"html": href(html), "pull_request": href(prURL)},
		"commit_id":          nullable(rv.CommitID),
		"author_association": authorAssociation(owner, rv.User.Login),
	}
	if rv.SubmittedAt != "" {
		m["submitted_at"] = rv.SubmittedAt
	}
	return m
}
