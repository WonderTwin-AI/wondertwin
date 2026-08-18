package api

import (
	//nolint:gosec // G501: md5 here derives deterministic placeholder colours
	// from a domain string. It is not used for authentication or integrity, and
	// changing the digest would change every generated logo.
	"crypto/md5"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// DescribeBrand handles GET /api/v1/describe/{domain} — returns brand metadata.
// Returns seeded brand data if available, otherwise generates deterministic metadata.
func (h *Handler) DescribeBrand(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")

	// Check seeded brands first
	for _, b := range h.store.Brands.List() {
		if b.Domain == domain {
			twincore.JSON(w, http.StatusOK, describeBrandResponse(b.Name, b.Domain, b.Ticker))
			return
		}
	}

	// Generate deterministic brand data from domain
	parts := strings.Split(domain, ".")
	name := strings.Title(parts[0]) //nolint:staticcheck
	twincore.JSON(w, http.StatusOK, describeBrandResponse(name, domain, ""))
}

// describeBrandResponse builds a Logo.dev-compatible describe response.
func describeBrandResponse(name, domain, ticker string) map[string]any {
	//nolint:gosec // G401: non-cryptographic use; see the import comment.
	hash := md5.Sum([]byte(domain))
	r, g, b := int(hash[0]), int(hash[1]), int(hash[2])

	resp := map[string]any{
		"name":        name,
		"domain":      domain,
		"description": fmt.Sprintf("%s — simulated brand data from twin-logodev.", name),
		"logo":        fmt.Sprintf("https://img.logo.dev/%s", domain),
		"colors": []map[string]any{
			{
				"hex": fmt.Sprintf("#%02x%02x%02x", r, g, b),
				"r":   r, "g": g, "b": b,
			},
		},
		"socials": map[string]any{},
	}

	if ticker != "" {
		resp["ticker"] = ticker
	}

	return resp
}
