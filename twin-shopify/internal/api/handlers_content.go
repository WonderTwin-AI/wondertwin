package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/store"
)

// --- Themes ---

func (h *Handler) ListThemes(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"themes": h.store.Themes.List()})
}

func (h *Handler) GetTheme(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, ok := h.store.Themes.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"theme": t})
}

func (h *Handler) CreateTheme(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Theme store.Theme `json:"theme"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	t := req.Theme
	if t.Name == "" {
		shopifyValidationError(w, "name", []string{"can't be blank"})
		return
	}
	t.ID = h.store.NextID()
	now := h.store.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Role == "" {
		t.Role = "unpublished"
	}

	h.store.Themes.Set(fmt.Sprintf("%d", t.ID), t)
	shopifyJSON(w, http.StatusCreated, map[string]any{"theme": t})
}

func (h *Handler) UpdateTheme(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, ok := h.store.Themes.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Theme store.Theme `json:"theme"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Theme.Name != "" {
		t.Name = req.Theme.Name
	}
	if req.Theme.Role != "" {
		t.Role = req.Theme.Role
	}
	t.UpdatedAt = h.store.Now()
	h.store.Themes.Set(id, t)
	shopifyJSON(w, http.StatusOK, map[string]any{"theme": t})
}

func (h *Handler) DeleteTheme(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Themes.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- Assets ---

func (h *Handler) ListAssets(w http.ResponseWriter, r *http.Request) {
	themeID := chi.URLParam(r, "theme_id")
	tid, err := strconv.ParseInt(themeID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	assets := h.store.Assets.Filter(func(_ string, a store.Asset) bool {
		return a.ThemeID == tid
	})
	shopifyJSON(w, http.StatusOK, map[string]any{"assets": assets})
}

func (h *Handler) CreateOrUpdateAsset(w http.ResponseWriter, r *http.Request) {
	themeID := chi.URLParam(r, "theme_id")
	tid, err := strconv.ParseInt(themeID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Asset store.Asset `json:"asset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	a := req.Asset
	if a.Key == "" {
		shopifyValidationError(w, "key", []string{"can't be blank"})
		return
	}
	a.ThemeID = tid
	now := h.store.Now()

	// Use theme_id + key as the store key
	storeKey := fmt.Sprintf("%d:%s", tid, a.Key)
	existing, ok := h.store.Assets.Get(storeKey)
	if ok {
		a.CreatedAt = existing.CreatedAt
	} else {
		a.CreatedAt = now
	}
	a.UpdatedAt = now

	h.store.Assets.Set(storeKey, a)
	shopifyJSON(w, http.StatusOK, map[string]any{"asset": a})
}

func (h *Handler) DeleteAsset(w http.ResponseWriter, r *http.Request) {
	themeID := chi.URLParam(r, "theme_id")
	assetKey := r.URL.Query().Get("asset[key]")
	if assetKey == "" {
		shopifyError(w, http.StatusBadRequest, "asset[key] is required")
		return
	}
	storeKey := fmt.Sprintf("%s:%s", themeID, assetKey)
	if !h.store.Assets.Delete(storeKey) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"message": "assets/deleted"})
}

// --- Pages ---

func (h *Handler) ListPages(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"pages": h.store.Pages.List()})
}

func (h *Handler) GetPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := h.store.Pages.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"page": p})
}

func (h *Handler) CreatePage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Page store.Page `json:"page"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	p := req.Page
	if p.Title == "" {
		shopifyValidationError(w, "title", []string{"can't be blank"})
		return
	}
	p.ID = h.store.NextID()
	now := h.store.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Handle == "" {
		p.Handle = strings.ToLower(strings.ReplaceAll(p.Title, " ", "-"))
	}

	h.store.Pages.Set(fmt.Sprintf("%d", p.ID), p)
	shopifyJSON(w, http.StatusCreated, map[string]any{"page": p})
}

func (h *Handler) UpdatePage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := h.store.Pages.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Page store.Page `json:"page"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Page.Title != "" {
		p.Title = req.Page.Title
	}
	if req.Page.BodyHTML != "" {
		p.BodyHTML = req.Page.BodyHTML
	}
	if req.Page.Author != "" {
		p.Author = req.Page.Author
	}
	p.Published = req.Page.Published
	p.UpdatedAt = h.store.Now()
	h.store.Pages.Set(id, p)
	shopifyJSON(w, http.StatusOK, map[string]any{"page": p})
}

func (h *Handler) DeletePage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Pages.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- Blogs ---

func (h *Handler) ListBlogs(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"blogs": h.store.Blogs.List()})
}

func (h *Handler) GetBlog(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b, ok := h.store.Blogs.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"blog": b})
}

func (h *Handler) CreateBlog(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Blog store.Blog `json:"blog"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	b := req.Blog
	if b.Title == "" {
		shopifyValidationError(w, "title", []string{"can't be blank"})
		return
	}
	b.ID = h.store.NextID()
	now := h.store.Now()
	b.CreatedAt = now
	b.UpdatedAt = now
	if b.Handle == "" {
		b.Handle = strings.ToLower(strings.ReplaceAll(b.Title, " ", "-"))
	}

	h.store.Blogs.Set(fmt.Sprintf("%d", b.ID), b)
	shopifyJSON(w, http.StatusCreated, map[string]any{"blog": b})
}

func (h *Handler) UpdateBlog(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b, ok := h.store.Blogs.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Blog store.Blog `json:"blog"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Blog.Title != "" {
		b.Title = req.Blog.Title
	}
	if req.Blog.Handle != "" {
		b.Handle = req.Blog.Handle
	}
	b.UpdatedAt = h.store.Now()
	h.store.Blogs.Set(id, b)
	shopifyJSON(w, http.StatusOK, map[string]any{"blog": b})
}

func (h *Handler) DeleteBlog(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Blogs.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- Articles ---

func (h *Handler) ListArticles(w http.ResponseWriter, r *http.Request) {
	blogID := chi.URLParam(r, "blog_id")
	bid, err := strconv.ParseInt(blogID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	articles := h.store.Articles.Filter(func(_ string, a store.Article) bool {
		return a.BlogID == bid
	})
	shopifyJSON(w, http.StatusOK, map[string]any{"articles": articles})
}

func (h *Handler) GetArticle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, ok := h.store.Articles.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"article": a})
}

func (h *Handler) CreateArticle(w http.ResponseWriter, r *http.Request) {
	blogID := chi.URLParam(r, "blog_id")
	bid, err := strconv.ParseInt(blogID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Article store.Article `json:"article"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	a := req.Article
	if a.Title == "" {
		shopifyValidationError(w, "title", []string{"can't be blank"})
		return
	}
	a.ID = h.store.NextID()
	a.BlogID = bid
	now := h.store.Now()
	a.CreatedAt = now
	a.UpdatedAt = now
	if a.Handle == "" {
		a.Handle = strings.ToLower(strings.ReplaceAll(a.Title, " ", "-"))
	}

	h.store.Articles.Set(fmt.Sprintf("%d", a.ID), a)
	shopifyJSON(w, http.StatusCreated, map[string]any{"article": a})
}

func (h *Handler) UpdateArticle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, ok := h.store.Articles.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Article store.Article `json:"article"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Article.Title != "" {
		a.Title = req.Article.Title
	}
	if req.Article.Author != "" {
		a.Author = req.Article.Author
	}
	if req.Article.BodyHTML != "" {
		a.BodyHTML = req.Article.BodyHTML
	}
	a.Published = req.Article.Published
	a.UpdatedAt = h.store.Now()
	h.store.Articles.Set(id, a)
	shopifyJSON(w, http.StatusOK, map[string]any{"article": a})
}

func (h *Handler) DeleteArticle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Articles.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- Script Tags ---

func (h *Handler) ListScriptTags(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"script_tags": h.store.ScriptTags.List()})
}

func (h *Handler) GetScriptTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	st, ok := h.store.ScriptTags.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"script_tag": st})
}

func (h *Handler) CreateScriptTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ScriptTag store.ScriptTag `json:"script_tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	st := req.ScriptTag
	if st.Src == "" {
		shopifyValidationError(w, "src", []string{"can't be blank"})
		return
	}
	st.ID = h.store.NextID()
	now := h.store.Now()
	st.CreatedAt = now
	st.UpdatedAt = now
	if st.Event == "" {
		st.Event = "onload"
	}

	h.store.ScriptTags.Set(fmt.Sprintf("%d", st.ID), st)
	shopifyJSON(w, http.StatusCreated, map[string]any{"script_tag": st})
}

func (h *Handler) UpdateScriptTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	st, ok := h.store.ScriptTags.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		ScriptTag store.ScriptTag `json:"script_tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.ScriptTag.Src != "" {
		st.Src = req.ScriptTag.Src
	}
	if req.ScriptTag.Event != "" {
		st.Event = req.ScriptTag.Event
	}
	st.UpdatedAt = h.store.Now()
	h.store.ScriptTags.Set(id, st)
	shopifyJSON(w, http.StatusOK, map[string]any{"script_tag": st})
}

func (h *Handler) DeleteScriptTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.ScriptTags.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- Redirects ---

func (h *Handler) ListRedirects(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"redirects": h.store.Redirects.List()})
}

func (h *Handler) GetRedirect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rd, ok := h.store.Redirects.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"redirect": rd})
}

func (h *Handler) CreateRedirect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Redirect store.Redirect `json:"redirect"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	rd := req.Redirect
	if rd.Path == "" || rd.Target == "" {
		shopifyValidationError(w, "redirect", []string{"path and target are required"})
		return
	}
	rd.ID = h.store.NextID()

	h.store.Redirects.Set(fmt.Sprintf("%d", rd.ID), rd)
	shopifyJSON(w, http.StatusCreated, map[string]any{"redirect": rd})
}

func (h *Handler) UpdateRedirect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rd, ok := h.store.Redirects.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Redirect store.Redirect `json:"redirect"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Redirect.Path != "" {
		rd.Path = req.Redirect.Path
	}
	if req.Redirect.Target != "" {
		rd.Target = req.Redirect.Target
	}
	h.store.Redirects.Set(id, rd)
	shopifyJSON(w, http.StatusOK, map[string]any{"redirect": rd})
}

func (h *Handler) DeleteRedirect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Redirects.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}
