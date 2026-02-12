package api

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/store"
)

// Handler holds logo API state.
type Handler struct {
	store *store.MemoryStore
}

// NewHandler creates a new logo API handler.
func NewHandler(s *store.MemoryStore) *Handler {
	return &Handler{store: s}
}

// Routes mounts the Logo.dev-compatible routes.
func (h *Handler) Routes(r chi.Router) {
	// Logo.dev uses GET /{domain} with ?token= param
	r.Get("/{domain}", h.GetLogo)

	// Admin extras
	r.Get("/admin/logos", h.ListLogos)
}

// GetLogo handles GET /{domain} - returns a deterministic SVG placeholder.
func (h *Handler) GetLogo(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")

	// Validate token (accept any non-empty token)
	token := r.URL.Query().Get("token")
	if token == "" {
		twincore.Error(w, http.StatusUnauthorized, "API token required")
		return
	}

	size := 128
	if s := r.URL.Query().Get("size"); s != "" {
		if parsed, err := strconv.Atoi(s); err == nil && parsed > 0 {
			size = parsed
		}
	}

	format := r.URL.Query().Get("format")
	greyscale := r.URL.Query().Get("greyscale") == "true"

	// Record the request
	h.store.RecordRequest(domain, size, format, greyscale)

	// Check for custom logo
	if custom, ok := h.store.CustomLogos[domain]; ok {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.WriteHeader(http.StatusOK)
		w.Write(custom)
		return
	}

	// Generate deterministic placeholder SVG
	svg := generatePlaceholderSVG(domain, size, greyscale)
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(svg))
}

// ListLogos handles GET /admin/logos - returns all requested domains.
func (h *Handler) ListLogos(w http.ResponseWriter, r *http.Request) {
	requests := h.store.Requests.List()
	domains := make(map[string]int)
	for _, req := range requests {
		domains[req.Domain]++
	}
	twincore.JSON(w, http.StatusOK, map[string]any{
		"domains":        domains,
		"total_requests": len(requests),
	})
}

// generatePlaceholderSVG creates a colored square with domain initials.
func generatePlaceholderSVG(domain string, size int, greyscale bool) string {
	// Generate deterministic color from domain
	hash := md5.Sum([]byte(domain))
	r, g, b := int(hash[0]), int(hash[1]), int(hash[2])

	if greyscale {
		avg := (r + g + b) / 3
		r, g, b = avg, avg, avg
	}

	// Get initials from domain
	parts := strings.Split(domain, ".")
	name := parts[0]
	initials := strings.ToUpper(name[:1])
	if len(name) > 1 {
		initials += strings.ToUpper(name[1:2])
	}

	color := fmt.Sprintf("#%02x%02x%02x", r, g, b)

	// Calculate text color (white or black based on luminance)
	luminance := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	textColor := "#ffffff"
	if luminance > 128 {
		textColor = "#000000"
	}

	fontSize := size / 3

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" rx="%d" fill="%s"/>
  <text x="50%%" y="50%%" dominant-baseline="central" text-anchor="middle" fill="%s" font-family="system-ui, sans-serif" font-size="%d" font-weight="600">%s</text>
</svg>`,
		size, size, size, size,
		size, size, size/8, color,
		textColor, fontSize, initials)
}
