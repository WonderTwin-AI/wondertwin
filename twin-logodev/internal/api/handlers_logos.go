package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// GetLogo handles GET /{domain} — returns a deterministic SVG placeholder.
func (h *Handler) GetLogo(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")

	size := 128
	if s := r.URL.Query().Get("size"); s != "" {
		if parsed, err := strconv.Atoi(s); err == nil && parsed > 0 && parsed <= 1024 {
			size = parsed
		}
	}

	format := r.URL.Query().Get("format")
	greyscale := r.URL.Query().Get("greyscale") == "true"

	// Record the request
	h.store.RecordRequest(domain, size, format, greyscale)

	// Check for custom logo
	if custom, ok := h.store.CustomLogos[domain]; ok {
		w.Header().Set("Content-Type", custom.ContentType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.WriteHeader(http.StatusOK)
		w.Write(custom.Data)
		return
	}

	// Generate deterministic placeholder SVG
	svg := generatePlaceholderSVG(domain, size, greyscale)
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(svg))
}

// AdminListLogos handles GET /admin/logos — returns all requested domains.
// Supports ?domain= filter for substring matching.
func (h *Handler) AdminListLogos(w http.ResponseWriter, r *http.Request) {
	domainFilter := r.URL.Query().Get("domain")

	requests := h.store.Requests.List()
	domains := make(map[string]int)
	for _, req := range requests {
		if domainFilter != "" && !strings.Contains(req.Domain, domainFilter) {
			continue
		}
		domains[req.Domain]++
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"domains":        domains,
		"total_requests": len(requests),
	})
}

// AdminGetLogo handles GET /admin/logos/{domain} — returns request history for a domain.
func (h *Handler) AdminGetLogo(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")

	requests := h.store.Requests.List()
	var matched []any
	for _, req := range requests {
		if req.Domain == domain {
			matched = append(matched, req)
		}
	}

	hasCustom := false
	if _, ok := h.store.CustomLogos[domain]; ok {
		hasCustom = true
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"domain":      domain,
		"requests":    matched,
		"total":       len(matched),
		"has_custom":  hasCustom,
	})
}
