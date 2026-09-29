package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// refTarget looks a fully qualified ref up in the git model.
func (h *Handler) refTarget(owner, repo, ref string) (string, bool) {
	if name, isBranch := strings.CutPrefix(ref, "refs/heads/"); isBranch {
		b, ok := h.store.GetBranch(owner, repo, name)
		return b.Commit.SHA, ok
	}
	t, ok := h.store.GetTagRef(owner, repo, ref)
	return t.Object.SHA, ok
}

// allRefs lists every ref of a repository, sorted by name.
func (h *Handler) allRefs(owner, repo string) [][2]string {
	var out [][2]string
	for _, b := range h.store.ListRepoBranches(owner, repo) {
		out = append(out, [2]string{"refs/heads/" + b.Name, b.Commit.SHA})
	}
	for _, t := range h.store.ListRepoGitRefs(owner, repo, "") {
		out = append(out, [2]string{t.Ref, t.Object.SHA})
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// GetGitRef handles GET /repos/{owner}/{repo}/git/ref/{ref}
func (h *Handler) GetGitRef(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	ref := "refs/" + param(r, "*")
	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}
	if !h.store.HasCommits(owner, repo) {
		ghError(w, 409, "Git Repository is empty.")
		return
	}
	sha, ok := h.refTarget(owner, repo, ref)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, h.rd(r).gitRef(owner, repo, ref, sha))
}

// ListMatchingRefs handles GET /repos/{owner}/{repo}/git/matching-refs/{ref}
func (h *Handler) ListMatchingRefs(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	prefix := "refs/" + param(r, "*")
	x := h.rd(r)
	out := []map[string]any{}
	for _, ref := range h.allRefs(owner, repo) {
		if strings.HasPrefix(ref[0], prefix) {
			out = append(out, x.gitRef(owner, repo, ref[0], ref[1]))
		}
	}
	ghJSON(w, 200, out)
}

// CreateGitRef handles POST /repos/{owner}/{repo}/git/refs
func (h *Handler) CreateGitRef(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if !strings.HasPrefix(req.Ref, "refs/") || strings.Count(req.Ref, "/") < 2 {
		ghValidationErrors(w, "Reference name is not valid")
		return
	}
	if _, exists := h.refTarget(owner, repo, req.Ref); exists {
		ghValidationErrors(w, "Reference already exists")
		return
	}
	c, ok := h.store.GetCommit(owner, repo, req.SHA)
	if !ok {
		ghValidationErrors(w, "Object does not exist")
		return
	}
	if name, isBranch := strings.CutPrefix(req.Ref, "refs/heads/"); isBranch {
		h.store.SetBranch(owner, repo, name, c.SHA)
	} else {
		h.store.SetTagRef(owner, repo, req.Ref, c.SHA)
	}
	h.onRefCreated(r, owner, repo, req.Ref, c.SHA)
	ghJSON(w, 201, h.rd(r).gitRef(owner, repo, req.Ref, c.SHA))
}

