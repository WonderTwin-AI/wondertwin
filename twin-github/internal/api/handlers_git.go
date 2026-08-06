package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// GetGitRef handles GET /repos/{owner}/{repo}/git/ref/{ref}
func (h *Handler) GetGitRef(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	ref := "refs/" + chi.URLParam(r, "*")

	refs := h.store.ListRepoGitRefs(owner, repo, ref)
	if len(refs) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, refs[0])
}

// ListMatchingRefs handles GET /repos/{owner}/{repo}/git/matching-refs/{ref}
func (h *Handler) ListMatchingRefs(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	prefix := "refs/" + chi.URLParam(r, "*")

	refs := h.store.ListRepoGitRefs(owner, repo, prefix)
	ghJSON(w, 200, refs)
}

// CreateGitRef handles POST /repos/{owner}/{repo}/git/refs
func (h *Handler) CreateGitRef(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	gitRef := store.GitRef{
		Ref:    req.Ref,
		NodeID: store.MakeSHA(req.Ref)[:20],
		URL:    fmt.Sprintf("%s/repos/%s/%s/git/%s", h.store.APIURL(), owner, repo, req.Ref),
		Object: store.GitObject{
			Type: "commit",
			SHA:  req.SHA,
			URL:  fmt.Sprintf("%s/repos/%s/%s/git/commits/%s", h.store.APIURL(), owner, repo, req.SHA),
		},
		RepoOwner: owner,
		RepoName:  repo,
	}

	id := h.store.GitRefs.NextID()
	h.store.GitRefs.Set(id, gitRef)
	ghJSON(w, 201, gitRef)
}

// UpdateGitRef handles PATCH /repos/{owner}/{repo}/git/refs/{ref}
func (h *Handler) UpdateGitRef(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	ref := "refs/" + chi.URLParam(r, "*")

	var req struct {
		SHA   string `json:"sha"`
		Force bool   `json:"force"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	ids, refs := h.store.GitRefs.FilterWithIDs(func(_ string, gr store.GitRef) bool {
		return gr.RepoOwner == owner && gr.RepoName == repo && gr.Ref == ref
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}

	gr := refs[0]
	gr.Object.SHA = req.SHA
	h.store.GitRefs.Set(ids[0], gr)
	ghJSON(w, 200, gr)
}

// DeleteGitRef handles DELETE /repos/{owner}/{repo}/git/refs/{ref}
func (h *Handler) DeleteGitRef(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	ref := "refs/" + chi.URLParam(r, "*")

	ids, _ := h.store.GitRefs.FilterWithIDs(func(_ string, gr store.GitRef) bool {
		return gr.RepoOwner == owner && gr.RepoName == repo && gr.Ref == ref
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.GitRefs.Delete(ids[0])
	w.WriteHeader(204)
}

// GetGitCommit handles GET /repos/{owner}/{repo}/git/commits/{commit_sha}
func (h *Handler) GetGitCommit(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	sha := chi.URLParam(r, "commit_sha")

	_, commits := h.store.GitCommits.FilterWithIDs(func(_ string, gc store.GitCommit) bool {
		return gc.RepoOwner == owner && gc.RepoName == repo && gc.SHA == sha
	})
	if len(commits) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, commits[0])
}

// CreateGitCommit handles POST /repos/{owner}/{repo}/git/commits
func (h *Handler) CreateGitCommit(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req struct {
		Message string   `json:"message"`
		Tree    string   `json:"tree"`
		Parents []string `json:"parents,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	sha := store.MakeSHA(req.Message + req.Tree)
	var parents []store.GitTreeRef
	for _, p := range req.Parents {
		parents = append(parents, store.GitTreeRef{SHA: p})
	}

	gc := store.GitCommit{
		SHA:     sha,
		Message: req.Message,
		Tree:    store.GitTreeRef{SHA: req.Tree},
		Parents: parents,
		Author: store.GitSignature{
			Name:  "twin-bot",
			Email: "bot@wondertwin.dev",
			Date:  h.store.Now(),
		},
		HTMLURL:   fmt.Sprintf("%s/%s/%s/commit/%s", h.store.BaseURL(), owner, repo, sha),
		RepoOwner: owner,
		RepoName:  repo,
	}

	id := h.store.GitCommits.NextID()
	h.store.GitCommits.Set(id, gc)
	ghJSON(w, 201, gc)
}

// GetGitTree handles GET /repos/{owner}/{repo}/git/trees/{tree_sha}
func (h *Handler) GetGitTree(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	sha := chi.URLParam(r, "tree_sha")

	_, trees := h.store.GitTrees.FilterWithIDs(func(_ string, gt store.GitTree) bool {
		return gt.RepoOwner == owner && gt.RepoName == repo && gt.SHA == sha
	})
	if len(trees) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, trees[0])
}

