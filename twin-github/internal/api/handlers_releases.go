package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"

	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

func (x renderer) asset(a store.ReleaseAsset) map[string]any {
	rel, _ := x.h.store.Releases.Get(strconv.FormatInt(a.ReleaseID, 10))
	return map[string]any{
		"url":                  x.api("/repos/%s/%s/releases/assets/%d", a.RepoOwner, a.RepoName, a.ID),
		"browser_download_url": x.web("/%s/%s/releases/download/%s/%s", a.RepoOwner, a.RepoName, rel.TagName, url.PathEscape(a.Name)),
		"id":                   a.ID,
		"node_id":              store.NodeID("RA", x.repoID(a.RepoOwner, a.RepoName), a.ID),
		"name":                 a.Name,
		"label":                nullable(a.Label),
		"state":                "uploaded",
		"content_type":         a.ContentType,
		"size":                 a.Size,
		"digest":               nullable(a.Digest),
		"download_count":       a.DownloadCount,
		"created_at":           a.CreatedAt,
		"updated_at":           a.UpdatedAt,
		"uploader":             x.user(a.Uploader.Login),
	}
}

// release renders the release schema. upload_url points at the emulator,
// which serves the upload route GitHub serves on uploads.github.com.
func (x renderer) release(rel store.Release) map[string]any {
	owner, repo := rel.RepoOwner, rel.RepoName
	u := x.api("/repos/%s/%s/releases/%d", owner, repo, rel.ID)
	assets := []map[string]any{}
	for _, a := range x.h.store.ListReleaseAssets(owner, repo, rel.ID) {
		assets = append(assets, x.asset(a))
	}
	var name any
	if rel.NameSet || rel.Name != "" {
		name = rel.Name
	}
	var tarball, zipball any
	if !rel.Draft {
		tarball = x.api("/repos/%s/%s/tarball/%s", owner, repo, rel.TagName)
		zipball = x.api("/repos/%s/%s/zipball/%s", owner, repo, rel.TagName)
	}
	html := x.web("/%s/%s/releases/tag/%s", owner, repo, url.PathEscape(rel.TagName))
	if rel.Draft {
		html = x.web("/%s/%s/releases/tag/untagged-%d", owner, repo, rel.ID)
	}
	return map[string]any{
		"url":              u,
		"html_url":         html,
		"assets_url":       u + "/assets",
		"upload_url":       u + "/assets{?name,label}",
		"tarball_url":      tarball,
		"zipball_url":      zipball,
		"id":               rel.ID,
		"node_id":          store.NodeID("RE", x.repoID(owner, repo), rel.ID),
		"tag_name":         rel.TagName,
		"target_commitish": rel.TargetCommitish,
		"name":             name,
		"body":             nullable(rel.Body),
		"draft":            rel.Draft,
		"prerelease":       rel.Prerelease,
		"immutable":        false,
		"created_at":       rel.CreatedAt,
		"published_at":     nullable(rel.PublishedAt),
		"updated_at":       nullable(rel.UpdatedAt),
		"author":           x.user(rel.Author.Login),
		"assets":           assets,
	}
}

// releasesNewestFirst lists a repository's releases, newest first.
func (h *Handler) releasesNewestFirst(owner, repo string) []store.Release {
	rels := h.store.ListRepoReleases(owner, repo)
	sort.SliceStable(rels, func(i, j int) bool {
		if rels[i].CreatedAt == rels[j].CreatedAt {
			return rels[i].ID > rels[j].ID
		}
		return rels[i].CreatedAt > rels[j].CreatedAt
	})
	return rels
}

// ListReleases handles GET /repos/{owner}/{repo}/releases
func (h *Handler) ListReleases(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}
	anonymous := principalFrom(r).Kind == principalAnonymous
	var rels []store.Release
	for _, rel := range h.releasesNewestFirst(owner, repo) {
		if rel.Draft && anonymous {
			continue // drafts are visible only to users with push access
		}
		rels = append(rels, rel)
	}
	x := h.rd(r)
	out := []map[string]any{}
	for _, rel := range paginate(w, r, rels) {
		out = append(out, x.release(rel))
	}
	ghJSON(w, 200, out)
}

type releaseRequest struct {
	TagName         *string `json:"tag_name"`
	TargetCommitish *string `json:"target_commitish"`
	Name            *string `json:"name"`
	Body            *string `json:"body"`
	Draft           *bool   `json:"draft"`
	Prerelease      *bool   `json:"prerelease"`
	MakeLatest      *string `json:"make_latest"`
}

// publishTag creates the release's tag at its target when the tag does not
// exist yet, which GitHub does when a release is published.
func (h *Handler) publishTag(r *http.Request, rel store.Release) {
	ref := "refs/tags/" + rel.TagName
	if _, exists := h.store.GetTagRef(rel.RepoOwner, rel.RepoName, ref); exists {
		return
	}
	if c, ok := h.store.ResolveRef(rel.RepoOwner, rel.RepoName, rel.TargetCommitish); ok {
		h.store.SetTagRef(rel.RepoOwner, rel.RepoName, ref, c.SHA)
		h.onRefCreated(r, rel.RepoOwner, rel.RepoName, ref, c.SHA)
	}
}

