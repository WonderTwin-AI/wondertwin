package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// ListBranches handles GET /repos/{owner}/{repo}/branches
func (h *Handler) ListBranches(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	branches := h.store.ListRepoBranches(owner, repo)
	if r.URL.Query().Get("protected") == "true" {
		var protected []store.Branch
		for _, b := range branches {
			if b.Protected {
				protected = append(protected, b)
			}
		}
		branches = protected
	}
	x := h.rd(r)
	out := make([]map[string]any, 0, len(branches))
	for _, b := range paginate(w, r, branches) {
		out = append(out, x.branch(b))
	}
	ghJSON(w, 200, out)
}

// GetBranch handles GET /repos/{owner}/{repo}/branches/{branch}
func (h *Handler) GetBranch(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	branchName := chi.URLParam(r, "branch")

	b, ok := h.store.GetBranch(owner, repo, branchName)
	if !ok {
		ghError(w, 404, "Branch not found")
		return
	}
	x := h.rd(r)
	out := x.branch(b)
	if c, found := h.store.GetCommit(owner, repo, b.Commit.SHA); found {
		out["commit"] = x.commit(c)
	}
	out["_links"] = map[string]any{
		"self": x.api("/repos/%s/%s/branches/%s", owner, repo, b.Name),
		"html": x.web("/%s/%s/tree/%s", owner, repo, b.Name),
	}
	out["protection"] = map[string]any{"enabled": b.Protected, "required_status_checks": map[string]any{"enforcement_level": "off", "contexts": []string{}, "checks": []any{}}}
	ghJSON(w, 200, out)
}
