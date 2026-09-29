package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// contentEntry renders one file or directory entry of the contents API.
func (x renderer) contentEntry(owner, repo, ref, path, kind, sha string, size int) map[string]any {
	self := x.api("/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, ref)
	html := x.web("/%s/%s/%s/%s/%s", owner, repo, map[string]string{"file": "blob", "dir": "tree"}[kind], ref, path)
	gitURL := x.api("/repos/%s/%s/git/%s/%s", owner, repo, map[string]string{"file": "blobs", "dir": "trees"}[kind], sha)
	var download any
	if kind == "file" {
		download = "https://raw.githubusercontent.com/" + owner + "/" + repo + "/" + ref + "/" + path
	}
	return map[string]any{
		"type":         kind,
		"name":         pathBase(path),
		"path":         path,
		"sha":          sha,
		"size":         size,
		"url":          self,
		"html_url":     html,
		"git_url":      gitURL,
		"download_url": download,
		"_links":       map[string]any{"self": self, "git": gitURL, "html": html},
	}
}

// wrapBase64 encodes content the way the contents API does: base64 broken
// into 60-character lines, each ending in a newline.
func wrapBase64(b []byte) string {
	enc := base64.StdEncoding.EncodeToString(b)
	var sb strings.Builder
	for len(enc) > 60 {
		sb.WriteString(enc[:60])
		sb.WriteByte('\n')
		enc = enc[60:]
	}
	if enc != "" {
		sb.WriteString(enc)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// GetContents handles GET /repos/{owner}/{repo}/contents/{path}
func (h *Handler) GetContents(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	path := strings.Trim(chi.URLParam(r, "*"), "/")
	h.serveContents(w, r, owner, repo, path)
}

func (h *Handler) serveContents(w http.ResponseWriter, r *http.Request, owner, repo, path string) {
	rp, ok := h.store.GetRepo(owner, repo)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	if !h.store.HasCommits(owner, repo) {
		ghError(w, 404, "This repository is empty.")
		return
	}
	ref := r.URL.Query().Get("ref")
	c, ok := h.store.ResolveRef(owner, repo, ref)
	if !ok {
		ghError(w, 404, "No commit found for the ref "+ref)
		return
	}
	if ref == "" {
		ref = rp.DefaultBranch
	}
	x := h.rd(r)

	if sha, isFile := c.Files[path]; isFile {
		content, _ := h.store.GetBlob(owner, repo, sha)
		e := x.contentEntry(owner, repo, ref, path, "file", sha, len(content))
		e["content"] = wrapBase64(content)
		e["encoding"] = "base64"
		ghJSON(w, 200, e)
		return
	}

	// A directory listing: the immediate children of path.
	prefix := ""
	if path != "" {
		prefix = path + "/"
	}
	children := map[string]map[string]any{}
	for p, sha := range c.Files {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := strings.TrimPrefix(p, prefix)
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			dir := prefix + rest[:i]
			if _, seen := children[dir]; !seen {
				children[dir] = x.contentEntry(owner, repo, ref, dir, "dir", h.subtreeSHA(c, dir), 0)
			}
			continue
		}
		content, _ := h.store.GetBlob(owner, repo, sha)
		children[p] = x.contentEntry(owner, repo, ref, p, "file", sha, len(content))
	}
	if len(children) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	names := make([]string, 0, len(children))
	for n := range children {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]map[string]any, 0, len(names))
	for _, n := range names {
		out = append(out, children[n])
	}
	ghJSON(w, 200, out)
}

func (h *Handler) subtreeSHA(c store.Commit, dir string) string {
	sub := map[string]string{}
	for p, sha := range c.Files {
		if strings.HasPrefix(p, dir+"/") {
			sub[strings.TrimPrefix(p, dir+"/")] = sha
		}
	}
	return store.TreeSHA(sub)
}

type contentsRequest struct {
	Message   string `json:"message"`
	Content   string `json:"content"`
	SHA       string `json:"sha"`
	Branch    string `json:"branch"`
	Committer *struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"committer"`
	Author *struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"author"`
}

func (h *Handler) signature(r *http.Request, req contentsRequest) store.Signature {
	login := actor(r)
	u := h.userRef(login)
	sig := store.Signature{Name: login, Email: login + "@users.noreply.github.com", Date: h.store.Now(), Login: login}
	if u.Name != "" {
		sig.Name = u.Name
	}
	if req.Author != nil && req.Author.Name != "" {
		sig.Name, sig.Email = req.Author.Name, req.Author.Email
	} else if req.Committer != nil && req.Committer.Name != "" {
		sig.Name, sig.Email = req.Committer.Name, req.Committer.Email
	}
	return sig
}

// CreateOrUpdateContents handles PUT /repos/{owner}/{repo}/contents/{path}.
// It commits the file to the branch (the default branch unless one is
// named), creating the branch's first commit on an empty repository.
func (h *Handler) CreateOrUpdateContents(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	path := strings.Trim(chi.URLParam(r, "*"), "/")

	rp, ok := h.store.GetRepo(owner, repo)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	var req contentsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.Message == "" {
		ghValidationErrors(w, "Invalid request.\n\n\"message\" wasn't supplied.")
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(req.Content, "\n", ""))
	if err != nil {
		ghValidationErrors(w, "content is not valid Base64")
		return
	}
	branch := req.Branch
	if branch == "" {
		branch = rp.DefaultBranch
	}

	status := 201
	before := zeroSHA
	if b, found := h.store.GetBranch(owner, repo, branch); found {
		before = b.Commit.SHA
	}
	if head, found := h.store.ResolveRef(owner, repo, branch); found {
		if cur, exists := head.Files[path]; exists {
			if req.SHA == "" {
				ghValidationErrors(w, "Invalid request.\n\n\"sha\" wasn't supplied.")
				return
			}
			if req.SHA != cur {
				ghError(w, 409, path+" does not match "+req.SHA)
				return
			}
			status = 200
		}
	}

	c, err := h.store.CommitChange(owner, repo, branch, path, decoded, req.Message, h.signature(r, req))
	if errors.Is(err, store.ErrNoBranch) {
		ghError(w, 404, "Branch "+branch+" not found")
		return
	}
	if err != nil {
		ghError(w, 500, err.Error())
		return
	}
	h.touchRepo(owner, repo)
	h.onRefUpdated(r, owner, repo, "refs/heads/"+branch, before, c.SHA, false)

	x := h.rd(r)
	content := x.contentEntry(owner, repo, branch, path, "file", c.Files[path], len(decoded))
	ghJSON(w, status, map[string]any{"content": content, "commit": x.gitCommit(c)})
}

// DeleteContents handles DELETE /repos/{owner}/{repo}/contents/{path}
func (h *Handler) DeleteContents(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	path := strings.Trim(chi.URLParam(r, "*"), "/")

	rp, ok := h.store.GetRepo(owner, repo)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	var req contentsRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	branch := req.Branch
	if branch == "" {
		branch = rp.DefaultBranch
	}
	head, found := h.store.ResolveRef(owner, repo, branch)
	cur, exists := head.Files[path]
	if !found || !exists {
		ghError(w, 404, "Not Found")
		return
	}
	if req.Message == "" || req.SHA == "" {
		ghValidationErrors(w, "Invalid request.\n\n\"message\" and \"sha\" are required.")
		return
	}
	if req.SHA != cur {
		ghError(w, 409, path+" does not match "+req.SHA)
		return
	}
	c, err := h.store.CommitChange(owner, repo, branch, path, nil, req.Message, h.signature(r, req))
	if err != nil {
		ghError(w, 500, err.Error())
		return
	}
	h.touchRepo(owner, repo)
	h.onRefUpdated(r, owner, repo, "refs/heads/"+branch, head.SHA, c.SHA, false)
	ghJSON(w, 200, map[string]any{"content": nil, "commit": h.rd(r).gitCommit(c)})
}

// GetReadme handles GET /repos/{owner}/{repo}/readme
func (h *Handler) GetReadme(w http.ResponseWriter, r *http.Request) {
	h.readme(w, r, "")
}

func (h *Handler) readme(w http.ResponseWriter, r *http.Request, dir string) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	c, ok := h.store.ResolveRef(owner, repo, r.URL.Query().Get("ref"))
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	prefix := ""
	if dir != "" {
		prefix = strings.Trim(dir, "/") + "/"
	}
	for _, name := range []string{"README.md", "README", "readme.md", "README.rst", "README.txt"} {
		if _, found := c.Files[prefix+name]; found {
			h.serveContents(w, r, owner, repo, prefix+name)
			return
		}
	}
	ghError(w, 404, "Not Found")
}

// touchRepo records a push on the repository.
func (h *Handler) touchRepo(owner, repo string) {
	if rp, ok := h.store.GetRepo(owner, repo); ok {
		now := h.store.Now()
		rp.PushedAt, rp.UpdatedAt = now, now
		h.store.Repos.Set(store.RepoKey(owner, repo), *rp)
	}
}

func pathBase(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[i+1:]
	}
	return path
}
