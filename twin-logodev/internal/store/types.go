package store

import "time"

// LogoRequest records a request for a logo.
type LogoRequest struct {
	Domain    string    `json:"domain"`
	Size      int       `json:"size,omitempty"`
	Format    string    `json:"format,omitempty"`
	Greyscale bool      `json:"greyscale,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// CustomLogo holds logo content with its media type.
type CustomLogo struct {
	ContentType string `json:"content_type"`
	Data        []byte `json:"data"`
}

// Brand represents a company/brand entry for the search/ticker API.
type Brand struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
	Ticker string `json:"ticker,omitempty"`
	LogoURL string `json:"logo_url,omitempty"`
}