// CreateRelease handles POST /repos/{owner}/{repo}/releases
func (h *Handler) CreateRelease(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	rp, ok := h.store.GetRepo(owner, repo)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	var req releaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.TagName == nil || *req.TagName == "" {
		ghValidationError(w, "Release", "tag_name", "missing_field")
		return
	}
	for _, rel := range h.store.ListRepoReleases(owner, repo) {
		if rel.TagName == *req.TagName {
			ghValidationError(w, "Release", "tag_name", "already_exists")
			return
		}
	}
	target := rp.DefaultBranch
	if req.TargetCommitish != nil && *req.TargetCommitish != "" {
		target = *req.TargetCommitish
	}
	_, tagExists := h.store.GetTagRef(owner, repo, "refs/tags/"+*req.TagName)
	if _, resolves := h.store.ResolveRef(owner, repo, target); !resolves && !tagExists {
		ghValidationError(w, "Release", "target_commitish", "invalid")
		return
	}

	now := h.store.Now()
	// GitHub's created_at for a release is the date of its commit, not of
	// the release; latest sorts by it.
	created := now
	if t, ok := h.store.GetTagRef(owner, repo, "refs/tags/"+*req.TagName); ok {
		if c, found := h.store.GetCommit(owner, repo, t.Object.SHA); found {
			created = c.CommitterDate
		}
	} else if c, found := h.store.ResolveRef(owner, repo, target); found {
		created = c.CommitterDate
	}
	rel := store.Release{
		ID: h.store.NewID(store.KindRelease), TagName: *req.TagName, TargetCommitish: target,
		Draft: req.Draft != nil && *req.Draft, Prerelease: req.Prerelease != nil && *req.Prerelease,
		Author: h.userRef(actor(r)), CreatedAt: created, UpdatedAt: now, RepoOwner: owner, RepoName: repo,
	}
	if req.Name != nil {
		rel.Name, rel.NameSet = *req.Name, true
	}
	if req.Body != nil {
		rel.Body = *req.Body
	}
	if req.MakeLatest != nil {
		rel.MakeLatest = *req.MakeLatest
	}
	if !rel.Draft {
		rel.PublishedAt = now
		h.publishTag(r, rel)
	}
	h.store.Releases.Set(strconv.FormatInt(rel.ID, 10), rel)
	ghJSON(w, 201, h.rd(r).release(rel))
}

func (h *Handler) releaseFromPath(w http.ResponseWriter, r *http.Request) (store.Release, bool) {
	rel, ok := h.store.Releases.Get(param(r, "release_id"))
	if !ok || rel.RepoOwner != param(r, "owner") || rel.RepoName != param(r, "repo") ||
		(rel.Draft && principalFrom(r).Kind == principalAnonymous) {
		ghError(w, 404, "Not Found")
		return rel, false
	}
	return rel, true
}

// GetRelease handles GET /repos/{owner}/{repo}/releases/{release_id}
func (h *Handler) GetRelease(w http.ResponseWriter, r *http.Request) {
	if rel, ok := h.releaseFromPath(w, r); ok {
		ghJSON(w, 200, h.rd(r).release(rel))
	}
}

// GetLatestRelease handles GET /repos/{owner}/{repo}/releases/latest: the
// newest published release that is neither a prerelease nor opted out with
// make_latest=false.
func (h *Handler) GetLatestRelease(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	for _, rel := range h.releasesNewestFirst(owner, repo) {
		if !rel.Draft && !rel.Prerelease && rel.MakeLatest != "false" {
			ghJSON(w, 200, h.rd(r).release(rel))
			return
		}
	}
	ghError(w, 404, "Not Found")
}

// GetReleaseByTag handles GET /repos/{owner}/{repo}/releases/tags/{tag}
func (h *Handler) GetReleaseByTag(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	tag := param(r, "tag")
	for _, rel := range h.store.ListRepoReleases(owner, repo) {
		if rel.TagName == tag && !rel.Draft {
			ghJSON(w, 200, h.rd(r).release(rel))
			return
		}
	}
	ghError(w, 404, "Not Found")
}

// UpdateRelease handles PATCH /repos/{owner}/{repo}/releases/{release_id}.
// Publishing a draft stamps published_at and creates its tag.
func (h *Handler) UpdateRelease(w http.ResponseWriter, r *http.Request) {
	rel, ok := h.releaseFromPath(w, r)
	if !ok {
		return
	}
	var req releaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.TagName != nil && *req.TagName != "" {
		rel.TagName = *req.TagName
	}
	if req.TargetCommitish != nil {
		rel.TargetCommitish = *req.TargetCommitish
	}
	if req.Name != nil {
		rel.Name, rel.NameSet = *req.Name, true
	}
	if req.Body != nil {
		rel.Body = *req.Body
	}
	if req.Prerelease != nil {
		rel.Prerelease = *req.Prerelease
	}
	if req.MakeLatest != nil {
		rel.MakeLatest = *req.MakeLatest
	}
	if req.Draft != nil && rel.Draft && !*req.Draft {
		rel.Draft, rel.PublishedAt = false, h.store.Now()
		h.publishTag(r, rel)
	}
	rel.UpdatedAt = h.store.Now()
	h.store.Releases.Set(strconv.FormatInt(rel.ID, 10), rel)
	ghJSON(w, 200, h.rd(r).release(rel))
}