// CreateGitTree handles POST /repos/{owner}/{repo}/git/trees
func (h *Handler) CreateGitTree(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req struct {
		BaseTree string               `json:"base_tree,omitempty"`
		Tree     []store.GitTreeEntry `json:"tree"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Generate SHA from tree content
	var parts []string
	for _, e := range req.Tree {
		parts = append(parts, e.Path+e.SHA)
	}
	sha := store.MakeSHA(strings.Join(parts, ""))

	gt := store.GitTree{
		SHA:       sha,
		Tree:      req.Tree,
		Truncated: false,
		URL:       fmt.Sprintf("%s/repos/%s/%s/git/trees/%s", h.store.APIURL(), owner, repo, sha),
		RepoOwner: owner,
		RepoName:  repo,
	}

	id := h.store.GitTrees.NextID()
	h.store.GitTrees.Set(id, gt)
	ghJSON(w, 201, gt)
}

// GetGitBlob handles GET /repos/{owner}/{repo}/git/blobs/{file_sha}
func (h *Handler) GetGitBlob(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	sha := chi.URLParam(r, "file_sha")

	_, blobs := h.store.GitBlobs.FilterWithIDs(func(_ string, gb store.GitBlob) bool {
		return gb.RepoOwner == owner && gb.RepoName == repo && gb.SHA == sha
	})
	if len(blobs) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, blobs[0])
}

// CreateGitBlob handles POST /repos/{owner}/{repo}/git/blobs
func (h *Handler) CreateGitBlob(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.Encoding == "" {
		req.Encoding = "utf-8"
	}

	sha := store.MakeSHA(req.Content)
	gb := store.GitBlob{
		SHA:       sha,
		Size:      len(req.Content),
		Content:   req.Content,
		Encoding:  req.Encoding,
		URL:       fmt.Sprintf("%s/repos/%s/%s/git/blobs/%s", h.store.APIURL(), owner, repo, sha),
		RepoOwner: owner,
		RepoName:  repo,
	}

	id := h.store.GitBlobs.NextID()
	h.store.GitBlobs.Set(id, gb)
	ghJSON(w, 201, gb)
}

// GetGitTag handles GET /repos/{owner}/{repo}/git/tags/{tag_sha}
func (h *Handler) GetGitTag(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	sha := chi.URLParam(r, "tag_sha")

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
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req struct {
		Tag     string `json:"tag"`
		Message string `json:"message"`
		Object  string `json:"object"`
		Type    string `json:"type"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	sha := store.MakeSHA(req.Tag + req.Object)
	gt := store.GitTag{
		Tag:     req.Tag,
		SHA:     sha,
		Message: req.Message,
		Tagger: store.GitSignature{
			Name:  "twin-bot",
			Email: "bot@wondertwin.dev",
			Date:  h.store.Now(),
		},
		Object: store.GitObject{
			Type: req.Type,
			SHA:  req.Object,
		},
		URL:       fmt.Sprintf("%s/repos/%s/%s/git/tags/%s", h.store.APIURL(), owner, repo, sha),
		RepoOwner: owner,
		RepoName:  repo,
	}

	id := h.store.GitTags.NextID()
	h.store.GitTags.Set(id, gt)
	ghJSON(w, 201, gt)
}