// UpdateGitRef handles PATCH /repos/{owner}/{repo}/git/refs/{ref}
func (h *Handler) UpdateGitRef(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	ref := "refs/" + param(r, "*")

	var req struct {
		SHA   string `json:"sha"`
		Force bool   `json:"force"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	old, ok := h.refTarget(owner, repo, ref)
	if !ok {
		ghValidationErrors(w, "Reference does not exist")
		return
	}
	c, ok := h.store.GetCommit(owner, repo, req.SHA)
	if !ok {
		ghValidationErrors(w, "Object does not exist")
		return
	}
	if !req.Force && !h.store.IsAncestor(owner, repo, old, c.SHA) {
		ghValidationErrors(w, "Update is not a fast forward")
		return
	}
	if name, isBranch := strings.CutPrefix(ref, "refs/heads/"); isBranch {
		h.store.SetBranch(owner, repo, name, c.SHA)
	} else {
		h.store.SetTagRef(owner, repo, ref, c.SHA)
	}
	h.onRefUpdated(r, owner, repo, ref, old, c.SHA, req.Force)
	ghJSON(w, 200, h.rd(r).gitRef(owner, repo, ref, c.SHA))
}

// DeleteGitRef handles DELETE /repos/{owner}/{repo}/git/refs/{ref}
func (h *Handler) DeleteGitRef(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	ref := "refs/" + param(r, "*")

	var deleted bool
	if name, isBranch := strings.CutPrefix(ref, "refs/heads/"); isBranch {
		deleted = h.store.DeleteBranch(owner, repo, name)
	} else {
		deleted = h.store.DeleteTagRef(owner, repo, ref)
	}
	if !deleted {
		ghValidationErrors(w, "Reference does not exist")
		return
	}
	w.WriteHeader(204)
}

// GetGitCommit handles GET /repos/{owner}/{repo}/git/commits/{commit_sha}
func (h *Handler) GetGitCommit(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	c, ok := h.store.GetCommit(owner, repo, param(r, "commit_sha"))
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, h.rd(r).gitCommit(c))
}

// CreateGitCommit handles POST /repos/{owner}/{repo}/git/commits
func (h *Handler) CreateGitCommit(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")

	var req struct {
		Message string   `json:"message"`
		Tree    string   `json:"tree"`
		Parents []string `json:"parents"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	tree, ok := h.store.GitTrees.Get(store.RepoKey(owner, repo) + "@" + req.Tree)
	if !ok {
		ghValidationErrors(w, "Tree SHA does not exist")
		return
	}
	files := map[string]string{}
	for _, e := range tree.Tree {
		if e.Type == "blob" {
			files[e.Path] = e.SHA
		}
	}
	for _, p := range req.Parents {
		if _, found := h.store.GetCommit(owner, repo, p); !found {
			ghValidationErrors(w, "Parent SHA does not exist or is not a commit object")
			return
		}
	}
	sig := h.signature(r, contentsRequest{})
	c := h.store.PutCommit(store.Commit{
		Message: req.Message, Parents: req.Parents, Files: files,
		AuthorName: sig.Name, AuthorEmail: sig.Email, AuthorDate: sig.Date,
		Login: sig.Login, RepoOwner: owner, RepoName: repo,
	})
	ghJSON(w, 201, h.rd(r).gitCommit(c))
}

func (x renderer) gitTree(owner, repo string, t store.GitTree) map[string]any {
	entries := make([]map[string]any, 0, len(t.Tree))
	for _, e := range t.Tree {
		entry := map[string]any{"path": e.Path, "mode": e.Mode, "type": e.Type, "sha": e.SHA,
			"url": x.api("/repos/%s/%s/git/%ss/%s", owner, repo, e.Type, e.SHA)}
		if e.Type == "blob" {
			entry["size"] = e.Size
		}
		entries = append(entries, entry)
	}
	return map[string]any{"sha": t.SHA, "url": x.api("/repos/%s/%s/git/trees/%s", owner, repo, t.SHA),
		"tree": entries, "truncated": false}
}

// GetGitTree handles GET /repos/{owner}/{repo}/git/trees/{tree_sha}. Trees
// are listed flat (as with ?recursive=1).
func (h *Handler) GetGitTree(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	sha := param(r, "tree_sha")

	if t, ok := h.store.GitTrees.Get(store.RepoKey(owner, repo) + "@" + sha); ok {
		ghJSON(w, 200, h.rd(r).gitTree(owner, repo, t))
		return
	}
	// A tree of a commit made through the contents API.
	if c, ok := h.commitByTree(owner, repo, sha); ok {
		ghJSON(w, 200, h.rd(r).gitTree(owner, repo, h.treeOf(owner, repo, c)))
		return
	}
	ghError(w, 404, "Not Found")
}

func (h *Handler) commitByTree(owner, repo, treeSHA string) (store.Commit, bool) {
	cs := h.store.Commits.Filter(func(_ string, c store.Commit) bool {
		return c.RepoOwner == owner && c.RepoName == repo && c.TreeSHA == treeSHA
	})
	if len(cs) == 0 {
		return store.Commit{}, false
	}
	return cs[0], true
}

func (h *Handler) treeOf(owner, repo string, c store.Commit) store.GitTree {
	paths := make([]string, 0, len(c.Files))
	for p := range c.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	t := store.GitTree{SHA: c.TreeSHA}
	for _, p := range paths {
		content, _ := h.store.GetBlob(owner, repo, c.Files[p])
		t.Tree = append(t.Tree, store.GitTreeEntry{Path: p, Mode: "100644", Type: "blob", SHA: c.Files[p], Size: len(content)})
	}
	return t
}

// CreateGitTree handles POST /repos/{owner}/{repo}/git/trees
func (h *Handler) CreateGitTree(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")

	var req struct {
		BaseTree string `json:"base_tree"`
		Tree     []struct {
			Path    string  `json:"path"`
			Mode    string  `json:"mode"`
			Type    string  `json:"type"`
			SHA     *string `json:"sha"`
			Content *string `json:"content"`
		} `json:"tree"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	files := map[string]string{}
	if req.BaseTree != "" {
		if base, ok := h.store.GitTrees.Get(store.RepoKey(owner, repo) + "@" + req.BaseTree); ok {
			for _, e := range base.Tree {
				files[e.Path] = e.SHA
			}
		} else if c, found := h.commitByTree(owner, repo, req.BaseTree); found {
			for p, s := range c.Files {
				files[p] = s
			}
		}
	}
	for _, e := range req.Tree {
		switch {
		case e.Content != nil:
			files[e.Path] = h.store.PutBlob(owner, repo, []byte(*e.Content))
		case e.SHA == nil:
			delete(files, e.Path)
		default:
			files[e.Path] = *e.SHA
		}
	}
	t := h.treeOf(owner, repo, store.Commit{Files: files, TreeSHA: store.TreeSHA(files)})
	h.store.GitTrees.Set(store.RepoKey(owner, repo)+"@"+t.SHA, t)
	ghJSON(w, 201, h.rd(r).gitTree(owner, repo, t))
}

// GetGitBlob handles GET /repos/{owner}/{repo}/git/blobs/{file_sha}
func (h *Handler) GetGitBlob(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	sha := param(r, "file_sha")

	content, ok := h.store.GetBlob(owner, repo, sha)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, map[string]any{
		"sha": sha, "node_id": "B_" + store.MakeSHA("blob:" + sha)[:20], "size": len(content),
		"url":     h.rd(r).api("/repos/%s/%s/git/blobs/%s", owner, repo, sha),
		"content": wrapBase64(content), "encoding": "base64",
	})
}

// CreateGitBlob handles POST /repos/{owner}/{repo}/git/blobs
func (h *Handler) CreateGitBlob(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")

	var req struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	content := []byte(req.Content)
	if req.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(req.Content, "\n", ""))
		if err != nil {
			ghValidationErrors(w, "content is not valid Base64")
			return
		}
		content = decoded
	}
	sha := h.store.PutBlob(owner, repo, content)
	ghJSON(w, 201, map[string]any{"sha": sha, "url": h.rd(r).api("/repos/%s/%s/git/blobs/%s", owner, repo, sha)})
}

// GetGitTag handles GET /repos/{owner}/{repo}/git/tags/{tag_sha}
func (h *Handler) GetGitTag(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	sha := param(r, "tag_sha")

	_, tags := h.store.GitTags.FilterWithIDs(func(_ string, gt store.GitTag) bool {
		return gt.RepoOwner == owner && gt.RepoName == repo && gt.SHA == sha
	})
	if len(tags) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, tags[0])
}

// CreateGitTag handles POST /repos/{owner}/{repo}/git/tags
func (h *Handler) CreateGitTag(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")

	var req struct {
		Tag     string `json:"tag"`
		Message string `json:"message"`
		Object  string `json:"object"`
		Type    string `json:"type"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	sha := store.MakeSHA(req.Tag + req.Object)
	x := h.rd(r)
	gt := store.GitTag{
		Tag:     req.Tag,
		SHA:     sha,
		Message: req.Message,
		Tagger: store.GitSignature{
			Name:  actor(r),
			Email: actor(r) + "@users.noreply.github.com",
			Date:  h.store.Now(),
		},
		Object: store.GitObject{
			Type: req.Type,
			SHA:  req.Object,
			URL:  x.api("/repos/%s/%s/git/commits/%s", owner, repo, req.Object),
		},
		URL:       x.api("/repos/%s/%s/git/tags/%s", owner, repo, sha),
		RepoOwner: owner,
		RepoName:  repo,
	}

	id := h.store.GitTags.NextID()
	h.store.GitTags.Set(id, gt)
	ghJSON(w, 201, gt)
}
