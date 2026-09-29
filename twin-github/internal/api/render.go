package api

import (
	"fmt"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// webBase is where html_url values point. They are browser links, which the
// emulator does not serve, so they keep GitHub's own host.
const webBase = "https://github.com"

// renderer turns stored state into GitHub's 2026-03-10 response shapes. API
// URLs are built from the request's origin so that every link a client
// follows (pagination, upload_url, url fields) stays on the emulator.
type renderer struct {
	h *Handler
	o string
}

func (h *Handler) rd(r *http.Request) renderer { return renderer{h: h, o: origin(r)} }

func (x renderer) api(format string, a ...any) string { return x.o + fmt.Sprintf(format, a...) }
func (x renderer) web(format string, a ...any) string { return webBase + fmt.Sprintf(format, a...) }

// user renders a simple-user for login.
func (x renderer) user(login string) map[string]any {
	return x.userObj(x.h.userRef(login))
}

func userNodePrefix(kind string) string {
	switch kind {
	case "Organization":
		return "O"
	case "Bot":
		return "BOT"
	}
	return "U"
}

func (x renderer) userObj(u store.User) map[string]any {
	if u.Type == "" {
		u.Type = "User"
	}
	l := u.Login
	users := "users"
	return map[string]any{
		"login":               l,
		"id":                  u.ID,
		"node_id":             store.NodeID(userNodePrefix(u.Type), u.ID),
		"avatar_url":          fmt.Sprintf("https://avatars.githubusercontent.com/u/%d?v=4", u.ID),
		"gravatar_id":         "",
		"url":                 x.api("/%s/%s", users, l),
		"html_url":            x.web("/%s", l),
		"followers_url":       x.api("/%s/%s/followers", users, l),
		"following_url":       x.api("/%s/%s/following{/other_user}", users, l),
		"gists_url":           x.api("/%s/%s/gists{/gist_id}", users, l),
		"starred_url":         x.api("/%s/%s/starred{/owner}{/repo}", users, l),
		"subscriptions_url":   x.api("/%s/%s/subscriptions", users, l),
		"organizations_url":   x.api("/%s/%s/orgs", users, l),
		"repos_url":           x.api("/%s/%s/repos", users, l),
		"events_url":          x.api("/%s/%s/events{/privacy}", users, l),
		"received_events_url": x.api("/%s/%s/received_events", users, l),
		"type":                u.Type,
		"user_view_type":      "public",
		"site_admin":          u.SiteAdmin,
	}
}

// gitCommit renders the git-level commit object used by the contents and
// git data APIs.
func (x renderer) gitCommit(c store.Commit) map[string]any {
	parents := make([]map[string]any, 0, len(c.Parents))
	for _, p := range c.Parents {
		parents = append(parents, map[string]any{
			"url":      x.api("/repos/%s/%s/git/commits/%s", c.RepoOwner, c.RepoName, p),
			"html_url": x.web("/%s/%s/commit/%s", c.RepoOwner, c.RepoName, p),
			"sha":      p,
		})
	}
	return map[string]any{
		"sha":       c.SHA,
		"node_id":   commitNodeID(c.SHA),
		"url":       x.api("/repos/%s/%s/git/commits/%s", c.RepoOwner, c.RepoName, c.SHA),
		"html_url":  x.web("/%s/%s/commit/%s", c.RepoOwner, c.RepoName, c.SHA),
		"author":    map[string]any{"name": c.AuthorName, "email": c.AuthorEmail, "date": c.AuthorDate},
		"committer": map[string]any{"name": c.CommitterName, "email": c.CommitterEmail, "date": c.CommitterDate},
		"message":   c.Message,
		"tree": map[string]any{
			"sha": c.TreeSHA,
			"url": x.api("/repos/%s/%s/git/trees/%s", c.RepoOwner, c.RepoName, c.TreeSHA),
		},
		"parents":      parents,
		"verification": map[string]any{"verified": false, "reason": "unsigned", "signature": nil, "payload": nil, "verified_at": nil},
	}
}

// commit renders the REST commit resource (GET /repos/{owner}/{repo}/commits/{ref}).
func (x renderer) commit(c store.Commit) map[string]any {
	gc := x.gitCommit(c)
	parents := make([]map[string]any, 0, len(c.Parents))
	for _, p := range c.Parents {
		parents = append(parents, map[string]any{
			"sha":      p,
			"url":      x.api("/repos/%s/%s/commits/%s", c.RepoOwner, c.RepoName, p),
			"html_url": x.web("/%s/%s/commit/%s", c.RepoOwner, c.RepoName, p),
		})
	}
	var author any = map[string]any{}
	if c.Login != "" {
		author = x.user(c.Login)
	}
	return map[string]any{
		"sha":          c.SHA,
		"node_id":      commitNodeID(c.SHA),
		"url":          x.api("/repos/%s/%s/commits/%s", c.RepoOwner, c.RepoName, c.SHA),
		"html_url":     x.web("/%s/%s/commit/%s", c.RepoOwner, c.RepoName, c.SHA),
		"comments_url": x.api("/repos/%s/%s/commits/%s/comments", c.RepoOwner, c.RepoName, c.SHA),
		"commit": map[string]any{
			"url":           gc["url"],
			"author":        gc["author"],
			"committer":     gc["committer"],
			"message":       c.Message,
			"comment_count": 0,
			"tree":          gc["tree"],
			"verification":  gc["verification"],
		},
		"author":    author,
		"committer": author,
		"parents":   parents,
	}
}

// commitNodeID is an opaque global ID for a commit. GitHub's own encodes the
// repository ID and the SHA; clients treat it as opaque.
func commitNodeID(sha string) string {
	return "C_" + store.MakeSHA("commit:" + sha)[:20]
}

// branch renders a short-branch.
func (x renderer) branch(b store.Branch) map[string]any {
	return map[string]any{
		"name": b.Name,
		"commit": map[string]any{
			"sha": b.Commit.SHA,
			"url": x.api("/repos/%s/%s/commits/%s", b.RepoOwner, b.RepoName, b.Commit.SHA),
		},
		"protected":      b.Protected,
		"protection_url": x.api("/repos/%s/%s/branches/%s/protection", b.RepoOwner, b.RepoName, b.Name),
	}
}

// gitRef renders a git-ref.
func (x renderer) gitRef(owner, repo, ref, sha string) map[string]any {
	return map[string]any{
		"ref":     ref,
		"node_id": "REF_" + store.MakeSHA(owner + "/" + repo + ":" + ref)[:24],
		"url":     x.api("/repos/%s/%s/git/%s", owner, repo, ref),
		"object": map[string]any{
			"type": "commit",
			"sha":  sha,
			"url":  x.api("/repos/%s/%s/git/commits/%s", owner, repo, sha),
		},
	}
}