// DeleteRelease handles DELETE /repos/{owner}/{repo}/releases/{release_id}.
// The tag stays, as it does on GitHub.
func (h *Handler) DeleteRelease(w http.ResponseWriter, r *http.Request) {
	rel, ok := h.releaseFromPath(w, r)
	if !ok {
		return
	}
	h.store.Releases.Delete(strconv.FormatInt(rel.ID, 10))
	w.WriteHeader(204)
}

// ListReleaseAssets handles GET /repos/{owner}/{repo}/releases/{release_id}/assets
func (h *Handler) ListReleaseAssets(w http.ResponseWriter, r *http.Request) {
	rel, ok := h.releaseFromPath(w, r)
	if !ok {
		return
	}
	x := h.rd(r)
	out := []map[string]any{}
	for _, a := range paginate(w, r, h.store.ListReleaseAssets(rel.RepoOwner, rel.RepoName, rel.ID)) {
		out = append(out, x.asset(a))
	}
	ghJSON(w, 200, out)
}

// UploadReleaseAsset handles POST /repos/{owner}/{repo}/releases/{release_id}/assets,
// the route GitHub serves on uploads.github.com. The body is the file.
func (h *Handler) UploadReleaseAsset(w http.ResponseWriter, r *http.Request) {
	rel, ok := h.releaseFromPath(w, r)
	if !ok {
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		ghValidationError(w, "ReleaseAsset", "name", "missing_field")
		return
	}
	for _, a := range h.store.ListReleaseAssets(rel.RepoOwner, rel.RepoName, rel.ID) {
		if a.Name == name {
			ghValidationError(w, "ReleaseAsset", "name", "already_exists")
			return
		}
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		ghError(w, 400, "Problems reading the upload")
		return
	}
	sum := sha256.Sum256(body)
	ctype := r.Header.Get("Content-Type")
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	now := h.store.Now()
	a := store.ReleaseAsset{
		ID: h.store.NewID(store.KindAsset), Name: name, Label: r.URL.Query().Get("label"), ContentType: ctype,
		Size: len(body), State: "uploaded", CreatedAt: now, UpdatedAt: now, Uploader: h.userRef(actor(r)),
		Digest: "sha256:" + hex.EncodeToString(sum[:]), RepoOwner: rel.RepoOwner, RepoName: rel.RepoName, ReleaseID: rel.ID,
	}
	h.store.ReleaseAssets.Set(strconv.FormatInt(a.ID, 10), a)
	ghJSON(w, 201, h.rd(r).asset(a))
}

func (h *Handler) assetFromPath(w http.ResponseWriter, r *http.Request) (store.ReleaseAsset, bool) {
	a, ok := h.store.ReleaseAssets.Get(param(r, "asset_id"))
	if !ok || a.RepoOwner != param(r, "owner") || a.RepoName != param(r, "repo") {
		ghError(w, 404, "Not Found")
		return a, false
	}
	return a, true
}

// GetReleaseAsset handles GET /repos/{owner}/{repo}/releases/assets/{asset_id}
func (h *Handler) GetReleaseAsset(w http.ResponseWriter, r *http.Request) {
	if a, ok := h.assetFromPath(w, r); ok {
		ghJSON(w, 200, h.rd(r).asset(a))
	}
}

// UpdateReleaseAsset handles PATCH /repos/{owner}/{repo}/releases/assets/{asset_id}
func (h *Handler) UpdateReleaseAsset(w http.ResponseWriter, r *http.Request) {
	a, ok := h.assetFromPath(w, r)
	if !ok {
		return
	}
	var req struct {
		Name  *string `json:"name"`
		Label *string `json:"label"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Name != nil {
		a.Name = *req.Name
	}
	if req.Label != nil {
		a.Label = *req.Label
	}
	a.UpdatedAt = h.store.Now()
	h.store.ReleaseAssets.Set(strconv.FormatInt(a.ID, 10), a)
	ghJSON(w, 200, h.rd(r).asset(a))
}

// DeleteReleaseAsset handles DELETE /repos/{owner}/{repo}/releases/assets/{asset_id}
func (h *Handler) DeleteReleaseAsset(w http.ResponseWriter, r *http.Request) {
	a, ok := h.assetFromPath(w, r)
	if !ok {
		return
	}
	h.store.ReleaseAssets.Delete(strconv.FormatInt(a.ID, 10))
	w.WriteHeader(204)
}
