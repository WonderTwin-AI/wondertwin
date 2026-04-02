package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// GetLogo handles GET /{domain} — returns a deterministic SVG placeholder.
// Supports Logo.dev query params: size, format, greyscale, retina, theme, fallback.
func (h *Handler) GetLogo(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	h.serveLogo(w, r, domain)
}

// GetLogoByName handles GET /name/{name} — looks up brand by name and returns logo.
func (h *Handler) GetLogoByName(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	nameL := strings.ToLower(name)

	for _, b := range h.store.Brands.List() {
		if strings.ToLower(b.Name) == nameL {
			h.serveLogo(w, r, b.Domain)
			return
		}
	}

	// No brand found — use the name as a pseudo-domain
	h.serveLogo(w, r, name+".com")
}

// GetLogoByTicker handles GET /ticker/{symbol} — looks up brand by stock ticker.
func (h *Handler) GetLogoByTicker(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(chi.URLParam(r, "symbol"))

	for _, b := range h.store.Brands.List() {
		if strings.ToUpper(b.Ticker) == symbol {
			h.serveLogo(w, r, b.Domain)
			return
		}
	}

	// No ticker match — check fallback preference
	if r.URL.Query().Get("fallback") == "404" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Default: monogram from ticker
	h.serveLogo(w, r, symbol+".com")
}

// serveLogo is the shared handler logic for all logo retrieval endpoints.
func (h *Handler) serveLogo(w http.ResponseWriter, r *http.Request, domain string) {
	q := r.URL.Query()

	size := 128
	if s := q.Get("size"); s != "" {
		if parsed, err := strconv.Atoi(s); err == nil && parsed > 0 && parsed <= 800 {
			size = parsed
		}
	}

	// Retina doubles the source resolution
	if q.Get("retina") == "true" {
		size *= 2
	}

	greyscale := q.Get("greyscale") == "true"
	theme := q.Get("theme") // "dark" or "light"
	format := q.Get("format")

	h.store.RecordRequest(domain, size, format, greyscale)

	// Check for custom logo
	if custom, ok := h.store.CustomLogos[domain]; ok {
		w.Header().Set("Content-Type", custom.ContentType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.WriteHeader(http.StatusOK)
		w.Write(custom.Data)
		return
	}

	// Check fallback=404 — no custom logo and no real logo to serve
	if q.Get("fallback") == "404" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Generate deterministic placeholder SVG
	svg := generatePlaceholderSVG(domain, size, greyscale, theme)
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
		"domain":     domain,
		"requests":   matched,
		"total":      len(matched),
		"has_custom": hasCustom,
	})
}
