package api

import (
	//nolint:gosec // G501: md5 here derives deterministic placeholder colours
	// from a domain string. It is not used for authentication or integrity, and
	// changing the digest would change every generated logo.
	"crypto/md5"
	"fmt"
	"strings"
)

// generatePlaceholderSVG creates a colored square with domain initials.
// theme: "dark" inverts light logos (white bg → dark), "light" inverts dark logos.
func generatePlaceholderSVG(domain string, size int, greyscale bool, theme string) string {
	//nolint:gosec // G401: non-cryptographic use; see the import comment.
	hash := md5.Sum([]byte(domain))
	r, g, b := int(hash[0]), int(hash[1]), int(hash[2])

	if greyscale {
		avg := (r + g + b) / 3
		r, g, b = avg, avg, avg
	}

	// Theme support: invert colors for dark/light backgrounds
	if theme == "dark" {
		r, g, b = 255-r, 255-g, 255-b
	}
	// "light" theme keeps original colors (already suitable for light backgrounds)

	parts := strings.Split(domain, ".")
	name := parts[0]
	initials := strings.ToUpper(name[:1])
	if len(name) > 1 {
		initials += strings.ToUpper(name[1:2])
	}

	color := fmt.Sprintf("#%02x%02x%02x", r, g, b)

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
