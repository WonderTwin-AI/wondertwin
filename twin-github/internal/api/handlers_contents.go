package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// GetContents handles GET /repos/{owner}/{repo}/contents/{path}
func (h *Handler) GetContents(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	path := chi.URLParam(r, "*")

	contents := h.store.ListRepoContents(owner, repo, path)
	if len(contents) == 0 {
		ghError(w, 404, "Not Found")
		return
	}

	// If it's a single file, return the object directly
	if len(contents) == 1 && contents[0].Type == "file" {
		ghJSON(w, 200, contents[0])
		return
	}

	// Multiple items = directory listing
	ghJSON(w, 200, contents)
}

// CreateOrUpdateContents handles PUT /repos/{owner}/{repo}/contents/{path}
func (h *Handler) CreateOrUpdateContents(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	path := chi.URLParam(r, "*")

	var req struct {
		Message string `json:"message"`
		Content string `json:"content"` // base64-encoded
		SHA     string `json:"sha,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(req.Content)
	if err != nil {
		ghError(w, 422, "content is not valid Base64")
		return
	}

	sha := store.MakeSHA(path + req.Content)
	content := store.Content{
		Type:        "file",
		Name:        pathBase(path),
		Path:        path,
		SHA:         sha,
		Size:        len(decoded),
		HTMLURL:     fmt.Sprintf("%s/%s/%s/blob/main/%s", h.store.BaseURL(), owner, repo, path),
		DownloadURL: fmt.Sprintf("%s/%s/%s/raw/main/%s", h.store.BaseURL(), owner, repo, path),
		Content:     req.Content,
		Encoding:    "base64",
		RepoOwner:   owner,
		RepoName:    repo,
	}

	// Check if updating existing
	existing := h.store.ListRepoContents(owner, repo, path)
	status := 201
	if len(existing) > 0 {
		status = 200
		// Remove old
		ids, _ := h.store.Contents.FilterWithIDs(func(_ string, c store.Content) bool {
			return c.RepoOwner == owner && c.RepoName == repo && c.Path == path
		})
		for _, id := range ids {
			h.store.Contents.Delete(id)
		}
	}

	id := h.store.Contents.NextID()
	h.store.Contents.Set(id, content)

	commitSHA := store.MakeSHA(req.Message + sha)
	ghJSON(w, status, map[string]any{
		"content": content,
		"commit": map[string]any{
			"sha":     commitSHA,
			"message": req.Message,
		},
	})
}

// DeleteContents handles DELETE /repos/{owner}/{repo}/contents/{path}
func (h *Handler) DeleteContents(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	path := chi.URLParam(r, "*")

	ids, _ := h.store.Contents.FilterWithIDs(func(_ string, c store.Content) bool {
		return c.RepoOwner == owner && c.RepoName == repo && c.Path == path
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}

	var req struct {
		Message string `json:"message"`
		SHA     string `json:"sha"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	for _, id := range ids {
		h.store.Contents.Delete(id)
	}

	commitSHA := store.MakeSHA(req.Message + req.SHA)
	ghJSON(w, 200, map[string]any{
		"content": nil,
		"commit": map[string]any{
			"sha":     commitSHA,
			"message": req.Message,
		},
	})
}

// GetReadme handles GET /repos/{owner}/{repo}/readme
func (h *Handler) GetReadme(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	// Look for README.md
	for _, name := range []string{"README.md", "readme.md", "README", "README.rst"} {
		contents := h.store.ListRepoContents(owner, repo, name)
		if len(contents) > 0 {
			ghJSON(w, 200, contents[0])
			return
		}
	}
	ghError(w, 404, "Not Found")
}

func pathBase(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
